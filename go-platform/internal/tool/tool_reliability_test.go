package tool

import (
	"context"
	"testing"
	"time"
)

// fakeTimedOutRepo 超时扫描器内存仓储。
type fakeTimedOutRepo struct {
	calls map[string]*ToolCall
}

func (f *fakeTimedOutRepo) ListTimedOutExecuting(ctx context.Context, now time.Time, limit int) ([]*ToolCall, error) {
	var result []*ToolCall
	for _, tc := range f.calls {
		if tc.Status == ToolCallStatusExecuting && tc.TimeoutAt != nil && tc.TimeoutAt.Before(now) {
			result = append(result, tc)
		}
	}
	return result, nil
}

func (f *fakeTimedOutRepo) UpdateStatusGuarded(ctx context.Context, id string, expectedFrom, status ToolCallStatus, fields map[string]any) error {
	tc, ok := f.calls[id]
	if !ok {
		return ErrToolCallNotFound
	}
	if tc.Status != expectedFrom {
		return &transitionError{from: string(tc.Status), to: string(status)}
	}
	tc.Status = status
	applyFakeFields(tc, fields)
	return nil
}

// TestTimeoutScannerConvertsToIndeterminate 超时扫描:executing 超时 →
// indeterminate(不是 failed),写入 TOOL_TIMEOUT 错误码。
func TestTimeoutScannerConvertsToIndeterminate(t *testing.T) {
	repo := &fakeTimedOutRepo{calls: map[string]*ToolCall{}}
	past := time.Now().UTC().Add(-1 * time.Minute)
	future := time.Now().UTC().Add(5 * time.Minute)
	repo.calls["tc-expired"] = &ToolCall{ID: "tc-expired", ToolID: "parse_csv", Status: ToolCallStatusExecuting, TimeoutAt: &past, TraceID: "trace-1"}
	repo.calls["tc-running"] = &ToolCall{ID: "tc-running", ToolID: "parse_csv", Status: ToolCallStatusExecuting, TimeoutAt: &future}
	repo.calls["tc-done"] = &ToolCall{ID: "tc-done", ToolID: "parse_csv", Status: ToolCallStatusSucceeded, TimeoutAt: &past}

	scanner := NewToolCallTimeoutScanner(repo, nil, time.Minute)
	n, err := scanner.ScanOnce(context.Background())
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	if n != 1 {
		t.Fatalf("converted = %d, want 1", n)
	}
	if repo.calls["tc-expired"].Status != ToolCallStatusIndeterminate {
		t.Fatalf("expired status = %s, want indeterminate", repo.calls["tc-expired"].Status)
	}
	if repo.calls["tc-expired"].ErrorJSON == nil {
		t.Fatal("error_json should record TOOL_TIMEOUT")
	}
	// 未超时与已终态的不动
	if repo.calls["tc-running"].Status != ToolCallStatusExecuting {
		t.Fatal("still-within-timeout call must not be touched")
	}
	if repo.calls["tc-done"].Status != ToolCallStatusSucceeded {
		t.Fatal("terminal call must not be touched")
	}
}

