// Package workflow 实现跨业务工作流引擎。
//
// 核心设计原则：
//   - 引擎只解释模板，不硬编码业务逻辑
//   - 所有流程差异通过 workflow_templates.definition_json 表达
//   - graph_key 显式路由到 Python Agent Graph，不靠 LLM 推断
//
// 模块分工：
//   - model.go    → 类型定义：数据库模型、状态常量、请求/响应体、模板 JSON 结构
//   - repository.go → 数据库 CRUD
//   - engine.go   → 状态机校验 + 模板解释器 + 节点调度
//   - service.go  → 业务逻辑：创建实例、启动、取消、重试、状态变更
//   - handler.go  → HTTP 端点
//   - worker.go   → Asynq 异步任务：入队和执行
package workflow

import (
	"time"
)

// ── 工作流实例状态常量 ──
// 对应 WORKFLOW_STATE_MACHINE.md 中定义的 8 种状态
const (
	StatusDraft         = "draft"          // 已创建，未启动
	StatusRunning       = "running"        // 执行中
	StatusWaitingReview = "waiting_review" // 等待人工审批
	StatusApproved      = "approved"       // 已批准
	StatusRejected      = "rejected"       // 已驳回
	StatusArchived      = "archived"       // 已归档（终态）
	StatusFailed        = "failed"         // 执行失败（终态）
	StatusCancelled     = "cancelled"      // 已取消（终态）
)

// ── 节点状态常量 ──
const (
	NodeStatusPending       = "pending"        // 未开始
	NodeStatusRunning       = "running"        // 执行中
	NodeStatusSucceeded     = "succeeded"      // 成功
	NodeStatusFailed        = "failed"         // 失败
	NodeStatusSkipped       = "skipped"        // 被跳过（边条件不满足）
	NodeStatusWaitingReview = "waiting_review" // 等待审批
	NodeStatusCancelled     = "cancelled"      // 已取消
)

// ── 节点类型常量 ──
const (
	NodeTypeFileUpload  = "file_upload"  // 文件上传节点
	NodeTypeAgentGraph  = "agent_graph"  // Agent 调用节点
	NodeTypeHumanReview = "human_review" // 人工审批节点
	NodeTypeSystem      = "system"       // 系统动作节点（归档、通知等）
)

// ── 模板状态常量 ──
const (
	TemplateStatusDraft      = "draft"
	TemplateStatusActive     = "active"
	TemplateStatusDeprecated = "deprecated"
	TemplateStatusDisabled   = "disabled"
)

// ── 边条件常量 ──
const (
	EdgeWhenSucceeded = "succeeded" // 上游节点成功 → 进入此下游
	EdgeWhenApproved  = "approved"  // 审批通过
	EdgeWhenRejected  = "rejected"  // 审批驳回
	EdgeWhenFailed    = "failed"    // 上游失败
)

// ── 数据库模型 ──

