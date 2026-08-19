package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// ── M3-C: Outbox Dispatcher ──
//
// 周期推进 connector_outbox 状态机,覆盖 M3-C Gate:
//
//	部分成功可恢复:
//	  - 投递失败按指数退避重试(attempts/next_attempt_at);
//	  - 重试耗尽且未发出 → failed(无副作用,直接失败);
//	  - ToolCall 被取消/审批拒绝且已发出 → compensate_pending
//	    → 调用 Connector 补偿能力撤销外部单据 → compensated;
//	  - "已发出未确认"(indeterminate)→ sent,由 stale Verify 收敛。
//
//	外部请求全链路关联:
//	  outbox.tool_call_id → tool_calls(trace_id/external_request_id)
//	  → Run/Step/Audit;结果经 ConfirmExecution 回写,状态机守卫保证幂等。
//
// 状态机(见 outbox_repository.go 头注):
//
//	pending ──dispatch ok──> sent ──verify confirmed──> confirmed
//	   │                        │
//	   │ dispatch fail          │ ToolCall cancelled / 人工触发
//	   │ (退避重试)             ↓
//	   ├─ attempts 耗尽 ──> compensate_pending ──cancel ok──> compensated
//	   │       │                   │ cancel fail(退避,耗尽→failed)
//	   │       └─ 无副作用 ──> failed
//	   └─ 业务拒绝(failed 结果)──> failed

// outboxStore Dispatcher 依赖的仓储接口(便于测试替换)。
type outboxStore interface {
	ListDispatchable(ctx context.Context, now time.Time, limit int) ([]*OutboxEntry, error)
	ListStaleSent(ctx context.Context, staleBefore time.Time, limit int) ([]*OutboxEntry, error)
	ListCompensatable(ctx context.Context, now time.Time, limit int) ([]*OutboxEntry, error)
	MarkSent(ctx context.Context, id, externalRequestID, externalObjectID string) error
	MarkConfirmed(ctx context.Context, id string) error
	MarkAttemptFailed(ctx context.Context, id string, attempts int, nextAttemptAt time.Time, lastErr string) error
	MarkCompensatePending(ctx context.Context, id, tenantID, reason string) error
	MarkCompensated(ctx context.Context, id string) error
	MarkFailedFinal(ctx context.Context, id, code string) error
	Touch(ctx context.Context, id string) error
}

// outboxExecutor Dispatcher 对 Runtime 的依赖(便于测试替换)。
type outboxExecutor interface {
	Execute(ctx context.Context, req *RuntimeExecuteRequest) (*ConnectorResult, error)
	Verify(ctx context.Context, connectorCode string, req *ConnectorVerifyRequest) (*ConnectorVerifyResult, error)
	OutboxConnectorFor(connectorCode string) (OutboxConnector, bool)
}

// outboxToolCalls Dispatcher 对 ToolCall 读取的依赖。
type outboxToolCalls interface {
	GetByID(ctx context.Context, id string) (*ToolCall, error)
}

// outboxConfirmer 结果回写 ToolCall(= tool.Service.ConfirmExecution)。
type outboxConfirmer interface {
	ConfirmExecution(ctx context.Context, toolCallID string, req *ConfirmExecutionRequest) (*ToolCall, error)
}

// OutboxDispatcher Outbox 周期调度器。
type OutboxDispatcher struct {
	store           outboxStore
	executor        outboxExecutor
	toolCalls       outboxToolCalls
	confirmer       outboxConfirmer
	every           time.Duration
	batch           int
	maxAttempts     int
	backoffBase     time.Duration
	confirmTimeout  time.Duration
}

// NewOutboxDispatcher 创建调度器。
// every<=0 → 5s;batch<=0 → 50;maxAttempts<=0 → 5;
// backoffBase<=0 → 10s;confirmTimeout<=0 → 60s。
func NewOutboxDispatcher(store outboxStore, executor outboxExecutor, toolCalls outboxToolCalls,
	confirmer outboxConfirmer, every time.Duration, batch, maxAttempts int,
	backoffBase, confirmTimeout time.Duration) *OutboxDispatcher {
	if every <= 0 {
		every = 5 * time.Second
	}
	if batch <= 0 {
		batch = 50
	}
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	if backoffBase <= 0 {
		backoffBase = 10 * time.Second
	}
	if confirmTimeout <= 0 {
		confirmTimeout = 60 * time.Second
	}
	return &OutboxDispatcher{
		store: store, executor: executor, toolCalls: toolCalls, confirmer: confirmer,
		every: every, batch: batch, maxAttempts: maxAttempts,
		backoffBase: backoffBase, confirmTimeout: confirmTimeout,
	}
}

