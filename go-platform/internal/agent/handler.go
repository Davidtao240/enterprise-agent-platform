package agent

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/enterprise-agent-platform/go-platform/internal/audit"
	"github.com/enterprise-agent-platform/go-platform/internal/platform"
	"github.com/enterprise-agent-platform/go-platform/pkg/apierror"
	"github.com/gin-gonic/gin"
)

// Handler 处理 Agent Registry、Agent Run Logs、Approval 相关的 HTTP 请求。
type Handler struct {
	repo           handlerRepository
	auditRepo      agentAuditLogger
	workflowSvc    ApprovalWorkflowService
	resumer        runResumer
	toolCallBinder ToolCallDecisionBinder // M2-C:高风险 Tool Call 审批绑定
	roleChecker    UserRoleChecker        // 审批 assignee_role 校验
}

// NewHandler 创建 Handler 实例。
func NewHandler(repo *Repository, auditRepo *audit.Repository) *Handler {
	h := &Handler{repo: repo}
	if auditRepo != nil {
		h.auditRepo = auditRepo
	}
	return h
}

type runResumer interface {
	ResumeInterruptedRun(ctx context.Context, tenantID, runID, interruptID string, resumeInput map[string]any) error
}

// UserRoleChecker 查询用户角色码(用于审批 assignee_role 校验)。
type UserRoleChecker interface {
	GetUserRoleCodes(ctx context.Context, userID string) ([]string, error)
}

type handlerRepository interface {
	ListAgents(ctx context.Context, domain, status string) ([]Agent, error)
	CreateAgent(ctx context.Context, a *Agent) error
	ListRunLogs(ctx context.Context, tenantID, workflowInstanceID, graphKey string, page, pageSize int) ([]AgentRunLog, int, error)
	ListApprovalTasks(ctx context.Context, tenantID, status, businessAppCode, workflowInstanceID string, page, pageSize int) ([]ApprovalTaskView, int, error)
	GetApprovalTaskView(ctx context.Context, tenantID, id string) (*ApprovalTaskView, error)
	FindApprovalByID(ctx context.Context, tenantID, id string) (*ApprovalTask, error)
	UpdateApprovalDecision(ctx context.Context, tenantID, id, status, comment, decisionBy string) error
	CompleteApprovalAndWorkflowDecision(ctx context.Context, tenantID, id, status, comment, decisionBy string) (*ApprovalTask, error)
}

type ApprovalWorkflowService interface {
	CompleteHumanReviewNode(ctx context.Context, nodeInstanceID, decision, userID, comment string) error
	ContinueAfterHumanReviewNode(ctx context.Context, nodeInstanceID, decision, userID, comment string) error
}

// ToolCallDecisionBinder M2-C:高风险 Tool Call 审批决定绑定到 Tool 生命周期
// (approved→重检→executing / rejected→cancelled)。
type ToolCallDecisionBinder interface {
	BindToolCallApprovalDecision(ctx context.Context, toolCallID, decision string) error
}

func (h *Handler) SetWorkflowService(svc ApprovalWorkflowService) {
	h.workflowSvc = svc
}

// SetRunResumer 注入 Runtime 中断恢复控制器(审批决策时恢复 Run)。
func (h *Handler) SetRunResumer(resumer runResumer) {
	h.resumer = resumer
}

// SetToolCallDecisionBinder 注入 Tool Call 生命周期绑定器(M2-C)。
func (h *Handler) SetToolCallDecisionBinder(binder ToolCallDecisionBinder) {
	h.toolCallBinder = binder
}

// SetRoleChecker 注入用户角色查询器(审批 assignee_role 校验)。
func (h *Handler) SetRoleChecker(checker UserRoleChecker) {
	h.roleChecker = checker
}

// checkApprovalAssignee 校验当前用户是否有权决定该审批任务:
// assignee_user_id 已设置时必须本人;assignee_role 已设置时必须持有该角色。
func (h *Handler) checkApprovalAssignee(c *gin.Context, task *ApprovalTask) bool {
	userID := c.GetString("user_id")
	if task.AssigneeUserID != nil && *task.AssigneeUserID != "" && *task.AssigneeUserID != userID {
		platform.APIError(c, apierror.ErrForbidden)
		return false
	}
	if task.AssigneeRole != nil && *task.AssigneeRole != "" {
		if h.roleChecker == nil {
			// 未配置角色查询器时保守拒绝,而不是放行
			platform.APIError(c, apierror.ErrForbidden)
			return false
		}
		roles, err := h.roleChecker.GetUserRoleCodes(c.Request.Context(), userID)
		if err != nil {
			platform.APIError(c, apierror.ErrInternalError)
			return false
		}
		for _, r := range roles {
			if r == *task.AssigneeRole {
				return true
			}
		}
		platform.APIError(c, apierror.ErrForbidden)
		return false
	}
	return true
}

// ── Agent Registry ──

// ListAgents 处理 GET /api/v1/agents。
// 返回所有 active 状态的 Agent 列表。
func (h *Handler) ListAgents(c *gin.Context) {
	agents, err := h.repo.ListAgents(c.Request.Context(), c.Query("domain"), c.Query("status"))
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	platform.Success(c, agents)
}

