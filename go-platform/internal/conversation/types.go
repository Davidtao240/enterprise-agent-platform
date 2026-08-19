package conversation

import (
	"time"
)

type ConversationStatus string

const (
	ConvStatusActive   ConversationStatus = "active"
	ConvStatusClosed   ConversationStatus = "closed"
	ConvStatusArchived ConversationStatus = "archived"
)

type MessageRole string

const (
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleSystem    MessageRole = "system"
	RoleTool      MessageRole = "tool"
)

type Conversation struct {
	ID              string     `json:"id"`
	TenantID        string     `json:"tenant_id"`
	CreatedBy       string     `json:"created_by"`
	ThreadID        string     `json:"thread_id"`
	AgentPackageCode string    `json:"agent_package_code"`
	Title           *string    `json:"title,omitempty"`
	Status          string     `json:"status"`
	BudgetJSON      *string    `json:"budget_json,omitempty"`
	LastMessageAt   *time.Time `json:"last_message_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type ConversationMessage struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	RunID          *string   `json:"run_id,omitempty"`
	Role           string    `json:"role"`
	Content        string    `json:"content"`
	Seq            int       `json:"seq"`
	Tokens         *int      `json:"tokens,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type ConversationGroup struct {
	AgentPackageCode string        `json:"agent_package_code"`
	Conversations    []Conversation `json:"conversations"`
}

type SSEEvent struct {
	ID    string `json:"id"`
	Seq   int    `json:"seq"`
	Event string `json:"event"`
	Data  any    `json:"data"`
}

type ClarificationSchema struct {
	SchemaVersion  string                    `json:"schema_version"`
	Title          string                    `json:"title"`
	Description    string                    `json:"description,omitempty"`
	Properties     map[string]FieldConfig    `json:"properties"`
	Required       []string                  `json:"required,omitempty"`
	SubmitLabel    string                    `json:"submit_label,omitempty"`
}

type FieldConfig struct {
	Component  string  `json:"component"`
	Label      string  `json:"label"`
	Required   bool    `json:"required,omitempty"`
	Default    any     `json:"default,omitempty"`
	Options    []Option `json:"options,omitempty"`
	MinLength  *int    `json:"min_length,omitempty"`
	MaxLength  *int    `json:"max_length,omitempty"`
	Min        *float64 `json:"min,omitempty"`
	Max        *float64 `json:"max,omitempty"`
	Step       *float64 `json:"step,omitempty"`
	Pattern    string  `json:"pattern,omitempty"`
	MinDate    string  `json:"min_date,omitempty"`
	MaxDate    string  `json:"max_date,omitempty"`
	Accept     string  `json:"accept,omitempty"`
	MaxFiles   *int    `json:"max_files,omitempty"`
	MaxSizeMB  *int    `json:"max_size_mb,omitempty"`
	Placeholder string  `json:"placeholder,omitempty"`
}

type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type CreateConversationRequest struct {
	AgentPackageCode string `json:"agent_package_code" binding:"required"`
	Title            string `json:"title"`
}

type CreateConversationResponse struct {
	ID             string `json:"id"`
	ThreadID       string `json:"thread_id"`
	AgentPackageCode string `json:"agent_package_code"`
	Title          string `json:"title,omitempty"`
	Status         string `json:"status"`
}

type SendMessageRequest struct {
	Content string `json:"content" binding:"required"`
}

type SendMessageResponse struct {
	MessageID string `json:"message_id"`
	RunID     string `json:"run_id"`
	Status    string `json:"status"`
}

type AnswerClarificationRequest struct {
	Answers map[string]any `json:"answers" binding:"required"`
}

type UpdateConversationRequest struct {
	Title  *string `json:"title,omitempty"`
	Status *string `json:"status,omitempty"`
}