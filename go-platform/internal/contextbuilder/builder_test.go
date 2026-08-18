package contextbuilder

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/enterprise-agent-platform/go-platform/internal/memory"
)

// fakeMemory 内存版 MemoryReader。
type fakeMemory struct {
	items map[string][]*memory.Memory // key: scope+"/"+scope_id
}

func (f *fakeMemory) QueryVisible(_ context.Context, req *memory.QueryRequest) ([]*memory.Memory, error) {
	return f.items[req.Scope+"/"+req.ScopeID], nil
}

func mem(scope, scopeID, content string) *memory.Memory {
	return &memory.Memory{Scope: memory.Scope(scope), ScopeID: scopeID, ContentJSON: json.RawMessage(content)}
}

func TestEstimateTokens(t *testing.T) {
	if got := EstimateTokens(""); got != 0 {
		t.Fatalf("empty = %d, want 0", got)
	}
	if got := EstimateTokens("ab"); got != 1 {
		t.Fatalf("2 runes = %d, want 1", got)
	}
	if got := EstimateTokens("12345"); got != 2 {
		t.Fatalf("5 runes = %d, want 2", got)
	}
	// 中文按 rune 计数
	if got := EstimateTokens("你好世界"); got != 1 {
		t.Fatalf("4 CJK runes = %d, want 1", got)
	}
}

func TestBuildAssemblyOrder(t *testing.T) {
	fm := &fakeMemory{items: map[string][]*memory.Memory{
		"domain/finance": {mem("domain", "finance", `{"rule":"GAAP"}`)},
		"team/finance":   {mem("team", "finance", `{"sop":"month-end"}`)},
		"user/u1":        {mem("user", "u1", `{"pref":"concise"}`)},
		"thread/th1":     {mem("thread", "th1", `{"topic":"May report"}`)},
	}}
	b := NewBuilder(fm)

	resp, err := b.Build(context.Background(), &BuildRequest{
		TenantID: "t1", UserID: "u1", ThreadID: "th1", Domain: "finance",
		SystemPrompt: "You are a finance agent.",
		ChatHistory:  []Message{{Role: "user", Content: "hi"}},
		TokenBudget:  1000,
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if resp.Truncated {
		t.Fatal("should not truncate within budget")
	}
	// 顺序:System -> Domain -> Team -> User -> Thread -> History
	wantOrder := []string{
		"You are a finance agent.",
		"[Memory domain]", "[Memory team]", "[Memory user]", "[Memory thread]",
		"hi",
	}
	if len(resp.Messages) != len(wantOrder) {
		t.Fatalf("messages = %d, want %d", len(resp.Messages), len(wantOrder))
	}
	for i, w := range wantOrder {
		if !strings.HasPrefix(resp.Messages[i].Content, w) {
			t.Errorf("msg[%d] = %q, want prefix %q", i, resp.Messages[i].Content, w)
		}
	}
	if resp.TokenUsed <= 0 {
		t.Fatal("token used must be positive")
	}
}

func TestBuildTrimsHistoryFirst(t *testing.T) {
	fm := &fakeMemory{items: map[string][]*memory.Memory{
		"domain/finance": {mem("domain", "finance", `{"rule":"GAAP"}`)},
	}}
	b := NewBuilder(fm)

	// 预算仅够 system + domain:history 被裁掉
	resp, err := b.Build(context.Background(), &BuildRequest{
		TenantID: "t1", Domain: "finance", SystemPrompt: "sys",
		ChatHistory: []Message{
			{Role: "user", Content: strings.Repeat("a", 200)}, // 50 tokens
		},
		TokenBudget: EstimateTokens("sys") + EstimateTokens("[Memory domain] {\"rule\":\"GAAP\"}"),
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !resp.Truncated {
		t.Fatal("expected truncated")
	}
	if len(resp.Messages) != 2 { // system + domain
		t.Fatalf("messages = %d, want 2 (system + domain)", len(resp.Messages))
	}
	if resp.TokenUsed > EstimateTokens("sys")+EstimateTokens("[Memory domain] {\"rule\":\"GAAP\"}") {
		t.Fatalf("token used %d exceeds expectation", resp.TokenUsed)
	}
}

func TestBuildTrimsLowPriorityBeforeDomain(t *testing.T) {
	fm := &fakeMemory{items: map[string][]*memory.Memory{
		"domain/finance": {mem("domain", "finance", `{"rule":"GAAP"}`)},
		"thread/th1":     {mem("thread", "th1", `{"topic":"x"}`)},
	}}
	b := NewBuilder(fm)

	domainTokens := EstimateTokens("[Memory domain] {\"rule\":\"GAAP\"}")
	threadTokens := EstimateTokens("[Memory thread] {\"topic\":\"x\"}")

	// 预算够 system + domain,但不够 thread:thread 应被裁,domain 保留
	budget := 1 + domainTokens
	resp, err := b.Build(context.Background(), &BuildRequest{
		TenantID: "t1", ThreadID: "th1", Domain: "finance", SystemPrompt: "s",
		TokenBudget: budget + threadTokens - 1, // 差一个 token,thread 必须被裁
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !resp.Truncated {
		t.Fatal("expected truncated")
	}
	for _, m := range resp.Messages {
		if strings.Contains(m.Content, "[Memory thread]") {
			t.Fatal("thread should be trimmed before domain")
		}
	}
}

func TestBuildKeepsSystemPromptAbsolutely(t *testing.T) {
	fm := &fakeMemory{items: map[string][]*memory.Memory{
		"domain/finance": {mem("domain", "finance", `{"rule":"GAAP"}`)},
	}}
	b := NewBuilder(fm)

	// 预算不足以容纳 domain:全部 memory/history 裁光,system 仍保留
	resp, err := b.Build(context.Background(), &BuildRequest{
		TenantID: "t1", Domain: "finance", SystemPrompt: "must survive",
		TokenBudget: EstimateTokens("must survive"),
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(resp.Messages) != 1 || resp.Messages[0].Content != "must survive" {
		t.Fatalf("system prompt must survive trimming, got %+v", resp.Messages)
	}
	if !resp.Truncated {
		t.Fatal("expected truncated")
	}
}

func TestBuildRejectsNonPositiveBudget(t *testing.T) {
	b := NewBuilder(&fakeMemory{})
	if _, err := b.Build(context.Background(), &BuildRequest{TenantID: "t1", TokenBudget: 0}); err == nil {
		t.Fatal("expected budget validation error")
	}
}
