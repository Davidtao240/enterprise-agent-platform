package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/enterprise-agent-platform/go-platform/internal/audit"
)

// ── M2-C: Tool Execution Lifecycle ──
//
// 完整链路:
//
//	high-risk Execute → pending_approval
//	  → RequestApproval(绑定不可变 input_hash)
//	  → 人工决定 → BindToolCallApprovalDecision(approved)
//	      → 执行前重检(tool 版本/域策略仍有效) → executing
//	  → ConfirmExecution → succeeded / failed / indeterminate(timeout)
//	  → VerifyResult(外部状态验证) → Reconcile(indeterminate → 终态)
//
// Retry Gate:仅 failed 且验证确认未执行时放行(failed → executing)。

// approvalStore Tool Call 审批持久化接口。
type approvalStore interface {
	CreateToolCallApproval(ctx context.Context, approval *ToolCallApproval) error
	FindToolCallApproval(ctx context.Context, toolCallID string) (*ToolCallApproval, error)
	UpdateDecision(ctx context.Context, id, decision, decisionBy string) error
}

// RequestApproval 为 pending_approval 的 Tool Call 创建审批任务。
// PayloadHash 绑定 tool_call.input_hash:审批内容与执行内容不可分离。
func (s *Service) RequestApproval(ctx context.Context, toolCallID, title string) (*ToolCallApproval, error) {
	if s.approvalRepo == nil {
		return nil, fmt.Errorf("approval repository not configured")
	}
	tc, err := s.toolCallRepo.GetByID(ctx, toolCallID)
	if err != nil {
		return nil, err
	}
	if tc.Status != ToolCallStatusPendingApproval {
		return nil, fmt.Errorf("%w: %s → request_approval (expect pending_approval)", ErrInvalidTransition, tc.Status)
	}
	if title == "" {
		title = fmt.Sprintf("高风险工具调用审批:%s@%s", tc.ToolID, tc.ToolVersion)
	}
	approval := &ToolCallApproval{
		ToolCallID:      tc.ID,
		BusinessAppCode: tc.BusinessAppCode,
		Title:           title,
		PayloadHash:     tc.InputHash,
	}
	if err := s.approvalRepo.CreateToolCallApproval(ctx, approval); err != nil {
		return nil, fmt.Errorf("create tool call approval: %w", err)
	}
	// 回写 approval_task_id
	if err := s.toolCallRepo.UpdateStatus(ctx, tc.ID, tc.Status, map[string]any{
		"approval_task_id": approval.ID,
	}); err != nil {
		return nil, fmt.Errorf("bind approval_task_id: %w", err)
	}
	s.auditLifecycle(ctx, tc, "tool_call.approval_requested", map[string]any{
		"approval_task_id": approval.ID,
		"payload_hash":     approval.PayloadHash,
	})
	return approval, nil
}

