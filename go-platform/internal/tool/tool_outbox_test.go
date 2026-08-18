package tool

import (
	"context"
	"errors"
	"testing"
	"time"
)

// ── M3-C: Service → Outbox 分流测试 ──
// 验证 autoExecuteViaConnector 对 Connector 自声明的 Saga capability
// 走 Outbox 入队(ToolCall 保持 executing),而非直连执行。

// fakeOutboxEnqueuer 内存 Outbox 入队器。
type fakeOutboxEnqueuer struct {
	entries []*OutboxEntry
	fail    bool
}

func (f *fakeOutboxEnqueuer) Enqueue(ctx context.Context, e *OutboxEntry) (*OutboxEntry, error) {
	if f.fail {
		return nil, errors.New("outbox down")
	}
	e.ID = "ob-generated"
	e.State = OutboxStatePending
	e.NextAttemptAt = time.Now().UTC()
	f.entries = append(f.entries, e)
	return e, nil
}

// setupOutboxService 构造带 ERP Connector Runtime + Outbox 的 Service。
func setupOutboxService(t *testing.T) (*Service, *fakeToolCallRepo, *fakeOutboxEnqueuer) {
	t.Helper()
	svc, _, callRepo, _ := setupLifecycleService()

	// 注册 ERP 工具(low risk:无审批直接 executing)。
	svc.toolRepo = &fakeToolRepo{tools: map[string]*Tool{
		"erp_purchase_request": {ToolID: "erp_purchase_request", Version: "1.0", Domain: "shared", RiskLevel: "low", Status: "active", IsShared: true},
	}}

	// Runtime:内存 connectors(同包可构造,不依赖 registry DB)。
	rt := &ConnectorRuntime{connectors: map[string]Connector{
		"erp_connector": NewMockERPConnector(),
	}}
	svc.connectorRT = rt
	svc.bindingRepo = &fakeBindingValidator{bindings: map[string]*ConnectorBinding{
		"bind-erp": {ID: "bind-erp", TenantID: "t1", Status: "active", ConnectorCode: "erp_connector"},
	}}

	outbox := &fakeOutboxEnqueuer{}
	svc.outboxRepo = outbox
	return svc, callRepo, outbox
}

func TestServiceExecute_ERPWriteDefersToOutbox(t *testing.T) {
	svc, callRepo, outbox := setupOutboxService(t)
	ctx := context.Background()

	result, err := svc.Execute(ctx, &ExecuteRequest{
		TenantID: "t1", RunID: "r1", AgentID: "a1", BusinessAppCode: "finance",
		ToolID: "erp_purchase_request", ToolVersion: "1.0",
		IdempotencyKey: "m3c-1", ConnectorBindingID: "bind-erp",
		ArgumentsJSON: `{"idempotency_key":"k-svc","title":"采购交换机","total_amount":12000}`,
	}, nil, nil)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	// Saga 写:入队后 ToolCall 保持 executing(Dispatcher 异步推进终态)。
	if result.Status != ToolCallStatusExecuting {
		t.Fatalf("status = %s, want executing (outbox in-flight)", result.Status)
	}
	if len(outbox.entries) != 1 {
		t.Fatalf("expected 1 outbox entry, got %d", len(outbox.entries))
	}
	e := outbox.entries[0]
	if e.ConnectorCode != "erp_connector" || e.Operation != "erp_purchase_request" {
		t.Fatalf("entry correlation wrong: %+v", e)
	}
	if e.ToolCallID != result.ToolCallID || e.TenantID != "t1" {
		t.Fatalf("entry must correlate tool_call/tenant: %+v", e)
	}
	// 不可变请求体 = 已审批参数原文。
	if e.PayloadJSON == "" || e.PayloadJSON == "{}" {
		t.Fatalf("payload must preserve arguments: %q", e.PayloadJSON)
	}

	// ToolCall 未被 Confirm(仍 executing,等待 Dispatcher)。
	tc, _ := callRepo.GetByID(ctx, result.ToolCallID)
	if tc.Status != ToolCallStatusExecuting {
		t.Fatalf("tool call should stay executing, got %s", tc.Status)
	}
}

func TestServiceExecute_OutboxEnqueueFailureFailsToolCall(t *testing.T) {
	svc, callRepo, outbox := setupOutboxService(t)
	outbox.fail = true // Outbox 不可用 → 无法保证 Saga 语义 → ToolCall 终态 failed
	ctx := context.Background()

	result, err := svc.Execute(ctx, &ExecuteRequest{
		TenantID: "t1", RunID: "r1", AgentID: "a1", BusinessAppCode: "finance",
		ToolID: "erp_purchase_request", ToolVersion: "1.0",
		IdempotencyKey: "m3c-2", ConnectorBindingID: "bind-erp",
		ArgumentsJSON: `{"idempotency_key":"k-fail"}`,
	}, nil, nil)
	// Connector/Outbox 执行失败属 ToolCall 状态语义(已 Confirm failed),
	// Execute API 本身不因外部系统失败而报错。
	if err != nil {
		t.Fatalf("execute should not propagate connector error: %v", err)
	}
	if result.Status != ToolCallStatusFailed {
		t.Fatalf("result status = %s, want failed", result.Status)
	}

	tc, _ := callRepo.GetByID(ctx, result.ToolCallID)
	if tc.Status != ToolCallStatusFailed {
		t.Fatalf("tool call should fail when outbox unavailable, got %s", tc.Status)
	}
}

func TestServiceExecute_NonOutboxConnectorNotDeflected(t *testing.T) {
	// 未实现 OutboxConnector 的 Connector(如 db_read)不受分流影响:
	// ShouldDeferToOutbox=false → 走 M3-A 直连路径(此处无 CredentialService,
	// 直连会失败落 Confirm failed,验证"未入队且不 panic"即可)。
	svc, callRepo, outbox := setupOutboxService(t)
	svc.toolRepo = &fakeToolRepo{tools: map[string]*Tool{
		"enterprise_db_read": {ToolID: "enterprise_db_read", Version: "1.0", Domain: "shared", RiskLevel: "low", Status: "active", IsShared: true},
	}}
	svc.connectorRT.connectors["db_read_connector"] = NewMockDBReadConnector()
	svc.bindingRepo = &fakeBindingValidator{bindings: map[string]*ConnectorBinding{
		"bind-db": {ID: "bind-db", TenantID: "t1", Status: "active", ConnectorCode: "db_read_connector"},
	}}

	_, _ = svc.Execute(context.Background(), &ExecuteRequest{
		TenantID: "t1", RunID: "r1", AgentID: "a1", BusinessAppCode: "finance",
		ToolID: "enterprise_db_read", ToolVersion: "1.0",
		IdempotencyKey: "m3c-3", ConnectorBindingID: "bind-db",
		ArgumentsJSON: `{"query_template":"list_accounts"}`,
	}, nil, nil)

	if len(outbox.entries) != 0 {
		t.Fatalf("non-outbox connector must not enqueue, got %d entries", len(outbox.entries))
	}
	_ = callRepo
}
