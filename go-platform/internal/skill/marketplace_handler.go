package skill

import (
	"net/http"

	"github.com/enterprise-agent-platform/go-platform/internal/audit"
	"github.com/enterprise-agent-platform/go-platform/internal/platform"
	"github.com/enterprise-agent-platform/go-platform/pkg/apierror"
	"github.com/gin-gonic/gin"
)

type MarketplaceHandler struct {
	svc       *MarketplaceService
	auditRepo *audit.Repository
}

func NewMarketplaceHandler(svc *MarketplaceService, auditRepo *audit.Repository) *MarketplaceHandler {
	return &MarketplaceHandler{svc: svc, auditRepo: auditRepo}
}

func (h *MarketplaceHandler) ListMarketplace(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	userID := c.GetString("user_id")

	var req MarketplaceListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		platform.APIError(c, apierror.ErrValidationFailed)
		return
	}

	resp, err := h.svc.ListMarketplace(c.Request.Context(), tenantID, userID, req.Category, req.Query, req.Status, req.Page, req.PageSize)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	platform.Success(c, resp)
}

func (h *MarketplaceHandler) InstallSkill(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	userID := c.GetString("user_id")

	var req InstallSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		platform.APIError(c, apierror.ErrValidationFailed)
		return
	}

	inst, err := h.svc.InstallSkill(c.Request.Context(), tenantID, userID, &req)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}

	if h.auditRepo != nil {
		h.auditRepo.InsertLog(c.Request.Context(), audit.AuditLogEntry{
			ActorUserID:  &userID,
			Action:       "skill.install",
			ResourceType: "skill",
			ResourceID:   req.SkillCode,
			Status:       "installed",
		})
	}

	platform.Success(c, inst)
}

func (h *MarketplaceHandler) UninstallSkill(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	userID := c.GetString("user_id")
	skillCode := c.Param("code")

	if err := h.svc.UninstallSkill(c.Request.Context(), tenantID, userID, skillCode); err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}

	if h.auditRepo != nil {
		h.auditRepo.InsertLog(c.Request.Context(), audit.AuditLogEntry{
			ActorUserID:  &userID,
			Action:       "skill.uninstall",
			ResourceType: "skill",
			ResourceID:   skillCode,
			Status:       "uninstalled",
		})
	}

	platform.Success(c, gin.H{"skill_code": skillCode, "uninstalled": true})
}

func (h *MarketplaceHandler) UpdateInstallation(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	userID := c.GetString("user_id")
	skillCode := c.Param("code")

	var req UpdateInstallationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		platform.APIError(c, apierror.ErrValidationFailed)
		return
	}
	if req.Status == nil {
		platform.APIError(c, apierror.ErrValidationFailed)
		return
	}

	if err := h.svc.UpdateInstallationStatus(c.Request.Context(), tenantID, userID, skillCode, *req.Status); err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}

	platform.Success(c, gin.H{"skill_code": skillCode, "status": *req.Status})
}

func (h *MarketplaceHandler) ListInstalled(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	userID := c.GetString("user_id")

	items, err := h.svc.ListInstalled(c.Request.Context(), tenantID, userID)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	platform.Success(c, gin.H{"items": items, "total": len(items)})
}

func (h *MarketplaceHandler) UpdateMetadata(c *gin.Context) {
	skillID := c.Param("id")

	var req UpdateSkillMetadataRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		platform.APIError(c, apierror.ErrValidationFailed)
		return
	}

	if err := h.svc.UpdateSkillMetadata(c.Request.Context(), skillID, &req); err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}

	platform.Success(c, gin.H{"id": skillID, "updated": true})
}

func (h *MarketplaceHandler) PublishNewVersion(c *gin.Context) {
	skillCode := c.Param("code")
	version := c.Param("version")

	if err := h.svc.PublishNewVersion(c.Request.Context(), skillCode, version); err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}

	platform.Success(c, gin.H{"skill_code": skillCode, "version": version, "published": true})
}

func (h *MarketplaceHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}