package agent

import "time"

type Agent struct {
	ID               string    `json:"id"`
	AgentID          string    `json:"agent_id"`
	Name             string    `json:"name"`
	Domain           string    `json:"domain"`
	ReusableScope    string    `json:"reusable_scope"`
	CapabilitiesJSON string    `json:"capabilities_json"`
	InputSchemaJSON  string    `json:"input_schema_json"`
	OutputSchemaJSON string    `json:"output_schema_json"`
	Endpoint         *string   `json:"endpoint,omitempty"`
	Status           string    `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type Graph struct {
	ID              string    `json:"id"`
	GraphKey        string    `json:"graph_key"`
	BusinessAppCode string    `json:"business_app_code"`
	Name            string    `json:"name"`
	Version         string    `json:"version"`
	Description     *string   `json:"description,omitempty"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type AgentRunLog struct {
	ID                 string     `json:"id"`
	RunID              string     `json:"run_id"`
	TraceID            string     `json:"trace_id"`
	WorkflowInstanceID string     `json:"workflow_instance_id"`
	NodeInstanceID     string     `json:"node_instance_id"`
	BusinessAppCode    string     `json:"business_app_code"`
	GraphKey           string     `json:"graph_key"`
	AgentID            *string    `json:"agent_id,omitempty"`
	Status             string     `json:"status"`
	InputSummaryJSON   *string    `json:"input_summary_json,omitempty"`
	OutputSummaryJSON  *string    `json:"output_summary_json,omitempty"`
	UsageJSON          *string    `json:"usage_json,omitempty"`
	ErrorJSON          *string    `json:"error_json,omitempty"`
	StartedAt          *time.Time `json:"started_at,omitempty"`
	FinishedAt         *time.Time `json:"finished_at,omitempty"`
	DurationMs         *int       `json:"duration_ms,omitempty"`
}

type ApprovalTask struct {
	ID                 string     `json:"id"`
	WorkflowInstanceID string     `json:"workflow_instance_id"`
	NodeInstanceID     string     `json:"node_instance_id"`
	BusinessAppCode    string     `json:"business_app_code"`
	Title              string     `json:"title"`
	Status             string     `json:"status"`
	AssigneeRole       *string    `json:"assignee_role,omitempty"`
	AssigneeUserID     *string    `json:"assignee_user_id,omitempty"`
	DecisionBy         *string    `json:"decision_by,omitempty"`
	DecisionComment    *string    `json:"decision_comment,omitempty"`
	DecidedAt          *time.Time `json:"decided_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type ApprovalTaskView struct {
	ApprovalTask
	WorkflowTitle      string     `json:"workflow_title"`
	WorkflowStatus     string     `json:"workflow_status"`
	NodeStatus         string     `json:"node_status"`
	AgentOutputJSON    *string    `json:"agent_output_json,omitempty"`
	AgentRunStatus     *string    `json:"agent_run_status,omitempty"`
	AgentRunFinishedAt *time.Time `json:"agent_run_finished_at,omitempty"`
}

type ListAgentsResponse struct {
	AgentID       string `json:"agent_id"`
	Name          string `json:"name"`
	Domain        string `json:"domain"`
	ReusableScope string `json:"reusable_scope"`
	Status        string `json:"status"`
}

type CreateAgentRequest struct {
	AgentID          string `json:"agent_id" binding:"required"`
	Name             string `json:"name" binding:"required"`
	Domain           string `json:"domain" binding:"required"`
	ReusableScope    string `json:"reusable_scope"`
	CapabilitiesJSON string `json:"capabilities_json"`
	InputSchemaJSON  string `json:"input_schema_json"`
	OutputSchemaJSON string `json:"output_schema_json"`
	Endpoint         string `json:"endpoint,omitempty"`
}

type AgentRunRequest struct {
	TraceID             string         `json:"trace_id"`
	BusinessAppCode     string         `json:"business_app_code"`
	WorkflowTemplateKey string         `json:"workflow_template_key"`
	GraphKey            string         `json:"graph_key"`
	WorkflowInstanceID  string         `json:"workflow_instance_id"`
	NodeInstanceID      string         `json:"node_instance_id"`
	Input               map[string]any `json:"input"`
	Context             AgentContext   `json:"context"`
}

type AgentContext struct {
	UserID       string `json:"user_id"`
	DepartmentID string `json:"department_id,omitempty"`
	TenantID     string `json:"tenant_id,omitempty"`
}

type AgentRunResponse struct {
	RunID    string         `json:"run_id"`
	GraphKey string         `json:"graph_key"`
	Status   string         `json:"status"`
	Output   map[string]any `json:"output"`
	Usage    *AgentUsage    `json:"usage,omitempty"`
	Error    *AgentRunError `json:"error,omitempty"`
}

type AgentUsage struct {
	Model            string  `json:"model"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	Cost             float64 `json:"cost"`
}

type AgentRunError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
