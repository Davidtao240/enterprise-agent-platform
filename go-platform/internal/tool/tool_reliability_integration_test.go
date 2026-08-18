package tool

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestToolReliabilityPostgresAcceptance M2-C 收尾 + M2-D + M2-E PG 验收:
// 超时扫描 → indeterminate、熔断 open 拒绝、DLQ、credential 边界、trace 串联。
//
//	TEST_DATABASE_URL=... go test ./internal/tool -run TestToolReliabilityPostgresAcceptance -v
func TestToolReliabilityPostgresAcceptance(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}

	var tenantID string
	if err := pool.QueryRow(ctx,
		`SELECT tenant_id FROM users WHERE username='finance_manager'`).Scan(&tenantID); err != nil {
		t.Fatalf("load actor: %v", err)
	}

	// ── M2-C.8: 超时扫描器 ──
	toolRepo := NewRepository(pool)
	callRepo := NewToolCallRepository(pool)
	approvalRepo := NewApprovalRepository(pool)
	scanner := NewToolCallTimeoutScanner(callRepo, nil, time.Minute)

	traceID := "trace-acc-" + uuid.NewString()[:8]
	result, err := NewService(toolRepo, callRepo, nil,
		WithApprovalRepository(approvalRepo),
		WithReliabilityPolicy(-1, 0), // 不设默认超时,手动构造过期 timeout_at
	).Execute(ctx, &ExecuteRequest{
		TenantID: tenantID, RunID: uuid.NewString(), AgentID: "data_extract_agent",
		BusinessAppCode: "finance", ToolID: "parse_csv", ToolVersion: "1.0",
		ArgumentsJSON: `{}`, IdempotencyKey: "rel-" + uuid.NewString(), TraceID: traceID,
	}, nil, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	// 手动把 timeout_at 拨到过去,模拟超时
	if _, err := pool.Exec(ctx,
		`UPDATE tool_calls SET timeout_at = now() - interval '1 minute' WHERE id = $1`,
		result.ToolCallID); err != nil {
		t.Fatalf("set expired timeout: %v", err)
	}
	n, err := scanner.ScanOnce(ctx)
	if err != nil || n != 1 {
		t.Fatalf("timeout scan converted=%d err=%v, want 1/nil", n, err)
	}
	tc, _ := callRepo.GetByID(ctx, result.ToolCallID)
	if tc.Status != ToolCallStatusIndeterminate || tc.ErrorJSON == nil ||
		!strings.Contains(*tc.ErrorJSON, "TOOL_TIMEOUT") {
		t.Fatalf("timed-out call: status=%s err=%v, want indeterminate+TOOL_TIMEOUT", tc.Status, tc.ErrorJSON)
	}
	// 补 verify + reconcile → succeeded(完整超时对账闭环)
	if _, err := NewService(toolRepo, callRepo, nil).VerifyResult(ctx, result.ToolCallID, `{"executed":true}`); err != nil {
		t.Fatalf("verify: %v", err)
	}
	svcPlain := NewService(toolRepo, callRepo, nil)
	if _, err := svcPlain.Reconcile(ctx, result.ToolCallID, ReconcileExecuted, ""); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	// ── M2-E: trace 串联 ──
	byTrace, err := svcPlain.ListByTrace(ctx, traceID)
	if err != nil || len(byTrace) != 1 || byTrace[0].ID != result.ToolCallID {
		t.Fatalf("ListByTrace=%v err=%v, want 1 hit", byTrace, err)
	}

	// ── M2-C.9: 熔断器 ──
	circuitRepo := NewCircuitBreakerRepository(pool)
	cb := NewCircuitBreaker(circuitRepo, 2, 2*time.Second)
	toolID := "parse_csv"
	// 2 次 failed(阈值 2)→ open
	if err := circuitRepo.RecordFailure(ctx, toolID, 2); err != nil {
		t.Fatalf("record failure 1: %v", err)
	}
	if err := circuitRepo.RecordFailure(ctx, toolID, 2); err != nil {
		t.Fatalf("record failure 2: %v", err)
	}
	st, _ := circuitRepo.GetState(ctx, toolID)
	if st.State != "open" {
		t.Fatalf("circuit state=%s, want open", st.State)
	}
	// open + 冷却期内:Execute 被拒
	svcCircuit := NewService(toolRepo, callRepo, nil, WithCircuitBreaker(cb))
	if _, err := svcCircuit.Execute(ctx, &ExecuteRequest{
		TenantID: tenantID, RunID: uuid.NewString(), AgentID: "data_extract_agent",
		BusinessAppCode: "finance", ToolID: toolID, ToolVersion: "1.0",
		ArgumentsJSON: `{}`, IdempotencyKey: "cir-" + uuid.NewString(),
	}, nil, nil); err == nil {
		t.Fatal("execute during open circuit must be rejected")
	}
	// 冷却期满 → half_open 探测放行
	time.Sleep(2100 * time.Millisecond)
	allow, state, _ := cb.Allow(ctx, toolID)
	if !allow || state != "half_open" {
		t.Fatalf("after cooldown allow=%v state=%s, want true/half_open", allow, state)
	}
	// 探测成功 → closed
	if err := circuitRepo.RecordSuccess(ctx, toolID); err != nil {
		t.Fatalf("record success: %v", err)
	}
	st, _ = circuitRepo.GetState(ctx, toolID)
	if st.State != "closed" || st.ConsecutiveFailures != 0 {
		t.Fatalf("after success state=%s failures=%d, want closed/0", st.State, st.ConsecutiveFailures)
	}
	// 清理熔断状态,避免影响其他用例
	if _, err := pool.Exec(ctx, `DELETE FROM tool_circuit_breakers WHERE tool_id=$1`, toolID); err != nil {
		t.Fatalf("cleanup circuit: %v", err)
	}

	// ── M2-C.10: DLQ ──
	svcDLQ := NewService(toolRepo, callRepo, nil, WithReliabilityPolicy(-1, 1)) // maxRetry=1
	dlqResult, err := svcDLQ.Execute(ctx, &ExecuteRequest{
		TenantID: tenantID, RunID: uuid.NewString(), AgentID: "data_extract_agent",
		BusinessAppCode: "finance", ToolID: toolID, ToolVersion: "1.0",
		ArgumentsJSON: `{}`, IdempotencyKey: "dlq-" + uuid.NewString(),
	}, nil, nil)
	if err != nil {
		t.Fatalf("execute for dlq: %v", err)
	}
	// 第一轮:indeterminate → verify(false) → reconcile(not_executed) → retry 放行(retry_count 0→1)
	_, _ = svcDLQ.ConfirmExecution(ctx, dlqResult.ToolCallID, &ConfirmExecutionRequest{Status: ToolCallStatusIndeterminate})
	_, _ = svcDLQ.VerifyResult(ctx, dlqResult.ToolCallID, `{"executed":false}`)
	_, _ = svcDLQ.Reconcile(ctx, dlqResult.ToolCallID, ReconcileNotExecuted, "")
	if _, err := svcDLQ.Retry(ctx, dlqResult.ToolCallID); err != nil {
		t.Fatalf("first retry should pass (max=1, count was 0): %v", err)
	}
	// 第二轮:retry_count=1 已达上限 → DLQ
	_, _ = svcDLQ.ConfirmExecution(ctx, dlqResult.ToolCallID, &ConfirmExecutionRequest{Status: ToolCallStatusIndeterminate})
	_, _ = svcDLQ.VerifyResult(ctx, dlqResult.ToolCallID, `{"executed":false}`)
	_, _ = svcDLQ.Reconcile(ctx, dlqResult.ToolCallID, ReconcileNotExecuted, "")
	if _, err := svcDLQ.Retry(ctx, dlqResult.ToolCallID); err == nil {
		t.Fatal("second retry must hit DLQ")
	}
	dlqTC, _ := callRepo.GetByID(ctx, dlqResult.ToolCallID)
	if !dlqTC.IsDeadLetter {
		t.Fatal("tool_call must be marked dead letter")
	}
	dlqList, err := svcDLQ.ListDeadLetters(ctx)
	if err != nil {
		t.Fatalf("list dead letters: %v", err)
	}
	found := false
	for _, d := range dlqList {
		if d.ID == dlqResult.ToolCallID {
			found = true
		}
	}
	if !found {
		t.Fatal("dead letter list must contain the call")
	}

	// ── M2-D: Credential 边界 ──
	credSvc, err := NewCredentialService(pool, "acceptance-test-key", nil)
	if err != nil {
		t.Fatalf("credential service: %v", err)
	}
	plaintext := "secret-token-" + uuid.NewString()
	binding, err := credSvc.CreateBinding(ctx, &CreateBindingRequest{
		TenantID: tenantID, BusinessAppCode: "finance",
		ConnectorCode: "erp_test", Name: "acceptance erp",
		CredentialPlaintext: plaintext,
	})
	if err != nil {
		t.Fatalf("create binding: %v", err)
	}
	if binding.CredentialRef == nil || !strings.HasPrefix(*binding.CredentialRef, "secret:") {
		t.Fatalf("credential_ref=%v, want secret:<uuid>", binding.CredentialRef)
	}
	// DB 无明文断言:全库扫描 credential_secrets 与 connector_bindings
	var leak string
	err = pool.QueryRow(ctx, `SELECT cipher_text FROM credential_secrets LIMIT 1`).Scan(&leak)
	if err == nil && strings.Contains(leak, plaintext) {
		t.Fatal("PLAINTEXT LEAK in credential_secrets.cipher_text")
	}
	var configJSON string
	_ = pool.QueryRow(ctx,
		`SELECT config_json::text FROM connector_bindings WHERE id=$1`, binding.ID).Scan(&configJSON)
	if strings.Contains(configJSON, plaintext) {
		t.Fatal("PLAINTEXT LEAK in connector_bindings.config_json")
	}
	// 解析:同租户成功往返
	resolved, err := credSvc.ResolveCredential(ctx, tenantID, binding.ID)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.Plaintext != plaintext {
		t.Fatal("resolved plaintext mismatch")
	}
	// 跨租户:视同不存在
	if _, err := credSvc.ResolveCredential(ctx, uuid.NewString(), binding.ID); err == nil {
		t.Fatal("cross-tenant resolve must fail")
	}
	// Execute 接线:不存在的绑定拒绝
	if _, err := NewService(toolRepo, callRepo, nil, WithBindingValidator(credSvc)).Execute(ctx, &ExecuteRequest{
		TenantID: tenantID, RunID: uuid.NewString(), AgentID: "data_extract_agent",
		BusinessAppCode: "finance", ToolID: toolID, ToolVersion: "1.0",
		ArgumentsJSON: `{}`, IdempotencyKey: "cb-" + uuid.NewString(),
		ConnectorBindingID: uuid.NewString(),
	}, nil, nil); err == nil {
		t.Fatal("execute with missing binding must fail")
	}
}
