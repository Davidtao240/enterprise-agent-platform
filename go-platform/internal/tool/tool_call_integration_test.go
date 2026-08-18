package tool

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestToolCallPostgresAcceptance M2-B PG 验收:tool_calls CRUD + 版本校验 + 幂等。
//
//	TEST_DATABASE_URL=... go test ./internal/tool -run TestToolCallPostgresAcceptance -v
func TestToolCallPostgresAcceptance(t *testing.T) {
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

	// 确保 017 迁移已应用
	var hasVersion bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='tool_registry' AND column_name='version')`).Scan(&hasVersion); err != nil {
		t.Fatalf("check version column: %v", err)
	}
	if !hasVersion {
		t.Fatal("tool_registry.version column missing; run migration 017 first")
	}

	var tenantID, userID string
	if err := pool.QueryRow(ctx,
		`SELECT tenant_id, id FROM users ORDER BY created_at LIMIT 1`,
	).Scan(&tenantID, &userID); err != nil {
		t.Fatalf("load acceptance actor: %v", err)
	}

	// 确保 seed 工具已有 version
	if _, err := pool.Exec(ctx,
		`UPDATE tool_registry SET version = '1.0' WHERE version IS NULL`); err != nil {
		t.Fatalf("backfill versions: %v", err)
	}

	toolRepo := NewRepository(pool)
	callRepo := NewToolCallRepository(pool)
	svc := NewService(toolRepo, callRepo, nil)

	// 1) 版本校验:已知工具 + 指定版本成功
	tool, err := toolRepo.FindToolByIDAndVersion(ctx, "parse_csv", "1.0")
	if err != nil {
		t.Fatalf("find parse_csv@1.0: %v", err)
	}
	if tool.ToolID != "parse_csv" || tool.Version != "1.0" {
		t.Fatalf("found wrong tool: %+v", tool)
	}

	// 2) 版本校验:不存在版本拒绝
	if _, err := toolRepo.FindToolByIDAndVersion(ctx, "parse_csv", "99.9"); err == nil {
		t.Fatal("expected error for nonexistent version")
	}

	// 3) Create tool_call
	runID := uuid.NewString()
	idemKey1 := "idem-" + uuid.NewString()
	result1, err := svc.Execute(ctx, &ExecuteRequest{
		TenantID: tenantID, RunID: runID, AgentID: "data_extract_agent",
		BusinessAppCode: "finance", ToolID: "parse_csv", ToolVersion: "1.0",
		ConnectorBindingID: uuid.NewString(), PolicyVersion: "policy-1",
		ArgumentsJSON: `{"file":"test.csv"}`, IdempotencyKey: idemKey1,
	}, nil, nil)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if result1.ToolCallID == "" || result1.Status != ToolCallStatusExecuting {
		t.Fatalf("unexpected result: %+v", result1)
	}

	// 4) GetByID 验证持久化
	tc, err := callRepo.GetByID(ctx, result1.ToolCallID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if tc.ToolID != "parse_csv" || tc.ToolVersion != "1.0" {
		t.Fatalf("persisted wrong tool/version: %s@%s", tc.ToolID, tc.ToolVersion)
	}
	if tc.TenantID != tenantID {
		t.Fatalf("tenant_id not preserved: %s", tc.TenantID)
	}

	// 5) 幂等重放:相同 key 返回已有记录
	resultReplay, err := svc.Execute(ctx, &ExecuteRequest{
		TenantID: tenantID, RunID: runID, AgentID: "data_extract_agent",
		BusinessAppCode: "finance", ToolID: "parse_csv", ToolVersion: "1.0",
		ConnectorBindingID: uuid.NewString(), PolicyVersion: "policy-1",
		ArgumentsJSON: `{"file":"different.csv"}`, IdempotencyKey: idemKey1,
	}, nil, nil)
	if err != nil {
		t.Fatalf("idempotent replay failed: %v", err)
	}
	if resultReplay.ToolCallID != result1.ToolCallID {
		t.Fatalf("replay should return same tool_call_id, got %s vs %s", resultReplay.ToolCallID, result1.ToolCallID)
	}

	// 6) 高风险工具→pending_approval
	result3, err := svc.Execute(ctx, &ExecuteRequest{
		TenantID: tenantID, RunID: runID, AgentID: "report_agent",
		BusinessAppCode: "finance", ToolID: "archive_report", ToolVersion: "1.0",
		ConnectorBindingID: uuid.NewString(), PolicyVersion: "policy-1",
		ArgumentsJSON: `{"report_id":1}`, IdempotencyKey: "idem-" + uuid.NewString(),
	}, nil, nil)
	if err != nil {
		t.Fatalf("high risk execute failed: %v", err)
	}
	if result3.Status != ToolCallStatusPendingApproval || !result3.ApprovalReq {
		t.Fatalf("high risk should be pending_approval: %+v", result3)
	}

	// 7) 验证 input_hash 持久化
	var inputHash string
	if err := pool.QueryRow(ctx,
		`SELECT input_hash FROM tool_calls WHERE id = $1`, result1.ToolCallID,
	).Scan(&inputHash); err != nil {
		t.Fatalf("load input_hash: %v", err)
	}
	if inputHash == "" {
		t.Fatal("input_hash should not be empty")
	}

	// 8) ListByRun(应包含 2 条:parse_csv + archive_report)
	list, err := callRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list by run: %v", err)
	}
	if len(list) < 2 {
		t.Fatalf("expected >=2 tool_calls for run %s, got %d", runID, len(list))
	}

	// 9) 清理
	for _, tc := range list {
		_, _ = pool.Exec(ctx, `DELETE FROM tool_calls WHERE id = $1`, tc.ID)
	}
}

// TestToolCallUpdateStatusPostgresAcceptance PG 验收:tool_call 状态更新 + reconciliation。
func TestToolCallUpdateStatusPostgresAcceptance(t *testing.T) {
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

	var tenantID string
	if err := pool.QueryRow(ctx, `SELECT tenant_id FROM users ORDER BY created_at LIMIT 1`).Scan(&tenantID); err != nil {
		t.Fatalf("load tenant: %v", err)
	}

	callRepo := NewToolCallRepository(pool)

	// 创建一个 executing 状态的 tool_call
	tc, err := callRepo.Create(ctx, &ToolCallRequest{
		TenantID: tenantID, RunID: uuid.NewString(), ToolID: "parse_csv",
		ToolVersion: "1.0", ConnectorBindingID: uuid.NewString(), PolicyVersion: "p-1",
		RiskLevel: "low", Status: ToolCallStatusExecuting,
		IdempotencyKey: "update-test-" + uuid.NewString(),
		InputHash: "hash-test", InputSummaryJSON: `{"test":true}`,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// 更新为 succeeded
	err = callRepo.UpdateStatus(ctx, tc.ID, ToolCallStatusSucceeded, map[string]any{
		"output_summary_json": `{"result":"ok"}`,
	})
	if err != nil {
		t.Fatalf("update status: %v", err)
	}

	// 验证更新
	updated, err := callRepo.GetByID(ctx, tc.ID)
	if err != nil {
		t.Fatalf("get updated: %v", err)
	}
	if updated.Status != ToolCallStatusSucceeded {
		t.Fatalf("status = %s, want succeeded", updated.Status)
	}

	// reconciliation:indeterminate
	err = callRepo.UpdateStatus(ctx, tc.ID, ToolCallStatusIndeterminate, map[string]any{
		"verification_json": `{"reconciled":true}`,
	})
	if err != nil {
		t.Fatalf("reconcile update: %v", err)
	}

	final, err := callRepo.GetByID(ctx, tc.ID)
	if err != nil {
		t.Fatalf("get final: %v", err)
	}
	if final.Status != ToolCallStatusIndeterminate {
		t.Fatalf("post-reconcile status = %s, want indeterminate", final.Status)
	}

	// 清理
	_, _ = pool.Exec(ctx, `DELETE FROM tool_calls WHERE id = $1`, tc.ID)
}

// TestToolCallCrossTenantIsolation PG 验收:tenant_id 范围内幂等键隔离。
func TestToolCallCrossTenantIsolation(t *testing.T) {
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

	rows, err := pool.Query(ctx, `SELECT DISTINCT tenant_id FROM users ORDER BY tenant_id LIMIT 2`)
	if err != nil {
		t.Fatalf("load tenants: %v", err)
	}
	defer rows.Close()
	var tenants []string
	for rows.Next() {
		var tid string
		if err := rows.Scan(&tid); err != nil {
			t.Fatal(err)
		}
		tenants = append(tenants, tid)
	}
	if len(tenants) < 2 {
		t.Skip("need at least 2 tenants for isolation test")
	}

	callRepo := NewToolCallRepository(pool)
	sharedKey := "cross-tenant-key-" + uuid.NewString()

	// tenant A 创建
	tcA, err := callRepo.Create(ctx, &ToolCallRequest{
		TenantID: tenants[0], RunID: uuid.NewString(), ToolID: "parse_csv",
		ToolVersion: "1.0", ConnectorBindingID: uuid.NewString(), PolicyVersion: "p-1",
		RiskLevel: "low", Status: ToolCallStatusExecuting,
		IdempotencyKey: sharedKey, InputHash: "hash-a",
	})
	if err != nil {
		t.Fatalf("tenant A create: %v", err)
	}

	// tenant B 用相同 key 创建:应该成功(不同租户隔离)
	tcB, err := callRepo.Create(ctx, &ToolCallRequest{
		TenantID: tenants[1], RunID: uuid.NewString(), ToolID: "parse_csv",
		ToolVersion: "1.0", ConnectorBindingID: uuid.NewString(), PolicyVersion: "p-1",
		RiskLevel: "low", Status: ToolCallStatusExecuting,
		IdempotencyKey: sharedKey, InputHash: "hash-b",
	})
	if err != nil {
		t.Fatalf("tenant B create (should succeed with same key, different tenant): %v", err)
	}

	// 验证两个调用独立
	if tcA.ID == tcB.ID {
		t.Fatal("cross-tenant idempotency should not collide")
	}

	// tenant A 重放同 key:应该返回已有记录
	replay, err := callRepo.GetByIdempotencyKey(ctx, tenants[0], sharedKey)
	if err != nil {
		t.Fatalf("replay lookup: %v", err)
	}
	if replay == nil || replay.ID != tcA.ID {
		t.Fatalf("tenant A replay should return tcA, got %+v", replay)
	}

	// 清理
	for _, id := range []string{tcA.ID, tcB.ID} {
		_, _ = pool.Exec(ctx, `DELETE FROM tool_calls WHERE id = $1`, id)
	}
}

// TestToolNotFoundRejected PG 验收:不存在的工具被拒绝。
func TestToolNotFoundRejected(t *testing.T) {
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

	var tenantID string
	if err := pool.QueryRow(ctx, `SELECT tenant_id FROM users ORDER BY created_at LIMIT 1`).Scan(&tenantID); err != nil {
		t.Fatalf("load tenant: %v", err)
	}

	toolRepo := NewRepository(pool)
	callRepo := NewToolCallRepository(pool)
	svc := NewService(toolRepo, callRepo, nil)

	_, err = svc.Execute(ctx, &ExecuteRequest{
		TenantID: tenantID, RunID: uuid.NewString(), AgentID: "data_extract_agent",
		BusinessAppCode: "finance", ToolID: "nonexistent_tool_xyz", ToolVersion: "1.0",
		IdempotencyKey: "not-found-" + uuid.NewString(), ArgumentsJSON: "{}",
	}, nil, nil)
	if err == nil {
		t.Fatal("expected error for nonexistent tool")
	}
	if !strings.Contains(err.Error(), "tool not found") {
		t.Fatalf("error should mention tool not found: %v", err)
	}
}
