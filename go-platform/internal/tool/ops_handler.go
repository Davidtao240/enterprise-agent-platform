package tool

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/enterprise-agent-platform/go-platform/internal/platform"
	"github.com/enterprise-agent-platform/go-platform/pkg/apierror"
	"github.com/gin-gonic/gin"
)

// ── M6-B: 可靠性运维查询(protected 端点,JWT 租户隔离) ──
//
//	GET  /api/v1/ops/tool-calls?status=&tool_id=&limit=  Tool Call 探索器 (tool:read)
//	GET  /api/v1/ops/tool-calls/dead-letters             DLQ (tool:read)
//	GET  /api/v1/ops/tool-calls/:id                      详情 (tool:read)
//	GET  /api/v1/ops/outbox?state=&limit=                Outbox 监控 (outbox:read)
//	GET  /api/v1/ops/outbox/:id                          详情 (outbox:read)
//	POST /api/v1/ops/outbox/:id/compensate               人工补偿 (outbox:read)
//
// 与 /internal/* 端点的区别:租户来自认证上下文而非信任边界,
// 所有查询强制 tenant_id 过滤(Spec WORKBENCH_DESIGN.md §6.2)。

// ── 租户隔离的仓储扩展 ──

// ListForTenant 租户内 Tool Call 列表(updated_at 倒序;status/tool_id 可选过滤)。
func (r *ToolCallRepository) ListForTenant(ctx context.Context, tenantID, status, toolID string, limit int) ([]*ToolCall, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	query := toolCallSelect + " WHERE tenant_id = $1"
	args := []any{tenantID}
	if status != "" {
		args = append(args, status)
		query += fmt.Sprintf(" AND status = $%d", len(args))
	}
	if toolID != "" {
		args = append(args, toolID)
		query += fmt.Sprintf(" AND tool_id = $%d", len(args))
	}
	args = append(args, limit)
	query += fmt.Sprintf(" ORDER BY updated_at DESC LIMIT $%d", len(args))
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*ToolCall
	for rows.Next() {
		tc, err := scanToolCall(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, tc)
	}
	return result, rows.Err()
}

// GetByIDForTenant 按 ID + 租户查找(未命中返回 ErrToolCallNotFound)。
func (r *ToolCallRepository) GetByIDForTenant(ctx context.Context, tenantID, id string) (*ToolCall, error) {
	tc, err := scanToolCall(r.pool.QueryRow(ctx, toolCallSelect+" WHERE tenant_id = $1 AND id = $2", tenantID, id))
	if err != nil {
		return nil, err
	}
	return tc, nil
}

// ListDeadLettersForTenant 租户内死信列表(DLQ)。
func (r *ToolCallRepository) ListDeadLettersForTenant(ctx context.Context, tenantID string, limit int) ([]*ToolCall, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return r.queryToolCallList(ctx,
		toolCallSelect+" WHERE tenant_id = $1 AND is_dead_letter ORDER BY updated_at DESC LIMIT $2",
		tenantID, limit)
}

// queryToolCallList ListForTenant 等共用的行集扫描。
func (r *ToolCallRepository) queryToolCallList(ctx context.Context, sql string, args ...any) ([]*ToolCall, error) {
	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*ToolCall
	for rows.Next() {
		tc, err := scanToolCall(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, tc)
	}
	return result, rows.Err()
}

// ListForTenant 租户内 Outbox 列表(state 可选过滤;updated_at 倒序)。
func (r *OutboxEntryRepository) ListForTenant(ctx context.Context, tenantID, state string, limit int) ([]*OutboxEntry, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if state != "" {
		return r.queryList(ctx,
			outboxSelect+" WHERE tenant_id = $1 AND state = $2 ORDER BY updated_at DESC LIMIT $3",
			tenantID, state, limit)
	}
	return r.queryList(ctx,
		outboxSelect+" WHERE tenant_id = $1 ORDER BY updated_at DESC LIMIT $2", tenantID, limit)
}

// GetByIDForTenant 按 ID + 租户查找(未命中返回 ErrOutboxNotFound)。
func (r *OutboxEntryRepository) GetByIDForTenant(ctx context.Context, tenantID, id string) (*OutboxEntry, error) {
	return scanOutboxEntry(r.pool.QueryRow(ctx, outboxSelect+" WHERE tenant_id = $1 AND id = $2", tenantID, id))
}

// ── Handler ──

// OpsStore OpsHandler 依赖(便于测试替换)。
type OpsStore interface {
	ListForTenant(ctx context.Context, tenantID, status, toolID string, limit int) ([]*ToolCall, error)
	GetByIDForTenant(ctx context.Context, tenantID, id string) (*ToolCall, error)
	ListDeadLettersForTenant(ctx context.Context, tenantID string, limit int) ([]*ToolCall, error)
}

// OutboxOpsStore Outbox 运维依赖。
type OutboxOpsStore interface {
	ListForTenant(ctx context.Context, tenantID, state string, limit int) ([]*OutboxEntry, error)
	GetByIDForTenant(ctx context.Context, tenantID, id string) (*OutboxEntry, error)
	MarkCompensatePending(ctx context.Context, id, reason string) error
}