// TestExecuteSetsTimeoutAt Execute 进入 executing 时写入超时阈值。
func TestExecuteSetsTimeoutAt(t *testing.T) {
	svc, _, callRepo, _ := setupLifecycleService()
	svc.defaultTimeout = 30 * time.Second
	ctx := context.Background()

	result, err := svc.Execute(ctx, &ExecuteRequest{
		TenantID: "t1", RunID: "r1", AgentID: "a1", BusinessAppCode: "finance",
		ToolID: "parse_csv", ToolVersion: "1.0", IdempotencyKey: "rel-timeout",
		ArgumentsJSON: `{}`, TraceID: "trace-rel",
	}, nil, nil)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	tc, _ := callRepo.GetByID(ctx, result.ToolCallID)
	if tc.TimeoutAt == nil {
		t.Fatal("executing tool_call must have timeout_at set")
	}
	if tc.TraceID != "trace-rel" {
		t.Fatalf("trace_id = %q, want trace-rel", tc.TraceID)
	}

	// pending_approval 的高风险不应设 timeout_at(尚未开始执行)
	result2, _ := svc.Execute(ctx, &ExecuteRequest{
		TenantID: "t1", RunID: "r1", AgentID: "a1", BusinessAppCode: "finance",
		ToolID: "archive_report", ToolVersion: "1.0", IdempotencyKey: "rel-timeout-hi",
		ArgumentsJSON: `{}`,
	}, nil, nil)
	tc2, _ := callRepo.GetByID(ctx, result2.ToolCallID)
	if tc2.Status == ToolCallStatusPendingApproval && tc2.TimeoutAt != nil {
		t.Fatal("pending_approval call should not have timeout_at")
	}
}

// TestRetryDeadLetterQueue M2-C.10:重试超上限 → DLQ。
func TestRetryDeadLetterQueue(t *testing.T) {
	svc, _, callRepo, _ := setupLifecycleService()
	svc.maxRetry = 2
	ctx := context.Background()

	result, err := svc.Execute(ctx, &ExecuteRequest{
		TenantID: "t1", RunID: "r1", AgentID: "a1", BusinessAppCode: "finance",
		ToolID: "parse_csv", ToolVersion: "1.0", IdempotencyKey: "rel-dlq",
		ArgumentsJSON: `{}`,
	}, nil, nil)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	tcID := result.ToolCallID

	// 循环:indeterminate → verify(false) → reconcile(not_executed) → retry
	for i := 1; i <= 3; i++ {
		_, _ = svc.ConfirmExecution(ctx, tcID, &ConfirmExecutionRequest{Status: ToolCallStatusIndeterminate})
		_, _ = svc.VerifyResult(ctx, tcID, `{"executed":false}`)
		tc, err := svc.Reconcile(ctx, tcID, ReconcileNotExecuted, "")
		if err != nil {
			t.Fatalf("round %d reconcile failed: %v", i, err)
		}
		if tc.Status != ToolCallStatusFailed {
			t.Fatalf("round %d status = %s, want failed", i, tc.Status)
		}
		_, err = svc.Retry(ctx, tcID)
		tc, _ = callRepo.GetByID(ctx, tcID)
		if i <= 2 {
			// 前两轮重试放行
			if err != nil {
				t.Fatalf("round %d retry should pass, got: %v", i, err)
			}
			if tc.RetryCount != i {
				t.Fatalf("round %d retry_count = %d", i, tc.RetryCount)
			}
		} else {
			// 第三轮:retry_count=2 已达上限 → DLQ
			if err == nil {
				t.Fatal("round 3 retry should hit DLQ")
			}
			if !tc.IsDeadLetter {
				t.Fatal("tool_call should be marked dead letter")
			}
			if tc.ErrorJSON == nil {
				t.Fatal("DLQ error_json should be recorded")
			}
			// DLQ 后再重试直接拒绝
			if _, err := svc.Retry(ctx, tcID); err == nil {
				t.Fatal("retry on dead letter must be rejected")
			}
		}
	}

	// ListDeadLetters 返回该记录
	dlq, err := svc.ListDeadLetters(ctx)
	if err != nil || len(dlq) != 1 {
		t.Fatalf("ListDeadLetters = %v (err %v), want 1 entry", dlq, err)
	}
}

// fakeCircuitRepo 内存熔断仓储,模拟 DB 原子语义。
type fakeCircuitRepo struct {
	states map[string]*CircuitBreakerState
}

func (f *fakeCircuitRepo) get(toolID string) *CircuitBreakerState {
	if _, ok := f.states[toolID]; !ok {
		f.states[toolID] = &CircuitBreakerState{ToolID: toolID, State: "closed"}
	}
	return f.states[toolID]
}

