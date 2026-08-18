package tool

import (
	"context"
	"testing"
)

// ── M3-B: Mock Ticket Connector 契约测试 ──
// 覆盖 M3-B 三大语义:幂等创建、乐观锁并发控制、状态机验证。

func ticketCreateReq(idemKey string) *ConnectorRequest {
	return &ConnectorRequest{
		TenantID:   "t-1",
		ToolCallID: "tc-1",
		Capability: "ticket_create_or_update",
		Input: map[string]any{
			"action":          "create",
			"idempotency_key": idemKey,
			"title":           "Sample ticket",
			"priority":        "high",
		},
	}
}

func TestTicketCreate_Idempotent(t *testing.T) {
	c := NewMockTicketConnector()
	ctx := context.Background()

	first, err := c.Execute(ctx, ticketCreateReq("idem-1"))
	if err != nil || first.Status != ConnectorStatusSucceeded {
		t.Fatalf("first create failed: %v %+v", err, first)
	}
	ticketID, _ := first.Output["ticket_id"].(string)

	// 重复同幂等键:返回同一工单,deduplicated=true,不产生新副作用。
	second, err := c.Execute(ctx, ticketCreateReq("idem-1"))
	if err != nil || second.Status != ConnectorStatusSucceeded {
		t.Fatalf("idempotent replay failed: %v %+v", err, second)
	}
	if got, _ := second.Output["ticket_id"].(string); got != ticketID {
		t.Fatalf("idempotent replay created new ticket: %s != %s", got, ticketID)
	}
	if dedup, _ := second.Output["deduplicated"].(bool); !dedup {
		t.Fatalf("idempotent replay should be marked deduplicated")
	}

	// 不同幂等键:新工单。
	third, _ := c.Execute(ctx, ticketCreateReq("idem-2"))
	if got, _ := third.Output["ticket_id"].(string); got == ticketID {
		t.Fatalf("different idempotency key should create new ticket")
	}
}

func TestTicketCreate_MissingIdempotencyKey(t *testing.T) {
	c := NewMockTicketConnector()
	req := ticketCreateReq("")
	req.Input["idempotency_key"] = ""
	result, err := c.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != ConnectorStatusFailed || result.Error.Code != "MISSING_IDEMPOTENCY_KEY" {
		t.Fatalf("expected MISSING_IDEMPOTENCY_KEY, got %+v", result)
	}
}

func TestTicketUpdate_OptimisticLock(t *testing.T) {
	c := NewMockTicketConnector()
	ctx := context.Background()

	created, _ := c.Execute(ctx, ticketCreateReq("idem-l"))
	ticketID, _ := created.Output["ticket_id"].(string)

	// 正确版本:成功,version+1。
	ok, _ := c.Execute(ctx, &ConnectorRequest{
		Capability: "ticket_create_or_update",
		Input: map[string]any{
			"action":           "update",
			"ticket_id":        ticketID,
			"status":           "pending",
			"expected_version": float64(1),
		},
	})
	if ok.Status != ConnectorStatusSucceeded {
		t.Fatalf("update with correct version failed: %+v", ok)
	}
	if v, _ := ok.Output["version"].(int); v != 2 {
		t.Fatalf("version should advance to 2, got %v", ok.Output["version"])
	}

	// 旧版本(并发被覆盖):VERSION_CONFLICT 且可重试。
	stale, _ := c.Execute(ctx, &ConnectorRequest{
		Capability: "ticket_create_or_update",
		Input: map[string]any{
			"action":           "update",
			"ticket_id":        ticketID,
			"status":           "resolved",
			"expected_version": float64(1), // 当前已是 2
		},
	})
	if stale.Status != ConnectorStatusFailed || stale.Error.Code != "VERSION_CONFLICT" {
		t.Fatalf("expected VERSION_CONFLICT, got %+v", stale)
	}
	if !stale.Error.Retryable {
		t.Fatalf("VERSION_CONFLICT should be retryable")
	}
}

