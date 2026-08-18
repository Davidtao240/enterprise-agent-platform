package contextbuilder

import (
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/enterprise-agent-platform/go-platform/internal/memory"
)

// Message 对话消息(role: system/user/assistant/tool 等)。
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// BuildRequest 对应 Spec BuildContextRequest。
type BuildRequest struct {
	TenantID     string    `json:"tenant_id" binding:"required"`
	AgentID      string    `json:"agent_id"`
	UserID       string    `json:"user_id"`
	ThreadID     string    `json:"thread_id"`
	RunID        string    `json:"run_id"`
	Domain       string    `json:"domain"`
	SystemPrompt string    `json:"system_prompt"`
	ViewerRoles  []string  `json:"viewer_roles"`
	ChatHistory  []Message `json:"chat_history"`
	TokenBudget  int       `json:"token_budget" binding:"required"`
}

// BuildResponse 对应 Spec BuildContextResponse。
type BuildResponse struct {
	SystemPrompt string    `json:"system_prompt"`
	Messages     []Message `json:"messages"`
	TokenUsed    int       `json:"token_used"`
	Truncated    bool      `json:"truncated"`
}

// MemoryReader Builder 依赖的 Memory 检索接口(M4-A Service 实现)。
type MemoryReader interface {
	QueryVisible(ctx context.Context, req *memory.QueryRequest) ([]*memory.Memory, error)
}

// Builder 上下文组装器。
type Builder struct {
	memories MemoryReader
}

// NewBuilder 创建 Builder。
func NewBuilder(memories MemoryReader) *Builder {
	return &Builder{memories: memories}
}

// EstimateTokens Token 估算 heuristic:rune 数 / 4,最少 1。
// 生产可替换为精确 tokenizer;估算只影响裁剪触发阈值,不影响正确性。
func EstimateTokens(s string) int {
	n := utf8.RuneCountInString(s)
	if n == 0 {
		return 0
	}
	tokens := (n + 3) / 4
	if tokens < 1 {
		tokens = 1
	}
	return tokens
}

// segment 带裁剪优先级的消息分段;priority 越大越保留。
type segment struct {
	priority int // 1=history(最先裁) ... 5=domain;system=∞(不裁)
	msg      Message
}

const (
	prioHistory = 1
	prioThread  = 2
	prioUser    = 3
	prioTeam    = 4
	prioDomain  = 5
)

// Build 执行完整管道。
func (b *Builder) Build(ctx context.Context, req *BuildRequest) (*BuildResponse, error) {
	if req.TokenBudget <= 0 {
		return nil, fmt.Errorf("token budget must be positive")
	}

	// 1. 来源聚合 + ACL 过滤(经由 memory.Service.QueryVisible)
	var segs []segment
	appendMemories := func(scope memory.Scope, scopeID string, priority int) error {
		if scopeID == "" {
			return nil
		}
		items, err := b.memories.QueryVisible(ctx, &memory.QueryRequest{
			TenantID: req.TenantID, Scope: string(scope), ScopeID: scopeID,
			ViewerID: req.UserID, ViewerRoles: req.ViewerRoles,
		})
		if err != nil {
			return err
		}
		for _, m := range items {
			segs = append(segs, segment{priority: priority, msg: Message{
				Role:    "system",
				Content: fmt.Sprintf("[Memory %s] %s", scope, string(m.ContentJSON)),
			}})
		}
		return nil
	}
	if err := appendMemories(memory.ScopeDomain, req.Domain, prioDomain); err != nil {
		return nil, err
	}
	if err := appendMemories(memory.ScopeTeam, req.Domain, prioTeam); err != nil {
		return nil, err
	}
	if err := appendMemories(memory.ScopeUser, req.UserID, prioUser); err != nil {
		return nil, err
	}
	if err := appendMemories(memory.ScopeThread, req.ThreadID, prioThread); err != nil {
		return nil, err
	}
	for _, h := range req.ChatHistory {
		segs = append(segs, segment{priority: prioHistory, msg: h})
	}

	systemTokens := 0
	if req.SystemPrompt != "" {
		systemTokens = EstimateTokens(req.SystemPrompt)
	}

	// 2. Token 预算裁剪:超预算时移除最低优先级分段(History 最旧优先 →
	// 稳定排序后从头部移除即可,因为 history 段在列表尾部追加前按原顺序保留)
	truncated := false
	tokenOf := func(segs []segment) int {
		total := systemTokens
		for _, s := range segs {
			total += EstimateTokens(s.msg.Content)
		}
		return total
	}
	for tokenOf(segs) > req.TokenBudget && len(segs) > 0 {
		// 找到可裁剪的最低优先级分段索引(同优先级取最旧,即最先出现)
		minIdx, minPrio := -1, 1<<30
		for i, s := range segs {
			if s.priority < minPrio {
				minPrio, minIdx = s.priority, i
			}
		}
		if minIdx < 0 {
			break
		}
		segs = append(segs[:minIdx], segs[minIdx+1:]...)
		truncated = true
	}

	// 3. 组装输出:System -> Domain -> Team -> User -> Thread -> History
	// 聚合时已按 domain→team→user→thread 顺序追加,history 在最后;直接保序输出。
	resp := &BuildResponse{
		SystemPrompt: req.SystemPrompt,
		Messages:     make([]Message, 0, len(segs)),
		TokenUsed:    tokenOf(segs),
		Truncated:    truncated,
	}
	if req.SystemPrompt != "" {
		resp.Messages = append(resp.Messages, Message{Role: "system", Content: req.SystemPrompt})
	}
	for _, s := range segs {
		resp.Messages = append(resp.Messages, s.msg)
	}
	return resp, nil
}