// Backoff 第 attempts 次失败后的退避间隔(base * 2^attempts)。
func (d *OutboxDispatcher) Backoff(attempts int) time.Duration {
	b := d.backoffBase << uint(min(attempts, 6)) // 封顶 64 倍防溢出
	return b
}

// Start 阻塞循环(由 goroutine 启动)。
func (d *OutboxDispatcher) Start(ctx context.Context) {
	ticker := time.NewTicker(d.every)
	defer ticker.Stop()
	log.Printf("[outbox] dispatcher started (every=%s batch=%d maxAttempts=%d backoff=%s confirmTimeout=%s)",
		d.every, d.batch, d.maxAttempts, d.backoffBase, d.confirmTimeout)
	for {
		select {
		case <-ctx.Done():
			log.Println("[outbox] dispatcher stopped")
			return
		case <-ticker.C:
			d.RunOnce(ctx)
		}
	}
}

// RunOnce 单轮调度:投递 → stale 收敛 → 补偿。返回 (dispatched, confirmed, compensated, failed)。
func (d *OutboxDispatcher) RunOnce(ctx context.Context) (int, int, int, int) {
	dispatched := d.dispatchPending(ctx)
	confirmed := d.confirmStaleSent(ctx)
	compensated, failed := d.runCompensations(ctx)
	return dispatched, confirmed, compensated, failed
}

// ── 1. 投递 pending ──

func (d *OutboxDispatcher) dispatchPending(ctx context.Context) int {
	entries, err := d.store.ListDispatchable(ctx, time.Now().UTC(), d.batch)
	if err != nil {
		log.Printf("[outbox] list dispatchable failed: %v", err)
		return 0
	}
	n := 0
	for _, e := range entries {
		if d.dispatchOne(ctx, e) {
			n++
		}
	}
	return n
}

