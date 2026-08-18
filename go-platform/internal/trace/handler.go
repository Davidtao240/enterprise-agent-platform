package trace

import (
	"net/http"

	"github.com/enterprise-agent-platform/go-platform/internal/platform"
	"github.com/enterprise-agent-platform/go-platform/pkg/apierror"
	"github.com/gin-gonic/gin"
)

// Handler M5-A Trace 端点:
//
//	POST /internal/v1/trace/events   批量追加 (InternalServiceToken + X-Tenant-ID)
//	GET  /api/v1/traces/:trace_id    查询某 Trace 全部事件 (trace:read)
type Handler struct {
	repo *Repository
	sink Sink
}

// NewHandler 创建 Handler。repo 用于查询,sink 用于同步小批量写入(内部端点)。
func NewHandler(repo *Repository, sink Sink) *Handler {
	return &Handler{repo: repo, sink: sink}
}

type appendRequest struct {
	Events []*Event `json:"events" binding:"required,dive"`
}

// Append 处理 POST /internal/v1/trace/events。
// 事件 tenant_id 必须与认证上下文 (X-Tenant-ID) 一致,缺失时以 header 填充。
func (h *Handler) Append(c *gin.Context) {
	tenantID := c.GetHeader("X-Tenant-ID")
	if tenantID == "" {
		platform.APIError(c, &apierror.APIError{
			Code: "UNAUTHORIZED", Message: "X-Tenant-ID header required", Status: http.StatusUnauthorized,
		})
		return
	}
	var req appendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		platform.APIError(c, &apierror.APIError{
			Code: "VALIDATION_FAILED", Message: err.Error(), Status: http.StatusBadRequest,
		})
		return
	}
	accepted := 0
	for _, e := range req.Events {
		if e == nil {
			continue
		}
		if e.TenantID == "" {
			e.TenantID = tenantID
		}
		if e.TenantID != tenantID {
			platform.APIError(c, &apierror.APIError{
				Code: "FORBIDDEN", Message: "event tenant_id mismatch with auth context", Status: http.StatusForbidden,
			})
			return
		}
		if e.Validate() != nil {
			platform.APIError(c, &apierror.APIError{
				Code: "VALIDATION_FAILED", Message: "invalid trace event (layer/event_type/trace_id required)", Status: http.StatusBadRequest,
			})
			return
		}
		accepted++
	}
	h.sink.Record(c.Request.Context(), req.Events...)
	platform.Success(c, gin.H{"accepted": accepted})
}

// GetTrace 处理 GET /api/v1/traces/:trace_id (trace:read)。
func (h *Handler) GetTrace(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	events, err := h.repo.ListByTraceID(c.Request.Context(), tenantID, c.Param("trace_id"))
	if err != nil {
		platform.APIError(c, &apierror.APIError{
			Code: "TRACE_QUERY_FAILED", Message: err.Error(), Status: http.StatusInternalServerError,
		})
		return
	}
	platform.Success(c, gin.H{"trace_id": c.Param("trace_id"), "events": events})
}
