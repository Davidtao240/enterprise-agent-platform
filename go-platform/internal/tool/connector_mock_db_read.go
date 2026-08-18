package tool

import (
	"context"
	"fmt"
	"time"
)

// ── M3-A: Mock Connector — enterprise_db_read(治理后的只读视图)──
//
// 演示 db_read Connector 的完整契约实现。真实实现将把 QueryTemplate
// 编译为治理视图上的参数化 SQL;Mock 版只做契约校验并返回确定性 Fixture,
// 但白名单/LIMIT/超时语义与真实实现完全一致 —— 这正是发布阶段
// mock_fixture 的意义:契约与故障语义先行,供应商接入后行为不变。

// queryTemplate 治理后的查询模板(表列白名单 + 参数白名单 + LIMIT 约束)。
type queryTemplate struct {
	// Name 模板标识:调用方只能引用,不能提交任意 SQL。
	Name string
	// Description 模板语义(供审批与审计展示)。
	Description string
	// AllowedParams 参数白名单:Input 中仅这些键被接受,其余丢弃。
	AllowedParams map[string]bool
	// RequiredParams 必填参数(缺省拒绝)。
	RequiredParams map[string]bool
	// DefaultLimit / MaxLimit 行数约束:未指定用默认,超过上限被钳制。
	DefaultLimit int
	MaxLimit     int
	// FixtureRows 确定性返回(mock 阶段数据源)。
	FixtureRows []map[string]any
}

// mockQueryTemplates M3-A 演示模板集(领域中立:通用只读视图)。
var mockQueryTemplates = map[string]*queryTemplate{
	"records_by_period": {
		Name:        "records_by_period",
		Description: "按时间区间查询治理视图记录(只读)",
		AllowedParams: map[string]bool{"period_start": true, "period_end": true, "category": true},
		RequiredParams: map[string]bool{"period_start": true, "period_end": true},
		DefaultLimit: 50,
		MaxLimit:     200,
		FixtureRows: []map[string]any{
			{"record_id": "rec-001", "category": "operating", "amount": 125000.50, "period": "2026-Q2"},
			{"record_id": "rec-002", "category": "investment", "amount": 89000.00, "period": "2026-Q2"},
			{"record_id": "rec-003", "category": "operating", "amount": 43210.75, "period": "2026-Q1"},
		},
	},
	"department_list": {
		Name:        "department_list",
		Description: "组织架构部门列表(只读)",
		AllowedParams: map[string]bool{"include_inactive": true},
		RequiredParams: map[string]bool{},
		DefaultLimit: 20,
		MaxLimit:     100,
		FixtureRows: []map[string]any{
			{"department_id": "dept-01", "name": "Finance", "active": true},
			{"department_id": "dept-02", "name": "Operations", "active": true},
		},
	},
}

// MockDBReadConnector enterprise_db_read 的 Mock 实现(release_stage=mock_fixture)。
type MockDBReadConnector struct {
	manifest ConnectorManifest
}

// NewMockDBReadConnector 创建 Mock 只读 Connector。
func NewMockDBReadConnector() *MockDBReadConnector {
	return &MockDBReadConnector{
		manifest: ConnectorManifest{
			ConnectorCode: "enterprise_db_read_connector",
			Version:       "1.0.0",
			ConnectorType: "mock",
			AuthType:      "none",
			ReleaseStage:  "mock_fixture",
		},
	}
}

// Manifest 返回不可变标识(与 migration 021 种子数据一致)。
func (m *MockDBReadConnector) Manifest() ConnectorManifest { return m.manifest }

// Capabilities 只读能力:enterprise_db_read。
func (m *MockDBReadConnector) Capabilities() []ConnectorCapability {
	return []ConnectorCapability{
		{
			Name: "enterprise_db_read",
			Kind: CapabilityKindRead,
			InputSchema: map[string]any{
				"query_template": "string (whitelisted template name)",
				"params":         "object (template-scoped allowlist)",
				"limit":          "integer (clamped to template max)",
			},
			OutputSchema: map[string]any{
				"rows":      "array<object>",
				"row_count": "integer",
				"template":  "string",
			},
		},
	}
}

