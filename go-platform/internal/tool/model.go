// Package tool 实现 Tool Registry 和 Agent-Tool 权限管理。
//
// Tool Registry 记录所有可用工具（如 parse_csv、validate_finance_metrics）。
// Agent-Tool Permission 定义哪些 Agent 在哪些业务下可以调用哪些 Tool。
//
// 权限校验在 Agent Gateway 中完成：
//   Gateway.Execute() → 查 agent_tool_permissions → 确认 agent 有权使用 tool → 调用 Python
package tool

import "time"

// Tool 对应 tool_registry 表，描述一个可用的工具。
type Tool struct {
	ID               string    // UUID
	ToolID           string    // 业务标识，如 "parse_csv"
	Name             string    // 显示名称，如 "Parse CSV"
	Domain           string    // 所属域：finance / hr / legal / shared
	RiskLevel        string    // 风险等级：low / medium / high
	IsShared         bool      // 是否跨域共享
	InputSchemaJSON  string    // JSON Schema 输入定义
	OutputSchemaJSON string    // JSON Schema 输出定义
	Status           string    // active / disabled
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// AgentToolPermission 对应 agent_tool_permissions 表。
type AgentToolPermission struct {
	AgentID          string // 授权哪个 Agent
	ToolID           string // 允许使用哪个 Tool
	BusinessAppCode  string // 在哪个业务下可用
	Status           string // active / disabled
}
