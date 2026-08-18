package tool

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// pgDomainPolicy 从 domain_policies 表读取允许域(等价 policy.Repository,避免测试跨包依赖)。
type pgDomainPolicy struct{ pool *pgxpool.Pool }

func (p pgDomainPolicy) FindAllowedDomains(ctx context.Context, businessAppCode string) ([]string, error) {
	var raw string
	err := p.pool.QueryRow(ctx,
		`SELECT allowed_tool_domains::text FROM domain_policies
		 WHERE business_app_code = $1 AND status = 'active' AND deleted_at IS NULL LIMIT 1`,
		businessAppCode).Scan(&raw)
	if err != nil {
		return nil, nil
	}
	var domains []string
	_ = json.Unmarshal([]byte(raw), &domains)
	return domains, nil
}

// TestToolLifecyclePostgresAcceptance M2-C PG 验收:完整生命周期
// 高风险→自动建审批→批准(重检)→executing→indeterminate→verify→reconcile→succeeded;
// 拒绝流→cancelled;Retry Gate→failed+未执行证据→executing。
//
//	TEST_DATABASE_URL=... go test ./internal/tool -run TestToolLifecyclePostgresAcceptance -v
func TestToolLifecyclePostgresAcceptance(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect acceptance database: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping acceptance database: %v", err)
	}

	// 确保 018 迁移已应用
	var hasPayloadHash, hasRetryCount bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='approval_tasks' AND column_name='payload_hash')`).Scan(&hasPayloadHash); err != nil {
		t.Fatalf("check payload_hash column: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='tool_calls' AND column_name='retry_count')`).Scan(&hasRetryCount); err != nil {
		t.Fatalf("check retry_count column: %v", err)
	}
	if !hasPayloadHash || !hasRetryCount {
		t.Fatal("migration 018 (tool execution lifecycle) not applied")
	}

	var tenantID, managerID string
	if err := pool.QueryRow(ctx,
		`SELECT tenant_id, id FROM users WHERE username='finance_manager' LIMIT 1`).Scan(&tenantID, &managerID); err != nil {
		t.Fatalf("load acceptance actor: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE tool_registry SET version = '1.0' WHERE version IS NULL`); err != nil {
		t.Fatalf("backfill versions: %v", err)
	}

	toolRepo := NewRepository(pool)
	callRepo := NewToolCallRepository(pool)
	approvalRepo := NewApprovalRepository(pool)
	svc := NewService(toolRepo, callRepo, nil,
		WithApprovalRepository(approvalRepo),
		WithDomainPolicy(pgDomainPolicy{pool: pool}),
	)

	// ── 流程 1:批准路径(完整生命周期)──
	runID := uuid.NewString()
	result, err := svc.Execute(ctx, &ExecuteRequest{
		TenantID: tenantID, RunID: runID, AgentID: "report_agent",
		BusinessAppCode: "finance", ToolID: "archive_report", ToolVersion: "1.0",
		ArgumentsJSON: `{"report_id":42}`, IdempotencyKey: "lc-" + uuid.NewString(),
	}, nil, nil)
	if err != nil {
		t.Fatalf("high risk execute failed: %v", err)
	}
	if result.Status != ToolCallStatusPendingApproval {
		t.Fatalf("status = %s, want pending_approval", result.Status)
	}

	// 自动创建审批任务,payload_hash 绑定 input_hash
	approval, err := approvalRepo.FindToolCallApproval(ctx, result.ToolCallID)
	if err != nil {
		t.Fatalf("approval should be auto-created: %v", err)
	}
	tc, err := callRepo.GetByID(ctx, result.ToolCallID)
	if err != nil {
		t.Fatalf("get tool_call: %v", err)
	}
	if approval.PayloadHash != tc.InputHash {
		t.Fatalf("payload_hash %s != input_hash %s", approval.PayloadHash, tc.InputHash)
	}
	if tc.ApprovalTaskID == nil || *tc.ApprovalTaskID != approval.ID {
		t.Fatalf("approval_task_id not bound: %+v", tc.ApprovalTaskID)
	}

	// 批准 → 重检通过 → executing
	if err := approvalRepo.UpdateDecision(ctx, approval.ID, "approved", managerID); err != nil {
		t.Fatalf("decide approval: %v", err)
	}
	if err := svc.BindToolCallApprovalDecision(ctx, result.ToolCallID, "approved"); err != nil {
		t.Fatalf("bind approved decision: %v", err)
	}
	tc, _ = callRepo.GetByID(ctx, result.ToolCallID)
	if tc.Status != ToolCallStatusExecuting {
		t.Fatalf("status = %s, want executing", tc.Status)
	}

	// 执行超时 → indeterminate(非 failed)
	tc, err = svc.ConfirmExecution(ctx, result.ToolCallID, &ConfirmExecutionRequest{
		Status: ToolCallStatusIndeterminate,
		ErrorJSON: `{"code":"TIMEOUT","message":"external api timeout"}`,
		ExternalRequestID: "ext-req-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("confirm indeterminate: %v", err)
	}
	if tc.Status != ToolCallStatusIndeterminate {
		t.Fatalf("status = %s, want indeterminate", tc.Status)
	}

	// Reconcile 前置:必须有验证证据
	if _, err := svc.Reconcile(ctx, result.ToolCallID, ReconcileExecuted, ""); err == nil {
		t.Fatal("reconcile without verification should fail")
	}

	// Verify → Reconcile:外部确认已执行 → succeeded
	if _, err := svc.VerifyResult(ctx, result.ToolCallID, `{"executed":true,"external_object_id":"obj-1"}`); err != nil {
		t.Fatalf("verify: %v", err)
	}
	tc, err = svc.Reconcile(ctx, result.ToolCallID, ReconcileExecuted, `{"source":"bank_api"}`)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if tc.Status != ToolCallStatusSucceeded {
		t.Fatalf("status = %s, want succeeded", tc.Status)
	}
	// 终态后非法转换被拦截
	if _, err := svc.ConfirmExecution(ctx, result.ToolCallID, &ConfirmExecutionRequest{Status: ToolCallStatusFailed}); err == nil {
		t.Fatal("terminal re-confirm should be rejected")
	}

	// ── 流程 2:拒绝路径 ──
	result2, err := svc.Execute(ctx, &ExecuteRequest{
		TenantID: tenantID, RunID: runID, AgentID: "report_agent",
		BusinessAppCode: "finance", ToolID: "archive_report", ToolVersion: "1.0",
		ArgumentsJSON: `{"report_id":43}`, IdempotencyKey: "lc-" + uuid.NewString(),
	}, nil, nil)
	if err != nil {
		t.Fatalf("high risk execute 2 failed: %v", err)
	}
	approval2, _ := approvalRepo.FindToolCallApproval(ctx, result2.ToolCallID)
	if err := approvalRepo.UpdateDecision(ctx, approval2.ID, "rejected", managerID); err != nil {
		t.Fatalf("decide rejection: %v", err)
	}
	if err := svc.BindToolCallApprovalDecision(ctx, result2.ToolCallID, "rejected"); err != nil {
		t.Fatalf("bind rejected decision: %v", err)
	}
	tc2, _ := callRepo.GetByID(ctx, result2.ToolCallID)
	if tc2.Status != ToolCallStatusCancelled {
		t.Fatalf("status = %s, want cancelled", tc2.Status)
	}

	// ── 流程 3:Retry Gate ──
	result3, err := svc.Execute(ctx, &ExecuteRequest{
		TenantID: tenantID, RunID: runID, AgentID: "data_extract_agent",
		BusinessAppCode: "finance", ToolID: "validate_metrics", ToolVersion: "2.0",
		ArgumentsJSON: `{}`, IdempotencyKey: "lc-" + uuid.NewString(),
	}, nil, nil)
	if err != nil {
		// validate_metrics 可能不在 seed,降级用 parse_csv(低风险同样走 retry 路径)
		result3, err = svc.Execute(ctx, &ExecuteRequest{
			TenantID: tenantID, RunID: runID, AgentID: "data_extract_agent",
			BusinessAppCode: "finance", ToolID: "parse_csv", ToolVersion: "1.0",
			ArgumentsJSON: `{}`, IdempotencyKey: "lc-" + uuid.NewString(),
		}, nil, nil)
		if err != nil {
			t.Fatalf("medium/low risk execute failed: %v", err)
		}
	}

	// 无验证证据的 failed 不可重试
	_, _ = svc.ConfirmExecution(ctx, result3.ToolCallID, &ConfirmExecutionRequest{Status: ToolCallStatusFailed, ErrorJSON: `{"code":"ERR"}`})
	if _, err := svc.Retry(ctx, result3.ToolCallID); err == nil {
		t.Fatal("retry without verification should be rejected")
	}

	// 补充"确认未执行"证据后放行;注意 failed 需先经 indeterminate 写入证据
	// (直接对 failed Verify 被拦截),此处验证 Verify 终态拦截:
	if _, err := svc.VerifyResult(ctx, result3.ToolCallID, `{"executed":false}`); err == nil {
		t.Fatal("verify on terminal should be rejected")
	}

	// 完整可重试路径:重新执行一次 → indeterminate → verify(false) → reconcile(not_executed) → retry
	result4, err := svc.Execute(ctx, &ExecuteRequest{
		TenantID: tenantID, RunID: runID, AgentID: "data_extract_agent",
		BusinessAppCode: "finance", ToolID: "parse_csv", ToolVersion: "1.0",
		ArgumentsJSON: `{}`, IdempotencyKey: "lc-" + uuid.NewString(),
	}, nil, nil)
	if err != nil {
		t.Fatalf("execute 4 failed: %v", err)
	}
	_, _ = svc.ConfirmExecution(ctx, result4.ToolCallID, &ConfirmExecutionRequest{Status: ToolCallStatusIndeterminate})
	_, _ = svc.VerifyResult(ctx, result4.ToolCallID, `{"executed":false}`)
	tc4, err := svc.Reconcile(ctx, result4.ToolCallID, ReconcileNotExecuted, "")
	if err != nil {
		t.Fatalf("reconcile not_executed: %v", err)
	}
	if tc4.Status != ToolCallStatusFailed {
		t.Fatalf("status = %s, want failed", tc4.Status)
	}
	tc4, err = svc.Retry(ctx, result4.ToolCallID)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if tc4.Status != ToolCallStatusExecuting || tc4.RetryCount != 1 {
		t.Fatalf("retry result: status=%s retry_count=%d, want executing/1", tc4.Status, tc4.RetryCount)
	}

	// ── 唯一约束:同一 tool_call 至多一条审批(待决期间重复请求幂等)──
	result5, err := svc.Execute(ctx, &ExecuteRequest{
		TenantID: tenantID, RunID: runID, AgentID: "report_agent",
		BusinessAppCode: "finance", ToolID: "archive_report", ToolVersion: "1.0",
		ArgumentsJSON: `{}`, IdempotencyKey: "lc-" + uuid.NewString(),
	}, nil, nil)
	if err != nil {
		t.Fatalf("high risk execute 5 failed: %v", err)
	}
	auto, _ := approvalRepo.FindToolCallApproval(ctx, result5.ToolCallID)
	dup, err := svc.RequestApproval(ctx, result5.ToolCallID, "")
	if err != nil {
		t.Fatalf("idempotent request approval: %v", err)
	}
	if dup.ID != auto.ID {
		t.Fatalf("expected same approval task %s, got %s", auto.ID, dup.ID)
	}
}
