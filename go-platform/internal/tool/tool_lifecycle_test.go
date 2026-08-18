package tool

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeApprovalRepo 审批任务内存仓储,模拟 approval_tasks 表行为。
type fakeApprovalRepo struct {
	approvals map[string]*ToolCallApproval // key: tool_call_id
	seq       int
}

func newFakeApprovalRepo() *fakeApprovalRepo {
	return &fakeApprovalRepo{approvals: make(map[string]*ToolCallApproval)}
}

func (f *fakeApprovalRepo) CreateToolCallApproval(ctx context.Context, approval *ToolCallApproval) error {
	if existing, ok := f.approvals[approval.ToolCallID]; ok {
		*approval = *existing
		return nil
	}
	f.seq++
	approval.ID = "appr-" + approval.ToolCallID
	approval.Status = "pending"
	now := time.Now().UTC()
	approval.CreatedAt = now
	approval.UpdatedAt = now
	f.approvals[approval.ToolCallID] = approval
	return nil
}

func (f *fakeApprovalRepo) FindToolCallApproval(ctx context.Context, toolCallID string) (*ToolCallApproval, error) {
	a, ok := f.approvals[toolCallID]
	if !ok {
		return nil, ErrApprovalNotFound
	}
	return a, nil
}

func (f *fakeApprovalRepo) UpdateDecision(ctx context.Context, id, decision, decisionBy string) error {
	for _, a := range f.approvals {
		if a.ID == id {
			if a.Status != "pending" && a.Status != decision {
				return errors.New("approval task already decided: " + a.Status)
			}
			a.Status = decision
			now := time.Now().UTC()
			a.DecidedAt = &now
			return nil
		}
	}
	return ErrApprovalNotFound
}

// setupLifecycleService 带审批仓储的生命周期测试服务。
func setupLifecycleService() (*Service, *fakeToolRepo, *fakeToolCallRepo, *fakeApprovalRepo) {
	toolRepo, callRepo := &fakeToolRepo{
		tools: map[string]*Tool{
			"parse_csv":      {ToolID: "parse_csv", Version: "1.0", Domain: "shared", RiskLevel: "low", Status: "active", IsShared: true},
			"archive_report": {ToolID: "archive_report", Version: "1.0", Domain: "finance", RiskLevel: "high", Status: "active", IsShared: false},
			"validate_metrics": {ToolID: "validate_metrics", Version: "2.0", Domain: "finance", RiskLevel: "medium", Status: "active", IsShared: false},
		},
	}, newFakeToolCallRepo()
	approvalRepo := newFakeApprovalRepo()
	svc := NewService(toolRepo, callRepo, noopAuditLogger{},
		WithApprovalRepository(approvalRepo),
		WithDomainPolicy(&fakeDomainPolicyProvider{allowed: map[string][]string{"finance": {"finance", "shared"}}}),
	)
	return svc, toolRepo, callRepo, approvalRepo
}

// ── 状态机 ──

func TestCanTransition(t *testing.T) {
	legal := []struct{ from, to ToolCallStatus }{
		{ToolCallStatusRequested, ToolCallStatusExecuting},
		{ToolCallStatusPendingApproval, ToolCallStatusExecuting},
		{ToolCallStatusPendingApproval, ToolCallStatusCancelled},
		{ToolCallStatusExecuting, ToolCallStatusSucceeded},
		{ToolCallStatusExecuting, ToolCallStatusFailed},
		{ToolCallStatusExecuting, ToolCallStatusIndeterminate},
		{ToolCallStatusIndeterminate, ToolCallStatusSucceeded},
		{ToolCallStatusIndeterminate, ToolCallStatusFailed},
		{ToolCallStatusFailed, ToolCallStatusExecuting}, // Retry Gate
	}
	for _, tr := range legal {
		if !CanTransition(tr.from, tr.to) {
			t.Errorf("expected legal: %s → %s", tr.from, tr.to)
		}
	}

	illegal := []struct{ from, to ToolCallStatus }{
		{ToolCallStatusSucceeded, ToolCallStatusExecuting}, // 终态不可迁移
		{ToolCallStatusCancelled, ToolCallStatusExecuting},
		{ToolCallStatusSucceeded, ToolCallStatusFailed},
		{ToolCallStatusFailed, ToolCallStatusSucceeded},    // 必须先 Retry→executing
		{ToolCallStatusPendingApproval, ToolCallStatusSucceeded}, // 必须经过 executing
		{ToolCallStatusRequested, ToolCallStatusSucceeded},
	}
	for _, tr := range illegal {
		if CanTransition(tr.from, tr.to) {
			t.Errorf("expected illegal: %s → %s", tr.from, tr.to)
		}
	}

	if !IsTerminal(ToolCallStatusSucceeded) || !IsTerminal(ToolCallStatusFailed) || !IsTerminal(ToolCallStatusCancelled) {
		t.Error("succeeded/failed/cancelled must be terminal")
	}
	if IsTerminal(ToolCallStatusIndeterminate) || IsTerminal(ToolCallStatusExecuting) {
		t.Error("indeterminate/executing must not be terminal")
	}
}

