package tool

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// ── M3-A: Connector Contract 门禁 + Mock Connector 契约测试 ──

func TestCapabilityAllowed_ReadAlwaysAllowed(t *testing.T) {
	caps := []ConnectorCapability{
		{Name: "enterprise_db_read", Kind: CapabilityKindRead},
		{Name: "erp_purchase_request", Kind: CapabilityKindWrite},
	}
	// 只读能力:任意阶段/环境放行
	for _, stage := range []string{"mock_fixture", "sandbox_readonly", "shadow", "production"} {
		for _, env := range []string{"mock", "sandbox", "shadow", "production"} {
			if err := CapabilityAllowed(stage, env, "enterprise_db_read", caps); err != nil {
				t.Fatalf("read capability should be allowed at stage=%s env=%s: %v", stage, env, err)
			}
		}
	}
}

func TestCapabilityAllowed_WriteGatedByStageAndEnv(t *testing.T) {
	caps := []ConnectorCapability{{Name: "erp_purchase_request", Kind: CapabilityKindWrite}}

	cases := []struct {
		name    string
		stage   string
		env     string
		wantErr bool
	}{
		{"mock stage rejected", "mock_fixture", "production", true},
		{"sandbox stage rejected", "sandbox_readonly", "production", true},
		{"shadow stage rejected", "shadow", "production", true},
		{"mock env rejected", "human_approved_write", "mock", true},
		{"sandbox env rejected", "human_approved_write", "sandbox", true},
		{"write approved", "human_approved_write", "shadow", false},
		{"canary allowed", "limited_canary", "production", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := CapabilityAllowed(tc.stage, tc.env, "erp_purchase_request", caps)
			if tc.wantErr && !errors.Is(err, ErrCapabilityNotAllowed) {
				t.Fatalf("want ErrCapabilityNotAllowed, got %v", err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("want allowed, got %v", err)
			}
		})
	}
}

func TestCapabilityAllowed_UndeclaredCapabilityRejected(t *testing.T) {
	caps := []ConnectorCapability{{Name: "enterprise_db_read", Kind: CapabilityKindRead}}
	err := CapabilityAllowed("production", "production", "drop_table", caps)
	if !errors.Is(err, ErrCapabilityNotAllowed) {
		t.Fatalf("undeclared capability must be rejected with ErrCapabilityNotAllowed, got %v", err)
	}
}

func TestCapabilityAllowed_UnknownStageOrEnvRejected(t *testing.T) {
	caps := []ConnectorCapability{{Name: "w", Kind: CapabilityKindWrite}}
	if err := CapabilityAllowed("beta", "production", "w", caps); !errors.Is(err, ErrCapabilityNotAllowed) {
		t.Fatalf("unknown release stage must be rejected, got %v", err)
	}
	if err := CapabilityAllowed("human_approved_write", "staging", "w", caps); !errors.Is(err, ErrCapabilityNotAllowed) {
		t.Fatalf("unknown environment must be rejected, got %v", err)
	}
}

// ── Mock Connector: enterprise_db_read 契约 ──

func newDBReadInput(template string, params map[string]any, limit float64) map[string]any {
	in := map[string]any{"query_template": template}
	if params != nil {
		in["params"] = params
	}
	if limit > 0 {
		in["limit"] = limit
	}
	return in
}

func TestMockDBRead_TemplateWhitelist(t *testing.T) {
	c := NewMockDBReadConnector()
	// 未注册模板(模拟任意 SQL)必须确定性拒绝
	res, err := c.Execute(context.Background(), &ConnectorRequest{
		TenantID: "t1", ToolCallID: "tc1", Capability: "enterprise_db_read",
		Input: newDBReadInput("SELECT * FROM users", nil, 0),
	})
	if err != nil || res.Status != ConnectorStatusFailed {
		t.Fatalf("non-whitelisted template must fail, res=%+v err=%v", res, err)
	}
	if res.Error == nil || res.Error.Code != "QUERY_TEMPLATE_NOT_ALLOWED" {
		t.Fatalf("expect QUERY_TEMPLATE_NOT_ALLOWED, got %+v", res.Error)
	}
}

