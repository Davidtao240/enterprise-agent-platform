package memory

import (
	"encoding/json"
	"time"
)

// Scope Memory 作用域层级 (M4-A 五层模型)。
type Scope string

const (
	ScopeRun    Scope = "run"
	ScopeThread Scope = "thread"
	ScopeUser   Scope = "user"
	ScopeTeam   Scope = "team"
	ScopeDomain Scope = "domain"
)

// ValidScope 校验 scope 是否为合法层级。
func ValidScope(s string) bool {
	switch Scope(s) {
	case ScopeRun, ScopeThread, ScopeUser, ScopeTeam, ScopeDomain:
		return true
	default:
		return false
	}
}

// Memory 对应 agent_memory 表。
type Memory struct {
	ID          string          `json:"id"`
	TenantID    string          `json:"tenant_id"`
	Scope       Scope           `json:"scope"`
	ScopeID     string          `json:"scope_id"`
	ContentJSON json.RawMessage `json:"content"`
	ACL         []string        `json:"acl,omitempty"` // 空/nil 表示仅创建者可见
	CreatedBy   string          `json:"created_by"`
	ExpiresAt   *time.Time      `json:"expires_at,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	DeletedAt   *time.Time      `json:"deleted_at,omitempty"`
}

// WriteRequest Memory 写入请求 (internal,服务身份调用)。
type WriteRequest struct {
	TenantID  string          `json:"tenant_id" binding:"required"`
	Scope     string          `json:"scope" binding:"required"`
	ScopeID   string          `json:"scope_id" binding:"required"`
	Content   json.RawMessage `json:"content" binding:"required"`
	ACL       []string        `json:"acl,omitempty"`
	CreatedBy string          `json:"created_by" binding:"required"`
	ExpiresAt *time.Time      `json:"expires_at,omitempty"`
}

// QueryRequest Memory 检索请求;ViewerID/ViewerRoles 用于 ACL 过滤。
type QueryRequest struct {
	TenantID    string
	Scope       string
	ScopeID     string
	ViewerID    string
	ViewerRoles []string
}
