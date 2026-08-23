package tool

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/enterprise-agent-platform/go-platform/internal/platform"
	"github.com/enterprise-agent-platform/go-platform/pkg/apierror"
	"github.com/gin-gonic/gin"
)

// ToolCallHandler 处理 Tool Execution API 的 HTTP 请求。
// 受信任执行边界:所有请求经 InternalServiceToken 认证,
// 身份/Tenant/Domain 由服务端注入,拒绝模型自报覆盖。
type ToolCallHandler struct {
	service       *Service
	domainPolicy  DomainPolicyProvider
	permProvider  AgentPermissionProvider
	runVerifier   RunIdentityVerifier
}

// RunIdentity 可信 Run 快照身份(由 Tool Gateway 反查校验)。
type RunIdentity struct {
	TenantID        string
	BusinessAppCode string
}

// RunIdentityVerifier 从可信 Run 快照反查身份,用于拒绝伪造的 Agent/App 身份。
type RunIdentityVerifier interface {
	FindRunIdentity(ctx context.Context, runID string) (*RunIdentity, error)
}

// NewToolCallHandler 创建 ToolCallHandler。
func NewToolCallHandler(service *Service, domainPolicy DomainPolicyProvider, permProvider AgentPermissionProvider) *ToolCallHandler {
	return &ToolCallHandler{
		service:      service,
		domainPolicy: domainPolicy,
		permProvider: permProvider,
	}
}

// SetRunIdentityVerifier 注入 Run 身份反查器。
func (h *ToolCallHandler) SetRunIdentityVerifier(v RunIdentityVerifier) {
	h.runVerifier = v
}

