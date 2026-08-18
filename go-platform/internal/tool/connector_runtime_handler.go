package tool

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ── M3-A: Connector Runtime HTTP(internal-only,治理视图 + 健康探测)──

// ConnectorRuntimeHandler 注册表查询与 Connector 健康检查。
type ConnectorRuntimeHandler struct {
	registry *ConnectorRegistryRepository
	runtime  *ConnectorRuntime
}

// NewConnectorRuntimeHandler 创建 handler。
func NewConnectorRuntimeHandler(registry *ConnectorRegistryRepository, runtime *ConnectorRuntime) *ConnectorRuntimeHandler {
	return &ConnectorRuntimeHandler{registry: registry, runtime: runtime}
}

// ListRegistry GET /internal/v1/connectors — 全量注册表。
func (h *ConnectorRuntimeHandler) ListRegistry(c *gin.Context) {
	entries, err := h.registry.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"connectors": entries})
}

// HealthCheck GET /internal/v1/connectors/:code/health — 供应商健康探测(只读)。
func (h *ConnectorRuntimeHandler) HealthCheck(c *gin.Context) {
	health, err := h.runtime.HealthCheck(c.Request.Context(), c.Param("code"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, health)
}
