package workflow

import "time"

const (
	StatusDraft         = "draft"
	StatusRunning       = "running"
	StatusWaitingReview = "waiting_review"
	StatusApproved      = "approved"
	StatusRejected      = "rejected"
	StatusArchived      = "archived"
	StatusFailed        = "failed"
	StatusCancelled     = "cancelled"
)

const (
	NodeStatusPending       = "pending"
	NodeStatusRunning       = "running"
	NodeStatusSucceeded     = "succeeded"
	NodeStatusFailed        = "failed"
	NodeStatusSkipped       = "skipped"
	NodeStatusWaitingReview = "waiting_review"
	NodeStatusCancelled     = "cancelled"
)

const (
	NodeTypeFileUpload  = "file_upload"
	NodeTypeAgentGraph  = "agent_graph"
	NodeTypeHumanReview = "human_review"
	NodeTypeSystem      = "system"
)

const (
	TemplateStatusDraft      = "draft"
	TemplateStatusActive     = "active"
	TemplateStatusDeprecated = "deprecated"
	TemplateStatusDisabled   = "disabled"
)

const (
	EdgeWhenSucceeded = "succeeded"
	EdgeWhenApproved  = "approved"
	EdgeWhenRejected  = "rejected"
	EdgeWhenFailed    = "failed"
)

type Template struct {
	ID                  string    `json:"id"`
	BusinessAppCode     string    `json:"business_app_code"`
	WorkflowTemplateKey string    `json:"workflow_template_key"`
	Name                string    `json:"name"`
	Version             string    `json:"version"`
	GraphKey            string    `json:"graph_key"`
	DefinitionJSON      string    `json:"definition_json"`
	Status              string    `json:"status"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type Instance struct {
	ID                      string     `json:"id"`
	BusinessAppCode         string     `json:"business_app_code"`
	WorkflowTemplateID      string     `json:"workflow_template_id"`
	WorkflowTemplateKey     string     `json:"workflow_template_key"`
	WorkflowTemplateVersion string     `json:"workflow_template_version"`
	GraphKey                string     `json:"graph_key"`
	Title                   string     `json:"title"`
	Status                  string     `json:"status"`
	InputJSON               string     `json:"input_json"`
	OutputJSON              *string    `json:"output_json"`
	CreatedBy               string     `json:"created_by"`
	StartedAt               *time.Time `json:"started_at"`
	FinishedAt              *time.Time `json:"finished_at"`
	TraceID                 string     `json:"trace_id"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
}

type NodeInstance struct {
	ID                 string     `json:"id"`
	WorkflowInstanceID string     `json:"workflow_instance_id"`
	NodeKey            string     `json:"node_key"`
	NodeType           string     `json:"node_type"`
	Name               string     `json:"name"`
	Status             string     `json:"status"`
	InputJSON          *string    `json:"input_json"`
	OutputJSON         *string    `json:"output_json"`
	ErrorJSON          *string    `json:"error_json"`
	RetryCount         int        `json:"retry_count"`
	MaxRetries         int        `json:"max_retries"`
	StartedAt          *time.Time `json:"started_at"`
	FinishedAt         *time.Time `json:"finished_at"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type TemplateDefinition struct {
	Nodes []TemplateNode `json:"nodes"`
	Edges []TemplateEdge `json:"edges"`
}

type TemplateNode struct {
	ID           string            `json:"id"`
	Type         string            `json:"type"`
	Name         string            `json:"name"`
	Required     bool              `json:"required"`
	GraphKey     string            `json:"graph_key,omitempty"`
	InputMapping map[string]string `json:"input_mapping,omitempty"`
	Role         string            `json:"role,omitempty"`
	Action       string            `json:"action,omitempty"`
	MaxRetries   int               `json:"max_retries,omitempty"`
}

type TemplateEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	When string `json:"when,omitempty"`
}

type CreateInstanceRequest struct {
	BusinessAppCode     string         `json:"business_app_code" binding:"required"`
	WorkflowTemplateKey string         `json:"workflow_template_key" binding:"required"`
	Title               string         `json:"title" binding:"required"`
	Input               map[string]any `json:"input"`
}

type CreateInstanceResponse struct {
	ID                      string `json:"id"`
	BusinessAppCode         string `json:"business_app_code"`
	WorkflowTemplateKey     string `json:"workflow_template_key"`
	WorkflowTemplateVersion string `json:"workflow_template_version"`
	GraphKey                string `json:"graph_key"`
	Title                   string `json:"title"`
	Status                  string `json:"status"`
	TraceID                 string `json:"trace_id"`
}

type StartResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type RetryRequest struct {
	NodeInstanceID string `json:"node_instance_id" binding:"required"`
}

type CancelRequest struct {
	Reason string `json:"reason"`
}

const (
	TaskTypeExecuteNode = "workflow:execute_node"
	WorkflowQueueName   = "workflow"
)

type ExecuteNodePayload struct {
	WorkflowInstanceID string `json:"workflow_instance_id"`
	NodeInstanceID     string `json:"node_instance_id"`
	NodeType           string `json:"node_type"`
	NodeKey            string `json:"node_key,omitempty"`
	GraphKey           string `json:"graph_key,omitempty"`
	TraceID            string `json:"trace_id"`
}
