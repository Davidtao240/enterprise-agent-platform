package experiment

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/enterprise-agent-platform/go-platform/internal/platform"
	"github.com/enterprise-agent-platform/go-platform/pkg/apierror"
	"github.com/gin-gonic/gin"
)

// Handler M5-C 实验机制端点(权限统一为 experiment:manage; Spec §4.5):
//
//	POST   /api/v1/replays                    发起回放(异步)
//	GET    /api/v1/replays/:id                查询回放状态与 Diff
//	POST   /api/v1/shadow-rules               创建 Shadow 规则
//	GET    /api/v1/shadow-rules               Shadow 规则列表
//	POST   /api/v1/shadow-rules/:id/stop      停止 Shadow 规则
//	GET    /api/v1/shadow-executions          影子执行列表
//	POST   /api/v1/canary-releases            创建 Canary 发布
//	GET    /api/v1/canary-releases            Canary 发布列表
//	GET    /api/v1/canary-releases/:id        Canary 发布详情
//	POST   /api/v1/canary-releases/:id/advance   进入下一阶梯
//	POST   /api/v1/canary-releases/:id/promote   全量发布
//	POST   /api/v1/canary-releases/:id/rollback  回滚
//	POST   /api/v1/canary-releases/:id/check     触发指标检查
type Handler struct {
	svc experimentService
}

// experimentService 便于 Handler 单测的 Service 契约。
type experimentService interface {
	StartReplay(ctx context.Context, tenantID string, req CreateReplayRequest, createdBy string) (*ReplaySession, error)
	GetReplay(ctx context.Context, tenantID, id string) (*ReplaySession, error)
	CreateShadowRule(ctx context.Context, tenantID string, req CreateShadowRuleRequest, createdBy string) (*ShadowRule, error)
	ListShadowRules(ctx context.Context, tenantID string) ([]*ShadowRule, error)
	StopShadowRule(ctx context.Context, tenantID, id string) error
	ListShadowExecutions(ctx context.Context, tenantID string, limit int) ([]*ShadowExecution, error)
	CreateCanaryRelease(ctx context.Context, tenantID string, req CreateCanaryReleaseRequest, createdBy string) error
	GetCanaryRelease(ctx context.Context, tenantID, id string) (*CanaryRelease, error)
	ListCanaryReleases(ctx context.Context, tenantID string) ([]*CanaryRelease, error)
	AdvanceCanary(ctx context.Context, tenantID, id string) (*CanaryRelease, error)
	PromoteCanary(ctx context.Context, tenantID, id string) (*CanaryRelease, error)
	RollbackCanary(ctx context.Context, tenantID, id string) (*CanaryRelease, error)
	CheckCanary(ctx context.Context, tenantID, id string) (*CanaryCheckResult, error)
}

// NewHandler 创建 Handler。
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) fail(c *gin.Context, code string, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, ErrNotFound) {
		status = http.StatusNotFound
		code = "NOT_FOUND"
	}
	platform.APIError(c, &apierror.APIError{Code: code, Message: err.Error(), Status: status})
}

// CreateReplay POST /api/v1/replays。
func (h *Handler) CreateReplay(c *gin.Context) {
	var req CreateReplayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.fail(c, "VALIDATION_FAILED", err)
		return
	}
	session, err := h.svc.StartReplay(c.Request.Context(), c.GetString("tenant_id"), req, c.GetString("user_id"))
	if err != nil {
		h.fail(c, "REPLAY_START_FAILED", err)
		return
	}
	platform.Success(c, session)
}

// GetReplay GET /api/v1/replays/:id。
func (h *Handler) GetReplay(c *gin.Context) {
	session, err := h.svc.GetReplay(c.Request.Context(), c.GetString("tenant_id"), c.Param("id"))
	if err != nil {
		h.fail(c, "REPLAY_GET_FAILED", err)
		return
	}
	platform.Success(c, session)
}

// CreateShadowRule POST /api/v1/shadow-rules。
func (h *Handler) CreateShadowRule(c *gin.Context) {
	var req CreateShadowRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.fail(c, "VALIDATION_FAILED", err)
		return
	}
	rule, err := h.svc.CreateShadowRule(c.Request.Context(), c.GetString("tenant_id"), req, c.GetString("user_id"))
	if err != nil {
		h.fail(c, "SHADOW_RULE_CREATE_FAILED", err)
		return
	}
	platform.Success(c, rule)
}

