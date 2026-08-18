package tool

import (
	"context"
	"testing"
)

// ── M3-C: Mock ERP Connector 契约测试 ──
// 覆盖:Saga 幂等创建、Dry-run 预检、补偿边界(仅 pending 可撤)、
// Verify、OutboxConnector 契约(UseOutbox/BuildCompensation)、发布阶段门禁。

func erpRequestReq(idemKey string) *ConnectorRequest {
	return &ConnectorRequest{
		TenantID:   "t-1",
		ToolCallID: "tc-erp",
		Capability: "erp_purchase_request",
		Input: map[string]any{
			"idempotency_key": idemKey,
			"title":           "采购服务器",
			"supplier":        "供应商A",
			"total_amount":    98000.0,
		},
	}
}

func TestERPRequest_Idempotent(t *testing.T) {
	c := NewMockERPConnector()
	ctx := context.Background()

	first, err := c.Execute(ctx, erpRequestReq("idem-erp-1"))
	if err != nil || first.Status != ConnectorStatusSucceeded {
		t.Fatalf("first request failed: %v %+v", err, first)
	}
	prID := first.ExternalObjectID

	// 至少一次投递场景:重复同幂等键返回同一单据,不重复创建。
	second, err := c.Execute(ctx, erpRequestReq("idem-erp-1"))
	if err != nil || second.Status != ConnectorStatusSucceeded {
		t.Fatalf("replay failed: %v %+v", err, second)
	}
	if second.ExternalObjectID != prID {
		t.Fatalf("replay created new PR: %s != %s", second.ExternalObjectID, prID)
	}
	if dedup, _ := second.Output["deduplicated"].(bool); !dedup {
		t.Fatalf("replay should be marked deduplicated")
	}

	third, _ := c.Execute(ctx, erpRequestReq("idem-erp-2"))
	if third.ExternalObjectID == prID {
		t.Fatalf("different key should create new PR")
	}
}

func TestERPRequest_MissingIdempotencyKey(t *testing.T) {
	c := NewMockERPConnector()
	r, _ := c.Execute(context.Background(), erpRequestReq(""))
	if r.Status != ConnectorStatusFailed || r.Error.Code != "MISSING_IDEMPOTENCY_KEY" {
		t.Fatalf("expected MISSING_IDEMPOTENCY_KEY, got %+v", r)
	}
}

func TestERPPreview_DryRunNoSideEffect(t *testing.T) {
	c := NewMockERPConnector()
	r, err := c.Execute(context.Background(), &ConnectorRequest{
		Capability: "erp_purchase_request_preview",
		Input: map[string]any{
			"idempotency_key": "idem-prev", "title": "采购显示器",
			"total_amount": 5000.0,
		},
	})
	if err != nil || r.Status != ConnectorStatusSucceeded {
		t.Fatalf("preview failed: %v %+v", err, r)
	}
	if would, _ := r.Output["would_create"].(bool); !would {
		t.Fatalf("valid input should would_create=true: %+v", r.Output)
	}
	// Dry-run 无副作用:不产生外部单据号。
	if r.ExternalObjectID != "" {
		t.Fatalf("preview must not produce external object id")
	}

	// 非法参数预检。
	bad, _ := c.Execute(context.Background(), &ConnectorRequest{
		Capability: "erp_purchase_request_preview",
		Input:      map[string]any{"total_amount": -1.0},
	})
	if would, _ := bad.Output["would_create"].(bool); would {
		t.Fatalf("invalid input should would_create=false: %+v", bad.Output)
	}
}

func TestERPCancel_OnlyPending(t *testing.T) {
	c := NewMockERPConnector()
	ctx := context.Background()

	created, _ := c.Execute(ctx, erpRequestReq("idem-cancel"))
	prID := created.ExternalObjectID

	// pending 可撤(补偿成功)。
	ok, _ := c.Execute(ctx, &ConnectorRequest{
		Capability: "erp_purchase_cancel",
		Input:      map[string]any{"purchase_request_id": prID, "reason": "outbox compensation"},
	})
	if ok.Status != ConnectorStatusSucceeded {
		t.Fatalf("cancel pending failed: %+v", ok)
	}

	// 重复取消幂等。
	dup, _ := c.Execute(ctx, &ConnectorRequest{
		Capability: "erp_purchase_cancel",
		Input:      map[string]any{"purchase_request_id": prID},
	})
	if dup.Status != ConnectorStatusSucceeded {
		t.Fatalf("repeat cancel should be idempotent: %+v", dup)
	}

	// 已审批单据不可撤(补偿边界)。
	created2, _ := c.Execute(ctx, erpRequestReq("idem-cancel2"))
	prID2 := created2.ExternalObjectID
	if !c.AdvancePurchaseRequest(prID2, "approved") {
		t.Fatalf("advance to approved failed")
	}
	rejected, _ := c.Execute(ctx, &ConnectorRequest{
		Capability: "erp_purchase_cancel",
		Input:      map[string]any{"purchase_request_id": prID2},
	})
	if rejected.Status != ConnectorStatusFailed || rejected.Error.Code != "CANCEL_NOT_ALLOWED" {
		t.Fatalf("expected CANCEL_NOT_ALLOWED, got %+v", rejected)
	}
}