// BindToolCallApprovalDecision 审批决定绑定(M2-C 核心)。
// approved:先重检(工具仍 active + 版本一致 + 域策略仍允许)再进入 executing;
// rejected:进入 cancelled。PayloadHash 必须与 input_hash 一致。
// 该方法同时实现 agent.ToolCallDecisionBinder 接口,由审批 UI 决策路径回调。
func (s *Service) BindToolCallApprovalDecision(ctx context.Context, toolCallID, decision string) error {
	if s.approvalRepo == nil {
		return fmt.Errorf("approval repository not configured")
	}
	if decision != "approved" && decision != "rejected" {
		return fmt.Errorf("unsupported approval decision: %s", decision)
	}
	tc, err := s.toolCallRepo.GetByID(ctx, toolCallID)
	if err != nil {
		return err
	}
	if tc.Status != ToolCallStatusPendingApproval {
		// 幂等:已绑定过则直接成功(approved→executing/succeeded/failed;rejected→cancelled)
		if decision == "approved" && (tc.Status == ToolCallStatusExecuting ||
			tc.Status == ToolCallStatusSucceeded || tc.Status == ToolCallStatusFailed) {
			return nil
		}
		if decision == "rejected" && tc.Status == ToolCallStatusCancelled {
			return nil
		}
		return fmt.Errorf("%w: %s → bind_decision (expect pending_approval)", ErrInvalidTransition, tc.Status)
	}

	approval, err := s.approvalRepo.FindToolCallApproval(ctx, toolCallID)
	if err != nil {
		return fmt.Errorf("find approval: %w", err)
	}
	if approval.Status != "approved" && approval.Status != "rejected" {
		return fmt.Errorf("%w (status=%s)", ErrApprovalNotDecided, approval.Status)
	}
	if approval.Status != decision {
		return fmt.Errorf("approval decision mismatch: task=%s request=%s", approval.Status, decision)
	}
	// 审批 Payload 与执行 Payload 完全一致(不可变绑定)
	if approval.PayloadHash != tc.InputHash {
		return ErrApprovalPayloadMismatch
	}

	if decision == "rejected" {
		if err := s.toolCallRepo.UpdateStatusGuarded(ctx, tc.ID, ToolCallStatusPendingApproval, ToolCallStatusCancelled, nil); err != nil {
			return err
		}
		s.auditLifecycle(ctx, tc, "tool_call.approval_rejected", map[string]any{
			"approval_task_id": approval.ID,
		})
		return nil
	}

	// 执行前重检:审批期间工具定义/域策略可能已变化
	if err := s.recheckAuthorization(ctx, tc); err != nil {
		if bindErr := s.toolCallRepo.UpdateStatusGuarded(ctx, tc.ID, ToolCallStatusPendingApproval, ToolCallStatusFailed, map[string]any{
			"error_json": mustJSON(map[string]any{
				"code":    "TOOL_PRECHECK_FAILED",
				"message": err.Error(),
			}),
		}); bindErr != nil {
			return fmt.Errorf("precheck failed (%v); mark failed: %w", err, bindErr)
		}
		return fmt.Errorf("pre-execution recheck failed: %w", err)
	}

	if err := s.toolCallRepo.UpdateStatusGuarded(ctx, tc.ID, ToolCallStatusPendingApproval, ToolCallStatusExecuting, map[string]any{
		"approval_task_id": approval.ID,
		"timeout_at":       s.executionDeadline(),
	}); err != nil {
		return err
	}
	s.auditLifecycle(ctx, tc, "tool_call.approval_bound", map[string]any{
		"approval_task_id": approval.ID,
		"decision":         decision,
	})

	// M3-A:审批通过后,若有 Connector Binding,自动驱动外部执行
	if tc.ConnectorBindingID != nil && *tc.ConnectorBindingID != "" && s.connectorRT != nil {
		finalStatus, execErr := s.autoExecuteViaConnector(ctx, tc)
		if execErr != nil {
			log.Printf("[tool-svc] post-approval connector execute failed for %s: %v (will be handled by timeout scanner)", tc.ID, execErr)
		} else {
			log.Printf("[tool-svc] post-approval connector execution: tool_call=%s status=%s", tc.ID, finalStatus)
		}
	}
	return nil
}

// ConfirmExecutionRequest 执行结果确认请求。
type ConfirmExecutionRequest struct {
	Status            ToolCallStatus `json:"status"` // succeeded / failed / indeterminate
	OutputSummaryJSON string         `json:"output_summary_json,omitempty"`
	ErrorJSON         string         `json:"error_json,omitempty"`
	ExternalRequestID string         `json:"external_request_id,omitempty"`
	ExternalObjectID  string         `json:"external_object_id,omitempty"`
	VerificationJSON  string         `json:"verification_json,omitempty"`
}