func TestMockDBRead_ParamWhitelistAndRequired(t *testing.T) {
	c := NewMockDBReadConnector()

	// 必填参数缺失拒绝
	res, _ := c.Execute(context.Background(), &ConnectorRequest{
		Capability: "enterprise_db_read",
		Input:      newDBReadInput("records_by_period", map[string]any{"category": "operating"}, 0),
	})
	if res.Status != ConnectorStatusFailed || res.Error.Code != "MISSING_REQUIRED_PARAM" {
		t.Fatalf("missing required param must fail, got %+v", res)
	}

	// 非白名单参数被静默丢弃(不报错、不透传)
	res, _ = c.Execute(context.Background(), &ConnectorRequest{
		Capability: "enterprise_db_read",
		Input: newDBReadInput("records_by_period",
			map[string]any{"period_start": "2026-01-01", "period_end": "2026-06-30", "injected_sql": "1=1"}, 0),
	})
	if res.Status != ConnectorStatusSucceeded {
		t.Fatalf("valid template call must succeed, got %+v", res)
	}
	params := res.Output["params"].(map[string]any)
	if _, leaked := params["injected_sql"]; leaked {
		t.Fatal("non-whitelisted param must not leak into output")
	}
	if params["period_start"] != "2026-01-01" {
		t.Fatalf("whitelisted param missing: %+v", params)
	}
}

func TestMockDBRead_LimitClamp(t *testing.T) {
	c := NewMockDBReadConnector()
	// 请求 9999 行 → 钳制到模板上限(records_by_period: 3 条 fixture < 200 上限)
	res, _ := c.Execute(context.Background(), &ConnectorRequest{
		Capability: "enterprise_db_read",
		Input:      newDBReadInput("records_by_period", map[string]any{"period_start": "a", "period_end": "b"}, 9999),
	})
	if res.Status != ConnectorStatusSucceeded {
		t.Fatalf("must succeed, got %+v", res)
	}
	if res.Output["row_count"].(int) > 200 {
		t.Fatalf("row_count must be clamped to template max, got %v", res.Output["row_count"])
	}
	// limit=1 → 只返回 1 行
	res2, _ := c.Execute(context.Background(), &ConnectorRequest{
		Capability: "enterprise_db_read",
		Input:      newDBReadInput("records_by_period", map[string]any{"period_start": "a", "period_end": "b"}, 1),
	})
	if res2.Output["row_count"].(int) != 1 {
		t.Fatalf("limit=1 must return 1 row, got %v", res2.Output["row_count"])
	}
}

func TestMockDBRead_UnsupportedCapability(t *testing.T) {
	c := NewMockDBReadConnector()
	res, err := c.Execute(context.Background(), &ConnectorRequest{Capability: "enterprise_db_write", Input: map[string]any{}})
	if err != nil || res.Status != ConnectorStatusFailed || res.Error.Code != "CONTRACT_VIOLATION" {
		t.Fatalf("unsupported capability must fail with CONTRACT_VIOLATION, res=%+v err=%v", res, err)
	}
}

func TestMockDBRead_ManifestMatchesRegistrySeed(t *testing.T) {
	m := NewMockDBReadConnector().Manifest()
	if m.ConnectorCode != "enterprise_db_read_connector" || m.Version != "1.0.0" {
		t.Fatalf("manifest must match migration 021 seed, got %+v", m)
	}
	if m.ReleaseStage != "mock_fixture" {
		t.Fatalf("mock connector stage must be mock_fixture, got %s", m.ReleaseStage)
	}
}

// ── Binding 级 capability 白名单 ──

func TestBindingCapabilityAllowed(t *testing.T) {
	// 空/空数组/null = 不限制(兼容存量)
	for _, empty := range []string{"", "[]", "null"} {
		if err := bindingCapabilityAllowed(empty, "anything"); err != nil {
			t.Fatalf("'%s' must mean unrestricted, got %v", empty, err)
		}
	}
	// 白名单命中
	if err := bindingCapabilityAllowed(`["enterprise_db_read"]`, "enterprise_db_read"); err != nil {
		t.Fatalf("allowlisted capability must pass: %v", err)
	}
	// 白名单未命中
	if err := bindingCapabilityAllowed(`["enterprise_db_read"]`, "erp_purchase_request"); !errors.Is(err, ErrCapabilityNotAllowed) {
		t.Fatalf("non-allowlisted capability must fail with ErrCapabilityNotAllowed, got %v", err)
	}
	// 畸形 JSON 拒绝(不静默放行)
	if err := bindingCapabilityAllowed(`{not-json`, "x"); err == nil {
		t.Fatal("malformed allowlist must fail closed")
	}
}

// CapabilityAllowed 的错误信息不应包含凭证类内容(契约自检示例)。
func TestConnectorRequestCredentialNeverSerialized(t *testing.T) {
	req := ConnectorRequest{
		TenantID: "t1", Capability: "enterprise_db_read",
		Input:      map[string]any{"query_template": "department_list"},
		Credential: &ResolvedCredential{Plaintext: "super-secret-value"},
	}
	// json 序列化(Credential 标记了 json:"-")
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) == "" || containsSubstr(string(b), "super-secret-value") {
		t.Fatal("serialized ConnectorRequest must never contain credential plaintext")
	}
}

func containsSubstr(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