// ListShadowRules GET /api/v1/shadow-rules。
func (h *Handler) ListShadowRules(c *gin.Context) {
	rules, err := h.svc.ListShadowRules(c.Request.Context(), c.GetString("tenant_id"))
	if err != nil {
		h.fail(c, "SHADOW_RULE_LIST_FAILED", err)
		return
	}
	platform.Success(c, gin.H{"items": rules})
}

// StopShadowRule POST /api/v1/shadow-rules/:id/stop。
func (h *Handler) StopShadowRule(c *gin.Context) {
	if err := h.svc.StopShadowRule(c.Request.Context(), c.GetString("tenant_id"), c.Param("id")); err != nil {
		h.fail(c, "SHADOW_RULE_STOP_FAILED", err)
		return
	}
	platform.Success(c, gin.H{"stopped": true})
}

// ListShadowExecutions GET /api/v1/shadow-executions?limit=100。
func (h *Handler) ListShadowExecutions(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	execs, err := h.svc.ListShadowExecutions(c.Request.Context(), c.GetString("tenant_id"), limit)
	if err != nil {
		h.fail(c, "SHADOW_EXECUTION_LIST_FAILED", err)
		return
	}
	platform.Success(c, gin.H{"items": execs})
}

// CreateCanaryRelease POST /api/v1/canary-releases。
func (h *Handler) CreateCanaryRelease(c *gin.Context) {
	var req CreateCanaryReleaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.fail(c, "VALIDATION_FAILED", err)
		return
	}
	if err := h.svc.CreateCanaryRelease(c.Request.Context(), c.GetString("tenant_id"), req, c.GetString("user_id")); err != nil {
		h.fail(c, "CANARY_CREATE_FAILED", err)
		return
	}
	platform.Success(c, gin.H{"created": true})
}

// GetCanaryRelease GET /api/v1/canary-releases/:id。
func (h *Handler) GetCanaryRelease(c *gin.Context) {
	release, err := h.svc.GetCanaryRelease(c.Request.Context(), c.GetString("tenant_id"), c.Param("id"))
	if err != nil {
		h.fail(c, "CANARY_GET_FAILED", err)
		return
	}
	platform.Success(c, release)
}

// ListCanaryReleases GET /api/v1/canary-releases。
func (h *Handler) ListCanaryReleases(c *gin.Context) {
	releases, err := h.svc.ListCanaryReleases(c.Request.Context(), c.GetString("tenant_id"))
	if err != nil {
		h.fail(c, "CANARY_LIST_FAILED", err)
		return
	}
	platform.Success(c, gin.H{"items": releases})
}

// AdvanceCanary POST /api/v1/canary-releases/:id/advance。
func (h *Handler) AdvanceCanary(c *gin.Context) {
	release, err := h.svc.AdvanceCanary(c.Request.Context(), c.GetString("tenant_id"), c.Param("id"))
	if err != nil {
		h.fail(c, "CANARY_ADVANCE_FAILED", err)
		return
	}
	platform.Success(c, release)
}

// PromoteCanary POST /api/v1/canary-releases/:id/promote。
func (h *Handler) PromoteCanary(c *gin.Context) {
	release, err := h.svc.PromoteCanary(c.Request.Context(), c.GetString("tenant_id"), c.Param("id"))
	if err != nil {
		h.fail(c, "CANARY_PROMOTE_FAILED", err)
		return
	}
	platform.Success(c, release)
}

// RollbackCanary POST /api/v1/canary-releases/:id/rollback。
func (h *Handler) RollbackCanary(c *gin.Context) {
	release, err := h.svc.RollbackCanary(c.Request.Context(), c.GetString("tenant_id"), c.Param("id"))
	if err != nil {
		h.fail(c, "CANARY_ROLLBACK_FAILED", err)
		return
	}
	platform.Success(c, release)
}

// CheckCanary POST /api/v1/canary-releases/:id/check。
func (h *Handler) CheckCanary(c *gin.Context) {
	result, err := h.svc.CheckCanary(c.Request.Context(), c.GetString("tenant_id"), c.Param("id"))
	if err != nil {
		h.fail(c, "CANARY_CHECK_FAILED", err)
		return
	}
	platform.Success(c, result)
}