// OpsHandler 可靠性运维 HTTP 处理器(ToolCall 探索器 + DLQ)。
type OpsHandler struct {
	store OpsStore
}

// NewOpsHandler 创建 ToolCall 运维处理器。
func NewOpsHandler(store OpsStore) *OpsHandler { return &OpsHandler{store: store} }

// ListToolCalls GET /api/v1/ops/tool-calls。
func (h *OpsHandler) ListToolCalls(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	calls, err := h.store.ListForTenant(c.Request.Context(), c.GetString("tenant_id"), c.Query("status"), c.Query("tool_id"), limit)
	if err != nil {
		platform.APIError(c, &apierror.APIError{
			Code: "TOOL_CALL_LIST_FAILED", Message: err.Error(), Status: http.StatusInternalServerError,
		})
		return
	}
	platform.Success(c, gin.H{"items": calls})
}

// GetToolCall GET /api/v1/ops/tool-calls/:id。
func (h *OpsHandler) GetToolCall(c *gin.Context) {
	tc, err := h.store.GetByIDForTenant(c.Request.Context(), c.GetString("tenant_id"), c.Param("id"))
	if err != nil {
		status, code := http.StatusInternalServerError, "TOOL_CALL_GET_FAILED"
		if err == ErrToolCallNotFound {
			status, code = http.StatusNotFound, "TOOL_CALL_NOT_FOUND"
		}
		platform.APIError(c, &apierror.APIError{Code: code, Message: err.Error(), Status: status})
		return
	}
	platform.Success(c, tc)
}

// ListDeadLetters GET /api/v1/ops/tool-calls/dead-letters。
func (h *OpsHandler) ListDeadLetters(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	calls, err := h.store.ListDeadLettersForTenant(c.Request.Context(), c.GetString("tenant_id"), limit)
	if err != nil {
		platform.APIError(c, &apierror.APIError{
			Code: "TOOL_CALL_DLQ_FAILED", Message: err.Error(), Status: http.StatusInternalServerError,
		})
		return
	}
	platform.Success(c, gin.H{"items": calls})
}

// OutboxOpsHandler Outbox 运维处理器(监控 + 人工补偿)。
type OutboxOpsHandler struct {
	store OutboxOpsStore
}

// NewOutboxOpsHandler 创建 Outbox 运维处理器。
func NewOutboxOpsHandler(store OutboxOpsStore) *OutboxOpsHandler {
	return &OutboxOpsHandler{store: store}
}

// ListOutbox GET /api/v1/ops/outbox。
func (h *OutboxOpsHandler) ListOutbox(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	entries, err := h.store.ListForTenant(c.Request.Context(), c.GetString("tenant_id"), c.Query("state"), limit)
	if err != nil {
		platform.APIError(c, &apierror.APIError{
			Code: "OUTBOX_LIST_FAILED", Message: err.Error(), Status: http.StatusInternalServerError,
		})
		return
	}
	platform.Success(c, gin.H{"items": entries})
}

// GetOutbox GET /api/v1/ops/outbox/:id。
func (h *OutboxOpsHandler) GetOutbox(c *gin.Context) {
	entry, err := h.store.GetByIDForTenant(c.Request.Context(), c.GetString("tenant_id"), c.Param("id"))
	if err != nil {
		status, code := http.StatusInternalServerError, "OUTBOX_GET_FAILED"
		if err == ErrOutboxNotFound {
			status, code = http.StatusNotFound, "OUTBOX_NOT_FOUND"
		}
		platform.APIError(c, &apierror.APIError{Code: code, Message: err.Error(), Status: status})
		return
	}
	platform.Success(c, entry)
}

// Compensate POST /api/v1/ops/outbox/:id/compensate — 人工触发补偿。
// 先做租户归属校验,再转入 compensate_pending(终态拒绝,守卫同 internal 端点)。
func (h *OutboxOpsHandler) Compensate(c *gin.Context) {
	tenantID, id := c.GetString("tenant_id"), c.Param("id")
	entry, err := h.store.GetByIDForTenant(c.Request.Context(), tenantID, id)
	if err != nil {
		status, code := http.StatusInternalServerError, "OUTBOX_COMPENSATE_FAILED"
		if err == ErrOutboxNotFound {
			status, code = http.StatusNotFound, "OUTBOX_NOT_FOUND"
		}
		platform.APIError(c, &apierror.APIError{Code: code, Message: err.Error(), Status: status})
		return
	}
	reason := c.DefaultQuery("reason", "manual reconcile via workbench")
	if err := h.store.MarkCompensatePending(c.Request.Context(), id, reason); err != nil {
		platform.APIError(c, &apierror.APIError{
			Code: "OUTBOX_COMPENSATE_FAILED", Message: err.Error(), Status: http.StatusInternalServerError,
		})
		return
	}
	platform.Success(c, gin.H{"id": entry.ID, "state": OutboxStateCompensatePending, "triggered_at": time.Now().UTC()})
}
