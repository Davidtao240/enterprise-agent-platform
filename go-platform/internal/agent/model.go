// Package agent 实现 Agent Registry、Graph Registry、Gateway 和 Agent Run Logs。
//
// 核心职责：
//   - Agent Registry：注册管理 Agent（agent_id、domain、capabilities、input/output schema）
//   - Graph Registry：注册管理 Python Agent Graph（graph_key → business_app 映射）
//   - Agent Gateway：统一调用入口，验证 → 调用 Python → 记录日志 → 返回结果
//   - Agent Run Logs：每次 Agent 执行的审计日志（token/cost/耗时/状态）
//   - Approval Tasks：human_review 节点的审批任务管理
//
// Gateway 调用链：
//   Worker → Gateway.Execute()
//     → 验证 graph_key（graph_registry）
//     → 校验 Domain Policy（domain_policies）
//     → 校验 Agent-Tool 权限（agent_tool_permissions）
//     → 调用 Python Agent Service（POST /internal/v1/agent-runs）
//     → 写入 agent_run_logs
//     → 返回结果给 Worker
package agent

import (
	"time"
)

// ── Agent Registry ──

// Agent 对应 agent_registry 表，描述一个可调用的 AI Agent。
type Agent struct {
	ID               string // UUID
	AgentID          string // 业务标识，如 "data_extract_agent"
	Name             string // 显示名称
	Domain           string // 所属域：finance / hr / legal / shared
	ReusableScope    string // domain_only / shared
	CapabilitiesJSON string // JSON 数组：["extract_table","parse_csv"]
	InputSchemaJSON  string // JSON Schema 格式的输入定义
	OutputSchemaJSON string // JSON Schema 格式的输出定义
	Endpoint         *string // 自定义 endpoint（可选）
	Status           string // active / disabled
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// ── Graph Registry ──

// Graph 对应 graph_registry 表，记录 Python Agent Graph 的注册信息。
type Graph struct {
	ID               string
	GraphKey         string // graph 唯一标识，如 "finance_operating_report_graph"
	BusinessAppCode  string // 所属业务
	Name             string // 显示名称
	Version          string // 版本号
	Description      *string
	Status           string // active / disabled / deprecated
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// ── Agent Run Log ──

// AgentRunLog 对应 agent_run_logs 表，记录单次 Agent 执行。
type AgentRunLog struct {
	ID                 string
	RunID              string    // 每次执行的唯一 ID
	TraceID            string    // 跨服务追踪 ID
	WorkflowInstanceID string    // 关联的工作流实例
	NodeInstanceID     string    // 关联的节点实例
	BusinessAppCode    string    // 业务域
	GraphKey           string    // 调用的 graph
	AgentID            *string   // 具体 agent（可选）
	Status             string    // succeeded / failed / retrying / cancelled
	InputSummaryJSON   *string   // 脱敏后的输入摘要
	OutputSummaryJSON  *string   // 脱敏后的输出摘要
	UsageJSON          *string   // {"model":"qwen-plus","prompt_tokens":1200,"completion_tokens":600,"cost":0.03}
	ErrorJSON          *string   // 错误详情
	StartedAt          *time.Time
	FinishedAt         *time.Time
	DurationMs         *int      // 执行耗时（毫秒）
}

// ── Approval Task ──

// ApprovalTask 对应 approval_tasks 表，记录人工审批任务。
type ApprovalTask struct {
	ID                 string
	WorkflowInstanceID string
	NodeInstanceID     string
	BusinessAppCode    string
	Title              string
	Status             string     // pending / approved / rejected / cancelled / expired
	AssigneeRole       *string    // 需要的审批角色
	AssigneeUserID     *string    // 指定审批人
	DecisionBy         *string    // 审批人
	DecisionComment    *string    // 审批意见
	DecidedAt          *time.Time // 决定时间
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// ── API 请求/响应 ──

// ListAgentsResponse Agent 列表项（不含完整 schema，减少传输量）。
type ListAgentsResponse struct {
	AgentID       string `json:"agent_id"`
	Name          string `json:"name"`
	Domain        string `json:"domain"`
	ReusableScope string `json:"reusable_scope"`
	Status        string `json:"status"`
}

// CreateAgentRequest POST /api/v1/agents
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

// ── Gateway 专用类型 ──

// AgentRunRequest Gateway 调用 Python Agent Service 的请求体。
type AgentRunRequest struct {
	TraceID            string         `json:"trace_id"`
	BusinessAppCode    string         `json:"business_app_code"`
	WorkflowTemplateKey string        `json:"workflow_template_key"`
	GraphKey           string         `json:"graph_key"`
	WorkflowInstanceID string         `json:"workflow_instance_id"`
	NodeInstanceID     string         `json:"node_instance_id"`
	Input              map[string]any `json:"input"`
	Context            AgentContext   `json:"context"`
}

// AgentContext 调用 Agent 时的上下文信息。
type AgentContext struct {
	UserID       string `json:"user_id"`
	DepartmentID string `json:"department_id,omitempty"`
	TenantID     string `json:"tenant_id,omitempty"`
}

// AgentRunResponse Python Agent Service 返回的响应体。
type AgentRunResponse struct {
	RunID    string              `json:"run_id"`
	GraphKey string              `json:"graph_key"`
	Status   string              `json:"status"`
	Output   map[string]any      `json:"output"`
	Usage    *AgentUsage         `json:"usage,omitempty"`
	Error    *AgentRunError      `json:"error,omitempty"`
}

// AgentUsage token 和成本统计。
type AgentUsage struct {
	Model            string  `json:"model"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	Cost             float64 `json:"cost"`
}

// AgentRunError Agent 执行错误信息。
type AgentRunError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