// CreateAgent 处理 POST /api/v1/agents（admin only）。
func (h *Handler) CreateAgent(c *gin.Context) {
	var req CreateAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		platform.APIError(c, apierror.ErrValidationFailed)
		return
	}

	a := &Agent{
		AgentID:          req.AgentID,
		Name:             req.Name,
		Domain:           req.Domain,
		ReusableScope:    req.ReusableScope,
		CapabilitiesJSON: req.CapabilitiesJSON,
		InputSchemaJSON:  req.InputSchemaJSON,
		OutputSchemaJSON: req.OutputSchemaJSON,
		Status:           "active",
	}
	if req.Endpoint != "" {
		a.Endpoint = &req.Endpoint
	}
	if a.ReusableScope == "" {
		a.ReusableScope = "domain_only"
	}
	if a.CapabilitiesJSON == "" {
		a.CapabilitiesJSON = "[]"
	}
	if a.InputSchemaJSON == "" {
		a.InputSchemaJSON = "{}"
	}
	if a.OutputSchemaJSON == "" {
		a.OutputSchemaJSON = "{}"
	}

	if err := h.repo.CreateAgent(c.Request.Context(), a); err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}

	platform.Success(c, gin.H{"agent_id": a.AgentID, "id": a.ID})
}

// ── Agent Run Logs ──

// ListRunLogs 处理 GET /api/v1/agent-run-logs。
// 支持 query 参数：workflow_instance_id, graph_key, page, page_size
func (h *Handler) ListRunLogs(c *gin.Context) {
	workflowInstanceID := c.Query("workflow_instance_id")
	graphKey := c.Query("graph_key")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}

	logs, total, err := h.repo.ListRunLogs(c.Request.Context(), c.GetString("tenant_id"), workflowInstanceID, graphKey, page, pageSize)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	platform.List(c, logs, page, pageSize, total)
}

// ── Approval ──

