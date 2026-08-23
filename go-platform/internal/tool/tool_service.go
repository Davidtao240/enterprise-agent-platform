package tool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/enterprise-agent-platform/go-platform/internal/audit"
)

// Service Tool Execution Service:可信执行边界。
// 职责:
//  1. 校验 tool_id + version 存在且 active
//  2. 校验 agent 在该业务下有该 Tool 的权限
//  3. 校验 domain policy 允许该 Tool 的 domain
//  4. 评估 risk level 与审批要求
//  5. 检查幂等键(tenant 范围内去重)
//  6. 创建 tool_call 记录并返回
//
// 信任边界:模型只能提供 tool_id/version/arguments,
//  身份/Tenant/Agent/Skill/Workflow 由调用方注入,拒绝模型自报覆盖。
type Service struct {
	toolRepo      toolResolver
	toolCallRepo  toolCallStore
	auditLogger   toolAuditLogger
	riskEvaluator riskLevelEvaluator
	approvalRepo  approvalStore   // M2-C:审批任务持久化
	domainPolicy  DomainPolicyProvider // M2-C:执行前重检用
	circuit       *CircuitBreaker      // M2-C.9:熔断器
	defaultTimeout time.Duration       // M2-C.8:executing 默认超时
	maxRetry       int                 // M2-C.10:DLQ 阈值
	bindingRepo   bindingValidator     // M2-D:Connector Binding 校验
	connectorRT   *ConnectorRuntime    // M3-A:Connector Runtime(驱动外部系统执行)
	outboxRepo    outboxEnqueuer       // M3-C:Outbox 投递(Saga 写操作)
	tracer        toolTraceSink        // M5-A:L4 Tool Call Trace
}

// toolResolver 工具查找接口(版本校验 + 工具解析)。
type toolResolver interface {
	FindToolByIDAndVersion(ctx context.Context, toolID, version string) (*Tool, error)
	FindToolByID(ctx context.Context, toolID string) (*Tool, error)
}

// toolCallStore 工具调用持久化接口。
type toolCallStore interface {
	Create(ctx context.Context, req *ToolCallRequest) (*ToolCall, error)
	GetByID(ctx context.Context, id string) (*ToolCall, error)
	GetByIdempotencyKey(ctx context.Context, tenantID, idempotencyKey string) (*ToolCall, error)
	UpdateStatus(ctx context.Context, id string, status ToolCallStatus, fields map[string]any) error
	UpdateStatusGuarded(ctx context.Context, id string, expectedFrom, status ToolCallStatus, fields map[string]any) error
	ListByRun(ctx context.Context, runID string) ([]*ToolCall, error)
	ListByTrace(ctx context.Context, traceID string) ([]*ToolCall, error)
	ListDeadLetters(ctx context.Context, limit int) ([]*ToolCall, error)
}

type toolAuditLogger interface {
	InsertLog(ctx context.Context, entry audit.AuditLogEntry) (string, time.Time, error)
}

type riskLevelEvaluator interface {
	EvaluateRisk(runRiskLevel, toolRiskLevel string) (string, bool)
}

// DomainPolicyProvider 提供业务域策略查询。
type DomainPolicyProvider interface {
	FindAllowedDomains(ctx context.Context, businessAppCode string) ([]string, error)
}

// AgentPermissionProvider 提供 Agent-Tool 权限查询。
type AgentPermissionProvider interface {
	HasToolPermission(ctx context.Context, agentID, toolID, businessAppCode string) (bool, error)
}

// bindingValidator M2-D:Connector Binding 校验(存在/active/租户匹配)。
type bindingValidator interface {
	FindBinding(ctx context.Context, id string) (*ConnectorBinding, error)
}

// ServiceOption 服务构造选项。
type ServiceOption func(*Service)

// WithRiskEvaluator 注入风险评估器。
func WithRiskEvaluator(e riskLevelEvaluator) ServiceOption {
	return func(s *Service) { s.riskEvaluator = e }
}

// WithApprovalRepository 注入审批任务仓储(M2-C)。
func WithApprovalRepository(a approvalStore) ServiceOption {
	return func(s *Service) { s.approvalRepo = a }
}

// WithDomainPolicy 注入域策略提供者(M2-C:执行前重检)。
func WithDomainPolicy(p DomainPolicyProvider) ServiceOption {
	return func(s *Service) { s.domainPolicy = p }
}

