package tool

import (
	"context"
	"net/http"
	"time"

	"github.com/enterprise-agent-platform/go-platform/internal/platform"
	"github.com/enterprise-agent-platform/go-platform/pkg/apierror"
	"github.com/gin-gonic/gin"
)

// ── M3-C: Outbox 治理端点(内部服务) ──
//
//	GET   /internal/v1/outbox?tool_call_id=  按 ToolCall 列出关联 Outbox(全链路关联入口)
//	GET   /internal/v1/outbox/:id            单条状态
//	POST  /internal/v1/outbox/:id/compensate 人工触发补偿(Reconcile 决策落地)
//
// 认证:InternalServiceToken(与 /internal/* 一致),由 main.go 路由组挂中间件。

// OutboxHandler Outbox HTTP 处理器。
type OutboxHandler struct {
	store outboxGovernStore
}

// outboxGovernStore Handler 依赖(便于测试替换)。
type outboxGovernStore interface {
	GetByID(ctx context.Context, id string) (*OutboxEntry, error)
	FindByToolCallID(ctx context.Context, toolCallID string) ([]*OutboxEntry, error)
	MarkCompensatePending(ctx context.Context, id, reason string) error
}

// NewOutboxHandler 创建处理器。
func NewOutboxHandler(store *OutboxEntryRepository) *OutboxHandler {
	return &OutboxHandler{store: store}
}

// List 列出某 ToolCall 关联的 Outbox 记录(全链路关联入口)。
func (h *OutboxHandler) List(c *gin.Context) {
	toolCallID := c.Query("tool_call_id")
	if toolCallID == "" {
		platform.APIError(c, &apierror.APIError{
			Code:    "OUTBOX_TOOL_CALL_ID_REQUIRED",
			Message: "tool_call_id query parameter required",
			Status:  http.StatusBadRequest,
		})
		return
	}
	entries, err := h.store.FindByToolCallID(c.Request.Context(), toolCallID)
	if err != nil {
		platform.APIError(c, &apierror.APIError{
			Code: "OUTBOX_QUERY_FAILED", Message: err.Error(), Status: http.StatusInternalServerError,
		})
		return
	}
	platform.Success(c, gin.H{"items": entries})
}

// Get 单条详情。
func (h *OutboxHandler) Get(c *gin.Context) {
	entry, err := h.store.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		status := http.StatusInternalServerError
		if err == ErrOutboxNotFound {
			status = http.StatusNotFound
		}
		platform.APIError(c, &apierror.APIError{
			Code: "OUTBOX_QUERY_FAILED", Message: err.Error(), Status: status,
		})
		return
	}
	platform.Success(c, entry)
}

// Compensate 人工触发补偿:非终态 → compensate_pending,
// 由 Dispatcher 下一轮执行补偿调用(补偿能力由 Connector 自声明)。
func (h *OutboxHandler) Compensate(c *gin.Context) {
	reason := c.DefaultQuery("reason", "manual reconcile trigger")
	if err := h.store.MarkCompensatePending(c.Request.Context(), c.Param("id"), reason); err != nil {
		platform.APIError(c, &apierror.APIError{
			Code: "OUTBOX_COMPENSATE_FAILED", Message: err.Error(), Status: http.StatusInternalServerError,
		})
		return
	}
	platform.Success(c, gin.H{
		"id": c.Param("id"), "state": OutboxStateCompensatePending, "triggered_at": time.Now().UTC(),
	})
}
