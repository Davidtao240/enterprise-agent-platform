package dashboard

import (
	"strconv"

	"github.com/enterprise-agent-platform/go-platform/internal/platform"
	"github.com/enterprise-agent-platform/go-platform/pkg/apierror"
	"github.com/gin-gonic/gin"
)

type Handler struct{ repo *Repository }

func NewHandler(repo *Repository) *Handler { return &Handler{repo: repo} }

func (h *Handler) GetDashboard(c *gin.Context) {
	days := 7
	if v := c.Query("days"); v != "" {
		if d, err := strconv.Atoi(v); err == nil && d > 0 && d <= 90 {
			days = d
		}
	}
	result, err := h.repo.GetDashboard(c.Request.Context(), c.GetString("tenant_id"), days)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	platform.Success(c, result)
}

func (h *Handler) GetDepartmentEfficiency(c *gin.Context) {
	days := 7
	if v := c.Query("days"); v != "" {
		if d, err := strconv.Atoi(v); err == nil && d > 0 && d <= 90 {
			days = d
		}
	}
	result, err := h.repo.GetDashboard(c.Request.Context(), c.GetString("tenant_id"), days)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	platform.Success(c, result.Departments)
}

func (h *Handler) GetAgentMetrics(c *gin.Context) {
	days := 7
	if v := c.Query("days"); v != "" {
		if d, err := strconv.Atoi(v); err == nil && d > 0 && d <= 90 {
			days = d
		}
	}
	result, err := h.repo.GetDashboard(c.Request.Context(), c.GetString("tenant_id"), days)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	platform.Success(c, result.AgentMetrics)
}

func (h *Handler) GetFailureReasons(c *gin.Context) {
	days := 7
	if v := c.Query("days"); v != "" {
		if d, err := strconv.Atoi(v); err == nil && d > 0 && d <= 90 {
			days = d
		}
	}
	result, err := h.repo.GetDashboard(c.Request.Context(), c.GetString("tenant_id"), days)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	platform.Success(c, result.FailureReasons)
}