// ApproveTask 处理 POST /api/v1/approval-tasks/{id}/approve。
func (h *Handler) ListApprovalTasks(c *gin.Context) {
	status := c.Query("status")
	businessAppCode := c.Query("business_app_code")
	workflowInstanceID := c.Query("workflow_instance_id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}

	tasks, total, err := h.repo.ListApprovalTasks(c.Request.Context(), c.GetString("tenant_id"), status, businessAppCode, workflowInstanceID, page, pageSize)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	platform.List(c, tasks, page, pageSize, total)
}

func (h *Handler) GetApprovalTask(c *gin.Context) {
	task, err := h.repo.GetApprovalTaskView(c.Request.Context(), c.GetString("tenant_id"), c.Param("id"))
	if err != nil {
		platform.APIError(c, apierror.ErrResourceNotFound)
		return
	}
	platform.Success(c, task)
}

func (h *Handler) ApproveTask(c *gin.Context) {
	id := c.Param("id")
	tenantID := c.GetString("tenant_id")
	userID := c.GetString("user_id")

	var req struct {
		Comment string `json:"comment"`
	}
	c.ShouldBindJSON(&req)

	task, err := h.repo.FindApprovalByID(c.Request.Context(), tenantID, id)
	if err != nil {
		platform.APIError(c, apierror.ErrResourceNotFound)
		return
	}
	if task.Status != "pending" {
		platform.APIError(c, apierror.ErrWorkflowInvalidState)
		return
	}
	if !h.checkApprovalAssignee(c, task) {
		return
	}
	if task.ToolCallID != nil {
		// M2-C:高风险 Tool Call 审批:记录决定 + 绑定 Tool 生命周期
		// (approved→执行前重检→executing / rejected→cancelled)。
		if err := h.repo.UpdateApprovalDecision(c.Request.Context(), tenantID, id, "approved", req.Comment, userID); err != nil {
			platform.APIError(c, apierror.ErrInternalError)
			return
		}
		if h.toolCallBinder == nil {
			platform.APIError(c, apierror.ErrInternalError)
			return
		}
		if err := h.toolCallBinder.BindToolCallApprovalDecision(c.Request.Context(), *task.ToolCallID, "approved"); err != nil {
			platform.APIError(c, &apierror.APIError{
				Code:    "TOOL_CALL_BIND_FAILED",
				Message: err.Error(),
				Status:  409,
			})
			return
		}
		h.auditApproval(c, task, userID, req.Comment, "approved")
		platform.Success(c, gin.H{"status": "approved"})
		return
	}
	if task.DurableRunID != nil && task.InterruptID != nil {
		// Runtime 中断审批:只记录决定,并恢复 Run;节点由 Run 后续终态事件推进。
		if err := h.repo.UpdateApprovalDecision(c.Request.Context(), tenantID, id, "approved", req.Comment, userID); err != nil {
			platform.APIError(c, apierror.ErrInternalError)
			return
		}
		if h.resumer == nil {
			platform.APIError(c, apierror.ErrInternalError)
			return
		}
		if err := h.resumer.ResumeInterruptedRun(c.Request.Context(), tenantID,
			*task.DurableRunID, *task.InterruptID, map[string]any{"decision": "approved"}); err != nil {
			platform.APIError(c, apierror.ErrInternalError)
			return
		}
		h.auditApproval(c, task, userID, req.Comment, "approved")
		platform.Success(c, gin.H{"status": "approved"})
		return
	}

	task, err = h.repo.CompleteApprovalAndWorkflowDecision(c.Request.Context(), tenantID, id, "approved", req.Comment, userID)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	if h.workflowSvc != nil {
		if err := h.workflowSvc.ContinueAfterHumanReviewNode(c.Request.Context(), task.NodeInstanceID, "approved", userID, req.Comment); err != nil {
			platform.APIError(c, apierror.ErrInternalError)
			return
		}
	}
	h.auditApproval(c, task, userID, req.Comment, "approved")
	platform.Success(c, gin.H{"status": "approved"})
}

// RejectTask 处理 POST /api/v1/approval-tasks/{id}/reject。
func (h *Handler) RejectTask(c *gin.Context) {
	id := c.Param("id")
	tenantID := c.GetString("tenant_id")
	userID := c.GetString("user_id")

	var req struct {
		Comment string `json:"comment"`
	}
	c.ShouldBindJSON(&req)

	task, err := h.repo.FindApprovalByID(c.Request.Context(), tenantID, id)
	if err != nil {
		platform.APIError(c, apierror.ErrResourceNotFound)
		return
	}
	if task.Status != "pending" {
		platform.APIError(c, apierror.ErrWorkflowInvalidState)
		return
	}
	if !h.checkApprovalAssignee(c, task) {
		return
	}
	if task.ToolCallID != nil {
		// M2-C:高风险 Tool Call 审批拒绝:Tool Call → cancelled。
		if err := h.repo.UpdateApprovalDecision(c.Request.Context(), tenantID, id, "rejected", req.Comment, userID); err != nil {
			platform.APIError(c, apierror.ErrInternalError)
			return
		}
		if h.toolCallBinder == nil {
			platform.APIError(c, apierror.ErrInternalError)
			return
		}
		if err := h.toolCallBinder.BindToolCallApprovalDecision(c.Request.Context(), *task.ToolCallID, "rejected"); err != nil {
			platform.APIError(c, &apierror.APIError{
				Code:    "TOOL_CALL_BIND_FAILED",
				Message: err.Error(),
				Status:  409,
			})
			return
		}
		h.auditApproval(c, task, userID, req.Comment, "rejected")
		platform.Success(c, gin.H{"status": "rejected"})
		return
	}
	if task.DurableRunID != nil && task.InterruptID != nil {
		// Runtime 中断审批:拒绝也以 decision 恢复 Run,由 Graph 决定终止路径。
		if err := h.repo.UpdateApprovalDecision(c.Request.Context(), tenantID, id, "rejected", req.Comment, userID); err != nil {
			platform.APIError(c, apierror.ErrInternalError)
			return
		}
		if h.resumer == nil {
			platform.APIError(c, apierror.ErrInternalError)
			return
		}
		if err := h.resumer.ResumeInterruptedRun(c.Request.Context(), tenantID,
			*task.DurableRunID, *task.InterruptID, map[string]any{"decision": "rejected"}); err != nil {
			platform.APIError(c, apierror.ErrInternalError)
			return
		}
		h.auditApproval(c, task, userID, req.Comment, "rejected")
		platform.Success(c, gin.H{"status": "rejected"})
		return
	}

	task, err = h.repo.CompleteApprovalAndWorkflowDecision(c.Request.Context(), tenantID, id, "rejected", req.Comment, userID)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	if h.workflowSvc != nil {
		if err := h.workflowSvc.ContinueAfterHumanReviewNode(c.Request.Context(), task.NodeInstanceID, "rejected", userID, req.Comment); err != nil {
			platform.APIError(c, apierror.ErrInternalError)
			return
		}
	}
	h.auditApproval(c, task, userID, req.Comment, "rejected")
	platform.Success(c, gin.H{"status": "rejected"})
}

// auditApproval 写入审批审计日志。
func (h *Handler) auditApproval(c *gin.Context, task *ApprovalTask, userID, comment, status string) {
	if h.auditRepo == nil {
		return
	}
	traceID := c.GetHeader("X-Trace-Id")
	if task.WorkflowTraceID != nil && *task.WorkflowTraceID != "" {
		traceID = *task.WorkflowTraceID
	}
	jsonBytes, _ := json.Marshal(map[string]string{
		"comment":              comment,
		"workflow_instance_id": task.WorkflowInstanceID,
		"node_instance_id":     task.NodeInstanceID,
	})
	detail := string(jsonBytes)
	h.auditRepo.InsertLog(c.Request.Context(), audit.AuditLogEntry{
		TraceID:         traceID,
		ActorUserID:     &userID,
		BusinessAppCode: &task.BusinessAppCode,
		Action:          "approval_" + status,
		ResourceType:    "approval_task",
		ResourceID:      task.ID,
		Status:          status,
		DetailJSON:      &detail,
	})
}