// ConfirmExecution 写入执行结果,推进 executing → 终态。
// timeout 场景应报 indeterminate(不是 failed),后续走 Reconcile。
func (s *Service) ConfirmExecution(ctx context.Context, toolCallID string, req *ConfirmExecutionRequest) (*ToolCall, error) {
	tc, err := s.toolCallRepo.GetByID(ctx, toolCallID)
	if err != nil {
		return nil, err
	}
	if err := validateTransition(tc.Status, req.Status); err != nil {
		return nil, err
	}
	fields := map[string]any{}
	if req.OutputSummaryJSON != "" {
		fields["output_summary_json"] = req.OutputSummaryJSON
	}
	if req.ErrorJSON != "" {
		fields["error_json"] = req.ErrorJSON
	}
	if req.ExternalRequestID != "" {
		fields["external_request_id"] = req.ExternalRequestID
	}
	if req.ExternalObjectID != "" {
		fields["external_object_id"] = req.ExternalObjectID
	}
	if req.VerificationJSON != "" {
		fields["verification_json"] = req.VerificationJSON
	}
	if err := s.toolCallRepo.UpdateStatusGuarded(ctx, tc.ID, tc.Status, req.Status, fields); err != nil {
		return nil, err
	}
	// M2-C.9:执行结果驱动熔断状态机(indeterminate 不计:外部结果未知)
	if s.circuit != nil {
		s.circuit.RecordResult(ctx, tc.ToolID, string(req.Status))
	}
	s.auditLifecycle(ctx, tc, "tool_call.confirmed", map[string]any{
		"final_status": req.Status,
	})
	return s.toolCallRepo.GetByID(ctx, toolCallID)
}

// VerifyResult 持久化外部状态验证结果(Reconcile 前置)。
// 不改变状态;Timeout/Indeterminate 必须先 Verify 再 Reconcile。
func (s *Service) VerifyResult(ctx context.Context, toolCallID, verificationJSON string) (*ToolCall, error) {
	tc, err := s.toolCallRepo.GetByID(ctx, toolCallID)
	if err != nil {
		return nil, err
	}
	if IsTerminal(tc.Status) {
		return nil, fmt.Errorf("%w: cannot verify terminal tool_call (%s)", ErrInvalidTransition, tc.Status)
	}
	if err := s.toolCallRepo.UpdateStatus(ctx, tc.ID, tc.Status, map[string]any{
		"verification_json": verificationJSON,
	}); err != nil {
		return nil, err
	}
	s.auditLifecycle(ctx, tc, "tool_call.verified", nil)
	return s.toolCallRepo.GetByID(ctx, toolCallID)
}

// ReconcileOutcome 对账结论。
type ReconcileOutcome string

const (
	// ReconcileExecuted 外部已成功执行 → succeeded
	ReconcileExecuted ReconcileOutcome = "executed"
	// ReconcileNotExecuted 外部确认未执行 → failed(可按 Policy 重试)
	ReconcileNotExecuted ReconcileOutcome = "not_executed"
	// ReconcileStillUnknown 仍无法确定 → 保持 indeterminate(人工处理)
	ReconcileStillUnknown ReconcileOutcome = "still_unknown"
)