// Template 对应 workflow_templates 表。
type Template struct {
	ID                  string
	BusinessAppCode     string // 所属业务
	WorkflowTemplateKey string // 模板逻辑标识
	Name                string // 模板名称
	Version             string // 语义化版本
	GraphKey            string // 路由到 Python Agent Graph 的 key
	DefinitionJSON      string // JSONB 格式的 nodes + edges
	Status              string // draft / active / deprecated / disabled
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// Instance 对应 workflow_instances 表。
type Instance struct {
	ID                      string
	BusinessAppCode         string     // 业务域
	WorkflowTemplateID      string     // 关联的模板 ID
	WorkflowTemplateKey     string     // 模板 key 快照
	WorkflowTemplateVersion string     // 模板版本快照
	GraphKey                string     // graph_key 快照
	Title                   string     // 任务标题
	Status                  string     // 当前状态
	InputJSON               string     // 初始输入 JSON
	OutputJSON              *string    // 最终输出 JSON
	CreatedBy               string     // 创建人 UUID
	StartedAt               *time.Time // 实际开始时间
	FinishedAt              *time.Time // 完成时间
	TraceID                 string     // 跨服务追踪 ID
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

// NodeInstance 对应 workflow_node_instances 表。
type NodeInstance struct {
	ID                 string
	WorkflowInstanceID string     // 所属工作流实例
	NodeKey            string     // 对应模板 definition_json 中节点的 id
	NodeType           string     // file_upload / agent_graph / human_review / system
	Name               string     // 节点名称快照
	Status             string     // 当前状态
	InputJSON          *string    // 节点输入
	OutputJSON         *string    // 节点输出
	ErrorJSON          *string    // 错误详情
	RetryCount         int        // 已重试次数
	MaxRetries         int        // 最大重试次数
	StartedAt          *time.Time // 开始执行时间
	FinishedAt         *time.Time // 结束时间
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// ── 模板 definition_json 的 Go 结构（用于解析 JSONB） ──

// TemplateDefinition 是 workflow_templates.definition_json 的结构化表示。
type TemplateDefinition struct {
	Nodes []TemplateNode `json:"nodes"` // 节点列表
	Edges []TemplateEdge `json:"edges"` // 边（连接 + 条件）
}

// TemplateNode 模板中的单个节点定义。
type TemplateNode struct {
	ID       string `json:"id"`       // 节点标识，如 "upload", "agent_graph"
	Type     string `json:"type"`     // file_upload / agent_graph / human_review / system
	Name     string `json:"name"`     // 显示名称
	Required bool   `json:"required"` // 是否必须

	// agent_graph 专用
	GraphKey     string            `json:"graph_key,omitempty"`     // 路由到哪个 Python Graph
	InputMapping map[string]string `json:"input_mapping,omitempty"` // 输入映射（占位符 → 实际值）

	// human_review 专用
	Role string `json:"role,omitempty"` // 需要的审批角色

	// system 专用
	Action string `json:"action,omitempty"` // 系统动作，如 "archive_result"

	// 重试配置
	MaxRetries int `json:"max_retries,omitempty"` // 最大重试次数
}

// TemplateEdge 定义节点间的流转关系。
type TemplateEdge struct {
	From string `json:"from"`           // 上游节点 id
	To   string `json:"to"`             // 下游节点 id
	When string `json:"when,omitempty"` // 条件：succeeded / approved / rejected / failed（空 = 无条件）
}

// ── API 请求/响应体 ──

// CreateInstanceRequest POST /api/v1/workflow-instances
type CreateInstanceRequest struct {
	BusinessAppCode      string `json:"business_app_code" binding:"required"`       // 业务域
	WorkflowTemplateKey  string `json:"workflow_template_key" binding:"required"`   // 模板 key
	Title                string `json:"title" binding:"required"`                   // 任务标题
	Input                map[string]any `json:"input"`                              // 初始输入
}

// CreateInstanceResponse 创建实例后的返回值。
type CreateInstanceResponse struct {
	ID                     string `json:"id"`
	BusinessAppCode        string `json:"business_app_code"`
	WorkflowTemplateKey    string `json:"workflow_template_key"`
	WorkflowTemplateVersion string `json:"workflow_template_version"`
	GraphKey               string `json:"graph_key"`
	Title                  string `json:"title"`
	Status                 string `json:"status"`
	TraceID                string `json:"trace_id"`
}

// StartResponse 启动/取消/重试 的通用响应。
type StartResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// RetryRequest POST /api/v1/workflow-instances/{id}/retry
type RetryRequest struct {
	NodeInstanceID string `json:"node_instance_id" binding:"required"` // 要重试的节点 ID
}

// CancelRequest POST /api/v1/workflow-instances/{id}/cancel
type CancelRequest struct {
	Reason string `json:"reason"` // 取消原因
}

// ── Asynq 任务 ──

const (
	// TaskTypeExecuteNode 异步执行工作流节点的任务类型。
	// By Asynq worker 消费，payload 为 ExecuteNodePayload。
	TaskTypeExecuteNode = "workflow:execute_node"
)

// ExecuteNodePayload Asynq 任务载荷：要执行的节点信息。
type ExecuteNodePayload struct {
	WorkflowInstanceID string `json:"workflow_instance_id"`
	NodeInstanceID     string `json:"node_instance_id"`
	NodeType           string `json:"node_type"`
	NodeKey            string `json:"node_key,omitempty"`  // 模板中的节点 id，如 "human_review"
	GraphKey           string `json:"graph_key,omitempty"` // agent_graph 节点用
	TraceID            string `json:"trace_id"`
}