func (d *OutboxDispatcher) dispatchOne(ctx context.Context, e *OutboxEntry) bool {
	// ToolCall 状态联动:取消/审批拒绝 → 补偿或终止。
	tc, err := d.toolCalls.GetByID(ctx, e.ToolCallID)
	if err != nil {
		log.Printf("[outbox] get tool_call %s failed: %v", e.ToolCallID, err)
		return false
	}
	switch tc.Status {
	case ToolCallStatusCancelled:
		// 部分成功可恢复:已发出的外部单据必须补偿,未发出的直接终止。
		// ToolCall 已处于 cancelled 终态,此处只推进 outbox 状态。
		if e.ExternalRequestID != nil && *e.ExternalRequestID != "" {
			if err := d.store.MarkCompensatePending(ctx, e.ID, "", "tool_call cancelled; compensating external side effect"); err != nil {
				log.Printf("[outbox] mark compensate for %s failed: %v", e.ID, err)
			}
		} else {
			_ = d.store.MarkFailedFinal(ctx, e.ID, "TOOL_CALL_CANCELLED")
		}
		return false
	case ToolCallStatusSucceeded, ToolCallStatusFailed, ToolCallStatusIndeterminate:
		// 终态后不再投递(executing 超时被扫描器转 indeterminate 的,
		// outbox 保持 pending 等待人工 Reconcile 决策;此处仅跳过轮次)。
		return false
	}

	// binding 来自 ToolCall 快照(全链路关联)。
	if tc.ConnectorBindingID == nil || *tc.ConnectorBindingID == "" {
		_ = d.store.MarkFailedFinal(ctx, e.ID, "TOOL_CALL_BINDING_MISSING")
		d.confirmToolCall(ctx, tc, ToolCallStatusFailed, &ConnectorError{
			Code: "TOOL_CALL_BINDING_MISSING", Message: "tool call has no connector binding",
		})
		return false
	}

	var input map[string]any
	if err := json.Unmarshal([]byte(e.PayloadJSON), &input); err != nil {
		// payload 损坏不可恢复(Outbox 不可变请求体不应损坏)。
		_ = d.store.MarkFailedFinal(ctx, e.ID, "PAYLOAD_UNPARSEABLE")
		d.confirmToolCall(ctx, tc, ToolCallStatusFailed, &ConnectorError{
			Code: "PAYLOAD_UNPARSEABLE", Message: err.Error(),
		})
		return false
	}

	result, execErr := d.executor.Execute(ctx, &RuntimeExecuteRequest{
		TenantID:   tc.TenantID,
		BindingID:  *tc.ConnectorBindingID,
		ToolCallID: tc.ID,
		TraceID:    tc.TraceID,
		Capability: e.Operation,
		Input:      input,
	})

	switch {
	case execErr != nil:
		// 传输/连接类失败:退避重试;耗尽 → 无副作用路径终态失败。
		attempts := e.Attempts + 1
		if attempts >= d.maxAttempts {
			_ = d.store.MarkFailedFinal(ctx, e.ID, fmt.Sprintf("DISPATCH_EXHAUSTED: %v", execErr))
			d.confirmToolCall(ctx, tc, ToolCallStatusFailed, &ConnectorError{
				Code: "TOOL_OUTBOX_DISPATCH_EXHAUSTED",
				Message: fmt.Sprintf("outbox dispatch failed after %d attempts: %v", attempts, execErr),
			})
			return false
		}
		_ = d.store.MarkAttemptFailed(ctx, e.ID, attempts, time.Now().UTC().Add(d.Backoff(attempts)), execErr.Error())
		return false

	case result.Status == ConnectorStatusSucceeded:
		// 投递成功 → sent + 立即 Verify(Mock 同步语义;真实异步靠 stale 扫描)。
		if err := d.store.MarkSent(ctx, e.ID, result.ExternalRequestID, result.ExternalObjectID); err != nil {
			log.Printf("[outbox] mark sent for %s failed: %v", e.ID, err)
			return false
		}
		e.ExternalRequestID = strPtr(result.ExternalRequestID)
		e.ExternalObjectID = strPtr(result.ExternalObjectID)
		vr, verr := d.executor.Verify(ctx, e.ConnectorCode, &ConnectorVerifyRequest{
			TenantID: tc.TenantID, ToolCallID: tc.ID,
			ExternalRequestID: result.ExternalRequestID, ExternalObjectID: result.ExternalObjectID,
		})
		if verr != nil || !vr.Confirmed {
			log.Printf("[outbox] immediate verify inconclusive for %s (err=%v confirmed=%v); awaiting stale scan",
				e.ID, verr, vr.Confirmed)
			return true
		}
		_ = d.store.MarkConfirmed(ctx, e.ID)
		d.confirmWithExternal(ctx, tc, result, vr)
		return true

	case result.Status == ConnectorStatusIndeterminate:
		// 部分成功:请求可能已被外部接受(副作用可能已发生)→ sent,
		// 由 stale Verify 收敛(不允许静默重发,防止副作用重复)。
		if err := d.store.MarkSent(ctx, e.ID, result.ExternalRequestID, result.ExternalObjectID); err != nil {
			log.Printf("[outbox] mark sent(indeterminate) for %s failed: %v", e.ID, err)
			return false
		}
		d.confirmToolCall(ctx, tc, ToolCallStatusIndeterminate, result.Error)
		return true

	default: // ConnectorStatusFailed(业务拒绝,不可重试)
		_ = d.store.MarkFailedFinal(ctx, e.ID, "BUSINESS_REJECTED")
		d.confirmToolCall(ctx, tc, ToolCallStatusFailed, result.Error)
		return false
	}
}

// ── 2. stale sent 收敛(Verify) ──

