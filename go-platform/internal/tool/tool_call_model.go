package tool

import "time"

// ToolCallStatus 工具调用生命周期状态。
type ToolCallStatus string

const (
	ToolCallStatusRequested        ToolCallStatus = "requested"
	ToolCallStatusPendingApproval  ToolCallStatus = "pending_approval"
	ToolCallStatusExecuting        ToolCallStatus = "executing"
	ToolCallStatusSucceeded        ToolCallStatus = "succeeded"
	ToolCallStatusFailed           ToolCallStatus = "failed"
	ToolCallStatusIndeterminate    ToolCallStatus = "indeterminate"
	ToolCallStatusCancelled        ToolCallStatus = "cancelled"
)

// ToolCallRequest 工具调用的入站请求(M2-B:受信任执行边界)。
// 调用方只能提供 tool_id/version/arguments,身份/Tenant/Domain/
// 由 Run 快照恢复,拒绝模型自报覆盖。
type ToolCallRequest struct {
	TenantID          string         `json:"tenant_id"`
	RunID             string         `json:"run_id"`
	StepID            string         `json:"step_id,omitempty"`
	ToolID            string         `json:"tool_id"`
	ToolVersion       string         `json:"tool_version,omitempty"`
	BusinessAppCode   string         `json:"business_app_code"` // M2-C:生命周期自包含
	ConnectorBindingID string        `json:"connector_binding_id"`
	PolicyVersion     string         `json:"policy_version"`
	RiskLevel         string         `json:"risk_level"`
	Status            ToolCallStatus `json:"status"`
	IdempotencyKey    string         `json:"idempotency_key"`
	InputHash         string         `json:"input_hash"`
	InputSummaryJSON   string         `json:"input_summary_json,omitempty"`
	ExternalRequestID string         `json:"external_request_id,omitempty"`
	TraceID           string         `json:"trace_id,omitempty"` // M2-E:全链 Trace
	TimeoutAt         *time.Time     `json:"timeout_at,omitempty"` // M2-C.8:executing 超时阈值
}

// ToolCall 对应 tool_calls 表,记录工具调用的完整执行契约。
type ToolCall struct {
	ID                string         `json:"id"`
	TenantID          string         `json:"tenant_id"`
	RunID             string         `json:"run_id"`
	StepID            *string        `json:"step_id,omitempty"`
	ToolID            string         `json:"tool_id"`
	ToolVersion       string         `json:"tool_version"`
	ConnectorBindingID *string       `json:"connector_binding_id,omitempty"`
	BusinessAppCode    string        `json:"business_app_code"`        // M2-C:业务域
	PolicyVersion     string         `json:"policy_version"`
	RiskLevel         string         `json:"risk_level"`
	Status            ToolCallStatus `json:"status"`
	IdempotencyKey    string         `json:"idempotency_key"`
	InputHash         string         `json:"input_hash"`
	InputSummaryJSON   *string       `json:"input_summary_json,omitempty"`
	ApprovalTaskID    *string        `json:"approval_task_id,omitempty"`
	ExternalRequestID *string        `json:"external_request_id,omitempty"`
	ExternalObjectID  *string        `json:"external_object_id,omitempty"`
	VerificationJSON  *string        `json:"verification_json,omitempty"`
	OutputSummaryJSON *string        `json:"output_summary_json,omitempty"`
	ErrorJSON         *string        `json:"error_json,omitempty"`
	TimeoutAt         *time.Time     `json:"timeout_at,omitempty"`  // M2-C: 超时阈值
	RetryCount        int            `json:"retry_count"`           // M2-C: 重试次数
	TraceID           string         `json:"trace_id,omitempty"`    // M2-E:全链 Trace
	IsDeadLetter      bool           `json:"is_dead_letter"`        // M2-C:DLQ
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}
