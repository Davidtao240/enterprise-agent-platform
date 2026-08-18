package tool

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// ── M3-B: Webhook Inbox 测试 ──
// 覆盖 Gate:重复、乱序、限流(失败)可恢复。

// fakeWebhookStore 内存实现 webhookStore 接口。
type fakeWebhookStore struct {
	mu     sync.Mutex
	events map[string]*WebhookEvent
	calls  struct{ markErr, markOK int }
}

func newFakeWebhookStore() *fakeWebhookStore {
	return &fakeWebhookStore{events: make(map[string]*WebhookEvent)}
}

func (f *fakeWebhookStore) ListUnprocessed(ctx context.Context, limit int) ([]*WebhookEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []*WebhookEvent
	for _, e := range f.events {
		if e.ProcessedAt == nil && e.SignatureValid {
			result = append(result, e)
		}
	}
	// 按 received_at 排序保证顺序稳定。
	for i := 1; i < len(result); i++ {
		for j := i; j > 0 && result[j].ReceivedAt.Before(result[j-1].ReceivedAt); j-- {
			result[j], result[j-1] = result[j-1], result[j]
		}
	}
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (f *fakeWebhookStore) MarkProcessed(ctx context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.events[id]
	if !ok {
		return errors.New("not found")
	}
	now := time.Now().UTC()
	e.ProcessedAt = &now
	e.ProcessError = nil
	f.calls.markOK++
	return nil
}

func (f *fakeWebhookStore) MarkProcessError(ctx context.Context, id string, processErr string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.events[id]
	if !ok {
		return errors.New("not found")
	}
	e.ProcessError = &processErr
	f.calls.markErr++
	return nil
}

func (f *fakeWebhookStore) add(e *WebhookEvent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events[e.ID] = e
}

// ── 签名校验 ──

func TestVerifyWebhookSignature(t *testing.T) {
	body := []byte(`{"ticket_id":"T1","status":"pending"}`)
	secret := "whsec-test"

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	validSig := hex.EncodeToString(mac.Sum(nil))

	if !verifyWebhookSignature(body, validSig, secret) {
		t.Fatalf("valid signature rejected")
	}
	if verifyWebhookSignature(body, validSig, "wrong-secret") {
		t.Fatalf("signature with wrong secret accepted")
	}
	if verifyWebhookSignature(body, "deadbeef", secret) {
		t.Fatalf("garbage signature accepted")
	}
	if verifyWebhookSignature(body, "", secret) {
		t.Fatalf("empty signature accepted")
	}
	// 篡改 body 后原签名失效。
	if verifyWebhookSignature([]byte(`{"tampered":true}`), validSig, secret) {
		t.Fatalf("tampered body accepted")
	}
}

// ── 乱序恢复:迟到事件不回退状态 ──

func TestTicketProcessor_OutOfOrderStaleDiscarded(t *testing.T) {
	tickets := NewMockTicketConnector()
	ctx := context.Background()

	// 本地已推进到 v3(两次 update 后)。
	created, _ := tickets.Execute(ctx, ticketCreateReq("idem-oo"))
	ticketID, _ := created.Output["ticket_id"].(string)
	tickets.Execute(ctx, &ConnectorRequest{Capability: "ticket_create_or_update",
		Input: map[string]any{"action": "update", "ticket_id": ticketID, "status": "pending"}})
	tickets.Execute(ctx, &ConnectorRequest{Capability: "ticket_create_or_update",
		Input: map[string]any{"action": "update", "ticket_id": ticketID, "status": "resolved"}})

	local, _ := tickets.GetTicket(ticketID)
	localUpdated := local["updated_at"].(time.Time)

	p := NewTicketWebhookProcessor(tickets)

	// 迟到的旧事件:occurred_at 早于本地 updated_at → stale 丢弃,状态保持 resolved。
	stalePayload, _ := json.Marshal(map[string]any{
		"external_event_id": "evt-stale",
		"ticket_id":         ticketID,
		"status":            "open",
		"version":           2,
		"occurred_at":       localUpdated.Add(-2 * time.Second),
	})
	if err := p.ProcessWebhookEvent(ctx, &WebhookEvent{
		ID: "we-1", ConnectorCode: "ticket_connector", ExternalEventID: "evt-stale",
		SignatureValid: true, PayloadJSON: string(stalePayload),
	}); err != nil {
		t.Fatalf("stale event should be discarded without error: %v", err)
	}

	final, _ := tickets.GetTicket(ticketID)
	if final["status"] != "resolved" {
		t.Fatalf("stale event regressed status: %v", final["status"])
	}

	// 正常事件(version=4,期望当前 3)→ 同步成功 v4。
	freshPayload, _ := json.Marshal(map[string]any{
		"external_event_id": "evt-fresh",
		"ticket_id":         ticketID,
		"status":            "closed",
		"version":           4,
		"occurred_at":       time.Now().UTC(),
	})
	if err := p.ProcessWebhookEvent(ctx, &WebhookEvent{
		ID: "we-2", ConnectorCode: "ticket_connector", ExternalEventID: "evt-fresh",
		SignatureValid: true, PayloadJSON: string(freshPayload),
	}); err != nil {
		t.Fatalf("fresh event processing failed: %v", err)
	}
	synced, _ := tickets.GetTicket(ticketID)
	if synced["status"] != "closed" || synced["version"] != 4 {
		t.Fatalf("fresh event not synced: %+v", synced)
	}
}

func TestTicketProcessor_VersionConflictDropped(t *testing.T) {
	tickets := NewMockTicketConnector()
	ctx := context.Background()

	created, _ := tickets.Execute(ctx, ticketCreateReq("idem-vc"))
	ticketID, _ := created.Output["ticket_id"].(string)

	p := NewTicketWebhookProcessor(tickets)

	// 事件 version=9(期望当前 8,实际 1):VERSION_CONFLICT → 丢弃不重试(nil)。
	payload, _ := json.Marshal(map[string]any{
		"external_event_id": "evt-conflict",
		"ticket_id":         ticketID,
		"status":            "pending",
		"version":           9,
		"occurred_at":       time.Now().UTC().Add(time.Hour), // 未来时间绕过 stale 判定
	})
	if err := p.ProcessWebhookEvent(ctx, &WebhookEvent{
		ID: "we-3", ConnectorCode: "ticket_connector", ExternalEventID: "evt-conflict",
		SignatureValid: true, PayloadJSON: string(payload),
	}); err != nil {
		t.Fatalf("version-conflict event should be dropped, not retried: %v", err)
	}
}

func TestTicketProcessor_MalformedPayloadDiscarded(t *testing.T) {
	p := NewTicketWebhookProcessor(NewMockTicketConnector())
	if err := p.ProcessWebhookEvent(context.Background(), &WebhookEvent{
		ID: "we-4", PayloadJSON: "not-json{", SignatureValid: true,
	}); err != nil {
		t.Fatalf("malformed payload should be discarded silently: %v", err)
	}
}

// ── Consumer:重复只处理一次 + 失败重试可恢复 ──

// flakyProcessor 前 N 次失败,之后成功(模拟限流恢复)。
type flakyProcessor struct {
	mu       sync.Mutex
	failN    int
	attempts int
}

func (p *flakyProcessor) ProcessWebhookEvent(ctx context.Context, e *WebhookEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.attempts++
	if p.attempts <= p.failN {
		return fmt.Errorf("rate limited (attempt %d)", p.attempts)
	}
	return nil
}

func TestWebhookConsumer_RetryUntilRecover(t *testing.T) {
	store := newFakeWebhookStore()
	proc := &flakyProcessor{failN: 2} // 前 2 次"限流",第 3 次恢复

	c := NewWebhookConsumer(store, time.Hour, 10) // 周期无关,手动 RunOnce
	c.RegisterProcessor("ticket_connector", proc)

	store.add(&WebhookEvent{
		ID: "we-retry", ConnectorCode: "ticket_connector", ExternalEventID: "evt-retry",
		SignatureValid: true, PayloadJSON: "{}", ReceivedAt: time.Now().UTC(),
	})

	// 第 1 轮:失败 → process_error,保持待处理。
	if _, failed := c.RunOnce(context.Background()); failed != 1 {
		t.Fatalf("round 1: expected 1 failure, got %d", failed)
	}
	if store.calls.markErr != 1 {
		t.Fatalf("process_error should be recorded")
	}

	// 第 2 轮:仍失败(可恢复语义:不丢失)。
	c.RunOnce(context.Background())

	// 第 3 轮:恢复成功 → processed。
	processed, failed := c.RunOnce(context.Background())
	if processed != 1 || failed != 0 {
		t.Fatalf("round 3: expected recovery, got processed=%d failed=%d", processed, failed)
	}
	if proc.attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", proc.attempts)
	}

	// 第 4 轮:已 processed 的事件不再出现(重复保护)。
	processed, _ = c.RunOnce(context.Background())
	if processed != 0 {
		t.Fatalf("processed event should not be re-consumed")
	}
}

