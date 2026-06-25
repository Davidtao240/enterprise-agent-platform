package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/enterprise-agent-platform/go-platform/internal/auth"
	"github.com/enterprise-agent-platform/go-platform/internal/platform"
	"github.com/enterprise-agent-platform/go-platform/pkg/apierror"
	"github.com/gin-gonic/gin"
)

type routePermissionChecker struct {
	allowed map[string]bool
}

func (r routePermissionChecker) HasPermission(ctx context.Context, userID, permission string) (bool, error) {
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
	protected.POST("/workflow-instances", auth.RequirePermission(checker, "workflow:create"), noContentHandler)
	protected.GET("/workflow-instances", auth.RequirePermission(checker, "workflow:read"), noContentHandler)
	protected.POST("/workflow-instances/:id/start", auth.RequirePermission(checker, "workflow:start"), noContentHandler)
	protected.POST("/workflow-instances/:id/cancel", auth.RequirePermission(checker, "workflow:cancel"), noContentHandler)
	protected.POST("/workflow-instances/:id/retry", auth.RequirePermission(checker, "workflow:retry"), noContentHandler)
	protected.GET("/approval-tasks", auth.RequirePermission(checker, "approval:read"), noContentHandler)
	protected.POST("/approval-tasks/:id/approve", auth.RequirePermission(checker, "approval:decide"), noContentHandler)
	protected.GET("/audit-logs", auth.RequirePermission(checker, "audit:read"), noContentHandler)
	protected.GET("/agents", auth.RequirePermission(checker, "agent:manage"), noContentHandler)
	protected.GET("/tools", auth.RequirePermission(checker, "tool:manage"), noContentHandler)
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
		{name: "workflow read", method: http.MethodGet, path: "/api/v1/workflow-instances", permission: "workflow:read"},
		{name: "workflow start", method: http.MethodPost, path: "/api/v1/workflow-instances/wf-1/start", permission: "workflow:start"},
		{name: "workflow cancel", method: http.MethodPost, path: "/api/v1/workflow-instances/wf-1/cancel", permission: "workflow:cancel"},
		{name: "workflow retry", method: http.MethodPost, path: "/api/v1/workflow-instances/wf-1/retry", permission: "workflow:retry"},
		{name: "approval read", method: http.MethodGet, path: "/api/v1/approval-tasks", permission: "approval:read"},
		{name: "approval decide", method: http.MethodPost, path: "/api/v1/approval-tasks/task-1/approve", permission: "approval:decide"},
		{name: "audit read", method: http.MethodGet, path: "/api/v1/audit-logs", permission: "audit:read"},
		{name: "agent manage", method: http.MethodGet, path: "/api/v1/agents", permission: "agent:manage"},
		{name: "tool manage", method: http.MethodGet, path: "/api/v1/tools", permission: "tool:manage"},
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