func (d *OutboxDispatcher) confirmStaleSent(ctx context.Context) int {
	staleBefore := time.Now().UTC().Add(-d.confirmTimeout)
	entries, err := d.store.ListStaleSent(ctx, staleBefore, d.batch)
	if err != nil {
		log.Printf("[outbox] list stale sent failed: %v", err)
		return 0
	}
	n := 0
	for _, e := range entries {
		tc, err := d.toolCalls.GetByID(ctx, e.ToolCallID)
		if err != nil {
			continue
		}
		// ToolCall 在 sent 期间被取消 → 自动补偿(部分成功可恢复)。
		if tc.Status == ToolCallStatusCancelled {
			if err := d.store.MarkCompensatePending(ctx, e.ID, "", "tool_call cancelled while awaiting confirmation; compensating"); err != nil {
				log.Printf("[outbox] mark compensate for %s failed: %v", e.ID, err)
			}
			continue
		}
		vr, verr := d.executor.Verify(ctx, e.ConnectorCode, &ConnectorVerifyRequest{
			TenantID: tc.TenantID, ToolCallID: tc.ID,
			ExternalRequestID: derefStr(e.ExternalRequestID), ExternalObjectID: derefStr(e.ExternalObjectID),
		})
		switch {
		case verr != nil:
			// Verify 自身失败(网络):增加 attempts 计数并退避重试。
			// 达到最大重试次数后转 compensation 或 failed,防止无限挂起。
			attempts := e.Attempts + 1
			if attempts >= d.maxAttempts {
				_ = d.store.MarkFailedFinal(ctx, e.ID, fmt.Sprintf("VERIFY_EXHAUSTED: %v", verr))
				d.confirmToolCall(ctx, tc, ToolCallStatusFailed, &ConnectorError{
					Code: "TOOL_OUTBOX_VERIFY_EXHAUSTED",
					Message: fmt.Sprintf("verify failed after %d attempts: %v", attempts, verr),
				})
			} else {
				_ = d.store.MarkAttemptFailed(ctx, e.ID, attempts, time.Now().UTC().Add(d.Backoff(attempts)), fmt.Sprintf("verify: %v", verr))
			}

		case vr.Confirmed:
			_ = d.store.MarkConfirmed(ctx, e.ID)
			outJSON := ""
			if vr.Observed != nil {
				if b, err := json.Marshal(vr.Observed); err == nil {
					outJSON = string(b)
				}
			}
			req := &ConfirmExecutionRequest{
				Status:            ToolCallStatusSucceeded,
				OutputSummaryJSON: outJSON,
				VerificationJSON:  verifyJSON(vr),
			}
			d.confirm(ctx, tc.ID, req)
			n++

		default:
			// 明确未执行(单据不存在):无副作用 → 终态失败。
			_ = d.store.MarkFailedFinal(ctx, e.ID, "NOT_CONFIRMED_NOT_EXECUTED")
			d.confirmToolCall(ctx, tc, ToolCallStatusFailed, &ConnectorError{
				Code: "TOOL_OUTBOX_NOT_CONFIRMED", Message: "external side effect not observed: " + vr.Detail,
			})
		}
	}
	return n
}

// ── 3. 补偿 ──

func (d *OutboxDispatcher) runCompensations(ctx context.Context) (int, int) {
	entries, err := d.store.ListCompensatable(ctx, time.Now().UTC(), d.batch)
	if err != nil {
		log.Printf("[outbox] list compensatable failed: %v", err)
		return 0, 0
	}
	comp, failed := 0, 0
	for _, e := range entries {
		switch d.compensateOne(ctx, e) {
		case compensationDone:
			comp++
		case compensationFailed:
			failed++
		}
	}
	return comp, failed
}

type compensationOutcome int

const (
	compensationDone compensationOutcome = iota
	compensationFailed
	compensationRetry
)

func (d *OutboxDispatcher) compensateOne(ctx context.Context, e *OutboxEntry) compensationOutcome {
	tc, err := d.toolCalls.GetByID(ctx, e.ToolCallID)
	if err != nil {
		return compensationRetry
	}

	conn, ok := d.executor.OutboxConnectorFor(e.ConnectorCode)
	if !ok {
		// Connector 未提供补偿契约:终态失败留痕,人工介入。
		_ = d.store.MarkFailedFinal(ctx, e.ID, "COMPENSATION_UNSUPPORTED")
		d.confirmToolCall(ctx, tc, ToolCallStatusFailed, &ConnectorError{
			Code: "TOOL_OUTBOX_COMPENSATION_UNSUPPORTED",
			Message: fmt.Sprintf("connector '%s' does not implement compensation", e.ConnectorCode),
		})
		return compensationFailed
	}

	capability, input, ok := conn.BuildCompensation(e, nil)
	if !ok {
		_ = d.store.MarkFailedFinal(ctx, e.ID, "COMPENSATION_UNBUILDABLE")
		d.confirmToolCall(ctx, tc, ToolCallStatusFailed, &ConnectorError{
			Code: "TOOL_OUTBOX_COMPENSATION_UNBUILDABLE",
			Message: "no external object id; nothing to compensate",
		})
		return compensationFailed
	}

	// 补偿调用也需要 binding(经 Runtime 门禁)。
	if tc.ConnectorBindingID == nil || *tc.ConnectorBindingID == "" {
		_ = d.store.MarkFailedFinal(ctx, e.ID, "TOOL_CALL_BINDING_MISSING")
		return compensationFailed
	}

	result, execErr := d.executor.Execute(ctx, &RuntimeExecuteRequest{
		TenantID:   tc.TenantID,
		BindingID:  *tc.ConnectorBindingID,
		ToolCallID: tc.ID,
		TraceID:    tc.TraceID,
		Capability: capability,
		Input:      input,
	})
	switch {
	case execErr != nil:
		return d.compensateRetryOrFail(ctx, e, tc, execErr.Error())
	case result.Status == ConnectorStatusSucceeded:
		_ = d.store.MarkCompensated(ctx, e.ID)
		errJSON, _ := json.Marshal(&ConnectorError{
			Code: "TOOL_OUTBOX_COMPENSATED",
			Message: fmt.Sprintf("external side effect compensated via %s; original tool call not fulfilled", capability),
		})
		d.confirm(ctx, tc.ID, &ConfirmExecutionRequest{
			Status:   ToolCallStatusFailed,
			ErrorJSON: string(errJSON),
		})
		return compensationDone
	case result.Status == ConnectorStatusIndeterminate:
		// 补偿结果未知:退避后重试补偿(补偿幂等由 Connector 保证)。
		return d.compensateRetryOrFail(ctx, e, tc, "compensation indeterminate: "+errText(result.Error))
	default:
		// 业务拒绝(如单据已审批不可撤):退避重试(外部可能被人工撤回)。
		return d.compensateRetryOrFail(ctx, e, tc, fmt.Sprintf("compensation rejected: %s", errText(result.Error)))
	}
}

