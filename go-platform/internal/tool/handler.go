package tool

import (
	"github.com/enterprise-agent-platform/go-platform/internal/platform"
	"github.com/enterprise-agent-platform/go-platform/pkg/apierror"
	"github.com/gin-gonic/gin"
)

// Handler 处理 Tool Registry 的 HTTP 请求。
type Handler struct {
	repo *Repository
}

// NewHandler 创建 Handler 实例。
func NewHandler(repo *Repository) *Handler {
	return &Handler{repo: repo}
}

// ListTools 处理 GET /api/v1/tools。
// 返回所有 active 状态的 Tool 列表。
func (h *Handler) ListTools(c *gin.Context) {
	tools, err := h.repo.ListTools(
		c.Request.Context(),
		c.Query("domain"),
		c.Query("risk_level"),
		c.Query("is_shared"),
		c.Query("status"),
	)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	platform.Success(c, tools)
}