// HealthCheck Mock 恒健康(真实实现探测治理视图可达性,不产生副作用)。
func (m *MockDBReadConnector) HealthCheck(ctx context.Context) ConnectorHealth {
	return ConnectorHealth{Healthy: true, Detail: "mock", CheckedAt: time.Now().UTC()}
}

// Execute 只读查询:模板白名单 → 参数白名单 → LIMIT 钳制 → Fixture 返回。
func (m *MockDBReadConnector) Execute(ctx context.Context, req *ConnectorRequest) (*ConnectorResult, error) {
	if req.Capability != "enterprise_db_read" {
		return contractViolation(fmt.Sprintf("unsupported capability '%s'", req.Capability)), nil
	}
	templateName, _ := req.Input["query_template"].(string)
	tmpl, ok := mockQueryTemplates[templateName]
	if !ok {
		return &ConnectorResult{
			Status: ConnectorStatusFailed,
			Error: &ConnectorError{
				Code: "QUERY_TEMPLATE_NOT_ALLOWED", Message: fmt.Sprintf("query_template '%s' is not whitelisted", templateName),
			},
		}, nil
	}

	// 参数白名单:Input["params"] 仅保留模板允许的键;必填缺失拒绝。
	rawParams, _ := req.Input["params"].(map[string]any)
	params := map[string]any{}
	for k, v := range rawParams {
		if tmpl.AllowedParams[k] {
			params[k] = v
		}
	}
	for k := range tmpl.RequiredParams {
		if _, present := params[k]; !present {
			return &ConnectorResult{
				Status: ConnectorStatusFailed,
				Error: &ConnectorError{
					Code: "MISSING_REQUIRED_PARAM", Message: fmt.Sprintf("param '%s' is required by template '%s'", k, templateName),
				},
			}, nil
		}
	}

	// LIMIT 钳制:默认值兜底,上限强制。
	limit := tmpl.DefaultLimit
	if l, ok := req.Input["limit"].(float64); ok && int(l) > 0 {
		limit = int(l)
	}
	if limit > tmpl.MaxLimit {
		limit = tmpl.MaxLimit
	}

	// 尊重调用方超时/取消(mock 阶段只做检查,真实实现在此执行查询)。
	if err := ctx.Err(); err != nil {
		return &ConnectorResult{
			Status: ConnectorStatusIndeterminate,
			Error:  &ConnectorError{Code: "CONTEXT_CANCELLED", Message: err.Error()},
		}, nil
	}

	// Fixture 切片(确定性输出)。
	rows := []map[string]any{}
	for i, r := range tmpl.FixtureRows {
		if i >= limit {
			break
		}
		rows = append(rows, r)
	}
	return &ConnectorResult{
		Status: ConnectorStatusSucceeded,
		Output: map[string]any{
			"template":  templateName,
			"params":    params,
			"row_count": len(rows),
			"rows":      rows,
		},
		ExternalRequestID: fmt.Sprintf("mock-dbread-%d", time.Now().UTC().UnixNano()),
	}, nil
}

// Verify 只读无副作用:恒 Confirmed。
func (m *MockDBReadConnector) Verify(ctx context.Context, req *ConnectorVerifyRequest) (*ConnectorVerifyResult, error) {
	return &ConnectorVerifyResult{
		Confirmed: true,
		Detail:    "read-only capability: no external side effect to verify",
	}, nil
}

func contractViolation(detail string) *ConnectorResult {
	return &ConnectorResult{
		Status: ConnectorStatusFailed,
		Error: &ConnectorError{
			Code: "CONTRACT_VIOLATION", Message: detail,
		},
	}
}
