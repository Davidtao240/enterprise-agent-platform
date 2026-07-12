package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/enterprise-agent-platform/go-platform/internal/audit"
	"github.com/gin-gonic/gin"
)

type fakePermissionChecker struct {
	allowed bool
	err     error
	userID  string
	perm    string
}

type fakeSecurityAudit struct {
	entries []audit.AuditLogEntry
}

func (f *fakeSecurityAudit) InsertLog(ctx context.Context, entry audit.AuditLogEntry) (string, time.Time, error) {
	f.entries = append(f.entries, entry)
	return "audit-1", time.Now(), nil
}

func (f *fakePermissionChecker) HasPermission(ctx context.Context, userID, permission string) (bool, error) {
	f.userID = userID
	f.perm = permission
	return f.allowed, f.err
}

func TestRequirePermissionAllowsAuthorizedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	checker := &fakePermissionChecker{allowed: true}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", "user-1")
		c.Next()
	})
	router.GET("/secure", RequirePermission(checker, "workflow:create"), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/secure", nil))

	if resp.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body=%s", resp.Code, resp.Body.String())
	}
	if checker.userID != "user-1" || checker.perm != "workflow:create" {
		t.Fatalf("permission check mismatch: %#v", checker)
	}
}

func TestRequirePermissionRejectsUnauthorizedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	checker := &fakePermissionChecker{allowed: false}
	auditRepo := &fakeSecurityAudit{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", "user-1")
		c.Next()
	})
	router.GET("/secure", RequirePermissionWithAudit(checker, "audit:read", auditRepo), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/secure", nil))

	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body=%s", resp.Code, resp.Body.String())
	}
	if len(auditRepo.entries) != 1 {
		t.Fatalf("expected one audit entry, got %#v", auditRepo.entries)
	}
	if auditRepo.entries[0].Action != "permission_denied" || auditRepo.entries[0].ActorUserID == nil || *auditRepo.entries[0].ActorUserID != "user-1" {
		t.Fatalf("unexpected audit entry: %#v", auditRepo.entries[0])
	}
}

func TestRequirePermissionRejectsMissingUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/secure", RequirePermission(&fakePermissionChecker{allowed: true}, "audit:read"), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/secure", nil))

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body=%s", resp.Code, resp.Body.String())
	}
}

func TestRequirePermissionReturnsInternalErrorWhenCheckerFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	checker := &fakePermissionChecker{err: errors.New("permission store unavailable")}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", "user-1")
		c.Next()
	})
	router.GET("/secure", RequirePermission(checker, "audit:read"), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/secure", nil))

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body=%s", resp.Code, resp.Body.String())
	}
}