func TestWebhookConsumer_UnregisteredConnectorHeld(t *testing.T) {
	store := newFakeWebhookStore()
	c := NewWebhookConsumer(store, time.Hour, 10)

	store.add(&WebhookEvent{
		ID: "we-noproc", ConnectorCode: "unknown_connector", ExternalEventID: "evt-x",
		SignatureValid: true, PayloadJSON: "{}", ReceivedAt: time.Now().UTC(),
	})

	_, failed := c.RunOnce(context.Background())
	if failed != 1 {
		t.Fatalf("unregistered connector event should be held with error")
	}
}

func TestWebhookConsumer_SignatureInvalidSkipped(t *testing.T) {
	store := newFakeWebhookStore()
	proc := &flakyProcessor{}
	c := NewWebhookConsumer(store, time.Hour, 10)
	c.RegisterProcessor("ticket_connector", proc)

	// 签名无效的事件:落库但不进入消费(ListUnprocessed 过滤 signature_valid)。
	store.add(&WebhookEvent{
		ID: "we-badsig", ConnectorCode: "ticket_connector", ExternalEventID: "evt-bad",
		SignatureValid: false, PayloadJSON: "{}", ReceivedAt: time.Now().UTC(),
	})

	processed, failed := c.RunOnce(context.Background())
	if processed != 0 || failed != 0 || proc.attempts != 0 {
		t.Fatalf("invalid-signature event must never be processed")
	}
}