// Reconcile 对账:indeterminate(或 executing 超时)→ 终态。
// 要求先有 verification_json(外部状态验证证据)。
func (s *Service) Reconcile(ctx context.Context, toolCallID string, outcome ReconcileOutcome, detailJSON string) (*ToolCall, error) {
	tc, err := s.toolCallRepo.GetByID(ctx, toolCallID)
	if err != nil {
		return nil, err
	}
	if tc.Status != ToolCallStatusIndeterminate && tc.Status != ToolCallStatusExecuting {
		return nil, fmt.Errorf("%w: reconcile from %s (expect indeterminate/executing)", ErrInvalidTransition, tc.Status)
	}
	if tc.VerificationJSON == nil || *tc.VerificationJSON == "" {
		return nil, ErrVerificationMissing
	}

	var finalStatus ToolCallStatus
	var errorCode string
	switch outcome {
	case ReconcileExecuted:
		finalStatus = ToolCallStatusSucceeded
	case ReconcileNotExecuted:
		finalStatus = ToolCallStatusFailed
		errorCode = "TOOL_NOT_EXECUTED"
	case ReconcileStillUnknown:
		finalStatus = ToolCallStatusIndeterminate // 保持观察,等待人工
	default:
		return nil, fmt.Errorf("unsupported reconcile outcome: %s", outcome)
	}

	fields := map[string]any{
		"verification_json": mergeVerification(tc.VerificationJSON, detailJSON, outcome),
	}
	if errorCode != "" {
		fields["error_json"] = mustJSON(map[string]any{
			"code":   errorCode,
			"source": "reconcile",
		})
	}
	if err := s.toolCallRepo.UpdateStatusGuarded(ctx, tc.ID, tc.Status, finalStatus, fields); err != nil {
		return nil, err
	}
	s.auditLifecycle(ctx, tc, "tool_call.reconciled", map[string]any{
		"outcome":      outcome,
		"final_status": finalStatus,
	})
	return s.toolCallRepo.GetByID(ctx, toolCallID)
}

// Retry 重试门:仅 failed 且外部验证确认未执行时放行。
// 高风险工具重试需重新审批(回到 pending_approval)。
// M2-C.10:超过 maxRetry 上限 → 死信(DLQ),拒绝重试并审计。
func (s *Service) Retry(ctx context.Context, toolCallID string) (*ToolCall, error) {
	tc, err := s.toolCallRepo.GetByID(ctx, toolCallID)
	if err != nil {
		return nil, err
	}
	if tc.Status != ToolCallStatusFailed {
		return nil, fmt.Errorf("%w: retry from %s (expect failed)", ErrRetryNotAllowed, tc.Status)
	}
	if tc.IsDeadLetter {
		return nil, fmt.Errorf("%w: tool_call already in dead letter queue", ErrRetryNotAllowed)
	}
	if tc.VerificationJSON == nil || *tc.VerificationJSON == "" {
		return nil, fmt.Errorf("%w: no verification evidence", ErrRetryNotAllowed)
	}
	var verification struct {
		Executed *bool `json:"executed"`
	}
	if err := json.Unmarshal([]byte(*tc.VerificationJSON), &verification); err != nil ||
		verification.Executed == nil || *verification.Executed {
		// 缺少"确认未执行"证据时拒绝重试
		return nil, fmt.Errorf("%w: verification must confirm not executed", ErrRetryNotAllowed)
	}

	// M2-C.10:重试上限 → DLQ
	if s.maxRetry > 0 && tc.RetryCount >= s.maxRetry {
		if err := s.toolCallRepo.UpdateStatusGuarded(ctx, tc.ID, ToolCallStatusFailed, ToolCallStatusFailed, map[string]any{
			"is_dead_letter": true,
			"error_json": mustJSON(map[string]any{
				"code":        "TOOL_RETRY_EXHAUSTED",
				"message":     fmt.Sprintf("retry_count reached max %d; moved to dead letter queue", s.maxRetry),
				"retry_count": tc.RetryCount,
			}),
		}); err != nil {
			return nil, err
		}
		s.auditLifecycle(ctx, tc, "tool_call.dead_letter", map[string]any{
			"retry_count": tc.RetryCount,
			"max_retry":   s.maxRetry,
		})
		return nil, fmt.Errorf("%w: retry_count %d reached max %d (dead letter)",
			ErrRetryNotAllowed, tc.RetryCount, s.maxRetry)
	}

	nextStatus := ToolCallStatusExecuting
	if tc.RiskLevel == "high" {
		// 高风险重试 = 新一次执行,必须重新审批
		nextStatus = ToolCallStatusPendingApproval
	}
	fields := map[string]any{
		"retry_count": tc.RetryCount + 1,
	}
	if nextStatus == ToolCallStatusExecuting {
		fields["timeout_at"] = s.executionDeadline()
	}
	if err := s.toolCallRepo.UpdateStatusGuarded(ctx, tc.ID, ToolCallStatusFailed, nextStatus, fields); err != nil {
		return nil, err
	}
	s.auditLifecycle(ctx, tc, "tool_call.retry", map[string]any{
		"retry_count": tc.RetryCount + 1,
		"next_status": nextStatus,
	})
	return s.toolCallRepo.GetByID(ctx, toolCallID)
}

