package skill

import (
	"encoding/json"
	"net/http"

	"github.com/enterprise-agent-platform/go-platform/internal/platform"
	"github.com/enterprise-agent-platform/go-platform/pkg/apierror"
	"github.com/gin-gonic/gin"
)

// Handler M4-B Skill 管理端点 (protected,skill:manage 权限):
//
//	POST /api/v1/skills                 创建 draft
//	GET  /api/v1/skills?skill_code=     列出(可按 code 过滤)
//	GET  /api/v1/skills/:id             详情
//	PUT  /api/v1/skills/:id/config      修改配置(仅 draft)
//	POST /api/v1/skills/:id/submit      draft -> review
//	POST /api/v1/skills/:id/publish     review -> published (body: reviewer)
//	POST /api/v1/skills/:id/deprecate   published -> deprecated
type Handler struct {
	svc *Service
}

// NewHandler 创建 Handler。
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) writeErr(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	code := "SKILL_INTERNAL_ERROR"
	switch {
	case err == ErrSkillNotFound:
		status, code = http.StatusNotFound, "SKILL_NOT_FOUND"
	case err == ErrDuplicateVersion:
		status, code = http.StatusConflict, "SKILL_DUPLICATE_VERSION"
	case err == ErrInvalidTransition:
		status, code = http.StatusConflict, "SKILL_INVALID_TRANSITION"
	}
	platform.APIError(c, &apierror.APIError{Code: code, Message: err.Error(), Status: status})
}

// Create 处理 POST /api/v1/skills。
func (h *Handler) Create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		platform.APIError(c, &apierror.APIError{
			Code: "VALIDATION_FAILED", Message: err.Error(), Status: http.StatusBadRequest,
		})
		return
	}
	s, err := h.svc.Create(c.Request.Context(), &req)
	if err != nil {
		h.writeErr(c, err)
		return
	}
	platform.Success(c, s)
}

// List 处理 GET /api/v1/skills。
func (h *Handler) List(c *gin.Context) {
	items, err := h.svc.List(c.Request.Context(), c.Query("skill_code"))
	if err != nil {
		h.writeErr(c, err)
		return
	}
	platform.Success(c, gin.H{"items": items})
}

// Get 处理 GET /api/v1/skills/:id。
func (h *Handler) Get(c *gin.Context) {
	s, err := h.svc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.writeErr(c, err)
		return
	}
	platform.Success(c, s)
}

type updateConfigRequest struct {
	ConfigJSON interface{} `json:"config_json" binding:"required"`
}

// UpdateConfig 处理 PUT /api/v1/skills/:id/config (仅 draft)。
func (h *Handler) UpdateConfig(c *gin.Context) {
	var req updateConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		platform.APIError(c, &apierror.APIError{
			Code: "VALIDATION_FAILED", Message: err.Error(), Status: http.StatusBadRequest,
		})
		return
	}
	cfg, err := json.Marshal(req.ConfigJSON)
	if err != nil {
		platform.APIError(c, &apierror.APIError{
			Code: "VALIDATION_FAILED", Message: err.Error(), Status: http.StatusBadRequest,
		})
		return
	}
	s, err := h.svc.UpdateConfig(c.Request.Context(), c.Param("id"), cfg)
	if err != nil {
		h.writeErr(c, err)
		return
	}
	platform.Success(c, s)
}

type publishRequest struct {
	Reviewer string `json:"reviewer" binding:"required"`
}

// Submit 处理 POST /api/v1/skills/:id/submit。
func (h *Handler) Submit(c *gin.Context) {
	s, err := h.svc.SubmitForReview(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.writeErr(c, err)
		return
	}
	platform.Success(c, s)
}

// Publish 处理 POST /api/v1/skills/:id/publish。
func (h *Handler) Publish(c *gin.Context) {
	var req publishRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		platform.APIError(c, &apierror.APIError{
			Code: "VALIDATION_FAILED", Message: "reviewer is required", Status: http.StatusBadRequest,
		})
		return
	}
	s, err := h.svc.Publish(c.Request.Context(), c.Param("id"), req.Reviewer)
	if err != nil {
		h.writeErr(c, err)
		return
	}
	platform.Success(c, s)
}

// Deprecate 处理 POST /api/v1/skills/:id/deprecate。
func (h *Handler) Deprecate(c *gin.Context) {
	s, err := h.svc.Deprecate(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.writeErr(c, err)
		return
	}
	platform.Success(c, s)
}