// ── Approval Binding ──

func TestHighRiskAutoCreatesApproval(t *testing.T) {
	svc, _, _, approvalRepo := setupLifecycleService()
	ctx := context.Background()

	result, err := svc.Execute(ctx, &ExecuteRequest{
		TenantID: "t1", RunID: "r1", AgentID: "a1", BusinessAppCode: "finance",
		ToolID: "archive_report", ToolVersion: "1.0",
		IdempotencyKey: "lc-auto-appr", ArgumentsJSON: `{"report":"q2"}`,
	}, nil, nil)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	// 高风险 Execute 应自动创建审批任务并回写 approval_task_id
	approval, err := approvalRepo.FindToolCallApproval(ctx, result.ToolCallID)
	if err != nil {
		t.Fatalf("approval should be auto-created: %v", err)
	}
	if approval.Status != "pending" {
		t.Fatalf("approval status = %s, want pending", approval.Status)
	}
	// PayloadHash 绑定 input_hash
	tc, _ := svc.GetToolCall(ctx, result.ToolCallID)
	if approval.PayloadHash != tc.InputHash {
		t.Fatalf("payload_hash %s != input_hash %s", approval.PayloadHash, tc.InputHash)
	}
	if tc.ApprovalTaskID == nil || *tc.ApprovalTaskID != approval.ID {
		t.Fatalf("tool_call.approval_task_id should be bound to %s", approval.ID)
	}
}

func TestApprovalBindingApprovedFlow(t *testing.T) {
	svc, _, _, approvalRepo := setupLifecycleService()
	ctx := context.Background()

	result, _ := svc.Execute(ctx, &ExecuteRequest{
		TenantID: "t1", RunID: "r1", AgentID: "a1", BusinessAppCode: "finance",
		ToolID: "archive_report", ToolVersion: "1.0",
		IdempotencyKey: "lc-approve", ArgumentsJSON: `{"a":1}`,
	}, nil, nil)

	approval, _ := approvalRepo.FindToolCallApproval(ctx, result.ToolCallID)

	// 未决定时绑定应失败
	if err := svc.BindToolCallApprovalDecision(ctx, result.ToolCallID, "approved"); !errors.Is(err, ErrApprovalNotDecided) {
		t.Fatalf("undecided bind should fail with ErrApprovalNotDecided, got: %v", err)
	}

	// 决定后绑定:pending_approval → executing(重检通过)
	if err := approvalRepo.UpdateDecision(ctx, approval.ID, "approved", "u1"); err != nil {
		t.Fatalf("update decision failed: %v", err)
	}
	if err := svc.BindToolCallApprovalDecision(ctx, result.ToolCallID, "approved"); err != nil {
		t.Fatalf("bind approved failed: %v", err)
	}
	tc, _ := svc.GetToolCall(ctx, result.ToolCallID)
	if tc.Status != ToolCallStatusExecuting {
		t.Fatalf("status = %s, want executing", tc.Status)
	}

	// 幂等:重复绑定同一决定直接成功
	if err := svc.BindToolCallApprovalDecision(ctx, result.ToolCallID, "approved"); err != nil {
		t.Fatalf("idempotent re-bind failed: %v", err)
	}
}

