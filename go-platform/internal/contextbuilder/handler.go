package contextbuilder

import (
	"net/http"

	"github.com/enterprise-agent-platform/go-platform/internal/platform"
	"github.com/enterprise-agent-platform/go-platform/pkg/apierror"
	"github.com/gin-gonic/gin"
)

// Handler M4-C 上下文组装端点 (internal-only,InternalServiceToken 保护):
//
//	POST /internal/v1/context/build  供 Python Agent Service 调用
//
// 租户 ID 必须与 X-Tenant-ID header 一致(与 memory/connector 模式对齐)。
type Handler struct {
	builder *Builder
}

// NewHandler 创建 Handler。
func NewHandler(builder *Builder) *Handler {
	return &Handler{builder: builder}
}

// Build 处理 POST /internal/v1/context/build。
func (h *Handler) Build(c *gin.Context) {
	var req BuildRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		platform.APIError(c, &apierror.APIError{
			Code: "VALIDATION_FAILED", Message: err.Error(), Status: http.StatusBadRequest,
		})
		return
	}
	tenantID := c.GetHeader("X-Tenant-ID")
	if tenantID == "" {
		platform.APIError(c, &apierror.APIError{
			Code: "UNAUTHORIZED", Message: "X-Tenant-ID header required", Status: http.StatusUnauthorized,
		})
		return
	}
	if req.TenantID != tenantID {
		platform.APIError(c, &apierror.APIError{
			Code: "FORBIDDEN", Message: "tenant_id mismatch with auth context", Status: http.StatusForbidden,
		})
		return
	}
	resp, err := h.builder.Build(c.Request.Context(), &req)
	if err != nil {
		platform.APIError(c, &apierror.APIError{
			Code: "CONTEXT_BUILD_FAILED", Message: err.Error(), Status: http.StatusBadRequest,
		})
		return
	}
	platform.Success(c, resp)
}