// CreateToolCall 处理 POST /internal/v1/tool-calls。
// 请求体包含:tool_id, tool_version, run_id, step_id, idempotency_key,
// arguments, connector_binding_id, policy_version。
// 可信身份(tenant_id, business_app_code)从服务端注入并经 Run 快照反查验证;
// agent_id 仅作审计元数据,不作为安全边界(agent_runs 表无 agent_id 列,
// 无法反查验证)。trace_id 为关联 ID,不参与鉴权。
func (h *ToolCallHandler) CreateToolCall(c *gin.Context) {
	var req struct {
		ToolID             string `json:"tool_id" binding:"required"`
		ToolVersion        string `json:"tool_version"`
		RunID              string `json:"run_id" binding:"required"`
		StepID             string `json:"step_id"`
		AgentID            string `json:"agent_id"`
		BusinessAppCode    string `json:"business_app_code" binding:"required"`
		ConnectorBindingID string `json:"connector_binding_id"`
		PolicyVersion      string `json:"policy_version"`
		ArgumentsJSON      string `json:"arguments_json"`
		IdempotencyKey     string `json:"idempotency_key" binding:"required"`
		TraceID            string `json:"trace_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		platform.APIError(c, &apierror.APIError{
			Code:    "VALIDATION_FAILED",
			Message: err.Error(),
			Status:  400,
		})
		return
	}

	// 从服务端认证上下文注入租户 ID(拒绝模型自报)
	tenantID := c.GetHeader("X-Tenant-ID")
	if tenantID == "" {
		platform.APIError(c, &apierror.APIError{
			Code:    "UNAUTHORIZED",
			Message: "tenant_id required in service auth context",
			Status:  401,
		})
		return
	}

	// 可信身份边界:从 Run 快照反查 tenant/business_app,拒绝伪造的 Agent/App 身份。
	// Run 不存在、租户不符或业务应用不符时拒绝执行。
	if h.runVerifier != nil {
		ident, err := h.runVerifier.FindRunIdentity(c.Request.Context(), req.RunID)
		if err != nil {
			platform.APIError(c, &apierror.APIError{
				Code:    "RUN_NOT_FOUND",
				Message: "tool call must reference an existing durable run",
				Status:  http.StatusNotFound,
			})
			return
		}
		if ident.TenantID != tenantID || ident.BusinessAppCode != req.BusinessAppCode {
			platform.APIError(c, &apierror.APIError{
				Code:    "IDENTITY_MISMATCH",
				Message: "tool call identity does not match trusted run snapshot",
				Status:  http.StatusForbidden,
			})
			return
		}
	}

	result, err := h.service.Execute(c.Request.Context(), &ExecuteRequest{
		TenantID:           tenantID,
		RunID:              req.RunID,
		StepID:             req.StepID,
		AgentID:            req.AgentID,
		BusinessAppCode:    req.BusinessAppCode,
		ToolID:             req.ToolID,
		ToolVersion:        req.ToolVersion,
		ConnectorBindingID: req.ConnectorBindingID,
		PolicyVersion:      req.PolicyVersion,
		ArgumentsJSON:      req.ArgumentsJSON,
		IdempotencyKey:     req.IdempotencyKey,
		TraceID:            req.TraceID,
	}, h.domainPolicy, h.permProvider)

	if err != nil {
		status, code := http.StatusInternalServerError, "INTERNAL_ERROR"
		switch {
		case errors.Is(err, ErrToolNotFound):
			status, code = http.StatusNotFound, "TOOL_NOT_FOUND"
		case errors.Is(err, ErrToolVersionMismatch):
			status, code = http.StatusNotFound, "TOOL_VERSION_MISMATCH"
		case errors.Is(err, ErrConnectorBindingNotFound):
			status, code = http.StatusNotFound, "CONNECTOR_BINDING_NOT_FOUND"
		case errors.Is(err, ErrToolNotAllowed), errors.Is(err, ErrDomainPolicyViolation):
			status, code = http.StatusForbidden, "TOOL_NOT_ALLOWED"
		case errors.Is(err, ErrIdempotencyKeyConflict):
			status, code = http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT"
		case errors.Is(err, ErrConnectorBindingDisabled):
			status, code = http.StatusConflict, "CONNECTOR_BINDING_DISABLED"
		case errors.Is(err, ErrCircuitOpen):
			status, code = http.StatusTooManyRequests, "CIRCUIT_OPEN"
		}
		c.JSON(status, gin.H{
			"code":    code,
			"error":   err.Error(),
			"details": result,
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetToolCall 处理 GET /internal/v1/tool-calls/:tool_call_id。
func (h *ToolCallHandler) GetToolCall(c *gin.Context) {
	id := c.Param("tool_call_id")
	tc, err := h.service.GetToolCall(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, ErrToolCallNotFound) {
			platform.APIError(c, &apierror.APIError{
				Code:    "RESOURCE_NOT_FOUND",
				Message: "tool call not found",
				Status:  404,
			})
			return
		}
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	platform.Success(c, tc)
}

// ConfirmToolCall 处理 POST /internal/v1/tool-calls/:tool_call_id/confirm。
// 执行器上报执行结果:executing → succeeded/failed/indeterminate(timeout)。
func (h *ToolCallHandler) ConfirmToolCall(c *gin.Context) {
	id := c.Param("tool_call_id")
	var req ConfirmExecutionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		platform.APIError(c, &apierror.APIError{
			Code:    "VALIDATION_FAILED",
			Message: err.Error(),
			Status:  400,
		})
		return
	}
	tc, err := h.service.ConfirmExecution(c.Request.Context(), id, &req)
	if err != nil {
		h.writeLifecycleError(c, err)
		return
	}
	platform.Success(c, tc)
}

// VerifyToolCall 处理 POST /internal/v1/tool-calls/:tool_call_id/verify。
// 外部状态验证证据持久化,不改状态;Reconcile 前置。
func (h *ToolCallHandler) VerifyToolCall(c *gin.Context) {
	id := c.Param("tool_call_id")
	var req struct {
		VerificationJSON string `json:"verification_json" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		platform.APIError(c, &apierror.APIError{
			Code:    "VALIDATION_FAILED",
			Message: err.Error(),
			Status:  400,
		})
		return
	}
	tc, err := h.service.VerifyResult(c.Request.Context(), id, req.VerificationJSON)
	if err != nil {
		h.writeLifecycleError(c, err)
		return
	}
	platform.Success(c, tc)
}

