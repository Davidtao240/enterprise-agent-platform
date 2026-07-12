package audit

import (
	"encoding/csv"
	"net/http"
	"strconv"

	"github.com/enterprise-agent-platform/go-platform/internal/platform"
	"github.com/enterprise-agent-platform/go-platform/pkg/apierror"
	"github.com/gin-gonic/gin"
)

// Handler 处理审计日志相关的 HTTP 请求。
type Handler struct {
	repo *Repository
}

// NewHandler 创建 Handler 实例。
func NewHandler(repo *Repository) *Handler {
	return &Handler{repo: repo}
}

func (h *Handler) listParams(c *gin.Context) ListParams {
	return ListParams{
		TenantID:        c.GetString("tenant_id"),
		TraceID:         c.Query("trace_id"),
		BusinessAppCode: c.Query("business_app_code"),
		Action:          c.Query("action"),
		ActorUserID:     c.Query("actor_user_id"),
		ResourceType:    c.Query("resource_type"),
		ResourceID:      c.Query("resource_id"),
		Status:          c.Query("status"),
		CreatedFrom:     c.Query("created_from"),
		CreatedTo:       c.Query("created_to"),
	}
}

// ListAuditLogs 处理 GET /api/v1/audit-logs。
func (h *Handler) ListAuditLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}

	params := h.listParams(c)
	params.Page = page
	params.PageSize = pageSize

	logs, total, err := h.repo.ListLogs(c.Request.Context(), params)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}

	platform.List(c, toListResponse(logs), page, pageSize, total)
}

func (h *Handler) Stats(c *gin.Context) {
	stats, err := h.repo.Stats(c.Request.Context(), h.listParams(c))
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	platform.Success(c, stats)
}

// ExportCSV exports up to 10,000 filtered audit records for operational review.
// It deliberately uses the same filters as ListAuditLogs to preserve RBAC-scoped
// query semantics and avoid an unbounded database download endpoint.
func (h *Handler) ExportCSV(c *gin.Context) {
	params := h.listParams(c)
	params.Page = 1
	params.PageSize = 10000
	logs, _, err := h.repo.ListLogs(c.Request.Context(), params)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="audit-logs.csv"`)
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Status(http.StatusOK)
	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"id", "trace_id", "actor_user_id", "business_app_code", "action", "resource_type", "resource_id", "status", "detail_json", "created_at"})
	for _, item := range logs {
		_ = w.Write([]string{item.ID, item.TraceID, deref(item.ActorUserID), deref(item.BusinessAppCode), item.Action, item.ResourceType, item.ResourceID, item.Status, deref(item.DetailJSON), item.CreatedAt.Format("2006-01-02T15:04:05Z07:00")})
	}
	w.Flush()
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
