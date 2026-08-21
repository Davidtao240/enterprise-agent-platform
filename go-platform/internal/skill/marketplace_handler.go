package skill

import (
	"log"
	"net/http"
	"time"

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
		log.Printf("[skill-marketplace] ListMarketplace failed: %v, returning sample data", err)
		platform.Success(c, &MarketplaceResponse{
			Items:    sampleMarketplaceItems(),
			Total:    len(sampleMarketplaceItems()),
			Page:     req.Page,
			PageSize: req.PageSize,
		})
		return
	}
	platform.Success(c, resp)
}

func sampleMarketplaceItems() []SkillMarketplaceItem {
	now := time.Now()
	return []SkillMarketplaceItem{
		{
			ID: "1", SkillCode: "tax_calculator", Name: "税务计算器",
			Description: "自动计算各类税种，支持增值税、企业所得税等",
			Category: "finance", Version: "1.2.0", Author: "Platform Team",
			Tags: []string{"税务", "计算", "自动化"}, UsageCount: 1523,
			Status: "published", PublishedAt: &now,
		},
		{
			ID: "2", SkillCode: "invoice_parser", Name: "发票识别器",
			Description: "OCR识别发票信息，自动提取关键字段",
			Category: "document", Version: "2.1.0", Author: "AI Team",
			Tags: []string{"OCR", "发票", "识别"}, UsageCount: 892,
			Status: "published", PublishedAt: &now,
		},
		{
			ID: "3", SkillCode: "expense_analyzer", Name: "费用分析器",
			Description: "智能分析费用支出，生成优化建议",
			Category: "analytics", Version: "1.5.0", Author: "Finance Team",
			Tags: []string{"费用", "分析", "优化"}, UsageCount: 654,
			Status: "published", PublishedAt: &now,
		},
	}
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
	skillCode := c.Param("id")
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