func TestTicketUpdate_ClosedTerminal(t *testing.T) {
	c := NewMockTicketConnector()
	ctx := context.Background()

	created, _ := c.Execute(ctx, ticketCreateReq("idem-c"))
	ticketID, _ := created.Output["ticket_id"].(string)

	// open → resolved → closed(状态前进合法)。
	for _, status := range []string{"resolved", "closed"} {
		r, _ := c.Execute(ctx, &ConnectorRequest{
			Capability: "ticket_create_or_update",
			Input: map[string]any{
				"action":    "update",
				"ticket_id": ticketID,
				"status":    status,
			},
		})
		if r.Status != ConnectorStatusSucceeded {
			t.Fatalf("transition to %s failed: %+v", status, r)
		}
	}

	// closed 后任何更新被拒绝(终态验证)。
	after, _ := c.Execute(ctx, &ConnectorRequest{
		Capability: "ticket_create_or_update",
		Input: map[string]any{
			"action":    "update",
			"ticket_id": ticketID,
			"title":     "should be rejected",
		},
	})
	if after.Status != ConnectorStatusFailed || after.Error.Code != "TICKET_ALREADY_CLOSED" {
		t.Fatalf("expected TICKET_ALREADY_CLOSED, got %+v", after)
	}
}

func TestTicketUpdate_StatusRegressionRejected(t *testing.T) {
	c := NewMockTicketConnector()
	ctx := context.Background()

	created, _ := c.Execute(ctx, ticketCreateReq("idem-r"))
	ticketID, _ := created.Output["ticket_id"].(string)

	// 前进到 resolved。
	if r, _ := c.Execute(ctx, &ConnectorRequest{Capability: "ticket_create_or_update",
		Input: map[string]any{"action": "update", "ticket_id": ticketID, "status": "resolved"}}); r.Status != ConnectorStatusSucceeded {
		t.Fatalf("advance failed: %+v", r)
	}

	// 乱序回退 resolved → open:状态机拒绝。
	regression, _ := c.Execute(ctx, &ConnectorRequest{Capability: "ticket_create_or_update",
		Input: map[string]any{"action": "update", "ticket_id": ticketID, "status": "open"}})
	if regression.Status != ConnectorStatusFailed || regression.Error.Code != "STATUS_REGRESSION" {
		t.Fatalf("expected STATUS_REGRESSION, got %+v", regression)
	}
}

func TestTicketVerify(t *testing.T) {
	c := NewMockTicketConnector()
	ctx := context.Background()

	created, _ := c.Execute(ctx, ticketCreateReq("idem-v"))
	ticketID, _ := created.Output["ticket_id"].(string)

	v, err := c.Verify(ctx, &ConnectorVerifyRequest{ExternalObjectID: ticketID})
	if err != nil || !v.Confirmed {
		t.Fatalf("verify existing ticket failed: %+v %v", v, err)
	}

	missing, _ := c.Verify(ctx, &ConnectorVerifyRequest{ExternalObjectID: "NOPE"})
	if missing.Confirmed {
		t.Fatalf("verify unknown ticket should not confirm")
	}
}

func TestTicketWebhookEventEmitted(t *testing.T) {
	c := NewMockTicketConnector()
	ctx := context.Background()

	created, _ := c.Execute(ctx, ticketCreateReq("idem-e"))
	ticketID, _ := created.Output["ticket_id"].(string)
	c.Execute(ctx, &ConnectorRequest{Capability: "ticket_create_or_update",
		Input: map[string]any{"action": "update", "ticket_id": ticketID, "status": "pending"}})

	events := c.DrainWebhookEvents()
	if len(events) != 2 { // create + update
		t.Fatalf("expected 2 webhook events, got %d", len(events))
	}
	last := events[1]
	if last.TicketID != ticketID || last.Status != "pending" || last.Version != 2 {
		t.Fatalf("unexpected last event: %+v", last)
	}
	if drained := c.DrainWebhookEvents(); len(drained) != 0 {
		t.Fatalf("drain should clear events")
	}
}

func TestTicketCapability_GatedByWrite(t *testing.T) {
	// ticket_create_or_update 是 write 能力:CapabilityAllowed 门禁矩阵验证。
	m := NewMockTicketConnector().Manifest()
	caps := NewMockTicketConnector().Capabilities()
	for _, tc := range []struct {
		stage, env string
		allowed    bool
	}{
		{"mock_fixture", "mock", false},           // mock 阶段拒绝写
		{"sandbox_readonly", "sandbox", false},    // sandbox 阶段拒绝写
		{"human_approved_write", "mock", false},   // 注册达标但环境不足
		{"human_approved_write", "shadow", true},  // 双达标放行
		{"production", "production", true},
	} {
		err := CapabilityAllowed(tc.stage, tc.env, "ticket_create_or_update", caps)
		if (err == nil) != tc.allowed {
			t.Fatalf("stage=%s env=%s: allowed=%v want %v (err=%v)", tc.stage, tc.env, err == nil, tc.allowed, err)
		}
	}
	_ = m
}
