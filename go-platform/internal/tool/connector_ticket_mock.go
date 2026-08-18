package tool

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ── M3-B: Mock Ticket Connector — ticket_create_or_update ──
//
// 演示工单 Connector 的完整契约实现,重点覆盖 M3-B 三大语义:
//
//	1. 幂等:idempotency_key 重复 create 返回已有工单,不重复创建;
//	2. 并发控制:expected_version 乐观锁(ETag 语义),不匹配返回
//	   VERSION_CONFLICT(retryable),防止覆盖人工修改;
//	3. 状态验证:closed 工单拒绝 update(状态机只前进不后退)。
//
// 状态变更同步生成 Webhook 事件(模拟外部系统回调环回),
// 供 Webhook Inbox 消费演示与测试。

// ticketStatusOrder 工单状态严格前进序(open → resolved → closed)。
var ticketStatusOrder = map[string]int{
	"open":     1,
	"pending":  2,
	"resolved": 3,
	"closed":   4,
}

// mockTicket Mock 工单存储记录。
type mockTicket struct {
	TicketID        string
	Version         int
	Status          string
	Title           string
	Priority        string
	IdempotencyKeys map[string]bool // create 幂等键集合
	UpdatedAt       time.Time
}

// TicketWebhookEvent 模拟外部系统推送的 Webhook 事件(不可信数据)。
type TicketWebhookEvent struct {
	ExternalEventID string         `json:"external_event_id"`
	TicketID        string         `json:"ticket_id"`
	Status          string         `json:"status"`
	Version         int            `json:"version"`
	OccurredAt      time.Time      `json:"occurred_at"`
	Extra           map[string]any `json:"extra,omitempty"`
}

// MockTicketConnector ticket_connector 的 Mock 实现。
// release_stage=human_approved_write:write 能力放行的最低注册阶段。
type MockTicketConnector struct {
	manifest ConnectorManifest

	mu      sync.Mutex
	tickets map[string]*mockTicket
	// webhookEvents 状态变更生成的事件,待外部环回推送(DrainWebhookEvents 取走)。
	webhookEvents []TicketWebhookEvent
}

// NewMockTicketConnector 创建 Mock 工单 Connector。
func NewMockTicketConnector() *MockTicketConnector {
	return &MockTicketConnector{
		manifest: ConnectorManifest{
			ConnectorCode: "ticket_connector",
			Version:       "1.0.0",
			ConnectorType: "ticket",
			AuthType:      "none",
			ReleaseStage:  "human_approved_write",
		},
		tickets: make(map[string]*mockTicket),
	}
}

// Manifest 返回不可变标识(与 migration 022 种子数据一致)。
func (m *MockTicketConnector) Manifest() ConnectorManifest { return m.manifest }

// Capabilities 写能力:ticket_create_or_update(受发布阶段门禁)。
func (m *MockTicketConnector) Capabilities() []ConnectorCapability {
	return []ConnectorCapability{
		{
			Name: "ticket_create_or_update",
			Kind: CapabilityKindWrite,
			InputSchema: map[string]any{
				"action":           "create | update",
				"ticket_id":        "string (update required)",
				"idempotency_key":  "string (create dedup, required)",
				"title":            "string",
				"priority":         "low | medium | high | urgent",
				"status":           "open | pending | resolved | closed",
				"expected_version": "integer (update optimistic lock)",
			},
			OutputSchema: map[string]any{
				"ticket_id":     "string",
				"version":       "integer",
				"status":        "string",
				"deduplicated":  "boolean",
			},
		},
	}
}

// HealthCheck Mock 恒健康(真实实现探测工单系统 API 可达性)。
func (m *MockTicketConnector) HealthCheck(ctx context.Context) ConnectorHealth {
	return ConnectorHealth{Healthy: true, Detail: "mock", CheckedAt: time.Now().UTC()}
}

