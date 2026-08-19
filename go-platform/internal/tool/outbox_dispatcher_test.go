package tool

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

// ── M3-C: Outbox Dispatcher 生命周期测试 ──
// 覆盖 Gate:投递成功 confirmed、退避重试恢复、重试耗尽终态、
// indeterminate → stale Verify 收敛、ToolCall 取消自动补偿、
// 补偿重试耗尽、业务拒绝。

// fakeOutboxStore 内存 outboxStore。
type fakeOutboxStore struct {
	mu      sync.Mutex
	entries map[string]*OutboxEntry
}

func newFakeOutboxStore() *fakeOutboxStore {
	return &fakeOutboxStore{entries: make(map[string]*OutboxEntry)}
}

func (f *fakeOutboxStore) put(e *OutboxEntry) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *e
	f.entries[e.ID] = &cp
}

func (f *fakeOutboxStore) get(id string) (*OutboxEntry, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.entries[id]
	if !ok {
		return nil, false
	}
	cp := *e
	return &cp, true
}

func (f *fakeOutboxStore) ListDispatchable(ctx context.Context, now time.Time, limit int) ([]*OutboxEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*OutboxEntry
	for _, e := range f.entries {
		if e.State == OutboxStatePending && !e.NextAttemptAt.After(now) {
			cp := *e
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (f *fakeOutboxStore) ListStaleSent(ctx context.Context, staleBefore time.Time, limit int) ([]*OutboxEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*OutboxEntry
	for _, e := range f.entries {
		if e.State == OutboxStateSent && e.UpdatedAt.Before(staleBefore) {
			cp := *e
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (f *fakeOutboxStore) ListCompensatable(ctx context.Context, now time.Time, limit int) ([]*OutboxEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*OutboxEntry
	for _, e := range f.entries {
		if e.State == OutboxStateCompensatePending && !e.NextAttemptAt.After(now) {
			cp := *e
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (f *fakeOutboxStore) MarkSent(ctx context.Context, id, extReqID, extObjID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.entries[id]
	if !ok || e.State != OutboxStatePending {
		return ErrOutboxStateConflict
	}
	e.State = OutboxStateSent
	e.ExternalRequestID = strPtr(extReqID)
	e.ExternalObjectID = strPtr(extObjID)
	e.UpdatedAt = time.Now().UTC()
	return nil
}

func (f *fakeOutboxStore) MarkConfirmed(ctx context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.entries[id]
	if !ok || e.State != OutboxStateSent {
		return ErrOutboxStateConflict
	}
	e.State = OutboxStateConfirmed
	e.UpdatedAt = time.Now().UTC()
	return nil
}

func (f *fakeOutboxStore) MarkAttemptFailed(ctx context.Context, id string, attempts int, next time.Time, lastErr string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.entries[id]
	if !ok {
		return ErrOutboxNotFound
	}
	e.Attempts = attempts
	e.NextAttemptAt = next
	s := lastErr
	e.LastError = &s
	e.UpdatedAt = time.Now().UTC()
	return nil
}

func (f *fakeOutboxStore) MarkCompensatePending(ctx context.Context, id, tenantID, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.entries[id]
	if !ok {
		return ErrOutboxNotFound
	}
	switch e.State {
	case OutboxStatePending, OutboxStateSent, OutboxStateCompensatePending:
		e.State = OutboxStateCompensatePending
		s := reason
		e.LastError = &s
		e.NextAttemptAt = time.Now().UTC()
		e.UpdatedAt = time.Now().UTC()
		return nil
	}
	return ErrOutboxStateConflict
}

func (f *fakeOutboxStore) MarkCompensated(ctx context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.entries[id]
	if !ok || e.State != OutboxStateCompensatePending {
		return ErrOutboxStateConflict
	}
	e.State = OutboxStateCompensated
	e.UpdatedAt = time.Now().UTC()
	return nil
}

func (f *fakeOutboxStore) MarkFailedFinal(ctx context.Context, id, code string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.entries[id]
	if !ok {
		return ErrOutboxNotFound
	}
	e.State = OutboxStateFailed
	s := code
	e.LastError = &s
	e.UpdatedAt = time.Now().UTC()
	return nil
}

func (f *fakeOutboxStore) Touch(ctx context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.entries[id]
	if !ok {
		return ErrOutboxNotFound
	}
	e.UpdatedAt = time.Now().UTC()
	return nil
}

// fakeOutboxExecutor 记录调用并返回预置行为。
type fakeOutboxExecutor struct {
	mu sync.Mutex
	rt *ConnectorRuntime

	// 行为队列:每次 Execute 弹出一个;空则默认成功。
	execScript []execStep
	execCalls  []RuntimeExecuteRequest
	// Verify 行为。
	verifyScript []verifyStep
	verifyCalls  int
}

type execStep struct {
	result *ConnectorResult
	err    error
}

type verifyStep struct {
	result *ConnectorVerifyResult
	err    error
}

func (f *fakeOutboxExecutor) Execute(ctx context.Context, req *RuntimeExecuteRequest) (*ConnectorResult, error) {
	f.mu.Lock()
	f.execCalls = append(f.execCalls, *req)
	step := execStep{result: &ConnectorResult{Status: ConnectorStatusSucceeded, Output: map[string]any{"ok": true}, ExternalRequestID: "ext-req-1", ExternalObjectID: "PR-1"}}
	if len(f.execScript) > 0 {
		step = f.execScript[0]
		f.execScript = f.execScript[1:]
	}
	f.mu.Unlock()
	return step.result, step.err
}

func (f *fakeOutboxExecutor) Verify(ctx context.Context, connectorCode string, req *ConnectorVerifyRequest) (*ConnectorVerifyResult, error) {
	f.mu.Lock()
	f.verifyCalls++
	step := verifyStep{result: &ConnectorVerifyResult{Confirmed: true, Detail: "fake", Observed: map[string]any{"status": "pending"}}}
	if len(f.verifyScript) > 0 {
		step = f.verifyScript[0]
		f.verifyScript = f.verifyScript[1:]
	}
	f.mu.Unlock()
	return step.result, step.err
}

func (f *fakeOutboxExecutor) OutboxConnectorFor(connectorCode string) (OutboxConnector, bool) {
	// 返回真实 MockERPConnector(补偿构造)。
	if connectorCode == "erp_connector" {
		return NewMockERPConnector(), true
	}
	return nil, false
}

// fakeOutboxToolCalls 内存 ToolCall 存储。
type fakeOutboxToolCalls struct {
	mu  sync.Mutex
	tcs map[string]*ToolCall
}

func newFakeOutboxToolCalls() *fakeOutboxToolCalls {
	return &fakeOutboxToolCalls{tcs: make(map[string]*ToolCall)}
}

func (f *fakeOutboxToolCalls) GetByID(ctx context.Context, id string) (*ToolCall, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	tc, ok := f.tcs[id]
	if !ok {
		return nil, ErrToolCallNotFound
	}
	return tc, nil
}

// fakeOutboxConfirmer 记录 ConfirmExecution 请求。
type fakeOutboxConfirmer struct {
	mu    sync.Mutex
	calls []confirmRecord
}

type confirmRecord struct {
	toolCallID string
	req        ConfirmExecutionRequest
}

func (f *fakeOutboxConfirmer) ConfirmExecution(ctx context.Context, toolCallID string, req *ConfirmExecutionRequest) (*ToolCall, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, confirmRecord{toolCallID, *req})
	return &ToolCall{ID: toolCallID, Status: req.Status}, nil
}

// ── 测试辅助 ──

func outboxTestHarness(t *testing.T) (*OutboxDispatcher, *fakeOutboxStore, *fakeOutboxExecutor, *fakeOutboxToolCalls, *fakeOutboxConfirmer) {
	t.Helper()
	store := newFakeOutboxStore()
	exec := &fakeOutboxExecutor{}
	tcs := newFakeOutboxToolCalls()
	confirm := &fakeOutboxConfirmer{}
	d := NewOutboxDispatcher(store, exec, tcs, confirm, time.Hour, 10, 5, time.Second, time.Hour)
	return d, store, exec, tcs, confirm
}

func outboxFixture(toolCallID string) *OutboxEntry {
	return &OutboxEntry{
		ID: "ob-" + toolCallID, ToolCallID: toolCallID, TenantID: "t-1",
		ConnectorCode: "erp_connector", Operation: "erp_purchase_request",
		PayloadJSON:   `{"idempotency_key":"k-1","title":"采购"}`,
		State:         OutboxStatePending,
		NextAttemptAt: time.Now().UTC().Add(-time.Minute),
	}
}

func executingToolCall(id string) *ToolCall {
	binding := "binding-1"
	return &ToolCall{
		ID: id, TenantID: "t-1", RunID: "run-1", ToolID: "erp_purchase_request",
		ToolVersion: "1.0.0", ConnectorBindingID: &binding,
		Status: ToolCallStatusExecuting, TraceID: "trace-1",
	}
}

// ── 用例 ──

func TestOutboxDispatch_SuccessConfirmed(t *testing.T) {
	d, store, exec, tcs, confirm := outboxTestHarness(t)
	tc := executingToolCall("tc-ok")
	tcs.tcs[tc.ID] = tc
	store.put(outboxFixture(tc.ID))

	d.RunOnce(context.Background())

	e, _ := store.get("ob-tc-ok")
	if e.State != OutboxStateConfirmed {
		t.Fatalf("expected confirmed, got %s", e.State)
	}
	if len(exec.execCalls) != 1 {
		t.Fatalf("expected 1 execute call, got %d", len(exec.execCalls))
	}
	// ToolCall 回写 succeeded + 外部 ID。
	if len(confirm.calls) != 1 || confirm.calls[0].req.Status != ToolCallStatusSucceeded {
		t.Fatalf("unexpected confirm: %+v", confirm.calls)
	}
	if confirm.calls[0].req.ExternalObjectID != "PR-1" {
		t.Fatalf("external ids not propagated: %+v", confirm.calls[0].req)
	}
}

func TestOutboxDispatch_RetryBackoffThenRecover(t *testing.T) {
	d, store, exec, tcs, _ := outboxTestHarness(t)
	tc := executingToolCall("tc-retry")
	tcs.tcs[tc.ID] = tc
	store.put(outboxFixture(tc.ID))

	// 前两次传输失败,第三次成功(限流恢复)。
	exec.execScript = []execStep{{err: errors.New("conn refused")}, {err: errors.New("timeout")}}

	// 第 1 轮:失败 → attempts=1,退避未到(NextAttemptAt 在未来)。
	d.dispatchPending(context.Background())
	e, _ := store.get("ob-tc-retry")
	if e.State != OutboxStatePending || e.Attempts != 1 || !e.NextAttemptAt.After(time.Now()) {
		t.Fatalf("round 1 state wrong: %+v", e)
	}

	// 模拟退避到期:手动重置 NextAttemptAt。
	e.NextAttemptAt = time.Now().UTC().Add(-time.Second)
	store.put(e)
	d.dispatchPending(context.Background())
	e, _ = store.get("ob-tc-retry")
	if e.Attempts != 2 {
		t.Fatalf("round 2 attempts wrong: %+v", e)
	}

	e.NextAttemptAt = time.Now().UTC().Add(-time.Second)
	store.put(e)
	d.RunOnce(context.Background())
	e, _ = store.get("ob-tc-retry")
	if e.State != OutboxStateConfirmed {
		t.Fatalf("round 3 should confirm, got %s", e.State)
	}
	if len(exec.execCalls) != 3 {
		t.Fatalf("expected 3 attempts, got %d", len(exec.execCalls))
	}
}

func TestOutboxDispatch_ExhaustedNoSideEffect(t *testing.T) {
	d, store, exec, tcs, confirm := outboxTestHarness(t)
	tc := executingToolCall("tc-exh")
	tcs.tcs[tc.ID] = tc
	store.put(outboxFixture(tc.ID))

	// 持续传输失败(maxAttempts=5)。
	exec.execScript = []execStep{
		{err: errors.New("f1")}, {err: errors.New("f2")}, {err: errors.New("f3")},
		{err: errors.New("f4")}, {err: errors.New("f5")},
	}
	for i := 0; i < 5; i++ {
		e, _ := store.get("ob-tc-exh")
		e.NextAttemptAt = time.Now().UTC().Add(-time.Second)
		store.put(e)
		d.dispatchPending(context.Background())
	}

	e, _ := store.get("ob-tc-exh")
	if e.State != OutboxStateFailed {
		t.Fatalf("expected failed, got %s", e.State)
	}
	// 无副作用 → ToolCall failed(而非补偿)。
	last := confirm.calls[len(confirm.calls)-1]
	if last.req.Status != ToolCallStatusFailed {
		t.Fatalf("expected tc failed, got %s", last.req.Status)
	}
}

func TestOutboxIndeterminate_StaleVerifyConverges(t *testing.T) {
	d, store, exec, tcs, confirm := outboxTestHarness(t)
	tc := executingToolCall("tc-indet")
	tcs.tcs[tc.ID] = tc
	store.put(outboxFixture(tc.ID))

	// 单据已创建但结果未知(indeterminate)。
	exec.execScript = []execStep{{
		result: &ConnectorResult{
			Status: ConnectorStatusIndeterminate,
			ExternalRequestID: "ext-k1", ExternalObjectID: "PR-9",
			Error: &ConnectorError{Code: "ERP_TIMEOUT", Message: "lost", Retryable: true},
		},
	}}
	d.dispatchPending(context.Background())

	e, _ := store.get("ob-tc-indet")
	if e.State != OutboxStateSent {
		t.Fatalf("indeterminate should go sent (no blind re-dispatch), got %s", e.State)
	}
	// ToolCall → indeterminate。
	if confirm.calls[len(confirm.calls)-1].req.Status != ToolCallStatusIndeterminate {
		t.Fatalf("expected tc indeterminate")
	}

	// 立即 Verify 失败(未决)→ 保持 sent;窗口过后 stale Verify 先失败再确认 → confirmed。
	exec.verifyScript = []verifyStep{
		{err: errors.New("verify net err")}, // 第一轮 stale:Verify 网络错误 → Touch
		{result: &ConnectorVerifyResult{Confirmed: true, Detail: "recovered"}}, // 第二轮:确认
	}
	// 置为 stale(UpdatedAt 拨回过去),第一轮 Touch 后再次拨回。
	e.UpdatedAt = time.Now().UTC().Add(-2 * time.Hour)
	store.put(e)
	d.RunOnce(context.Background())
	e, _ = store.get("ob-tc-indet")
	if e.State != OutboxStateSent {
		t.Fatalf("verify error should keep sent, got %s", e.State)
	}

	// 第三轮:再次 stale → Verify 确认 → confirmed + ToolCall succeeded。
	e.UpdatedAt = time.Now().UTC().Add(-2 * time.Hour)
	store.put(e)
	d.RunOnce(context.Background())
	e, _ = store.get("ob-tc-indet")
	if e.State != OutboxStateConfirmed {
		t.Fatalf("stale verify should confirm, got %s", e.State)
	}
	if confirm.calls[len(confirm.calls)-1].req.Status != ToolCallStatusSucceeded {
		t.Fatalf("final confirm should be succeeded")
	}
}

func TestOutboxStaleVerify_NotExecutedFails(t *testing.T) {
	d, store, exec, tcs, confirm := outboxTestHarness(t)
	tc := executingToolCall("tc-notexec")
	tcs.tcs[tc.ID] = tc

	e := outboxFixture(tc.ID)
	e.State = OutboxStateSent
	e.UpdatedAt = time.Now().UTC().Add(-2 * time.Hour)
	store.put(e)

	// Verify 明确:副作用未发生(单据不存在)。
	exec.verifyScript = []verifyStep{{
		result: &ConnectorVerifyResult{Confirmed: false, Detail: "purchase request not found"},
	}}
	d.RunOnce(context.Background())

	got, _ := store.get("ob-tc-notexec")
	if got.State != OutboxStateFailed {
		t.Fatalf("not-executed should fail final, got %s", got.State)
	}
	if confirm.calls[len(confirm.calls)-1].req.Status != ToolCallStatusFailed {
		t.Fatalf("expected tc failed")
	}
}

func TestOutboxCancelledToolCall_AutoCompensates(t *testing.T) {
	d, store, exec, tcs, confirm := outboxTestHarness(t)
	tc := executingToolCall("tc-cancel")
	tc.Status = ToolCallStatusCancelled // 审批撤回/人工取消
	tcs.tcs[tc.ID] = tc

	// 已发出的单据(sent + external ids),超过确认窗口未收敛。
	e := outboxFixture(tc.ID)
	e.State = OutboxStateSent
	extReq, extObj := "ext-k9", "PR-77"
	e.ExternalRequestID, e.ExternalObjectID = &extReq, &extObj
	e.UpdatedAt = time.Now().UTC().Add(-2 * time.Hour)
	store.put(e)

	d.RunOnce(context.Background())

	got, _ := store.get("ob-tc-cancel")
	if got.State != OutboxStateCompensated {
		t.Fatalf("cancelled tc should compensate, got %s", got.State)
	}
	// 补偿调用 = erp_purchase_cancel + 对象 ID。
	if len(exec.execCalls) != 1 || exec.execCalls[0].Capability != "erp_purchase_cancel" {
		t.Fatalf("expected cancel call, got %+v", exec.execCalls)
	}
	if exec.execCalls[0].Input["purchase_request_id"] != "PR-77" {
		t.Fatalf("compensation must target external object")
	}
	// ToolCall 回写 failed + COMPENSATED 语义。
	last := confirm.calls[len(confirm.calls)-1]
	if last.req.Status != ToolCallStatusFailed || last.req.ErrorJSON == "" {
		t.Fatalf("expected failed with compensation error, got %+v", last.req)
	}
}

func TestOutboxCancelledToolCall_NotSentTerminates(t *testing.T) {
	d, store, _, tcs, _ := outboxTestHarness(t)
	tc := executingToolCall("tc-cancel2")
	tc.Status = ToolCallStatusCancelled
	tcs.tcs[tc.ID] = tc
	store.put(outboxFixture(tc.ID)) // pending,未发出

	d.RunOnce(context.Background())

	got, _ := store.get("ob-tc-cancel2")
	if got.State != OutboxStateFailed {
		t.Fatalf("not-sent cancelled should fail final, got %s", got.State)
	}
}

func TestOutboxCompensation_RetryExhausted(t *testing.T) {
	d, store, exec, tcs, confirm := outboxTestHarness(t)
	tc := executingToolCall("tc-compex")
	tcs.tcs[tc.ID] = tc

	// 人工触发补偿 + 已发出。
	e := outboxFixture(tc.ID)
	e.State = OutboxStateCompensatePending
	extReq, extObj := "ext-k", "PR-88"
	e.ExternalRequestID, e.ExternalObjectID = &extReq, &extObj
	store.put(e)

	// 补偿持续失败(单据已审批不可撤 → 业务拒绝)。
	rejected := &ConnectorResult{
		Status: ConnectorStatusFailed,
		Error:  &ConnectorError{Code: "CANCEL_NOT_ALLOWED", Message: "approved"},
	}
	exec.execScript = []execStep{{result: rejected}, {result: rejected}, {result: rejected}, {result: rejected}, {result: rejected}}

	for i := 0; i < 5; i++ {
		got, _ := store.get("ob-tc-compex")
		got.NextAttemptAt = time.Now().UTC().Add(-time.Second)
		store.put(got)
		d.RunOnce(context.Background())
	}

	got, _ := store.get("ob-tc-compex")
	if got.State != OutboxStateFailed {
		t.Fatalf("exhausted compensation should fail, got %s", got.State)
	}
	last := confirm.calls[len(confirm.calls)-1]
	if last.req.Status != ToolCallStatusFailed {
		t.Fatalf("expected tc failed")
	}
	var cerr ConnectorError
	if err := json.Unmarshal([]byte(last.req.ErrorJSON), &cerr); err != nil {
		t.Fatalf("error_json unparseable: %v", err)
	}
	if cerr.Code != "TOOL_OUTBOX_COMPENSATION_EXHAUSTED" {
		t.Fatalf("expected COMPENSATION_EXHAUSTED, got %s", cerr.Code)
	}
}

func TestOutboxBusinessRejected_FailsImmediately(t *testing.T) {
	d, store, exec, tcs, confirm := outboxTestHarness(t)
	tc := executingToolCall("tc-biz")
	tcs.tcs[tc.ID] = tc
	store.put(outboxFixture(tc.ID))

	// 业务拒绝(不可重试)。
	exec.execScript = []execStep{{
		result: &ConnectorResult{
			Status: ConnectorStatusFailed,
			Error:  &ConnectorError{Code: "BUDGET_EXCEEDED", Message: "预算不足"},
		},
	}}
	d.RunOnce(context.Background())

	e, _ := store.get("ob-tc-biz")
	if e.State != OutboxStateFailed {
		t.Fatalf("business rejection should fail final, got %s", e.State)
	}
	if len(exec.execCalls) != 1 {
		t.Fatalf("business rejection must not retry")
	}
	if confirm.calls[len(confirm.calls)-1].req.Status != ToolCallStatusFailed {
		t.Fatalf("expected tc failed")
	}
}

func TestOutboxBackoff_Exponential(t *testing.T) {
	d, _, _, _, _ := outboxTestHarness(t)
	base := time.Second
	for i, want := range []time.Duration{base * 2, base * 4, base * 8, base * 16} {
		if got := d.Backoff(i + 1); got != want {
			t.Fatalf("backoff(%d)=%s want %s", i+1, got, want)
		}
	}
	// 封顶。
	if got := d.Backoff(20); got != base<<6 {
		t.Fatalf("backoff should cap at 64x, got %s", got)
	}
}

func TestOutboxPayloadUnparseable_FailsFinal(t *testing.T) {
	d, store, _, tcs, _ := outboxTestHarness(t)
	tc := executingToolCall("tc-badpayload")
	tcs.tcs[tc.ID] = tc
	e := outboxFixture(tc.ID)
	e.PayloadJSON = "not-json{"
	store.put(e)

	d.RunOnce(context.Background())

	got, _ := store.get("ob-tc-badpayload")
	if got.State != OutboxStateFailed {
		t.Fatalf("unparseable payload should fail final, got %s", got.State)
	}
}

// 全链路关联:outbox 携带 tool_call_id,Dispatcher 投递请求透传 trace_id。
func TestOutboxFullChainCorrelation(t *testing.T) {
	d, store, exec, tcs, _ := outboxTestHarness(t)
	tc := executingToolCall("tc-chain")
	tc.TraceID = "trace-m3c-42"
	tcs.tcs[tc.ID] = tc
	store.put(outboxFixture(tc.ID))

	d.RunOnce(context.Background())

	if len(exec.execCalls) != 1 {
		t.Fatalf("expected dispatch")
	}
	call := exec.execCalls[0]
	if call.TraceID != "trace-m3c-42" || call.ToolCallID != "tc-chain" || call.TenantID != "t-1" {
		t.Fatalf("full-chain correlation fields lost: %+v", call)
	}
}
