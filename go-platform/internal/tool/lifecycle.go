package tool

import "fmt"

// M2-C: Tool Call 状态机。
//
//	requested ──→ executing ──→ succeeded
//	   │              │ │
//	   │              │ └──→ indeterminate ──→ succeeded/failed
//	   │              └────→ failed
//	   ↓
//	pending_approval ──(approved+重检通过)→ executing
//	   │
//	   └──(rejected/超时)→ cancelled
//
// 终态(succeeded/failed/cancelled)不允许再迁移;
// indeterminate 只能经 Reconcile 进入终态或保持观察。
var toolCallTransitions = map[ToolCallStatus][]ToolCallStatus{
	ToolCallStatusRequested:       {ToolCallStatusExecuting, ToolCallStatusFailed, ToolCallStatusCancelled},
	ToolCallStatusPendingApproval: {ToolCallStatusExecuting, ToolCallStatusFailed, ToolCallStatusCancelled},
	ToolCallStatusExecuting:       {ToolCallStatusSucceeded, ToolCallStatusFailed, ToolCallStatusIndeterminate},
	ToolCallStatusIndeterminate:   {ToolCallStatusSucceeded, ToolCallStatusFailed, ToolCallStatusIndeterminate},
	ToolCallStatusSucceeded:       {},
	ToolCallStatusFailed:          {ToolCallStatusExecuting}, // 仅 Retry Gate 放行
	ToolCallStatusCancelled:       {},
}

// ErrInvalidTransition 非法状态转换。
var ErrInvalidTransition = fmt.Errorf("invalid tool_call status transition")

// ErrApprovalPayloadMismatch 审批 Payload 与执行 Payload 不一致。
var ErrApprovalPayloadMismatch = fmt.Errorf("approval payload hash does not match tool_call input_hash")

// ErrApprovalNotDecided 审批任务尚未作出决定。
var ErrApprovalNotDecided = fmt.Errorf("approval task has not been decided")

// ErrRetryNotAllowed 当前状态或校验结果不允许重试。
var ErrRetryNotAllowed = fmt.Errorf("tool_call retry not allowed")

// ErrVerificationMissing 缺少执行确认前的外部验证记录。
var ErrVerificationMissing = fmt.Errorf("tool_call verification result required before reconcile")

// CanTransition 判断状态转换是否合法。
func CanTransition(from, to ToolCallStatus) bool {
	allowed, ok := toolCallTransitions[from]
	if !ok {
		return false
	}
	for _, t := range allowed {
		if t == to {
			return true
		}
	}
	return false
}

// validateTransition 返回非法转换错误(带上下文)。
func validateTransition(from, to ToolCallStatus) error {
	if CanTransition(from, to) {
		return nil
	}
	return fmt.Errorf("%w: %s → %s", ErrInvalidTransition, from, to)
}

// IsTerminal 判断是否终态。
func IsTerminal(s ToolCallStatus) bool {
	return s == ToolCallStatusSucceeded || s == ToolCallStatusFailed || s == ToolCallStatusCancelled
}