func TestApprovalBindingRejectedFlow(t *testing.T) {
	svc, _, _, approvalRepo := setupLifecycleService()
	ctx := context.Background()

	result, _ := svc.Execute(ctx, &ExecuteRequest{
		TenantID: "t1", RunID: "r1", AgentID: "a1", BusinessAppCode: "finance",
		ToolID: "archive_report", ToolVersion: "1.0",
		IdempotencyKey: "lc-reject", ArgumentsJSON: `{"a":1}`,
	}, nil, nil)

	approval, _ := approvalRepo.FindToolCallApproval(ctx, result.ToolCallID)
	if err := approvalRepo.UpdateDecision(ctx, approval.ID, "rejected", "u1"); err != nil {
		t.Fatalf("update decision failed: %v", err)
	}
	if err := svc.BindToolCallApprovalDecision(ctx, result.ToolCallID, "rejected"); err != nil {
		t.Fatalf("bind rejected failed: %v", err)
	}
	tc, _ := svc.GetToolCall(ctx, result.ToolCallID)
	if tc.Status != ToolCallStatusCancelled {
		t.Fatalf("status = %s, want cancelled", tc.Status)
	}
}

func TestApprovalBindingDecisionMismatch(t *testing.T) {
	svc, _, _, approvalRepo := setupLifecycleService()
	ctx := context.Background()

	result, _ := svc.Execute(ctx, &ExecuteRequest{
		TenantID: "t1", RunID: "r1", AgentID: "a1", BusinessAppCode: "finance",
		ToolID: "archive_report", ToolVersion: "1.0",
		IdempotencyKey: "lc-mismatch", ArgumentsJSON: `{"a":1}`,
	}, nil, nil)

	// 审批任务被 rejected,但请求绑定 approved
	approval, _ := approvalRepo.FindToolCallApproval(ctx, result.ToolCallID)
	_ = approvalRepo.UpdateDecision(ctx, approval.ID, "rejected", "u1")
	if err := svc.BindToolCallApprovalDecision(ctx, result.ToolCallID, "approved"); err == nil {
		t.Fatal("decision mismatch should fail")
	}
}

func TestApprovalBindingPrecheckFailure(t *testing.T) {
	svc, toolRepo, _, approvalRepo := setupLifecycleService()
	ctx := context.Background()

	result, _ := svc.Execute(ctx, &ExecuteRequest{
		TenantID: "t1", RunID: "r1", AgentID: "a1", BusinessAppCode: "finance",
		ToolID: "archive_report", ToolVersion: "1.0",
		IdempotencyKey: "lc-precheck", ArgumentsJSON: `{"a":1}`,
	}, nil, nil)

	// 审批期间工具被停用
	toolRepo.tools["archive_report"].Status = "deprecated"

	approval, _ := approvalRepo.FindToolCallApproval(ctx, result.ToolCallID)
	_ = approvalRepo.UpdateDecision(ctx, approval.ID, "approved", "u1")

	err := svc.BindToolCallApprovalDecision(ctx, result.ToolCallID, "approved")
	if err == nil {
		t.Fatal("precheck failure should reject binding")
	}
	tc, _ := svc.GetToolCall(ctx, result.ToolCallID)
	if tc.Status != ToolCallStatusFailed {
		t.Fatalf("status = %s, want failed after precheck failure", tc.Status)
	}
	if tc.ErrorJSON == nil {
		t.Fatal("error_json should record TOOL_PRECHECK_FAILED")
	}
}

// ── Confirm / Verify / Reconcile ──

func TestConfirmExecutionHappyPath(t *testing.T) {
	svc, _, _, _ := setupLifecycleService()
	ctx := context.Background()

	result, _ := svc.Execute(ctx, &ExecuteRequest{
		TenantID: "t1", RunID: "r1", AgentID: "a1", BusinessAppCode: "finance",
		ToolID: "validate_metrics", ToolVersion: "2.0",
		IdempotencyKey: "lc-confirm", ArgumentsJSON: `{"m":1}`,
	}, nil, nil)
	if result.Status != ToolCallStatusExecuting {
		t.Fatalf("medium risk should start executing, got %s", result.Status)
	}

	tc, err := svc.ConfirmExecution(ctx, result.ToolCallID, &ConfirmExecutionRequest{
		Status:            ToolCallStatusSucceeded,
		OutputSummaryJSON: `{"rows":10}`,
		ExternalRequestID: "req-100",
	})
	if err != nil {
		t.Fatalf("confirm failed: %v", err)
	}
	if tc.Status != ToolCallStatusSucceeded {
		t.Fatalf("status = %s, want succeeded", tc.Status)
	}
	if tc.OutputSummaryJSON == nil || tc.ExternalRequestID == nil {
		t.Fatal("output/external id should be persisted")
	}

	// 终态后再次 confirm:非法转换
	if _, err := svc.ConfirmExecution(ctx, result.ToolCallID, &ConfirmExecutionRequest{Status: ToolCallStatusFailed}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("terminal re-confirm should fail with ErrInvalidTransition, got: %v", err)
	}
}