// ReconcileToolCall 处理 POST /internal/v1/tool-calls/:tool_call_id/reconcile。
// M2-C:indeterminate/executing(超时)→ 终态对账,要求先有 verification_json。
func (h *ToolCallHandler) ReconcileToolCall(c *gin.Context) {
	id := c.Param("tool_call_id")
	var req struct {
		Outcome    string `json:"outcome" binding:"required"` // executed / not_executed / still_unknown
		DetailJSON string `json:"detail_json"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		platform.APIError(c, &apierror.APIError{
			Code:    "VALIDATION_FAILED",
			Message: err.Error(),
			Status:  400,
		})
		return
	}
	tc, err := h.service.Reconcile(c.Request.Context(), id, ReconcileOutcome(req.Outcome), req.DetailJSON)
	if err != nil {
		h.writeLifecycleError(c, err)
		return
	}
	platform.Success(c, tc)
}

// RetryToolCall 处理 POST /internal/v1/tool-calls/:tool_call_id/retry。
// Retry Gate:仅 failed 且外部验证确认未执行时放行;高风险回到 pending_approval。
// M2-C.10:超过重试上限 → 死信(409 + DEAD_LETTER 语义)。
func (h *ToolCallHandler) RetryToolCall(c *gin.Context) {
	id := c.Param("tool_call_id")
	tc, err := h.service.Retry(c.Request.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "dead letter") {
			platform.APIError(c, &apierror.APIError{
				Code:    "TOOL_CALL_DEAD_LETTER",
				Message: err.Error(),
				Status:  http.StatusConflict,
			})
			return
		}
		h.writeLifecycleError(c, err)
		return
	}
	platform.Success(c, tc)
}

// ListDeadLetterToolCalls 处理 GET /internal/v1/tool-calls/dead-letters(M2-C.10 DLQ)。
func (h *ToolCallHandler) ListDeadLetterToolCalls(c *gin.Context) {
	calls, err := h.service.ListDeadLetters(c.Request.Context())
	if err != nil {
		h.writeLifecycleError(c, err)
		return
	}
	platform.Success(c, calls)
}

// ListByTrace 处理 GET /internal/v1/tool-calls/by-trace/:trace_id(M2-E 全链查询)。
func (h *ToolCallHandler) ListByTrace(c *gin.Context) {
	calls, err := h.service.ListByTrace(c.Request.Context(), c.Param("trace_id"))
	if err != nil {
		h.writeLifecycleError(c, err)
		return
	}
	platform.Success(c, calls)
}

// RequestApprovalToolCall 处理 POST /internal/v1/tool-calls/:tool_call_id/request-approval。
// 手动补建审批任务(高风险 Execute 已自动创建,此端点用于补偿/重建)。
func (h *ToolCallHandler) RequestApprovalToolCall(c *gin.Context) {
	id := c.Param("tool_call_id")
	var req struct {
		Title string `json:"title"`
	}
	c.ShouldBindJSON(&req)
	approval, err := h.service.RequestApproval(c.Request.Context(), id, req.Title)
	if err != nil {
		h.writeLifecycleError(c, err)
		return
	}
	platform.Success(c, approval)
}

// writeLifecycleError 生命周期错误映射:状态冲突/非法转换 → 409。
func (h *ToolCallHandler) writeLifecycleError(c *gin.Context, err error) {
	if errors.Is(err, ErrToolCallNotFound) {
		platform.APIError(c, &apierror.APIError{
			Code:    "RESOURCE_NOT_FOUND",
			Message: "tool call not found",
			Status:  404,
		})
		return
	}
	status := http.StatusInternalServerError
	code := "INTERNAL_ERROR"
	switch {
	case errors.Is(err, ErrInvalidTransition):
		code = "INVALID_STATE_TRANSITION"
		status = http.StatusConflict
	case errors.Is(err, ErrApprovalNotDecided):
		code = "APPROVAL_NOT_DECIDED"
		status = http.StatusConflict
	case errors.Is(err, ErrApprovalPayloadMismatch):
		code = "APPROVAL_PAYLOAD_MISMATCH"
		status = http.StatusConflict
	case errors.Is(err, ErrRetryNotAllowed):
		code = "RETRY_NOT_ALLOWED"
		status = http.StatusConflict
	case errors.Is(err, ErrVerificationMissing):
		code = "VERIFICATION_MISSING"
		status = http.StatusConflict
	case errors.Is(err, ErrApprovalNotFound):
		code = "APPROVAL_NOT_FOUND"
		status = http.StatusNotFound
	case errors.Is(err, ErrCircuitOpen):
		code = "CIRCUIT_OPEN"
		status = http.StatusTooManyRequests
	}
	platform.APIError(c, &apierror.APIError{
		Code:    code,
		Message: err.Error(),
		Status:  status,
	})
}