// WithCircuitBreaker 注入熔断器(M2-C.9)。
func WithCircuitBreaker(cb *CircuitBreaker) ServiceOption {
	return func(s *Service) { s.circuit = cb }
}

// WithReliabilityPolicy 注入可靠性参数(M2-C.8/10:超时与 DLQ 阈值)。
func WithReliabilityPolicy(defaultTimeout time.Duration, maxRetry int) ServiceOption {
	return func(s *Service) {
		if defaultTimeout > 0 {
			s.defaultTimeout = defaultTimeout
		}
		if maxRetry > 0 {
			s.maxRetry = maxRetry
		}
	}
}

// WithBindingValidator 注入 Connector Binding 校验器(M2-D)。
func WithBindingValidator(v bindingValidator) ServiceOption {
	return func(s *Service) { s.bindingRepo = v }
}

// WithConnectorRuntime 注入 Connector Runtime(M3-A:外部系统调用执行器)。
func WithConnectorRuntime(rt *ConnectorRuntime) ServiceOption {
	return func(s *Service) { s.connectorRT = rt }
}

// outboxEnqueuer M3-C:Outbox 投递依赖(便于测试替换)。
type outboxEnqueuer interface {
	Enqueue(ctx context.Context, e *OutboxEntry) (*OutboxEntry, error)
}

// WithOutboxRepository 注入 Outbox 仓储(M3-C:Saga 写操作经 Outbox 投递)。
func WithOutboxRepository(r outboxEnqueuer) ServiceOption {
	return func(s *Service) { s.outboxRepo = r }
}