func TestConfirmExecutionTimeoutToIndeterminate(t *testing.T) {
	svc, _, _, _ := setupLifecycleService()
	ctx := context.Background()

	result, _ := svc.Execute(ctx, &ExecuteRequest{
		TenantID: "t1", RunID: "r1", AgentID: "a1", BusinessAppCode: "finance",
		ToolID: "validate_metrics", ToolVersion: "2.0",
		IdempotencyKey: "lc-timeout", ArgumentsJSON: `{}`,
	}, nil, nil)

	// 超时上报 indeterminate(不是 failed)
	tc, err := svc.ConfirmExecution(ctx, result.ToolCallID, &ConfirmExecutionRequest{
		Status:    ToolCallStatusIndeterminate,
		ErrorJSON: `{"code":"TIMEOUT"}`,
	})
	if err != nil {
		t.Fatalf("confirm indeterminate failed: %v", err)
	}
	if tc.Status != ToolCallStatusIndeterminate {
		t.Fatalf("status = %s, want indeterminate", tc.Status)
	}
}

func TestReconcileFlow(t *testing.T) {
	svc, _, _, _ := setupLifecycleService()
	ctx := context.Background()

	result, _ := svc.Execute(ctx, &ExecuteRequest{
		TenantID: "t1", RunID: "r1", AgentID: "a1", BusinessAppCode: "finance",
		ToolID: "validate_metrics", ToolVersion: "2.0",
		IdempotencyKey: "lc-reconcile", ArgumentsJSON: `{}`,
	}, nil, nil)
	_, _ = svc.ConfirmExecution(ctx, result.ToolCallID, &ConfirmExecutionRequest{Status: ToolCallStatusIndeterminate})

	// Reconcile 前必须有验证证据
	if _, err := svc.Reconcile(ctx, result.ToolCallID, ReconcileExecuted, ""); !errors.Is(err, ErrVerificationMissing) {
		t.Fatalf("reconcile without verification should fail, got: %v", err)
	}

	// 写入验证证据:外部确认已执行
	if _, err := svc.VerifyResult(ctx, result.ToolCallID, `{"executed":true,"external_object_id":"obj-9"}`); err != nil {
		t.Fatalf("verify failed: %v", err)
	}

	tc, err := svc.Reconcile(ctx, result.ToolCallID, ReconcileExecuted, `{"source":"external_api"}`)
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}
	if tc.Status != ToolCallStatusSucceeded {
		t.Fatalf("status = %s, want succeeded", tc.Status)
	}
	if tc.VerificationJSON == nil {
		t.Fatal("verification evidence should be merged and persisted")
	}
}

func TestReconcileNotExecutedToFailedThenRetry(t *testing.T) {
	svc, _, _, _ := setupLifecycleService()
	ctx := context.Background()

	result, _ := svc.Execute(ctx, &ExecuteRequest{
		TenantID: "t1", RunID: "r1", AgentID: "a1", BusinessAppCode: "finance",
		ToolID: "validate_metrics", ToolVersion: "2.0",
		IdempotencyKey: "lc-retry", ArgumentsJSON: `{}`,
	}, nil, nil)
	_, _ = svc.ConfirmExecution(ctx, result.ToolCallID, &ConfirmExecutionRequest{Status: ToolCallStatusIndeterminate})

	// 外部确认未执行 → failed
	_, _ = svc.VerifyResult(ctx, result.ToolCallID, `{"executed":false}`)
	tc, err := svc.Reconcile(ctx, result.ToolCallID, ReconcileNotExecuted, "")
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}
	if tc.Status != ToolCallStatusFailed {
		t.Fatalf("status = %s, want failed", tc.Status)
	}

	// Retry Gate:failed + 确认未执行证据 → executing(中低风险)
	tc, err = svc.Retry(ctx, result.ToolCallID)
	if err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	if tc.Status != ToolCallStatusExecuting {
		t.Fatalf("status = %s, want executing", tc.Status)
	}
	if tc.RetryCount != 1 {
		t.Fatalf("retry_count = %d, want 1", tc.RetryCount)
	}
}