func (d *OutboxDispatcher) compensateRetryOrFail(ctx context.Context, e *OutboxEntry, tc *ToolCall, lastErr string) compensationOutcome {
	attempts := e.Attempts + 1
	if attempts >= d.maxAttempts {
		_ = d.store.MarkFailedFinal(ctx, e.ID, fmt.Sprintf("COMPENSATION_EXHAUSTED: %s", lastErr))
		d.confirmToolCall(ctx, tc, ToolCallStatusFailed, &ConnectorError{
			Code: "TOOL_OUTBOX_COMPENSATION_EXHAUSTED",
			Message: fmt.Sprintf("compensation failed after %d attempts: %s", attempts, lastErr),
		})
		return compensationFailed
	}
	_ = d.store.MarkAttemptFailed(ctx, e.ID, attempts, time.Now().UTC().Add(d.Backoff(attempts)), "compensate: "+lastErr)
	return compensationRetry
}

// ── ToolCall 回写 ──

// confirmToolCall 简单状态回写(带可选结构化错误)。
func (d *OutboxDispatcher) confirmToolCall(ctx context.Context, tc *ToolCall, status ToolCallStatus, cerr *ConnectorError) {
	req := &ConfirmExecutionRequest{Status: status}
	if cerr != nil {
		if b, err := json.Marshal(cerr); err == nil {
			req.ErrorJSON = string(b)
		}
	}
	d.confirm(ctx, tc.ID, req)
}

// confirmWithExternal 成功路径回写:外部 ID + Verify 观测。
func (d *OutboxDispatcher) confirmWithExternal(ctx context.Context, tc *ToolCall, result *ConnectorResult, vr *ConnectorVerifyResult) {
	req := &ConfirmExecutionRequest{
		Status:            ToolCallStatusSucceeded,
		ExternalRequestID: result.ExternalRequestID,
		ExternalObjectID:  result.ExternalObjectID,
		VerificationJSON:  verifyJSON(vr),
	}
	if result.Output != nil {
		if b, err := json.Marshal(result.Output); err == nil {
			req.OutputSummaryJSON = string(b)
		}
	}
	d.confirm(ctx, tc.ID, req)
}

func (d *OutboxDispatcher) confirm(ctx context.Context, toolCallID string, req *ConfirmExecutionRequest) {
	if _, err := d.confirmer.ConfirmExecution(ctx, toolCallID, req); err != nil {
		// 状态机守卫拒绝(如已被 timeout scanner 推进)属预期,日志留痕即可。
		log.Printf("[outbox] confirm tool_call %s → %s failed: %v", toolCallID, req.Status, err)
	}
}

func verifyJSON(vr *ConnectorVerifyResult) string {
	if vr == nil {
		return ""
	}
	b, err := json.Marshal(map[string]any{"confirmed": vr.Confirmed, "detail": vr.Detail})
	if err != nil {
		return ""
	}
	return string(b)
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// errText ConnectorError 安全转文本(nil 安全)。
func errText(e *ConnectorError) string {
	if e == nil {
		return "<nil>"
	}
	return e.Error()
}
