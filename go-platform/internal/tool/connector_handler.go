package tool

import (
	"errors"
	"net/http"

	"github.com/enterprise-agent-platform/go-platform/internal/platform"
	"github.com/enterprise-agent-platform/go-platform/pkg/apierror"
	"github.com/gin-gonic/gin"
)

// ConnectorCredentialHandler M2-D: Connector Binding 与凭证解析端点。
// 全部 internal-only(InternalServiceToken):凭证解析只允许执行器调用。
type ConnectorCredentialHandler struct {
	credentials *CredentialService
}

// NewConnectorCredentialHandler 创建 handler。
func NewConnectorCredentialHandler(credentials *CredentialService) *ConnectorCredentialHandler {
	return &ConnectorCredentialHandler{credentials: credentials}
}

// CreateBinding 处理 POST /internal/v1/connector-bindings。
// 请求体中的 credential_plaintext 加密后立即丢弃,响应只含 credential_ref。
// 租户 ID 必须与认证上下文一致(拒绝跨租户伪造)。
func (h *ConnectorCredentialHandler) CreateBinding(c *gin.Context) {
	var req CreateBindingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		platform.APIError(c, &apierror.APIError{
			Code: "VALIDATION_FAILED", Message: err.Error(), Status: 400,
		})
		return
	}
	// 租户 ID 必须与认证上下文一致(服务端注入,拒绝模型自报覆盖)
	tenantID := c.GetHeader("X-Tenant-ID")
	if tenantID == "" {
		platform.APIError(c, &apierror.APIError{
			Code: "UNAUTHORIZED", Message: "X-Tenant-ID header required", Status: 401,
		})
		return
	}
	if req.TenantID != tenantID {
		platform.APIError(c, &apierror.APIError{
			Code: "FORBIDDEN", Message: "tenant_id mismatch with auth context", Status: 403,
		})
		return
	}
	binding, err := h.credentials.CreateBinding(c.Request.Context(), &req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	platform.Success(c, binding)
}

// ListBindings 处理 GET /internal/v1/connector-bindings。
// 租户 ID 必须从认证上下文 header 获取,拒绝客户端通过 query 参数伪造。
func (h *ConnectorCredentialHandler) ListBindings(c *gin.Context) {
	tenantID := c.GetHeader("X-Tenant-ID")
	if tenantID == "" {
		platform.APIError(c, &apierror.APIError{
			Code: "UNAUTHORIZED", Message: "X-Tenant-ID header required", Status: 401,
		})
		return
	}
	bindings, err := h.credentials.ListBindings(c.Request.Context(), tenantID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	platform.Success(c, bindings)
}

// GetBinding 处理 GET /internal/v1/connector-bindings/:id。
// 绑定按租户隔离,必须与认证上下文一致。
func (h *ConnectorCredentialHandler) GetBinding(c *gin.Context) {
	tenantID := c.GetHeader("X-Tenant-ID")
	if tenantID == "" {
		platform.APIError(c, &apierror.APIError{
			Code: "UNAUTHORIZED", Message: "X-Tenant-ID header required", Status: 401,
		})
		return
	}
	binding, err := h.credentials.FindBinding(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	if binding.TenantID != tenantID {
		platform.APIError(c, &apierror.APIError{
			Code: "CONNECTOR_BINDING_NOT_FOUND", Message: "connector binding not found", Status: 404,
		})
		return
	}
	platform.Success(c, binding)
}

// ResolveCredential 处理 POST /internal/v1/connector-bindings/:id/resolve-credential。
// 执行瞬间解密:响应含明文,仅限执行器(InternalServiceToken)调用;
// 审计只记 credential_ref + resolved 标记。
func (h *ConnectorCredentialHandler) ResolveCredential(c *gin.Context) {
	tenantID := c.GetHeader("X-Tenant-ID")
	if tenantID == "" {
		platform.APIError(c, &apierror.APIError{
			Code: "VALIDATION_FAILED", Message: "X-Tenant-ID header required", Status: 400,
		})
		return
	}
	cred, err := h.credentials.ResolveCredential(c.Request.Context(), tenantID, c.Param("id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	platform.Success(c, gin.H{
		"binding_id":     cred.BindingID,
		"credential_ref": cred.Ref,
		"secret":         cred.Plaintext, // 仅执行器可见;永不入库/日志/审计
		"resolved_at":    cred.ResolvedAt,
	})
}

func (h *ConnectorCredentialHandler) writeError(c *gin.Context, err error) {
	status, code := http.StatusInternalServerError, "INTERNAL_ERROR"
	switch {
	case errors.Is(err, ErrConnectorBindingNotFound):
		status, code = http.StatusNotFound, "CONNECTOR_BINDING_NOT_FOUND"
	case errors.Is(err, ErrConnectorBindingDisabled):
		status, code = http.StatusConflict, "CONNECTOR_BINDING_DISABLED"
	case errors.Is(err, ErrSecretNotConfigured):
		status, code = http.StatusServiceUnavailable, "SECRET_KEY_NOT_CONFIGURED"
	case errors.Is(err, ErrCredentialRefInvalid):
		status, code = http.StatusConflict, "CREDENTIAL_REF_INVALID"
	}
	platform.APIError(c, &apierror.APIError{Code: code, Message: err.Error(), Status: status})
}