func TestReconcileStillUnknownStaysIndeterminate(t *testing.T) {
	svc, _, _, _ := setupLifecycleService()
	ctx := context.Background()

	result, _ := svc.Execute(ctx, &ExecuteRequest{
		TenantID: "t1", RunID: "r1", AgentID: "a1", BusinessAppCode: "finance",
		ToolID: "validate_metrics", ToolVersion: "2.0",
		IdempotencyKey: "lc-unknown", ArgumentsJSON: `{}`,
	}, nil, nil)
	_, _ = svc.ConfirmExecution(ctx, result.ToolCallID, &ConfirmExecutionRequest{Status: ToolCallStatusIndeterminate})
	_, _ = svc.VerifyResult(ctx, result.ToolCallID, `{"executed":null}`)

	tc, err := svc.Reconcile(ctx, result.ToolCallID, ReconcileStillUnknown, "")
	if err != nil {
		t.Fatalf("reconcile still_unknown failed: %v", err)
	}
	if tc.Status != ToolCallStatusIndeterminate {
		t.Fatalf("status = %s, want indeterminate", tc.Status)
	}
}

// ── Retry Gate ──

func TestRetryGateBlocks(t *testing.T) {
	svc, _, _, _ := setupLifecycleService()
	ctx := context.Background()

	// 场景 1:succeeded 终态不可重试
	result, _ := svc.Execute(ctx, &ExecuteRequest{
		TenantID: "t1", RunID: "r1", AgentID: "a1", BusinessAppCode: "finance",
		ToolID: "validate_metrics", ToolVersion: "2.0",
		IdempotencyKey: "lc-retry-ok", ArgumentsJSON: `{}`,
	}, nil, nil)
	_, _ = svc.ConfirmExecution(ctx, result.ToolCallID, &ConfirmExecutionRequest{Status: ToolCallStatusSucceeded})
	if _, err := svc.Retry(ctx, result.ToolCallID); !errors.Is(err, ErrRetryNotAllowed) {
		t.Fatalf("succeeded retry should fail with ErrRetryNotAllowed, got: %v", err)
	}

	// 场景 2:failed 但验证证据显示已执行(副作用已发生)不可重试
	result2, _ := svc.Execute(ctx, &ExecuteRequest{
		TenantID: "t1", RunID: "r2", AgentID: "a1", BusinessAppCode: "finance",
		ToolID: "validate_metrics", ToolVersion: "2.0",
		IdempotencyKey: "lc-retry-executed", ArgumentsJSON: `{}`,
	}, nil, nil)
	_, _ = svc.ConfirmExecution(ctx, result2.ToolCallID, &ConfirmExecutionRequest{Status: ToolCallStatusIndeterminate})
	_, _ = svc.VerifyResult(ctx, result2.ToolCallID, `{"executed":true}`)
	_, _ = svc.Reconcile(ctx, result2.ToolCallID, ReconcileNotExecuted, "") // 注意:证据为 executed 却对账 not_executed 不应放行 retry
	if _, err := svc.Retry(ctx, result2.ToolCallID); !errors.Is(err, ErrRetryNotAllowed) {
		t.Fatalf("retry with executed=true evidence should fail with ErrRetryNotAllowed, got: %v", err)
	}

	// 场景 3:failed 但没有验证证据不可重试
	result3, _ := svc.Execute(ctx, &ExecuteRequest{
		TenantID: "t1", RunID: "r3", AgentID: "a1", BusinessAppCode: "finance",
		ToolID: "validate_metrics", ToolVersion: "2.0",
		IdempotencyKey: "lc-retry-noverify", ArgumentsJSON: `{}`,
	}, nil, nil)
	_, _ = svc.ConfirmExecution(ctx, result3.ToolCallID, &ConfirmExecutionRequest{Status: ToolCallStatusFailed})
	if _, err := svc.Retry(ctx, result3.ToolCallID); !errors.Is(err, ErrRetryNotAllowed) {
		t.Fatalf("retry without verification should fail with ErrRetryNotAllowed, got: %v", err)
	}
}

