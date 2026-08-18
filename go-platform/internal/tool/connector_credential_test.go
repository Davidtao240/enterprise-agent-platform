package tool

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// ── M2-D: CredentialRef 边界单元测试 ──

// TestSecretCipherRoundTrip 加解密往返 + 密钥隔离 + 密文不含明文。
func TestSecretCipherRoundTrip(t *testing.T) {
	c1, err := newSecretCipher("test-key-material-1")
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	plaintext := "erp-api-token-super-secret-42"

	cipherText, err := c1.encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	// 密文不含明文片段
	if strings.Contains(cipherText, plaintext) || strings.Contains(cipherText, "super-secret") {
		t.Fatal("cipher text must not contain plaintext fragments")
	}

	got, err := c1.decrypt(cipherText)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got != plaintext {
		t.Fatalf("round trip mismatch: %q", got)
	}

	// 不同密钥解密必须失败
	c2, _ := newSecretCipher("different-key")
	if _, err := c2.decrypt(cipherText); err == nil {
		t.Fatal("decrypt with wrong key must fail")
	}

	// 密钥为空拒绝创建
	if _, err := newSecretCipher(""); !errors.Is(err, ErrSecretNotConfigured) {
		t.Fatalf("empty key must fail with ErrSecretNotConfigured, got: %v", err)
	}

	// 篡改密文解密失败
	tampered := cipherText[:len(cipherText)-4] + "AAAA"
	if _, err := c1.decrypt(tampered); err == nil {
		t.Fatal("tampered cipher text must fail authentication")
	}
}

// fakeBindingValidator 内存 Connector Binding 校验器。
type fakeBindingValidator struct {
	bindings map[string]*ConnectorBinding
}

func (f *fakeBindingValidator) FindBinding(ctx context.Context, id string) (*ConnectorBinding, error) {
	b, ok := f.bindings[id]
	if !ok {
		return nil, ErrConnectorBindingNotFound
	}
	return b, nil
}

// TestExecuteBindingValidation M2-D:Execute 校验绑定存在/租户匹配/active。
func TestExecuteBindingValidation(t *testing.T) {
	svc, _, _, _ := setupLifecycleService()
	svc.bindingRepo = &fakeBindingValidator{bindings: map[string]*ConnectorBinding{
		"bind-ok":   {ID: "bind-ok", TenantID: "t1", Status: "active"},
		"bind-off":  {ID: "bind-off", TenantID: "t1", Status: "disabled"},
		"bind-x":    {ID: "bind-x", TenantID: "t2", Status: "active"}, // 跨租户
	}}
	ctx := context.Background()

	base := &ExecuteRequest{
		TenantID: "t1", RunID: "r1", AgentID: "a1", BusinessAppCode: "finance",
		ToolID: "parse_csv", ToolVersion: "1.0", ArgumentsJSON: `{}`,
	}

	// 不存在的绑定
	req := *base
	req.IdempotencyKey, req.ConnectorBindingID = "bind-miss", "bind-none"
	if _, err := svc.Execute(ctx, &req, nil, nil); !errors.Is(err, ErrConnectorBindingNotFound) {
		t.Fatalf("missing binding must fail, got: %v", err)
	}
	// 跨租户绑定:视同不存在
	req2 := *base
	req2.IdempotencyKey, req2.ConnectorBindingID = "bind-cross", "bind-x"
	if _, err := svc.Execute(ctx, &req2, nil, nil); !errors.Is(err, ErrConnectorBindingNotFound) {
		t.Fatalf("cross-tenant binding must fail as not-found, got: %v", err)
	}
	// 停用绑定
	req3 := *base
	req3.IdempotencyKey, req3.ConnectorBindingID = "bind-off", "bind-off"
	if _, err := svc.Execute(ctx, &req3, nil, nil); !errors.Is(err, ErrConnectorBindingDisabled) {
		t.Fatalf("disabled binding must fail, got: %v", err)
	}
	// 正常绑定放行
	req4 := *base
	req4.IdempotencyKey, req4.ConnectorBindingID = "bind-ok", "bind-ok"
	result, err := svc.Execute(ctx, &req4, nil, nil)
	if err != nil {
		t.Fatalf("active binding must pass: %v", err)
	}
	if result.Status != ToolCallStatusExecuting {
		t.Fatalf("status = %s, want executing", result.Status)
	}
}