// Execute 幂等创建 + 乐观锁更新 + 状态机验证。
func (m *MockTicketConnector) Execute(ctx context.Context, req *ConnectorRequest) (*ConnectorResult, error) {
	if req.Capability != "ticket_create_or_update" {
		return contractViolation(fmt.Sprintf("unsupported capability '%s'", req.Capability)), nil
	}
	action, _ := req.Input["action"].(string)
	switch action {
	case "create":
		return m.executeCreate(req)
	case "update":
		return m.executeUpdate(req)
	default:
		return &ConnectorResult{
			Status: ConnectorStatusFailed,
			Error: &ConnectorError{
				Code:    "INVALID_ACTION",
				Message: fmt.Sprintf("action must be create or update (got '%s')", action),
			},
		}, nil
	}
}

// executeCreate 幂等创建:同 idempotency_key 返回已有工单(deduplicated=true)。
func (m *MockTicketConnector) executeCreate(req *ConnectorRequest) (*ConnectorResult, error) {
	idemKey, _ := req.Input["idempotency_key"].(string)
	if idemKey == "" {
		return &ConnectorResult{
			Status: ConnectorStatusFailed,
			Error:  &ConnectorError{Code: "MISSING_IDEMPOTENCY_KEY", Message: "create requires idempotency_key"},
		}, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 幂等:同 key 已创建过 → 返回已有工单,不产生新副作用。
	for _, t := range m.tickets {
		if t.IdempotencyKeys[idemKey] {
			return &ConnectorResult{
				Status:            ConnectorStatusSucceeded,
				Output:            ticketOutput(t, true),
				ExternalObjectID:  t.TicketID,
				ExternalRequestID: fmt.Sprintf("ticket-create-%s", idemKey),
			}, nil
		}
	}

	ticketID := fmt.Sprintf("TICKET-%d", time.Now().UTC().UnixNano())
	title, _ := req.Input["title"].(string)
	priority, _ := req.Input["priority"].(string)
	if priority == "" {
		priority = "medium"
	}
	t := &mockTicket{
		TicketID:        ticketID,
		Version:         1,
		Status:          "open",
		Title:           title,
		Priority:        priority,
		IdempotencyKeys: map[string]bool{idemKey: true},
		UpdatedAt:       time.Now().UTC(),
	}
	m.tickets[ticketID] = t
	m.emitEventLocked(t)

	return &ConnectorResult{
		Status:            ConnectorStatusSucceeded,
		Output:            ticketOutput(t, false),
		ExternalObjectID:  t.TicketID,
		ExternalRequestID: fmt.Sprintf("ticket-create-%s", idemKey),
	}, nil
}

// executeUpdate 乐观锁更新:expected_version 不匹配 → 冲突;
// 状态只前进不后退;closed 终态拒绝更新。
func (m *MockTicketConnector) executeUpdate(req *ConnectorRequest) (*ConnectorResult, error) {
	ticketID, _ := req.Input["ticket_id"].(string)
	if ticketID == "" {
		return &ConnectorResult{
			Status: ConnectorStatusFailed,
			Error:  &ConnectorError{Code: "MISSING_TICKET_ID", Message: "update requires ticket_id"},
		}, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	t, ok := m.tickets[ticketID]
	if !ok {
		return &ConnectorResult{
			Status: ConnectorStatusFailed,
			Error: &ConnectorError{
				Code: "TICKET_NOT_FOUND", Message: fmt.Sprintf("ticket '%s' does not exist", ticketID),
			},
		}, nil
	}

	// 并发控制:expected_version 必须等于当前版本(ETag 语义)。
	expectedVer, hasVer := intInput(req.Input["expected_version"])
	if hasVer && expectedVer != t.Version {
		return &ConnectorResult{
			Status: ConnectorStatusFailed,
			Error: &ConnectorError{
				Code: "VERSION_CONFLICT",
				Message: fmt.Sprintf("expected_version=%d but current=%d (ticket modified concurrently)",
					expectedVer, t.Version),
				Retryable: true, // 乐观锁冲突可重试(重新读取后重试)
			},
		}, nil
	}

	// 状态验证:closed 为终态,拒绝任何更新。
	if t.Status == "closed" {
		return &ConnectorResult{
			Status: ConnectorStatusFailed,
			Error: &ConnectorError{
				Code: "TICKET_ALREADY_CLOSED", Message: fmt.Sprintf("ticket '%s' is closed; terminal state rejects update", ticketID),
			},
		}, nil
	}

	newStatus, _ := req.Input["status"].(string)
	if newStatus != "" {
		cur, okCur := ticketStatusOrder[t.Status]
		next, okNext := ticketStatusOrder[newStatus]
		if !okNext {
			return &ConnectorResult{
				Status: ConnectorStatusFailed,
				Error: &ConnectorError{
					Code: "INVALID_STATUS", Message: fmt.Sprintf("unknown ticket status '%s'", newStatus),
				},
			}, nil
		}
		// 状态机只前进:乱序到达的旧状态事件不回退(resolved 不退回 open)。
		if okCur && next < cur {
			return &ConnectorResult{
				Status: ConnectorStatusFailed,
				Error: &ConnectorError{
					Code: "STATUS_REGRESSION",
					Message: fmt.Sprintf("status %s → %s is a regression (state machine only advances)",
						t.Status, newStatus),
				},
			}, nil
		}
		t.Status = newStatus
	}
	if title, _ := req.Input["title"].(string); title != "" {
		t.Title = title
	}
	if priority, _ := req.Input["priority"].(string); priority != "" {
		t.Priority = priority
	}
	t.Version++
	t.UpdatedAt = time.Now().UTC()
	m.emitEventLocked(t)

	return &ConnectorResult{
		Status:            ConnectorStatusSucceeded,
		Output:            ticketOutput(t, false),
		ExternalObjectID:  t.TicketID,
		ExternalRequestID: fmt.Sprintf("ticket-update-%s-v%d", t.TicketID, t.Version),
	}, nil
}

// Verify 核验工单外部状态(写类必选)。
func (m *MockTicketConnector) Verify(ctx context.Context, req *ConnectorVerifyRequest) (*ConnectorVerifyResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// ExternalObjectID 即 ticket_id;缺失时按 tool_call 查证视为未确认。
	if req.ExternalObjectID == "" {
		return &ConnectorVerifyResult{Confirmed: false, Detail: "no external_object_id to verify"}, nil
	}
	t, ok := m.tickets[req.ExternalObjectID]
	if !ok {
		return &ConnectorVerifyResult{Confirmed: false, Detail: "ticket not found"}, nil
	}
	return &ConnectorVerifyResult{
		Confirmed: true,
		Detail:    fmt.Sprintf("ticket exists: status=%s version=%d", t.Status, t.Version),
		Observed:  ticketOutput(t, false),
	}, nil
}

// DrainWebhookEvents 取走累积的模拟 Webhook 事件(测试/演示环回推送用)。
func (m *MockTicketConnector) DrainWebhookEvents() []TicketWebhookEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	events := m.webhookEvents
	m.webhookEvents = nil
	return events
}

// GetTicket 读取工单快照(测试/Reconcile 观测用)。
func (m *MockTicketConnector) GetTicket(ticketID string) (map[string]any, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tickets[ticketID]
	if !ok {
		return nil, false
	}
	return ticketOutput(t, false), true
}

// emitEventLocked 状态变更生成事件(调用方持锁)。
func (m *MockTicketConnector) emitEventLocked(t *mockTicket) {
	m.webhookEvents = append(m.webhookEvents, TicketWebhookEvent{
		ExternalEventID: fmt.Sprintf("evt-%s-v%d", t.TicketID, t.Version),
		TicketID:        t.TicketID,
		Status:          t.Status,
		Version:         t.Version,
		OccurredAt:      t.UpdatedAt,
	})
}

func ticketOutput(t *mockTicket, deduplicated bool) map[string]any {
	return map[string]any{
		"ticket_id":    t.TicketID,
		"version":      t.Version,
		"status":       t.Status,
		"title":        t.Title,
		"priority":     t.Priority,
		"deduplicated": deduplicated,
		"updated_at":   t.UpdatedAt,
	}
}

// intInput 兼容 JSON 数字反序列化为 float64 的情况。
func intInput(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	default:
		return 0, false
	}
}