func TestHighRiskRetryRequiresNewApproval(t *testing.T) {
	svc, _, _, approvalRepo := setupLifecycleService()
	ctx := context.Background()

	result, _ := svc.Execute(ctx, &ExecuteRequest{
		TenantID: "t1", RunID: "r1", AgentID: "a1", BusinessAppCode: "finance",
		ToolID: "archive_report", ToolVersion: "1.0",
		IdempotencyKey: "lc-highretry", ArgumentsJSON: `{}`,
	}, nil, nil)

	// 审批通过 → executing → 对账确认未执行 → failed
	approval, _ := approvalRepo.FindToolCallApproval(ctx, result.ToolCallID)
	_ = approvalRepo.UpdateDecision(ctx, approval.ID, "approved", "u1")
	_ = svc.BindToolCallApprovalDecision(ctx, result.ToolCallID, "approved")
	_, _ = svc.ConfirmExecution(ctx, result.ToolCallID, &ConfirmExecutionRequest{Status: ToolCallStatusIndeterminate})
	_, _ = svc.VerifyResult(ctx, result.ToolCallID, `{"executed":false}`)
	_, _ = svc.Reconcile(ctx, result.ToolCallID, ReconcileNotExecuted, "")

	// 高风险重试 → 回到 pending_approval(重新审批)
	tc, err := svc.Retry(ctx, result.ToolCallID)
	if err != nil {
		t.Fatalf("high risk retry failed: %v", err)
	}
	if tc.Status != ToolCallStatusPendingApproval {
		t.Fatalf("status = %s, want pending_approval", tc.Status)
	}
	if tc.RetryCount != 1 {
		t.Fatalf("retry_count = %d, want 1", tc.RetryCount)
	}
}

func TestVerifyTerminalRejected(t *testing.T) {
	svc, _, _, _ := setupLifecycleService()
	ctx := context.Background()

	result, _ := svc.Execute(ctx, &ExecuteRequest{
		TenantID: "t1", RunID: "r1", AgentID: "a1", BusinessAppCode: "finance",
		ToolID: "validate_metrics", ToolVersion: "2.0",
		IdempotencyKey: "lc-verify-term", ArgumentsJSON: `{}`,
	}, nil, nil)
	_, _ = svc.ConfirmExecution(ctx, result.ToolCallID, &ConfirmExecutionRequest{Status: ToolCallStatusSucceeded})

	if _, err := svc.VerifyResult(ctx, result.ToolCallID, `{}`); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("verify on terminal should fail with ErrInvalidTransition, got: %v", err)
	}
}

func TestRequestApprovalIdempotent(t *testing.T) {
	svc, _, callRepo, _ := setupLifecycleService()
	ctx := context.Background()

	result, _ := svc.Execute(ctx, &ExecuteRequest{
		TenantID: "t1", RunID: "r1", AgentID: "a1", BusinessAppCode: "finance",
		ToolID: "archive_report", ToolVersion: "1.0",
		IdempotencyKey: "lc-req-appr", ArgumentsJSON: `{}`,
	}, nil, nil)

	// Execute 已自动创建;手动补建应幂等返回同一任务
	a1, err := svc.RequestApproval(ctx, result.ToolCallID, "")
	if err != nil {
		t.Fatalf("request approval failed: %v", err)
	}
	a2, err := svc.RequestApproval(ctx, result.ToolCallID, "")
	if err != nil {
		t.Fatalf("idempotent request approval failed: %v", err)
	}
	if a1.ID != a2.ID {
		t.Fatalf("expected same approval task, got %s vs %s", a1.ID, a2.ID)
	}

	// 非 pending_approval 状态调用应被拦截
	tc, _ := callRepo.GetByID(ctx, result.ToolCallID)
	tc.Status = ToolCallStatusExecuting
	if _, err := svc.RequestApproval(ctx, result.ToolCallID, ""); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("request approval on executing should fail with ErrInvalidTransition, got: %v", err)
	}
}
