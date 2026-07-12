package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/enterprise-agent-platform/go-platform/internal/auth"
	"github.com/enterprise-agent-platform/go-platform/internal/platform"
	"github.com/enterprise-agent-platform/go-platform/pkg/apierror"
	"github.com/gin-gonic/gin"
)

type routePermissionChecker struct {
	allowed map[string]bool
	err     error
}

func (r routePermissionChecker) HasPermission(ctx context.Context, userID, permission string) (bool, error) {
	if r.err != nil {
		return false, r.err
	}
	return r.allowed[permission], nil
}

func testAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("Authorization") != "Bearer valid-token" {
			platform.APIError(c, apierror.ErrUnauthorized)
			return
		}
		c.Set("user_id", "user-1")
		c.Next()
	}
}

func newRBACRouteTestRouter(allowed map[string]bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(platform.TraceMiddleware())

	checker := routePermissionChecker{allowed: allowed}
	protected := router.Group("/api/v1")
	protected.Use(testAuthMiddleware())
	protected.GET("/business-apps", auth.RequirePermission(checker, "business_app:read"), noContentHandler)
	protected.GET("/business-apps/registry", auth.RequirePermission(checker, "business_app:read"), noContentHandler)
	protected.GET("/domain-policies", auth.RequirePermission(checker, "business_app:read"), noContentHandler)
	protected.GET("/configuration-versions", auth.RequirePermission(checker, "configuration:manage"), noContentHandler)
	protected.POST("/configuration-versions", auth.RequirePermission(checker, "configuration:manage"), noContentHandler)
	protected.POST("/configuration-versions/:id/submit", auth.RequirePermission(checker, "configuration:manage"), noContentHandler)
	protected.POST("/configuration-versions/:id/approve", auth.RequirePermission(checker, "configuration:approve"), noContentHandler)
	protected.POST("/configuration-versions/:id/deprecate", auth.RequirePermission(checker, "configuration:manage"), noContentHandler)
	protected.POST("/workflow-instances", auth.RequirePermission(checker, "workflow:create"), noContentHandler)
	protected.GET("/workflow-templates", auth.RequirePermission(checker, "workflow_template:read"), noContentHandler)
	protected.GET("/workflow-instances", auth.RequirePermission(checker, "workflow:read"), noContentHandler)
	protected.POST("/workflow-instances/:id/start", auth.RequirePermission(checker, "workflow:start"), noContentHandler)
	protected.POST("/workflow-instances/:id/cancel", auth.RequirePermission(checker, "workflow:cancel"), noContentHandler)
	protected.POST("/workflow-instances/:id/retry", auth.RequirePermission(checker, "workflow:retry"), noContentHandler)
	protected.GET("/approval-tasks", auth.RequirePermission(checker, "approval:read"), noContentHandler)
	protected.POST("/approval-tasks/:id/approve", auth.RequirePermission(checker, "approval:decide"), noContentHandler)
	protected.GET("/audit-logs", auth.RequirePermission(checker, "audit:read"), noContentHandler)
	protected.GET("/audit-logs/stats", auth.RequirePermission(checker, "audit:read"), noContentHandler)
	protected.GET("/audit-logs/export", auth.RequirePermission(checker, "audit:read"), noContentHandler)
	protected.GET("/rbac/permission-matrix", auth.RequirePermission(checker, "role:manage"), noContentHandler)
	protected.GET("/rbac/user-roles", auth.RequirePermission(checker, "user:manage"), noContentHandler)
	protected.GET("/agents", auth.RequirePermission(checker, "agent:manage"), noContentHandler)
	protected.GET("/tools", auth.RequirePermission(checker, "tool:manage"), noContentHandler)
	protected.GET("/platform-observability/summary", auth.RequirePermission(checker, "observability:read"), noContentHandler)
	protected.POST("/files", auth.RequirePermission(checker, "file:upload"), noContentHandler)
	protected.GET("/files/:id", auth.RequirePermission(checker, "file:read"), noContentHandler)

	return router
}

func noContentHandler(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

func TestProtectedRoutesRejectMissingToken(t *testing.T) {
	router := newRBACRouteTestRouter(map[string]bool{"workflow:read": true})

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/v1/workflow-instances", nil))

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d, body=%s", resp.Code, http.StatusUnauthorized, resp.Body.String())
	}
}

