package governance

import (
	"context"
	"encoding/json"
	"time"

	"github.com/enterprise-agent-platform/go-platform/internal/audit"
	"github.com/enterprise-agent-platform/go-platform/internal/platform"
	"github.com/enterprise-agent-platform/go-platform/pkg/apierror"
	"github.com/gin-gonic/gin"
)

type auditLogger interface {
	InsertLog(context.Context, audit.AuditLogEntry) (string, time.Time, error)
}
type Handler struct {
	repo      *Repository
	auditRepo auditLogger
}

func NewHandler(repo *Repository, auditRepo auditLogger) *Handler {
	return &Handler{repo: repo, auditRepo: auditRepo}
}

func (h *Handler) List(c *gin.Context) {
	items, err := h.repo.List(c.Request.Context(), c.GetString("tenant_id"), c.Query("resource_type"), c.Query("resource_key"), c.Query("status"))
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	platform.Success(c, items)
}

func (h *Handler) Create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil || !IsSupportedResourceType(req.ResourceType) || !json.Valid(req.SnapshotJSON) {
		platform.APIError(c, apierror.ErrValidationFailed)
		return
	}
	item := &ConfigurationVersion{TenantID: c.GetString("tenant_id"), ResourceType: req.ResourceType, ResourceKey: req.ResourceKey, Version: req.Version, LifecycleStatus: StatusDraft, SnapshotJSON: req.SnapshotJSON, ChangeSummary: req.ChangeSummary, CreatedBy: c.GetString("user_id"), TraceID: c.GetHeader(platform.TraceIDHeader)}
	if err := h.repo.Create(c.Request.Context(), item); err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	h.audit(c, item, "configuration_draft_created", "succeeded")
	platform.Success(c, item)
}

func (h *Handler) Submit(c *gin.Context) {
	h.transition(c, StatusDraft, StatusPendingApproval, "configuration_submitted")
}
func (h *Handler) Approve(c *gin.Context) {
	h.transition(c, StatusPendingApproval, StatusPublished, "configuration_published")
}
func (h *Handler) Deprecate(c *gin.Context) {
	h.transition(c, StatusPublished, StatusDeprecated, "configuration_deprecated")
}

func (h *Handler) transition(c *gin.Context, from, to, action string) {
	id, actor := c.Param("id"), c.GetString("user_id")
	if from == StatusPendingApproval {
		item, err := h.repo.FindByID(c.Request.Context(), c.GetString("tenant_id"), id)
		if err != nil {
			platform.APIError(c, apierror.ErrResourceNotFound)
			return
		}
		if item.CreatedBy == actor {
			platform.APIErrorWithMessage(c, apierror.ErrForbidden, "configuration author cannot approve their own change")
			return
		}
	}
	ok, err := h.repo.Transition(c.Request.Context(), c.GetString("tenant_id"), id, from, to, &actor)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	if !ok {
		platform.APIErrorWithMessage(c, apierror.ErrWorkflowInvalidState, "configuration lifecycle transition is not allowed")
		return
	}
	if item, err := h.repo.FindByID(c.Request.Context(), c.GetString("tenant_id"), id); err == nil {
		h.audit(c, item, action, "succeeded")
	}
	platform.Success(c, gin.H{"id": id, "lifecycle_status": to})
}

func (h *Handler) audit(c *gin.Context, item *ConfigurationVersion, action, status string) {
	if h.auditRepo == nil {
		return
	}
	detail, _ := json.Marshal(gin.H{"version": item.Version, "lifecycle_status": item.LifecycleStatus})
	actor := c.GetString("user_id")
	_, _, _ = h.auditRepo.InsertLog(c.Request.Context(), audit.AuditLogEntry{TraceID: c.GetHeader(platform.TraceIDHeader), TenantID: c.GetString("tenant_id"), ActorUserID: &actor, Action: action, ResourceType: item.ResourceType, ResourceID: item.ResourceKey, Status: status, DetailJSON: ptr(string(detail))})
}
func ptr(v string) *string { return &v }
