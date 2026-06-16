package audit

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/enterprise-agent-platform/go-platform/internal/platform"
	"github.com/enterprise-agent-platform/go-platform/pkg/apierror"
)

// Handler 处理审计日志相关的 HTTP 请求。
type Handler struct {
	repo *Repository
}

// NewHandler 创建 Handler 实例。
func NewHandler(repo *Repository) *Handler {
	return &Handler{repo: repo}
}

// ListAuditLogs 处理 GET /api/v1/audit-logs。
// 支持 query 参数：business_app_code, action, actor_user_id, resource_type, page, page_size
func (h *Handler) ListAuditLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}

	logs, total, err := h.repo.ListLogs(c.Request.Context(), ListParams{
		BusinessAppCode: c.Query("business_app_code"),
		Action:          c.Query("action"),
		ActorUserID:     c.Query("actor_user_id"),
		ResourceType:    c.Query("resource_type"),
		Page:            page,
		PageSize:        pageSize,
	})
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}

	// 转换为精简响应
	items := make([]ListResponse, len(logs))
	for i, l := range logs {
		items[i] = ListResponse{
			ID:              l.ID,
			TraceID:         l.TraceID,
			ActorUserID:     l.ActorUserID,
			BusinessAppCode: l.BusinessAppCode,
			Action:          l.Action,
			ResourceType:    l.ResourceType,
			ResourceID:      l.ResourceID,
			Status:          l.Status,
			DetailJSON:      l.DetailJSON,
			CreatedAt:       l.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}
	platform.List(c, items, page, pageSize, total)
}
