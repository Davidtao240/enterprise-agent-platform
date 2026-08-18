package tool

import (
	"context"
	"net/http"

	"github.com/enterprise-agent-platform/go-platform/internal/platform"
	"github.com/enterprise-agent-platform/go-platform/pkg/apierror"
	"github.com/gin-gonic/gin"
)

// ── M6-C: 连接器授权范围可视化端点(protected,tool:manage) ──
//
//	GET /api/v1/connector-registry   Connector 注册表(全局平台数据:code/版本/能力/outbox 声明)
//	GET /api/v1/connector-bindings   租户内 Binding 列表(工具 ↔ 连接器授权关系)
//
// 与 internal 端点区别:JWT 认证 + tenant_id 注入(Binding 严格租户隔离);
// 不暴露 ResolveCredential(明文凭证仅限执行器)。
// Spec WORKBENCH_DESIGN.md §6.3。

// ConnectorOpsStore ConnectorOpsHandler 依赖(便于测试替换)。
type ConnectorOpsStore interface {
	List(ctx context.Context) ([]*RegistryEntry, error)
}

// ConnectorBindingLister Binding 列表契约(CredentialService 实现)。
type ConnectorBindingLister interface {
	ListBindings(ctx context.Context, tenantID string) ([]*ConnectorBinding, error)
}

// ConnectorOpsHandler 连接器可视化 HTTP 处理器。
type ConnectorOpsHandler struct {
	registry   ConnectorOpsStore
	credentials ConnectorBindingLister
}

// NewConnectorOpsHandler 创建处理器。
func NewConnectorOpsHandler(registry ConnectorOpsStore, credentials ConnectorBindingLister) *ConnectorOpsHandler {
	return &ConnectorOpsHandler{registry: registry, credentials: credentials}
}

// ListRegistry GET /api/v1/connector-registry。
func (h *ConnectorOpsHandler) ListRegistry(c *gin.Context) {
	entries, err := h.registry.List(c.Request.Context())
	if err != nil {
		platform.APIError(c, &apierror.APIError{
			Code: "CONNECTOR_REGISTRY_LIST_FAILED", Message: err.Error(), Status: http.StatusInternalServerError,
		})
		return
	}
	platform.Success(c, gin.H{"items": entries})
}

// ListBindings GET /api/v1/connector-bindings — tenant_id 由认证上下文注入。
func (h *ConnectorOpsHandler) ListBindings(c *gin.Context) {
	bindings, err := h.credentials.ListBindings(c.Request.Context(), c.GetString("tenant_id"))
	if err != nil {
		platform.APIError(c, &apierror.APIError{
			Code: "CONNECTOR_BINDING_LIST_FAILED", Message: err.Error(), Status: http.StatusInternalServerError,
		})
		return
	}
	platform.Success(c, gin.H{"items": bindings})
}