// executionDeadline 当前时刻 + 默认超时(未配置返回 nil,不设超时)。
func (s *Service) executionDeadline() *time.Time {
	if s.defaultTimeout <= 0 {
		return nil
	}
	t := time.Now().UTC().Add(s.defaultTimeout)
	return &t
}

// recheckAuthorization 执行前重检:工具仍 active、版本一致、域策略仍允许。
func (s *Service) recheckAuthorization(ctx context.Context, tc *ToolCall) error {
	tool, err := s.toolRepo.FindToolByIDAndVersion(ctx, tc.ToolID, tc.ToolVersion)
	if err != nil || tool.Status != "active" {
		return fmt.Errorf("%w: %s@%s no longer active", ErrToolNotFound, tc.ToolID, tc.ToolVersion)
	}
	if s.domainPolicy != nil && !tool.IsShared {
		allowedDomains, err := s.domainPolicy.FindAllowedDomains(ctx, tc.BusinessAppCode)
		if err == nil && !containsDomain(allowedDomains, tool.Domain) {
			return fmt.Errorf("%w during precheck", ErrDomainPolicyViolation)
		}
	}
	return nil
}

// auditLifecycle 生命周期审计(脱敏摘要)。
// 当 tc.ID 为空时(如熔断拒绝无 tool_call 记录),resource_id 使用
// "run:<run_id>" 占位,避免空值约束导致插入失败。
func (s *Service) auditLifecycle(ctx context.Context, tc *ToolCall, action string, extra map[string]any) {
	if s.auditLogger == nil {
		return
	}
	resourceID := tc.ID
	if resourceID == "" {
		resourceID = "run:" + tc.RunID
	}
	payload := map[string]any{
		"tool_call_id": tc.ID,
		"tool_id":      tc.ToolID,
		"tool_version": tc.ToolVersion,
		"risk_level":   tc.RiskLevel,
		"status":       tc.Status,
	}
	for k, v := range extra {
		payload[k] = v
	}
	detailStr := string(mustJSON(payload))
	businessApp := tc.BusinessAppCode
	// M2-E:Trace 优先用显式 trace_id(Workflow→Run→ToolCall 传递);
	// 缺省回落 RunID,保证链路不中断。
	traceID := tc.TraceID
	if traceID == "" {
		traceID = tc.RunID
	}
	if _, _, err := s.auditLogger.InsertLog(ctx, audit.AuditLogEntry{
		TraceID:         traceID,
		BusinessAppCode: &businessApp,
		Action:          action,
		ResourceType:    "tool_call",
		ResourceID:      resourceID,
		Status:          "success",
		DetailJSON:      &detailStr,
	}); err != nil {
		log.Printf("[tool-svc] lifecycle audit %s failed: %v", action, err)
	}
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}

// mergeVerification 合并已有验证证据与对账结论。
func mergeVerification(existing *string, detailJSON string, outcome ReconcileOutcome) string {
	merged := map[string]any{}
	if existing != nil && *existing != "" {
		_ = json.Unmarshal([]byte(*existing), &merged)
	}
	if detailJSON != "" {
		var detail map[string]any
		if json.Unmarshal([]byte(detailJSON), &detail) == nil {
			merged["reconcile_detail"] = detail
		} else {
			merged["reconcile_detail"] = detailJSON
		}
	}
	merged["reconcile_outcome"] = string(outcome)
	merged["reconciled_at"] = time.Now().UTC().Format(time.RFC3339)
	return string(mustJSON(merged))
}
