package observability

import (
	"github.com/enterprise-agent-platform/go-platform/internal/platform"
	"github.com/enterprise-agent-platform/go-platform/pkg/apierror"
	"github.com/gin-gonic/gin"
)

type Handler struct{ repo *Repository }

func NewHandler(repo *Repository) *Handler { return &Handler{repo: repo} }

func (h *Handler) Summary(c *gin.Context) {
	summary, err := h.repo.Summary(c.Request.Context(), c.GetString("tenant_id"))
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	alerts := []Alert{}
	if summary.FailedRunsLast24h > 0 {
		alerts = append(alerts, Alert{Severity: "warning", Code: "AGENT_RUN_FAILURES", Message: "agent runs failed during the last 24 hours"})
	}
	if summary.AverageDurationMs > 120000 {
		alerts = append(alerts, Alert{Severity: "warning", Code: "AGENT_RUN_LATENCY", Message: "average agent run duration exceeds 120 seconds"})
	}
	platform.Success(c, SummaryResponse{Summary: *summary, Alerts: alerts})
}
