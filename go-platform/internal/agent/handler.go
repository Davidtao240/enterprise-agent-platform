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
	repo        handlerRepository
	auditRepo   agentAuditLogger
	workflowSvc ApprovalWorkflowService
}

// NewHandler 创建 Handler 实例。
func NewHandler(repo *Repository, auditRepo *audit.Repository) *Handler {
	h := &Handler{repo: repo}
	if auditRepo != nil {
		h.auditRepo = auditRepo
	}
	return h
}

type handlerRepository interface {
	ListAgents(ctx context.Context) ([]Agent, error)
	CreateAgent(ctx context.Context, a *Agent) error
	ListRunLogs(ctx context.Context, workflowInstanceID, graphKey string, page, pageSize int) ([]AgentRunLog, int, error)
	ListApprovalTasks(ctx context.Context, status, businessAppCode, workflowInstanceID string, page, pageSize int) ([]ApprovalTaskView, int, error)
	GetApprovalTaskView(ctx context.Context, id string) (*ApprovalTaskView, error)
	FindApprovalByID(ctx context.Context, id string) (*ApprovalTask, error)
	UpdateApprovalDecision(ctx context.Context, id, status, comment, decisionBy string) error
	CompleteApprovalAndWorkflowDecision(ctx context.Context, id, status, comment, decisionBy string) (*ApprovalTask, error)
}

type ApprovalWorkflowService interface {
	CompleteHumanReviewNode(ctx context.Context, nodeInstanceID, decision, userID, comment string) error
	ContinueAfterHumanReviewNode(ctx context.Context, nodeInstanceID, decision, userID, comment string) error
}

func (h *Handler) SetWorkflowService(svc ApprovalWorkflowService) {
	h.workflowSvc = svc
}

// ── Agent Registry ──

// ListAgents 处理 GET /api/v1/agents。
// 返回所有 active 状态的 Agent 列表。
func (h *Handler) ListAgents(c *gin.Context) {
	agents, err := h.repo.ListAgents(c.Request.Context())
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}

	// 转换为精简响应（不含完整 schema）
	items := make([]ListAgentsResponse, len(agents))
	for i, a := range agents {
		items[i] = ListAgentsResponse{
			AgentID:       a.AgentID,
			Name:          a.Name,
			Domain:        a.Domain,
			ReusableScope: a.ReusableScope,
			Status:        a.Status,
		}
	}
	platform.Success(c, items)
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

	logs, total, err := h.repo.ListRunLogs(c.Request.Context(), workflowInstanceID, graphKey, page, pageSize)
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

	tasks, total, err := h.repo.ListApprovalTasks(c.Request.Context(), status, businessAppCode, workflowInstanceID, page, pageSize)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	platform.List(c, tasks, page, pageSize, total)
}

func (h *Handler) GetApprovalTask(c *gin.Context) {
	task, err := h.repo.GetApprovalTaskView(c.Request.Context(), c.Param("id"))
	if err != nil {
		platform.APIError(c, apierror.ErrResourceNotFound)
		return
	}
	platform.Success(c, task)
}

func (h *Handler) ApproveTask(c *gin.Context) {
	id := c.Param("id")
	userID := c.GetString("user_id")

	var req struct {
		Comment string `json:"comment"`
	}
	c.ShouldBindJSON(&req)

	if _, err := h.repo.FindApprovalByID(c.Request.Context(), id); err != nil {
		platform.APIError(c, apierror.ErrResourceNotFound)
		return
	}
	task, err := h.repo.CompleteApprovalAndWorkflowDecision(c.Request.Context(), id, "approved", req.Comment, userID)
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
	userID := c.GetString("user_id")

	var req struct {
		Comment string `json:"comment"`
	}
	c.ShouldBindJSON(&req)

	if _, err := h.repo.FindApprovalByID(c.Request.Context(), id); err != nil {
		platform.APIError(c, apierror.ErrResourceNotFound)
		return
	}
	task, err := h.repo.CompleteApprovalAndWorkflowDecision(c.Request.Context(), id, "rejected", req.Comment, userID)
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