func TestProtectedRoutesRejectMissingPermission(t *testing.T) {
	router := newRBACRouteTestRouter(map[string]bool{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body=%s", resp.Code, http.StatusForbidden, resp.Body.String())
	}
}

func TestProtectedRoutesForbiddenIncludesGeneratedTraceID(t *testing.T) {
	router := newRBACRouteTestRouter(map[string]bool{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body=%s", resp.Code, http.StatusForbidden, resp.Body.String())
	}
	traceID := resp.Header().Get(platform.TraceIDHeader)
	if traceID == "" {
		t.Fatal("expected generated trace id response header")
	}
	if !strings.Contains(resp.Body.String(), `"trace_id":"`+traceID+`"`) {
		t.Fatalf("expected body to include generated trace id %q, body=%s", traceID, resp.Body.String())
	}
}

func TestProtectedRoutesPermissionStoreFailureIncludesIncomingTraceID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(platform.TraceMiddleware())

	checker := routePermissionChecker{
		allowed: map[string]bool{},
		err:     errors.New("permission store unavailable"),
	}
	protected := router.Group("/api/v1")
	protected.Use(testAuthMiddleware())
	protected.GET("/audit-logs", auth.RequirePermission(checker, "audit:read"), noContentHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	req.Header.Set(platform.TraceIDHeader, "trace-from-client")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d, body=%s", resp.Code, http.StatusInternalServerError, resp.Body.String())
	}
	if got := resp.Header().Get(platform.TraceIDHeader); got != "trace-from-client" {
		t.Fatalf("trace header = %q, want %q", got, "trace-from-client")
	}
	if !strings.Contains(resp.Body.String(), `"trace_id":"trace-from-client"`) {
		t.Fatalf("expected body to include incoming trace id, body=%s", resp.Body.String())
	}
}

func TestProtectedRoutesAllowAuthorizedUser(t *testing.T) {
	router := newRBACRouteTestRouter(map[string]bool{"audit:read": true})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body=%s", resp.Code, http.StatusNoContent, resp.Body.String())
	}
}

func TestV12RepresentativeRoutesRequireExpectedPermissions(t *testing.T) {
	cases := []struct {
		name       string
		method     string
		path       string
		permission string
	}{
		{name: "workflow create", method: http.MethodPost, path: "/api/v1/workflow-instances", permission: "workflow:create"},
		{name: "business app read", method: http.MethodGet, path: "/api/v1/business-apps", permission: "business_app:read"},
		{name: "business app registry read", method: http.MethodGet, path: "/api/v1/business-apps/registry", permission: "business_app:read"},
		{name: "domain policy read", method: http.MethodGet, path: "/api/v1/domain-policies", permission: "business_app:read"},
		{name: "configuration list", method: http.MethodGet, path: "/api/v1/configuration-versions", permission: "configuration:manage"},
		{name: "configuration create", method: http.MethodPost, path: "/api/v1/configuration-versions", permission: "configuration:manage"},
		{name: "configuration submit", method: http.MethodPost, path: "/api/v1/configuration-versions/config-1/submit", permission: "configuration:manage"},
		{name: "configuration approve", method: http.MethodPost, path: "/api/v1/configuration-versions/config-1/approve", permission: "configuration:approve"},
		{name: "workflow template read", method: http.MethodGet, path: "/api/v1/workflow-templates", permission: "workflow_template:read"},
		{name: "workflow read", method: http.MethodGet, path: "/api/v1/workflow-instances", permission: "workflow:read"},
		{name: "workflow start", method: http.MethodPost, path: "/api/v1/workflow-instances/wf-1/start", permission: "workflow:start"},
		{name: "workflow cancel", method: http.MethodPost, path: "/api/v1/workflow-instances/wf-1/cancel", permission: "workflow:cancel"},
		{name: "workflow retry", method: http.MethodPost, path: "/api/v1/workflow-instances/wf-1/retry", permission: "workflow:retry"},
		{name: "approval read", method: http.MethodGet, path: "/api/v1/approval-tasks", permission: "approval:read"},
		{name: "approval decide", method: http.MethodPost, path: "/api/v1/approval-tasks/task-1/approve", permission: "approval:decide"},
		{name: "audit read", method: http.MethodGet, path: "/api/v1/audit-logs", permission: "audit:read"},
		{name: "audit stats", method: http.MethodGet, path: "/api/v1/audit-logs/stats", permission: "audit:read"},
		{name: "audit export", method: http.MethodGet, path: "/api/v1/audit-logs/export", permission: "audit:read"},
		{name: "permission matrix read", method: http.MethodGet, path: "/api/v1/rbac/permission-matrix", permission: "role:manage"},
		{name: "user roles read", method: http.MethodGet, path: "/api/v1/rbac/user-roles", permission: "user:manage"},
		{name: "agent manage", method: http.MethodGet, path: "/api/v1/agents", permission: "agent:manage"},
		{name: "tool manage", method: http.MethodGet, path: "/api/v1/tools", permission: "tool:manage"},
		{name: "observability read", method: http.MethodGet, path: "/api/v1/platform-observability/summary", permission: "observability:read"},
		{name: "file upload", method: http.MethodPost, path: "/api/v1/files", permission: "file:upload"},
		{name: "file read", method: http.MethodGet, path: "/api/v1/files/file-1", permission: "file:read"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := newRBACRouteTestRouter(map[string]bool{tc.permission: true})
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.Header.Set("Authorization", "Bearer valid-token")
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d for permission %s, body=%s", resp.Code, http.StatusNoContent, tc.permission, resp.Body.String())
			}
		})
	}
}
