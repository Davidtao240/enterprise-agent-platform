package tool

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ── M3-C: Mock ERP Connector — erp_purchase_request Saga 演示 ──
//
// 覆盖 M3-C Gate 语义:
//
//	1. 幂等:idempotency_key 重复 request 返回同一采购申请(至少一次投递
//	   + 供应商幂等 = 恰好一次副作用);
//	2. Dry-run 优先:erp_purchase_request_preview(read)预检参数、返回
//	   模拟单据,无外部副作用,任意发布阶段放行;
//	3. Compensation:erp_purchase_cancel 仅能撤销 pending 状态的申请
//	   (已审批/已发货不可撤 → 需走真实业务流程),实现 OutboxConnector。
//
// 采购申请状态机:pending → approved → shipped → completed
// (cancelled 仅可从 pending 迁入;补偿语义 = 撤销未审批单据)。

// erpPRStatusOrder 采购申请状态前进序。
var erpPRStatusOrder = map[string]int{
	"pending":   1,
	"approved":  2,
	"shipped":   3,
	"completed": 4,
}

// mockPurchaseRequest Mock ERP 采购申请记录。
type mockPurchaseRequest struct {
	PRID             string
	Status           string
	Version          int
	Title            string
	Supplier         string
	TotalAmount      float64
	IdempotencyKeys  map[string]bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// MockERPConnector erp_connector 的 Mock 实现(release_stage=human_approved_write)。
type MockERPConnector struct {
	manifest ConnectorManifest

	mu  sync.Mutex
	prs map[string]*mockPurchaseRequest

	// 故障注入(测试/Sandbox 演示):接下来 N 次对应模式调用受影响。
	failSend     int // request 调用返回可重试 error(模拟网络失败)
	failPostSend int // request 调用已创建单据但返回 indeterminate(模拟超时)
}

// NewMockERPConnector 创建 Mock ERP Connector。
func NewMockERPConnector() *MockERPConnector {
	return &MockERPConnector{
		manifest: ConnectorManifest{
			ConnectorCode: "erp_connector",
			Version:       "1.0.0",
			ConnectorType: "erp",
			AuthType:      "none",
			ReleaseStage:  "human_approved_write",
		},
		prs: make(map[string]*mockPurchaseRequest),
	}
}

// Manifest 不可变标识(与 migration 023 种子一致)。
func (m *MockERPConnector) Manifest() ConnectorManifest { return m.manifest }

// Capabilities preview(read)/request(write)/cancel(write)。
func (m *MockERPConnector) Capabilities() []ConnectorCapability {
	return []ConnectorCapability{
		{
			Name: "erp_purchase_request_preview",
			Kind: CapabilityKindRead,
			InputSchema: map[string]any{
				"idempotency_key": "string", "title": "string",
				"items": "array", "total_amount": "number", "supplier": "string",
			},
			OutputSchema: map[string]any{
				"would_create": "boolean", "estimated_pr": "string", "validations": "array",
			},
		},
		{
			Name: "erp_purchase_request",
			Kind: CapabilityKindWrite,
			InputSchema: map[string]any{
				"idempotency_key": "string (required, dedup)", "title": "string",
				"items": "array", "total_amount": "number", "supplier": "string",
			},
			OutputSchema: map[string]any{
				"purchase_request_id": "string", "status": "string", "version": "integer",
			},
		},
		{
			Name: "erp_purchase_cancel",
			Kind: CapabilityKindWrite,
			InputSchema: map[string]any{
				"purchase_request_id": "string (required)", "reason": "string",
			},
			OutputSchema: map[string]any{"cancelled": "boolean", "status": "string"},
		},
	}
}

// HealthCheck Mock 恒健康。
func (m *MockERPConnector) HealthCheck(ctx context.Context) ConnectorHealth {
	return ConnectorHealth{Healthy: true, Detail: "mock", CheckedAt: time.Now().UTC()}
}

// Execute 按 capability 分派。
func (m *MockERPConnector) Execute(ctx context.Context, req *ConnectorRequest) (*ConnectorResult, error) {
	switch req.Capability {
	case "erp_purchase_request_preview":
		return m.executePreview(req)
	case "erp_purchase_request":
		return m.executeRequest(req)
	case "erp_purchase_cancel":
		return m.executeCancel(req)
	default:
		return contractViolation(fmt.Sprintf("unsupported capability '%s'", req.Capability)), nil
	}
}

// executePreview Dry-run 预检:参数校验 + 模拟单据号,无副作用。
func (m *MockERPConnector) executePreview(req *ConnectorRequest) (*ConnectorResult, error) {
	validations := []string{}
	if _, ok := req.Input["idempotency_key"].(string); !ok {
		validations = append(validations, "idempotency_key required")
	}
	if _, ok := req.Input["title"].(string); !ok {
		validations = append(validations, "title required")
	}
	if amt, ok := req.Input["total_amount"].(float64); ok && amt <= 0 {
		validations = append(validations, "total_amount must be positive")
	}
	return &ConnectorResult{
		Status: ConnectorStatusSucceeded,
		Output: map[string]any{
			"would_create": len(validations) == 0,
			"estimated_pr": fmt.Sprintf("PR-PREVIEW-%s", req.Input["idempotency_key"]),
			"validations":  validations,
		},
	}, nil
}

// executeRequest 幂等创建采购申请。
// 故障注入语义:
//   - failSend:    返回 retryable error(单据未创建)→ Dispatcher 退避重试;
//   - failPostSend:单据已创建但返回 indeterminate → Dispatcher 转 sent,
//     由 stale Verify 收敛(部分成功可恢复)。
func (m *MockERPConnector) executeRequest(req *ConnectorRequest) (*ConnectorResult, error) {
	idemKey, _ := req.Input["idempotency_key"].(string)
	if idemKey == "" {
		return &ConnectorResult{
			Status: ConnectorStatusFailed,
			Error:  &ConnectorError{Code: "MISSING_IDEMPOTENCY_KEY", Message: "erp_purchase_request requires idempotency_key"},
		}, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 幂等:同 key 已创建 → 返回已有单据(deduplicated),不重复创建。
	for _, pr := range m.prs {
		if pr.IdempotencyKeys[idemKey] {
			return &ConnectorResult{
				Status:            ConnectorStatusSucceeded,
				Output:            prOutput(pr, true),
				ExternalObjectID:  pr.PRID,
				ExternalRequestID: fmt.Sprintf("pr-create-%s", idemKey),
			}, nil
		}
	}

	// 故障注入:发送失败(可重试,无副作用)。
	if m.failSend > 0 {
		m.failSend--
		return nil, fmt.Errorf("injected ERP transport failure (idempotency_key=%s)", idemKey)
	}

	prID := fmt.Sprintf("PR-%d", time.Now().UTC().UnixNano())
	title, _ := req.Input["title"].(string)
	supplier, _ := req.Input["supplier"].(string)
	total, _ := req.Input["total_amount"].(float64)
	now := time.Now().UTC()
	pr := &mockPurchaseRequest{
		PRID:            prID,
		Status:          "pending",
		Version:         1,
		Title:           title,
		Supplier:        supplier,
		TotalAmount:     total,
		IdempotencyKeys: map[string]bool{idemKey: true},
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	m.prs[prID] = pr

	// 故障注入:单据已创建但调用方未收到确认(indeterminate)。
	if m.failPostSend > 0 {
		m.failPostSend--
		return &ConnectorResult{
			Status:            ConnectorStatusIndeterminate,
			ExternalRequestID: fmt.Sprintf("pr-create-%s", idemKey),
			ExternalObjectID:  prID,
			Error:             &ConnectorError{Code: "ERP_TIMEOUT", Message: "ERP accepted request but response was lost", Retryable: true},
		}, nil
	}

	return &ConnectorResult{
		Status:            ConnectorStatusSucceeded,
		Output:            prOutput(pr, false),
		ExternalObjectID:  prID,
		ExternalRequestID: fmt.Sprintf("pr-create-%s", idemKey),
	}, nil
}

// executeCancel 补偿撤销:仅 pending 状态可撤(部分成功可恢复的补偿边界)。
func (m *MockERPConnector) executeCancel(req *ConnectorRequest) (*ConnectorResult, error) {
	prID, _ := req.Input["purchase_request_id"].(string)
	if prID == "" {
		return &ConnectorResult{
			Status: ConnectorStatusFailed,
			Error:  &ConnectorError{Code: "MISSING_PURCHASE_REQUEST_ID", Message: "cancel requires purchase_request_id"},
		}, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	pr, ok := m.prs[prID]
	if !ok {
		return &ConnectorResult{
			Status: ConnectorStatusFailed,
			Error:  &ConnectorError{Code: "PR_NOT_FOUND", Message: fmt.Sprintf("purchase request '%s' does not exist", prID)},
		}, nil
	}

	// 幂等:已 cancelled 的重复取消返回成功。
	if pr.Status == "cancelled" {
		return &ConnectorResult{
			Status:            ConnectorStatusSucceeded,
			Output:            map[string]any{"cancelled": true, "status": pr.Status, "deduplicated": true},
			ExternalObjectID:  pr.PRID,
			ExternalRequestID: fmt.Sprintf("pr-cancel-%s", pr.PRID),
		}, nil
	}

	// 补偿边界:仅未审批单据可直接撤销;已进入业务流程需走真实取消流程。
	if erpPRStatusOrder[pr.Status] > erpPRStatusOrder["pending"] {
		return &ConnectorResult{
			Status: ConnectorStatusFailed,
			Error: &ConnectorError{
				Code:    "CANCEL_NOT_ALLOWED",
				Message: fmt.Sprintf("purchase request '%s' is %s; only pending requests can be compensated", prID, pr.Status),
			},
		}, nil
	}

	pr.Status = "cancelled"
	pr.Version++
	pr.UpdatedAt = time.Now().UTC()
	return &ConnectorResult{
		Status:            ConnectorStatusSucceeded,
		Output:            map[string]any{"cancelled": true, "status": pr.Status},
		ExternalObjectID:  pr.PRID,
		ExternalRequestID: fmt.Sprintf("pr-cancel-%s", pr.PRID),
	}, nil
}

// Verify 核验采购申请外部状态(写类必选)。
func (m *MockERPConnector) Verify(ctx context.Context, req *ConnectorVerifyRequest) (*ConnectorVerifyResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if req.ExternalObjectID == "" {
		return &ConnectorVerifyResult{Confirmed: false, Detail: "no external_object_id to verify"}, nil
	}
	pr, ok := m.prs[req.ExternalObjectID]
	if !ok {
		return &ConnectorVerifyResult{Confirmed: false, Detail: "purchase request not found"}, nil
	}
	return &ConnectorVerifyResult{
		Confirmed: true,
		Detail:    fmt.Sprintf("purchase request exists: status=%s version=%d", pr.Status, pr.Version),
		Observed:  prOutput(pr, false),
	}, nil
}

// UseOutbox OutboxConnector 契约:采购申请创建经 Outbox 投递(Saga);
// preview(read)与 cancel(补偿,由 Dispatcher 直接调用)不经 Outbox。
func (m *MockERPConnector) UseOutbox(capability string) bool {
	return capability == "erp_purchase_request"
}

// BuildCompensation OutboxConnector 契约:采购申请创建的补偿 = 撤销该申请。
func (m *MockERPConnector) BuildCompensation(entry *OutboxEntry, execResult *ConnectorResult) (string, map[string]any, bool) {
	prID := ""
	if entry.ExternalObjectID != nil {
		prID = *entry.ExternalObjectID
	}
	if prID == "" && execResult != nil {
		prID = execResult.ExternalObjectID
	}
	if prID == "" {
		return "", nil, false // 未发出/无对象 ID:无副作用可补偿
	}
	return "erp_purchase_cancel", map[string]any{
		"purchase_request_id": prID,
		"reason":              fmt.Sprintf("outbox compensation for %s", entry.ID),
	}, true
}

// ── 测试/演示辅助 ──

// InjectSendFailures 注入接下来 n 次 request 的传输失败(可重试,无副作用)。
func (m *MockERPConnector) InjectSendFailures(n int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failSend = n
}

// InjectPostSendFailures 注入接下来 n 次 request 的"已创建未确认"(indeterminate)。
func (m *MockERPConnector) InjectPostSendFailures(n int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failPostSend = n
}

// GetPurchaseRequest 读取采购申请快照(测试/对账观测)。
func (m *MockERPConnector) GetPurchaseRequest(prID string) (map[string]any, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	pr, ok := m.prs[prID]
	if !ok {
		return nil, false
	}
	return prOutput(pr, false), true
}

// AdvancePurchaseRequest 推进采购申请状态(演示已审批/已发货场景)。
func (m *MockERPConnector) AdvancePurchaseRequest(prID, nextStatus string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	pr, ok := m.prs[prID]
	if !ok {
		return false
	}
	cur, okCur := erpPRStatusOrder[pr.Status]
	next, okNext := erpPRStatusOrder[nextStatus]
	if !okNext || (okCur && next <= cur) {
		return false
	}
	pr.Status = nextStatus
	pr.Version++
	pr.UpdatedAt = time.Now().UTC()
	return true
}

func prOutput(pr *mockPurchaseRequest, deduplicated bool) map[string]any {
	return map[string]any{
		"purchase_request_id": pr.PRID,
		"status":              pr.Status,
		"version":             pr.Version,
		"title":               pr.Title,
		"supplier":            pr.Supplier,
		"total_amount":        pr.TotalAmount,
		"deduplicated":        deduplicated,
		"updated_at":          pr.UpdatedAt,
	}
}