func TestERPVerify(t *testing.T) {
	c := NewMockERPConnector()
	ctx := context.Background()

	created, _ := c.Execute(ctx, erpRequestReq("idem-verify"))
	prID := created.ExternalObjectID

	v, _ := c.Verify(ctx, &ConnectorVerifyRequest{ExternalObjectID: prID})
	if !v.Confirmed {
		t.Fatalf("verify existing PR should confirm: %+v", v)
	}
	missing, _ := c.Verify(ctx, &ConnectorVerifyRequest{ExternalObjectID: "PR-NOPE"})
	if missing.Confirmed {
		t.Fatalf("verify unknown PR must not confirm")
	}
}

func TestERPPostSendFailure_Indeterminate(t *testing.T) {
	c := NewMockERPConnector()
	c.InjectPostSendFailures(1)

	// 已创建单据但返回 indeterminate(部分成功)。
	r, err := c.Execute(context.Background(), erpRequestReq("idem-post"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Status != ConnectorStatusIndeterminate || r.ExternalObjectID == "" {
		t.Fatalf("expected indeterminate with object id, got %+v", r)
	}
	// 副作用实际已发生(Verify 可确认)→ stale Verify 收敛路径可行。
	v, _ := c.Verify(context.Background(), &ConnectorVerifyRequest{ExternalObjectID: r.ExternalObjectID})
	if !v.Confirmed {
		t.Fatalf("side effect should be verifiable")
	}

	// 幂等重放:同键再调返回 succeeded 同一单据(重投递安全)。
	replay, _ := c.Execute(context.Background(), erpRequestReq("idem-post"))
	if replay.Status != ConnectorStatusSucceeded || replay.ExternalObjectID != r.ExternalObjectID {
		t.Fatalf("replay after indeterminate should succeed with same PR: %+v", replay)
	}
}

func TestERPOutboxContract(t *testing.T) {
	c := NewMockERPConnector()

	// UseOutbox:仅 erp_purchase_request 经 Outbox(Saga);
	// preview(read)与 cancel(补偿,Dispatcher 直调)不经。
	if !c.UseOutbox("erp_purchase_request") {
		t.Fatalf("erp_purchase_request should use outbox")
	}
	if c.UseOutbox("erp_purchase_request_preview") {
		t.Fatalf("preview should not use outbox")
	}
	if c.UseOutbox("erp_purchase_cancel") {
		t.Fatalf("cancel should not use outbox (called directly by dispatcher)")
	}

	// BuildCompensation:有外部对象 → cancel 调用;无 → ok=false。
	prID := "PR-123"
	capability, input, ok := c.BuildCompensation(&OutboxEntry{
		ID: "ob-1", ExternalObjectID: &prID,
	}, nil)
	if !ok || capability != "erp_purchase_cancel" || input["purchase_request_id"] != prID {
		t.Fatalf("unexpected compensation: %s %v %v", capability, input, ok)
	}
	if _, _, ok := c.BuildCompensation(&OutboxEntry{ID: "ob-2"}, nil); ok {
		t.Fatalf("no object id → no compensation possible")
	}
}

func TestERPWrite_GatedByStage(t *testing.T) {
	// write 门禁矩阵:preview 任意放行,request/cancel 需双达标。
	caps := NewMockERPConnector().Capabilities()
	for _, tc := range []struct {
		capability     string
		stage, env     string
		allowed        bool
	}{
		{"erp_purchase_request_preview", "sandbox_readonly", "sandbox", true},
		{"erp_purchase_request", "sandbox_readonly", "sandbox", false},
		{"erp_purchase_request", "human_approved_write", "sandbox", false},
		{"erp_purchase_request", "human_approved_write", "shadow", true},
		{"erp_purchase_request", "production", "production", true},
		{"erp_purchase_cancel", "human_approved_write", "shadow", true},
		{"erp_purchase_cancel", "mock_fixture", "mock", false},
	} {
		err := CapabilityAllowed(tc.stage, tc.env, tc.capability, caps)
		if (err == nil) != tc.allowed {
			t.Fatalf("%s stage=%s env=%s: allowed=%v want %v (%v)",
				tc.capability, tc.stage, tc.env, err == nil, tc.allowed, err)
		}
	}
}

func TestRuntimeShouldDeferToOutbox(t *testing.T) {
	// 同包测试可直接构造私有字段(不依赖 registry DB)。
	erp := NewMockERPConnector()
	dbRead := NewMockDBReadConnector()
	rt := &ConnectorRuntime{connectors: map[string]Connector{
		"erp_connector":   erp,
		"db_read_connector": dbRead,
	}}

	if !rt.ShouldDeferToOutbox("erp_connector", "erp_purchase_request") {
		t.Fatalf("erp request should defer to outbox")
	}
	if rt.ShouldDeferToOutbox("erp_connector", "erp_purchase_cancel") {
		t.Fatalf("erp cancel is dispatched directly, not via outbox")
	}
	if rt.ShouldDeferToOutbox("db_read_connector", "enterprise_db_read") {
		t.Fatalf("db_read does not implement OutboxConnector")
	}
	if rt.ShouldDeferToOutbox("unknown", "x") {
		t.Fatalf("unknown connector should not defer")
	}

	if _, ok := rt.OutboxConnectorFor("erp_connector"); !ok {
		t.Fatalf("erp should expose OutboxConnector")
	}
	if _, ok := rt.OutboxConnectorFor("db_read_connector"); ok {
		t.Fatalf("db_read should not expose OutboxConnector")
	}
}
