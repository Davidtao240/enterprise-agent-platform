package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// ── M3-B: Webhook Consumer ──
//
// 周期消费 webhook_events Inbox,覆盖 M3-B Gate 三大语义:
//
//	1. 重复:唯一约束 + processed_at 幂等标记,重复投递只处理一次;
//	2. 乱序:事件 occurred_at 早于本地工单当前 updated_at → stale 丢弃
//	   (状态机只前进不后退,resolved 不被迟到的 open 事件回退);
//	3. 限流/失败可恢复:处理失败写 process_error 保留待处理状态,
//	   下一轮扫描自动重试,不丢失事件。
//
// payload 视为不可信数据:仅提取白名单字段,不解释为指令。

// WebhookProcessor 消费接口:按 connector 分派事件处理。
// 返回 error 表示处理失败(将重试);nil 表示完成(含 stale 丢弃)。
type WebhookProcessor interface {
	ProcessWebhookEvent(ctx context.Context, e *WebhookEvent) error
}

// webhookStore Consumer 依赖的仓储接口(便于测试替换)。
type webhookStore interface {
	ListUnprocessed(ctx context.Context, limit int) ([]*WebhookEvent, error)
	MarkProcessed(ctx context.Context, id string) error
	MarkProcessError(ctx context.Context, id string, processErr string) error
}

// WebhookConsumer 周期消费器。
type WebhookConsumer struct {
	store      webhookStore
	processors map[string]WebhookProcessor // connector_code → processor
	every      time.Duration
	batch      int
}

// NewWebhookConsumer 创建消费器。every<=0 用默认 10s,batch<=0 用默认 50。
func NewWebhookConsumer(store webhookStore, every time.Duration, batch int) *WebhookConsumer {
	if every <= 0 {
		every = 10 * time.Second
	}
	if batch <= 0 {
		batch = 50
	}
	return &WebhookConsumer{
		store:      store,
		processors: make(map[string]WebhookProcessor),
		every:      every,
		batch:      batch,
	}
}

// RegisterProcessor 注册 connector 的处理器。
func (c *WebhookConsumer) RegisterProcessor(connectorCode string, p WebhookProcessor) {
	c.processors[connectorCode] = p
}

// Start 阻塞循环消费(由 goroutine 启动)。
func (c *WebhookConsumer) Start(ctx context.Context) {
	ticker := time.NewTicker(c.every)
	defer ticker.Stop()
	log.Printf("[webhook-consumer] started (every=%s batch=%d)", c.every, c.batch)
	for {
		select {
		case <-ctx.Done():
			log.Println("[webhook-consumer] stopped")
			return
		case <-ticker.C:
			c.RunOnce(ctx)
		}
	}
}

// RunOnce 执行一轮消费(测试可直接调用),返回 (processed, failed)。
func (c *WebhookConsumer) RunOnce(ctx context.Context) (int, int) {
	events, err := c.store.ListUnprocessed(ctx, c.batch)
	if err != nil {
		log.Printf("[webhook-consumer] list unprocessed failed: %v", err)
		return 0, 0
	}
	processed, failed := 0, 0
	for _, e := range events {
		p, ok := c.processors[e.ConnectorCode]
		if !ok {
			// 未注册处理器的 connector:记录错误待人工介入(不吞事件)。
			if err := c.store.MarkProcessError(ctx, e.ID,
				fmt.Sprintf("no processor registered for connector '%s'", e.ConnectorCode)); err != nil {
				log.Printf("[webhook-consumer] mark error failed for %s: %v", e.ID, err)
			}
			failed++
			continue
		}
		if err := p.ProcessWebhookEvent(ctx, e); err != nil {
			if markErr := c.store.MarkProcessError(ctx, e.ID, err.Error()); markErr != nil {
				log.Printf("[webhook-consumer] mark error failed for %s: %v", e.ID, markErr)
			}
			failed++
			continue
		}
		if err := c.store.MarkProcessed(ctx, e.ID); err != nil {
			log.Printf("[webhook-consumer] mark processed failed for %s: %v", e.ID, err)
			continue
		}
		processed++
	}
	if len(events) > 0 {
		log.Printf("[webhook-consumer] cycle: scanned=%d processed=%d failed=%d", len(events), processed, failed)
	}
	return processed, failed
}

// ── Ticket Webhook Processor ──

// TicketWebhookProcessor 工单事件处理器:把外部状态变更同步到本地
// 工单镜像(Mock 阶段即 MockTicketConnector 的存储;真实实现写本地缓存表)。
//
// 乱序判定:payload.occurred_at 早于本地工单 updated_at → stale 丢弃
// (返回 nil,标记完成);payload 的 status 回退由状态机拒绝。
type TicketWebhookProcessor struct {
	tickets *MockTicketConnector
}

// NewTicketWebhookProcessor 创建工单事件处理器。
func NewTicketWebhookProcessor(tickets *MockTicketConnector) *TicketWebhookProcessor {
	return &TicketWebhookProcessor{tickets: tickets}
}

// ProcessWebhookEvent 白名单提取 payload 字段(不可信数据不透传)。
func (p *TicketWebhookProcessor) ProcessWebhookEvent(ctx context.Context, e *WebhookEvent) error {
	var payload struct {
		TicketID   string    `json:"ticket_id"`
		Status     string    `json:"status"`
		Version    int       `json:"version"`
		OccurredAt time.Time `json:"occurred_at"`
	}
	// 恶意/损坏 payload:不可恢复,丢弃不重试(Inbox 已留原始 payload 供审计)。
	if err := json.Unmarshal([]byte(e.PayloadJSON), &payload); err != nil {
		return nil
	}
	if payload.TicketID == "" {
		return nil // 无关联工单,丢弃
	}

	local, ok := p.tickets.GetTicket(payload.TicketID)
	if !ok {
		// 本地无此工单(系统外部创建):Mock 阶段仅记录,不报错重试。
		log.Printf("[webhook-ticket] event %s references unknown ticket %s; discarded", e.ExternalEventID, payload.TicketID)
		return nil
	}

	// 乱序恢复:事件早于本地已知状态 → stale 丢弃,不回退。
	if localUpdated, ok := local["updated_at"].(time.Time); ok &&
		!payload.OccurredAt.IsZero() && payload.OccurredAt.Before(localUpdated) {
		log.Printf("[webhook-ticket] stale event %s discarded (occurred_at=%s before local updated_at=%s)",
			e.ExternalEventID, payload.OccurredAt.Format(time.RFC3339), localUpdated.Format(time.RFC3339))
		return nil
	}

	// 同步外部状态:经 Execute 走完整乐观锁/状态机校验(非直接改库)。
	result, err := p.tickets.Execute(ctx, &ConnectorRequest{
		TenantID:   e.ConnectorCode, // webhook 无租户上下文,Mock 不校验
		ToolCallID: "webhook:" + e.ID,
		Capability: "ticket_create_or_update",
		Input: map[string]any{
			"action":           "update",
			"ticket_id":        payload.TicketID,
			"status":           payload.Status,
			"expected_version": float64(payload.Version - 1), // 事件携带目标版本,期望当前为 n-1
		},
	})
	if err != nil {
		return err // 连接类错误(真实实现),可重试
	}
	if result.Status != ConnectorStatusSucceeded && result.Error != nil {
		// 版本冲突/状态回退/终态拒绝:外部与本地不一致且不可由此事件修复,
		// 丢弃留痕,等待 Reconcile 周期对账或更新的事件到达。
		log.Printf("[webhook-ticket] event %s dropped (code=%s): %s",
			e.ExternalEventID, result.Error.Code, result.Error.Message)
	}
	return nil
}
