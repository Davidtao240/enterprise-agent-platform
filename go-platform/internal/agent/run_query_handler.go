package agent

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/enterprise-agent-platform/go-platform/internal/platform"
	"github.com/enterprise-agent-platform/go-platform/pkg/apierror"
	"github.com/gin-gonic/gin"
)

// ── M6-A: Run 查询端点(protected,JWT 租户隔离) ──
//
//	GET /api/v1/runs?status=&limit=   Run 列表(workflow:read)
//	GET /api/v1/runs/:id               Run 详情 = 基本信息 + steps + runtime events
//
// Run 时间线页数据 = 本端点(Run/Step/Event) + GET /traces/:trace_id(L1-L6),
// 经 run.trace_id 关联(Spec WORKBENCH_DESIGN.md §6.1)。

// RunQueryStore Handler 依赖(便于测试替换)。
type RunQueryStore interface {
	ListDurableRuns(ctx context.Context, tenantID, status string, limit int) ([]*DurableRun, error)
	FindDurableRunByIDForTenant(ctx context.Context, tenantID, runID string) (*DurableRun, error)
	ListRunSteps(ctx context.Context, tenantID, runID string, limit int) ([]*AgentRunStep, error)
	ListRunEvents(ctx context.Context, tenantID, runID string, limit int) ([]*RuntimeEvent, error)
}

// RunQueryHandler Run 查询 HTTP 处理器。
type RunQueryHandler struct {
	store RunQueryStore
}

// NewRunQueryHandler 创建处理器。
func NewRunQueryHandler(store RunQueryStore) *RunQueryHandler { return &RunQueryHandler{store: store} }

// RunDetail GetRun 响应体。
type RunDetail struct {
	Run    *DurableRun      `json:"run"`
	Steps  []*AgentRunStep  `json:"steps"`
	Events []*RuntimeEvent  `json:"events"`
}

// ListRuns GET /api/v1/runs — tenant_id 由认证上下文注入。
func (h *RunQueryHandler) ListRuns(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	runs, err := h.store.ListDurableRuns(c.Request.Context(), c.GetString("tenant_id"), c.Query("status"), limit)
	if err != nil {
		platform.APIError(c, &apierror.APIError{
			Code: "RUN_LIST_FAILED", Message: err.Error(), Status: http.StatusInternalServerError,
		})
		return
	}
	platform.Success(c, gin.H{"items": runs})
}

// GetRun GET /api/v1/runs/:id — Run 详情(基本信息 + steps + events)。
func (h *RunQueryHandler) GetRun(c *gin.Context) {
	tenantID, runID := c.GetString("tenant_id"), c.Param("id")
	run, err := h.store.FindDurableRunByIDForTenant(c.Request.Context(), tenantID, runID)
	if err != nil {
		status, code := http.StatusInternalServerError, "RUN_GET_FAILED"
		if errors.Is(err, ErrDurableRunNotFound) {
			status, code = http.StatusNotFound, "RUN_NOT_FOUND"
		}
		platform.APIError(c, &apierror.APIError{Code: code, Message: err.Error(), Status: status})
		return
	}
	steps, err := h.store.ListRunSteps(c.Request.Context(), tenantID, runID, 200)
	if err != nil {
		platform.APIError(c, &apierror.APIError{
			Code: "RUN_GET_FAILED", Message: err.Error(), Status: http.StatusInternalServerError,
		})
		return
	}
	events, err := h.store.ListRunEvents(c.Request.Context(), tenantID, runID, 300)
	if err != nil {
		platform.APIError(c, &apierror.APIError{
			Code: "RUN_GET_FAILED", Message: err.Error(), Status: http.StatusInternalServerError,
		})
		return
	}
	platform.Success(c, &RunDetail{Run: run, Steps: steps, Events: events})
}