// 模拟 RecordFailure SQL 语义。
func (f *fakeCircuitRepo) recordFailure(toolID string, threshold int) {
	st := f.get(toolID)
	now := time.Now().UTC()
	wasHalfOpen := st.State == "half_open"
	if st.State == "closed" {
		st.ConsecutiveFailures++
	} else {
		st.ConsecutiveFailures = 1
	}
	if wasHalfOpen || (st.State == "closed" && st.ConsecutiveFailures >= threshold) {
		st.State = "open"
		st.OpenedAt = &now
	}
	st.HalfOpenProbeAt = nil
	st.LastFailureAt = &now
}

// 模拟 TryEnterHalfOpen SQL 语义:仅 open 且冷却期满时成功。
func (f *fakeCircuitRepo) tryHalfOpen(toolID string, cooldown time.Duration) bool {
	st := f.get(toolID)
	if st.State != "open" || st.OpenedAt == nil {
		return false
	}
	if time.Since(*st.OpenedAt) < cooldown {
		return false
	}
	now := time.Now().UTC()
	st.State = "half_open"
	st.HalfOpenProbeAt = &now
	return true
}

// TestCircuitStateMachine 熔断状态机编排:closed→open→half_open→closed/re-open。
func TestCircuitStateMachine(t *testing.T) {
	repo := &fakeCircuitRepo{states: map[string]*CircuitBreakerState{}}
	// 用内存仓储模拟 DB 原子语义,验证判定编排
	allow := func(toolID string) (bool, string) {
		st := repo.get(toolID)
		switch st.State {
		case "closed":
			return true, st.State
		case "open":
			if repo.tryHalfOpen(toolID, 50*time.Millisecond) {
				return true, "half_open"
			}
			return false, "open"
		default:
			return false, st.State
		}
	}

	toolID := "archive_report"
	// closed 放行
	if ok, _ := allow(toolID); !ok {
		t.Fatal("closed must allow")
	}
	// 2 次失败:未达阈值 3,仍 closed
	repo.recordFailure(toolID, 3)
	repo.recordFailure(toolID, 3)
	if st := repo.get(toolID).State; st != "closed" {
		t.Fatalf("after 2 failures state = %s, want closed", st)
	}
	// 第 3 次失败 → open
	repo.recordFailure(toolID, 3)
	if st := repo.get(toolID).State; st != "open" {
		t.Fatalf("after 3 failures state = %s, want open", st)
	}
	// open 且冷却期内:拒绝
	if ok, state := allow(toolID); ok {
		t.Fatalf("open within cooldown must reject (state=%s)", state)
	}
	// 冷却期满:放行探测,进入 half_open
	time.Sleep(60 * time.Millisecond)
	if ok, state := allow(toolID); !ok || state != "half_open" {
		t.Fatalf("after cooldown expect half_open probe, got ok=%v state=%s", ok, state)
	}
	// half_open 探测在途:其他调用拒绝
	if ok, _ := allow(toolID); ok {
		t.Fatal("half_open probe in flight must reject concurrent calls")
	}
	// 探测成功 → closed(RecordSuccess 语义)
	st := repo.get(toolID)
	now := time.Now().UTC()
	st.State, st.ConsecutiveFailures, st.OpenedAt, st.HalfOpenProbeAt = "closed", 0, nil, nil
	st.LastSuccessAt = &now
	if ok, _ := allow(toolID); !ok {
		t.Fatal("closed after successful probe must allow")
	}
}

// TestCircuitOpenBlocksExecute 熔断 open 时 Execute 被拒(需要真 CircuitBreaker,
// 用 PG 集成测试覆盖;此处验证错误映射)。
func TestCheckCircuitStateError(t *testing.T) {
	err := CheckCircuitStateError("open")
	if err == nil || err.Error() != "tool circuit breaker is open (state=open)" {
		t.Fatalf("unexpected error: %v", err)
	}
}