// NewService 创建 Tool Execution Service。
func NewService(toolRepo toolResolver, toolCallRepo toolCallStore, auditLogger toolAuditLogger, opts ...ServiceOption) *Service {
	s := &Service{
		toolRepo:     toolRepo,
		toolCallRepo: toolCallRepo,
		auditLogger:  auditLogger,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// ErrToolNotFound 工具不存在或非 active。
var ErrToolNotFound = errors.New("tool not found or inactive")

// ErrToolNotAllowed 该 Agent 无权使用该 Tool。
var ErrToolNotAllowed = errors.New("tool not allowed for agent in this business")

// ErrDomainPolicyViolation 跨域调用被 Domain Policy 拦截。
var ErrDomainPolicyViolation = errors.New("tool domain violates domain policy")

// ErrToolVersionMismatch 指定版本的工具不存在。
var ErrToolVersionMismatch = errors.New("tool version mismatch")

// ExecuteRequest 工具执行请求(从 Run 快照恢复的受信任身份)。
type ExecuteRequest struct {
	TenantID          string            `json:"tenant_id"`
	RunID             string            `json:"run_id"`
	StepID            string            `json:"step_id,omitempty"`
	AgentID           string            `json:"agent_id"`
	BusinessAppCode   string            `json:"business_app_code"`
	ToolID            string            `json:"tool_id"`
	ToolVersion       string            `json:"tool_version,omitempty"`
	ConnectorBindingID string           `json:"connector_binding_id"`
	PolicyVersion     string            `json:"policy_version"`
	ArgumentsJSON     string            `json:"arguments_json"`
	IdempotencyKey    string            `json:"idempotency_key"`
	TraceID           string            `json:"trace_id,omitempty"`
}

// ExecuteResult 工具执行结果。
type ExecuteResult struct {
	ToolCallID   string         `json:"tool_call_id"`
	Status       ToolCallStatus `json:"status"`
	RiskLevel    string         `json:"risk_level"`
	ApprovalReq  bool           `json:"approval_required"`
	ErrorMessage string         `json:"error_message,omitempty"`
}

// Execute 执行工具调用的受信任边界校验与创建。
// 完整的执行契约:版本校验→权限→域策略→风险→幂等→创建记录。
func (s *Service) Execute(ctx context.Context, req *ExecuteRequest, domainPolicy DomainPolicyProvider, permProvider AgentPermissionProvider) (*ExecuteResult, error) {
	// 1) 版本校验:tool_id + version 必须存在且 active
	tool, err := s.resolveTool(ctx, req.ToolID, req.ToolVersion)
	if err != nil {
		return &ExecuteResult{
			Status:       ToolCallStatusFailed,
			RiskLevel:    "", // 工具未解析时风险未知
			ErrorMessage: err.Error(),
		}, err
	}

	// 2) Agent-Tool 权限校验
	if permProvider != nil {
		allowed, err := permProvider.HasToolPermission(ctx, req.AgentID, req.ToolID, req.BusinessAppCode)
		if err != nil {
			return nil, fmt.Errorf("check agent tool permission: %w", err)
		}
		if !allowed {
			return &ExecuteResult{
				Status:       ToolCallStatusFailed,
				RiskLevel:    tool.RiskLevel,
				ErrorMessage: ErrToolNotAllowed.Error(),
			}, ErrToolNotAllowed
		}
	}

	// 3) Domain Policy 校验
	if domainPolicy != nil && !tool.IsShared {
		allowedDomains, err := domainPolicy.FindAllowedDomains(ctx, req.BusinessAppCode)
		if err != nil {
			return nil, fmt.Errorf("check domain policy: %w", err)
		}
		if !containsDomain(allowedDomains, tool.Domain) {
			return &ExecuteResult{
				Status:       ToolCallStatusFailed,
				RiskLevel:    tool.RiskLevel,
				ErrorMessage: ErrDomainPolicyViolation.Error(),
			}, ErrDomainPolicyViolation
		}
	}

	// 3.5) M2-D:Connector Binding 校验(指定绑定时必须存在/active/租户匹配)
	if req.ConnectorBindingID != "" && s.bindingRepo != nil {
		binding, err := s.bindingRepo.FindBinding(ctx, req.ConnectorBindingID)
		if err != nil {
			return &ExecuteResult{
				Status:       ToolCallStatusFailed,
				RiskLevel:    tool.RiskLevel,
				ErrorMessage: ErrConnectorBindingNotFound.Error(),
			}, ErrConnectorBindingNotFound
		}
		if binding.TenantID != req.TenantID {
			return &ExecuteResult{
				Status:       ToolCallStatusFailed,
				RiskLevel:    tool.RiskLevel,
				ErrorMessage: ErrConnectorBindingNotFound.Error(),
			}, ErrConnectorBindingNotFound // 跨租户视同不存在
		}
		if binding.Status != "active" {
			return &ExecuteResult{
				Status:       ToolCallStatusFailed,
				RiskLevel:    tool.RiskLevel,
				ErrorMessage: ErrConnectorBindingDisabled.Error(),
			}, ErrConnectorBindingDisabled
		}
	}

	// 4) M2-C.9:熔断检查(open 拒绝新调用,不产生新的外部副作用)
	if s.circuit != nil {
		allow, state, _ := s.circuit.Allow(ctx, req.ToolID)
		if !allow {
			// 审计:熔断拒绝无 tool_call 记录,使用占位 ID 避免空值约束失败
			s.auditLifecycle(ctx, &ToolCall{
				ID: "", RunID: req.RunID, TraceID: req.TraceID, ToolID: req.ToolID,
				ToolVersion: req.ToolVersion, RiskLevel: tool.RiskLevel, BusinessAppCode: req.BusinessAppCode,
			}, "tool_call.circuit_open", map[string]any{
				"circuit_state": state,
			})
			return &ExecuteResult{
				Status:       ToolCallStatusFailed,
				RiskLevel:    tool.RiskLevel,
				ErrorMessage: CheckCircuitStateError(state).Error(),
			}, CheckCircuitStateError(state)
		}
	}

	// 5) 风险评估:决定执行状态与审批要求
	status, approval := s.evaluateRisk(tool.RiskLevel)

	// 6) 幂等键校验:同一 key 重放必须同工具同输入,否则视为 key 冲突
	inputHash := computeInputHash(req.ArgumentsJSON)
	existing, err := s.toolCallRepo.GetByIdempotencyKey(ctx, req.TenantID, req.IdempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("check idempotency: %w", err)
	}
	if existing != nil {
		if existing.ToolID != req.ToolID || existing.InputHash != inputHash {
			// 同一 idempotency_key 但工具或输入不同:不是重放,是冲突
			return &ExecuteResult{
				Status:       ToolCallStatusFailed,
				RiskLevel:    tool.RiskLevel,
				ErrorMessage: ErrIdempotencyKeyConflict.Error(),
			}, ErrIdempotencyKeyConflict
		}
		log.Printf("[tool-svc] idempotent replay: returning existing tool_call %s (status=%s)", existing.ID, existing.Status)
		return &ExecuteResult{
			ToolCallID:   existing.ID,
			Status:       existing.Status,
			RiskLevel:    existing.RiskLevel,
			ApprovalReq:  existing.Status == ToolCallStatusPendingApproval,
		}, nil
	}

	// 7) 创建 tool_call 记录(executing 状态同时写入超时阈值,供 M2-C.8 扫描器接管)
	createReq := &ToolCallRequest{
		TenantID:          req.TenantID,
		RunID:             req.RunID,
		StepID:            req.StepID,
		ToolID:            req.ToolID,
		ToolVersion:       tool.Version,
		BusinessAppCode:   req.BusinessAppCode,
		ConnectorBindingID: req.ConnectorBindingID,
		PolicyVersion:     req.PolicyVersion,
		RiskLevel:         tool.RiskLevel,
		Status:            status,
		IdempotencyKey:    req.IdempotencyKey,
		InputHash:         inputHash,
		InputSummaryJSON:   req.ArgumentsJSON,
		TraceID:           req.TraceID,
	}
	if status == ToolCallStatusExecuting {
		deadline := s.executionDeadline()
		if deadline != nil {
			createReq.TimeoutAt = deadline
		}
	}
	tc, err := s.toolCallRepo.Create(ctx, createReq)
	if err != nil {
		return nil, fmt.Errorf("create tool_call: %w", err)
	}
	s.recordToolTrace(ctx, tc, "created") // M5-A: L4 created

	// 7) M2-C:高风险自动创建审批任务(绑定不可变 input_hash)
	if approval && s.approvalRepo != nil {
		if _, err := s.RequestApproval(ctx, tc.ID, ""); err != nil {
			log.Printf("[tool-svc] auto request approval failed for %s: %v", tc.ID, err)
			return &ExecuteResult{
				ToolCallID:   tc.ID,
				Status:       status,
				RiskLevel:    tool.RiskLevel,
				ApprovalReq:  true,
				ErrorMessage: "approval task creation failed: " + err.Error(),
			}, fmt.Errorf("auto request approval: %w", err)
		}
	}

	// 8) 审计日志
	if s.auditLogger != nil {
		auditPayload := map[string]any{
			"tool_call_id":   tc.ID,
			"tool_id":        req.ToolID,
			"tool_version":   tool.Version,
			"risk_level":     tool.RiskLevel,
			"status":         status,
			"agent_id":       req.AgentID,
			"business_app":   req.BusinessAppCode,
			"idempotency_key": req.IdempotencyKey,
		}
		detailJSON, _ := json.Marshal(auditPayload)
		detailStr := string(detailJSON)
		if _, _, err := s.auditLogger.InsertLog(ctx, audit.AuditLogEntry{
			TraceID:         req.TraceID,
			BusinessAppCode: &req.BusinessAppCode,
			Action:          "tool_call.execute",
			ResourceType:    "tool_call",
			ResourceID:      tc.ID,
			Status:          "success",
			DetailJSON:      &detailStr,
		}); err != nil {
			log.Printf("[tool-svc] audit log failed: %v", err)
		}
	}

	// 9) M3-A/M3-C:executing 状态 + Connector Binding → 自动驱动外部执行
	//    (M3-C:Saga 写操作分流至 Outbox;失败路径已 Confirm failed)
	if status == ToolCallStatusExecuting && req.ConnectorBindingID != "" && s.connectorRT != nil {
		finalStatus, execErr := s.autoExecuteViaConnector(ctx, tc)
		switch {
		case execErr != nil:
			// 执行/入队失败:tc 已被 Confirm failed,返回状态与落库一致。
			log.Printf("[tool-svc] connector auto-execution failed for %s: %v", tc.ID, execErr)
			status = ToolCallStatusFailed
		case finalStatus != "":
			status = finalStatus
		}
	}

	return &ExecuteResult{
		ToolCallID:  tc.ID,
		Status:      status,
		RiskLevel:   tool.RiskLevel,
		ApprovalReq: approval,
	}, nil
}
func (s *Service) resolveTool(ctx context.Context, toolID, version string) (*Tool, error) {
	var tool *Tool
	var err error
	if version != "" {
		tool, err = s.toolRepo.FindToolByIDAndVersion(ctx, toolID, version)
	} else {
		tool, err = s.toolRepo.FindToolByID(ctx, toolID)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %s@%s", ErrToolNotFound, toolID, version)
	}
	if tool.Status != "active" {
		return nil, fmt.Errorf("%w: tool %s is not active", ErrToolNotFound, toolID)
	}
	return tool, nil
}

// evaluateRisk 评估风险等级,决定初始状态与是否需要审批。
func (s *Service) evaluateRisk(riskLevel string) (ToolCallStatus, bool) {
	switch strings.ToLower(riskLevel) {
	case "high":
		return ToolCallStatusPendingApproval, true
	case "medium":
		return ToolCallStatusExecuting, false
	default:
		return ToolCallStatusExecuting, false
	}
}

func containsDomain(domains []string, domain string) bool {
	for _, d := range domains {
		if d == domain {
			return true
		}
	}
	return false
}

func computeInputHash(inputJSON string) string {
	h := sha256.Sum256([]byte(inputJSON))
	return hex.EncodeToString(h[:])
}

// GetToolCall 按 ID 查找工具调用。
func (s *Service) GetToolCall(ctx context.Context, toolCallID string) (*ToolCall, error) {
	return s.toolCallRepo.GetByID(ctx, toolCallID)
}

// ListDeadLetters 列出死信调用(M2-C.10 DLQ)。
func (s *Service) ListDeadLetters(ctx context.Context) ([]*ToolCall, error) {
	return s.toolCallRepo.ListDeadLetters(ctx, 100)
}

// ListByTrace 按 trace 串联查询(M2-E:ToolCall→Policy→Approval→Result→Verify)。
func (s *Service) ListByTrace(ctx context.Context, traceID string) ([]*ToolCall, error) {
	return s.toolCallRepo.ListByTrace(ctx, traceID)
}

// autoExecuteViaConnector M3-A/M3-C:驱动外部系统调用。
//
// M3-A 直连路径:
//	Create ToolCall(executing) → ConnectorRuntime.Execute → ConfirmExecution → 终态
//
// M3-C Outbox 路径(Saga,Connector 自声明 UseOutbox 的 capability):
//	Create ToolCall(executing) → Outbox.Enqueue(pending) → [Dispatcher 异步投递]
//	→ sent → Verify confirmed → ConfirmExecution(succeeded)
//	   └→ ToolCall 取消/重试耗尽 → compensate_pending → compensated → Confirm(failed)
//
// 分流依据 ConnectorRuntime.ShouldDeferToOutbox(平台不硬编码业务操作)。
func (s *Service) autoExecuteViaConnector(ctx context.Context, tc *ToolCall) (ToolCallStatus, error) {
	if tc.ConnectorBindingID == nil || *tc.ConnectorBindingID == "" {
		return tc.Status, nil // 无绑定,跳过 Connector 执行
	}

	// M3-C 分流:Saga 写操作经 Outbox 投递(ToolCall 保持 executing,
	// 由 OutboxDispatcher 异步推进并回写终态)。
	if s.outboxRepo != nil {
		if binding, err := s.bindingRepo.FindBinding(ctx, *tc.ConnectorBindingID); err == nil &&
			s.connectorRT.ShouldDeferToOutbox(binding.ConnectorCode, tc.ToolID) {
			return s.enqueueOutbox(ctx, tc)
		}
	}

	// 1. 解析 input_summary 为 Connector Input。
	input := map[string]any{}
	if tc.InputSummaryJSON != nil && *tc.InputSummaryJSON != "" {
		if err := json.Unmarshal([]byte(*tc.InputSummaryJSON), &input); err != nil {
			input = map[string]any{"raw": *tc.InputSummaryJSON}
		}
	}

	// 2. 通过 ConnectorRuntime 执行。
	result, err := s.connectorRT.Execute(ctx, &RuntimeExecuteRequest{
		TenantID:   tc.TenantID,
		BindingID:  *tc.ConnectorBindingID,
		ToolCallID: tc.ID,
		TraceID:    tc.TraceID,
		Capability: tc.ToolID, // M3-A 简化:tool_id = capability name
		Input:      input,
	})
	if err != nil {
		log.Printf("[tool-svc] connector runtime execute error for %s: %v", tc.ID, err)
		if confirmErr := s.confirmFailedExecution(ctx, tc, err); confirmErr != nil {
			log.Printf("[tool-svc] auto-confirm failed for %s: %v", tc.ID, confirmErr)
		}
		return ToolCallStatusFailed, err
	}

	// 3. Confirm 结果。
	finalStatus := ToolCallStatus(result.Status)
	if !isValidConfirmStatus(finalStatus) {
		finalStatus = ToolCallStatusFailed
	}

	confirmReq := &ConfirmExecutionRequest{
		Status:            finalStatus,
		ExternalRequestID: result.ExternalRequestID,
		ExternalObjectID:  result.ExternalObjectID,
	}

	if result.Output != nil {
		outputJSON, err := json.Marshal(result.Output)
		if err == nil {
			confirmReq.OutputSummaryJSON = string(outputJSON)
		}
	}

	if result.Error != nil {
		errorJSON, err := json.Marshal(result.Error)
		if err == nil {
			confirmReq.ErrorJSON = string(errorJSON)
		}
	}

	if _, err := s.ConfirmExecution(ctx, tc.ID, confirmReq); err != nil {
		log.Printf("[tool-svc] auto-confirm execution for %s failed: %v", tc.ID, err)
	}

	log.Printf("[tool-svc] connector auto-execution: tool_call=%s status=%s binding=%s", tc.ID, finalStatus, *tc.ConnectorBindingID)
	return finalStatus, nil
}

// confirmFailedExecution 将 Connector Runtime 错误标记为 ToolCall failed。
func (s *Service) confirmFailedExecution(ctx context.Context, tc *ToolCall, execErr error) error {
	errJSON, _ := json.Marshal(map[string]any{
		"code":    "CONNECTOR_EXECUTION_FAILED",
		"message": execErr.Error(),
	})
	_, err := s.ConfirmExecution(ctx, tc.ID, &ConfirmExecutionRequest{
		Status:   ToolCallStatusFailed,
		ErrorJSON: string(errJSON),
	})
	return err
}

// isValidConfirmStatus ConnectorRuntime → ToolCall 状态映射。
func isValidConfirmStatus(s ToolCallStatus) bool {
	switch s {
	case ToolCallStatusSucceeded, ToolCallStatusFailed, ToolCallStatusIndeterminate:
		return true
	}
	return false
}

// enqueueOutbox M3-C:Saga 写操作入 Outbox。
// ToolCall 保持 executing(由 Dispatcher 异步推进终态),不可变请求体落库,
// tool_call_id 关联实现外部请求全链路追溯(→ trace_id → Run/Step/Audit)。
func (s *Service) enqueueOutbox(ctx context.Context, tc *ToolCall) (ToolCallStatus, error) {
	binding, err := s.bindingRepo.FindBinding(ctx, *tc.ConnectorBindingID)
	if err != nil {
		if confirmErr := s.confirmFailedExecution(ctx, tc, err); confirmErr != nil {
			log.Printf("[tool-svc] auto-confirm failed for %s: %v", tc.ID, confirmErr)
		}
		return ToolCallStatusFailed, err
	}

	payload := "{}"
	if tc.InputSummaryJSON != nil && *tc.InputSummaryJSON != "" {
		payload = *tc.InputSummaryJSON
	}

	entry, err := s.outboxRepo.Enqueue(ctx, &OutboxEntry{
		ToolCallID:    tc.ID,
		TenantID:      tc.TenantID,
		ConnectorCode: binding.ConnectorCode,
		Operation:     tc.ToolID,
		PayloadJSON:   payload,
	})
	if err != nil {
		if confirmErr := s.confirmFailedExecution(ctx, tc, err); confirmErr != nil {
			log.Printf("[tool-svc] auto-confirm failed for %s: %v", tc.ID, confirmErr)
		}
		return ToolCallStatusFailed, err
	}

	s.auditLifecycle(ctx, tc, "tool_call.outbox_enqueued", map[string]any{
		"outbox_id": entry.ID, "connector_code": binding.ConnectorCode, "operation": tc.ToolID,
	})
	log.Printf("[tool-svc] outbox enqueued: tool_call=%s outbox=%s connector=%s op=%s",
		tc.ID, entry.ID, binding.ConnectorCode, tc.ToolID)
	return tc.Status, nil // 保持 executing
}
