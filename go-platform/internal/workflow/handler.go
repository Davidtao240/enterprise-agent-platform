package workflow

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/enterprise-agent-platform/go-platform/internal/platform"
	"github.com/enterprise-agent-platform/go-platform/pkg/apierror"
)

// Handler 处理工作流相关的 HTTP 请求。
type Handler struct {
	svc *Service
}

// NewHandler 创建 Handler 实例。
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// ── 模板端点 ──

// GetTemplates 处理 GET /api/v1/business-apps/{code}/workflow-templates。
// 返回某业务下所有可用的工作流模板列表。
func (h *Handler) GetTemplates(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		platform.APIError(c, apierror.ErrValidationFailed)
		return
	}

	templates, err := h.svc.GetTemplates(c.Request.Context(), code)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	platform.Success(c, templates)
}

// ── 实例端点 ──

// CreateInstance 处理 POST /api/v1/workflow-instances。
// 根据模板创建一个新的工作流实例（状态 = draft）。
func (h *Handler) CreateInstance(c *gin.Context) {
	var req CreateInstanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		platform.APIError(c, apierror.ErrValidationFailed)
		return
	}

	userID := c.GetString("user_id")
	resp, err := h.svc.CreateInstance(c.Request.Context(), userID, req)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"trace_id": c.GetHeader("X-Trace-Id"),
		"data":     resp,
	})
}

// ListInstances 处理 GET /api/v1/workflow-instances。
// 支持 query 参数：business_app_code, status, created_by, page, page_size
func (h *Handler) ListInstances(c *gin.Context) {
	businessAppCode := c.Query("business_app_code")
	status := c.Query("status")
	createdBy := c.Query("created_by")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	instances, total, err := h.svc.ListInstances(c.Request.Context(), businessAppCode, status, createdBy, page, pageSize)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	platform.List(c, instances, page, pageSize, total)
}

// GetInstance 处理 GET /api/v1/workflow-instances/{id}。
func (h *Handler) GetInstance(c *gin.Context) {
	id := c.Param("id")
	inst, err := h.svc.GetInstance(c.Request.Context(), id)
	if err != nil {
		platform.APIError(c, apierror.ErrResourceNotFound)
		return
	}
	platform.Success(c, inst)
}

// StartInstance 处理 POST /api/v1/workflow-instances/{id}/start。
// 将 draft 实例转为 running，入口节点入队。
func (h *Handler) StartInstance(c *gin.Context) {
	id := c.Param("id")
	userID := c.GetString("user_id")

	resp, err := h.svc.StartWorkflow(c.Request.Context(), userID, id)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"trace_id": c.GetHeader("X-Trace-Id"),
			"error":    gin.H{"code": "WORKFLOW_INVALID_STATE", "message": err.Error()},
		})
		return
	}
	platform.Success(c, resp)
}

// CancelInstance 处理 POST /api/v1/workflow-instances/{id}/cancel。
func (h *Handler) CancelInstance(c *gin.Context) {
	id := c.Param("id")

	var req CancelRequest
	c.ShouldBindJSON(&req) // reason 字段可选

	resp, err := h.svc.CancelWorkflow(c.Request.Context(), id)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"trace_id": c.GetHeader("X-Trace-Id"),
			"error":    gin.H{"code": "WORKFLOW_INVALID_STATE", "message": err.Error()},
		})
		return
	}
	platform.Success(c, resp)
}

// RetryNode 处理 POST /api/v1/workflow-instances/{id}/retry。
// 重试实例中某个失败的节点。
func (h *Handler) RetryNode(c *gin.Context) {
	id := c.Param("id")

	var req RetryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		platform.APIError(c, apierror.ErrValidationFailed)
		return
	}

	resp, err := h.svc.RetryNode(c.Request.Context(), id, req.NodeInstanceID)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"trace_id": c.GetHeader("X-Trace-Id"),
			"error":    gin.H{"code": "WORKFLOW_INVALID_STATE", "message": err.Error()},
		})
		return
	}
	platform.Success(c, resp)
}

// GetNodes 处理 GET /api/v1/workflow-instances/{id}/nodes。
func (h *Handler) GetNodes(c *gin.Context) {
	id := c.Param("id")
	nodes, err := h.svc.GetNodeInstances(c.Request.Context(), id)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	platform.Success(c, nodes)
}
