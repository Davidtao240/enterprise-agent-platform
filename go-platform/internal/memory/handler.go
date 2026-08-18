package memory

import (
	"net/http"
	"strings"

	"github.com/enterprise-agent-platform/go-platform/internal/platform"
	"github.com/enterprise-agent-platform/go-platform/pkg/apierror"
	"github.com/gin-gonic/gin"
)

// Handler M4-A Memory HTTP 端点 (internal-only,InternalServiceToken 保护):
//
//	POST   /internal/v1/memory           写入记忆 (Runtime 服务身份)
//	GET    /internal/v1/memory           检索可见记忆 (?scope=&scope_id=&viewer_user_id=&viewer_roles=a,b)
//	DELETE /internal/v1/memory/:id       软删除
//
// 租户 ID 必须与 X-Tenant-ID header 一致(拒绝跨租户伪造,与 connector 模式对齐)。
type Handler struct {
	svc *Service
}

// NewHandler 创建 Handler。
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func tenantFromHeader(c *gin.Context) (string, bool) {
	tenantID := c.GetHeader("X-Tenant-ID")
	if tenantID == "" {
		platform.APIError(c, &apierror.APIError{
			Code: "UNAUTHORIZED", Message: "X-Tenant-ID header required", Status: http.StatusUnauthorized,
		})
		return "", false
	}
	return tenantID, true
}

// Write 处理 POST /internal/v1/memory。
func (h *Handler) Write(c *gin.Context) {
	var req WriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		platform.APIError(c, &apierror.APIError{
			Code: "VALIDATION_FAILED", Message: err.Error(), Status: http.StatusBadRequest,
		})
		return
	}
	tenantID, ok := tenantFromHeader(c)
	if !ok {
		return
	}
	if req.TenantID != tenantID {
		platform.APIError(c, &apierror.APIError{
			Code: "FORBIDDEN", Message: "tenant_id mismatch with auth context", Status: http.StatusForbidden,
		})
		return
	}
	m, err := h.svc.Write(c.Request.Context(), &req)
	if err != nil {
		platform.APIError(c, &apierror.APIError{
			Code: "MEMORY_WRITE_FAILED", Message: err.Error(), Status: http.StatusBadRequest,
		})
		return
	}
	platform.Success(c, m)
}

// Query 处理 GET /internal/v1/memory。
func (h *Handler) Query(c *gin.Context) {
	tenantID, ok := tenantFromHeader(c)
	if !ok {
		return
	}
	var viewerRoles []string
	if v := c.Query("viewer_roles"); v != "" {
		viewerRoles = strings.Split(v, ",")
	}
	items, err := h.svc.QueryVisible(c.Request.Context(), &QueryRequest{
		TenantID:    tenantID,
		Scope:       c.Query("scope"),
		ScopeID:     c.Query("scope_id"),
		ViewerID:    c.Query("viewer_user_id"),
		ViewerRoles: viewerRoles,
	})
	if err != nil {
		platform.APIError(c, &apierror.APIError{
			Code: "MEMORY_QUERY_FAILED", Message: err.Error(), Status: http.StatusBadRequest,
		})
		return
	}
	platform.Success(c, gin.H{"items": items})
}

// Delete 处理 DELETE /internal/v1/memory/:id。
func (h *Handler) Delete(c *gin.Context) {
	tenantID, ok := tenantFromHeader(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), tenantID, c.Param("id")); err != nil {
		status := http.StatusInternalServerError
		if err == ErrMemoryNotFound {
			status = http.StatusNotFound
		}
		platform.APIError(c, &apierror.APIError{
			Code: "MEMORY_DELETE_FAILED", Message: err.Error(), Status: status,
		})
		return
	}
	platform.Success(c, gin.H{"id": c.Param("id"), "deleted": true})
}
