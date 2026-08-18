package eval

import (
	"context"
	"net/http"

	"github.com/enterprise-agent-platform/go-platform/internal/platform"
	"github.com/enterprise-agent-platform/go-platform/pkg/apierror"
	"github.com/gin-gonic/gin"
)

// reportGenerator 便于 Handler 单测的 Service 契约。
type reportGenerator interface {
	GenerateReport(ctx context.Context, tenantID string, req ReportRequest) (*ReportResponse, error)
}

// Handler M5-B Eval 端点:
//
//	POST /api/v1/eval/reports  生成评估报告 (eval:read)
type Handler struct {
	svc reportGenerator
}

// NewHandler 创建 Handler。
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// GenerateReport 处理 POST /api/v1/eval/reports。
// tenant_id 由认证上下文注入,请求体不得携带(Spec §3.2)。
func (h *Handler) GenerateReport(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	var req ReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		platform.APIError(c, &apierror.APIError{
			Code: "VALIDATION_FAILED", Message: err.Error(), Status: http.StatusBadRequest,
		})
		return
	}
	resp, err := h.svc.GenerateReport(c.Request.Context(), tenantID, req)
	if err != nil {
		platform.APIError(c, &apierror.APIError{
			Code: "EVAL_REPORT_FAILED", Message: err.Error(), Status: http.StatusBadRequest,
		})
		return
	}
	platform.Success(c, resp)
}
