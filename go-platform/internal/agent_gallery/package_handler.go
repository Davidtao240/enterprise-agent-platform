package agent_gallery

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/enterprise-agent-platform/go-platform/internal/audit"
	"github.com/enterprise-agent-platform/go-platform/internal/platform"
	"github.com/enterprise-agent-platform/go-platform/pkg/apierror"
	"github.com/gin-gonic/gin"
)

type packageService interface {
	CreateVersion(ctx context.Context, tenantID string, v *PackageVersion) (*PackageVersion, error)
	ListVersions(ctx context.Context, tenantID, packageCode string) ([]PackageVersion, error)
	PublishVersion(ctx context.Context, tenantID, packageCode, version string) error

	InstallPackage(ctx context.Context, tenantID, userID string, req *InstallPackageRequest) (*InstallationResponse, error)
	UninstallPackage(ctx context.Context, tenantID, packageCode string) error
	UpdateInstallationStatus(ctx context.Context, tenantID, packageCode, status string) error
	ListInstalled(ctx context.Context, tenantID, status, category, query string) (*InstalledListResponse, error)

	RegisterPackage(ctx context.Context, tenantID, userID string, req *RegisterPackageRequest) (*PackageRegistration, error)
	VerifyRegistration(ctx context.Context, tenantID, packageCode string) error
	RejectRegistration(ctx context.Context, tenantID, packageCode string) error
	ListRegistrations(ctx context.Context, tenantID, status, sourceType string) (*RegistrationListResponse, error)
	GetRegistrationDetail(ctx context.Context, tenantID, packageCode string) (*PackageRegistration, error)
}

type PackageHandler struct {
	svc       packageService
	auditRepo *audit.Repository
}

func NewPackageHandler(svc packageService, auditRepo *audit.Repository) *PackageHandler {
	return &PackageHandler{svc: svc, auditRepo: auditRepo}
}

func (h *PackageHandler) CreateVersion(c *gin.Context) {
	tenantID := c.GetString("tenant_id")

	var v PackageVersion
	if err := c.ShouldBindJSON(&v); err != nil {
		platform.APIError(c, apierror.ErrValidationFailed)
		return
	}
	v.TenantID = tenantID

	result, err := h.svc.CreateVersion(c.Request.Context(), tenantID, &v)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	platform.Success(c, result)
}

func (h *PackageHandler) ListVersions(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	packageCode := c.Param("code")

	versions, err := h.svc.ListVersions(c.Request.Context(), tenantID, packageCode)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	platform.Success(c, VersionListResponse{Items: versions, Total: len(versions)})
}

func (h *PackageHandler) PublishVersion(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	packageCode := c.Param("code")
	version := c.Param("version")

	if err := h.svc.PublishVersion(c.Request.Context(), tenantID, packageCode, version); err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}

	if h.auditRepo != nil {
		userID := c.GetString("user_id")
		h.auditRepo.InsertLog(c.Request.Context(), audit.AuditLogEntry{
			ActorUserID:  &userID,
			Action:       "agent_package_version.publish",
			ResourceType: "agent_package_version",
			ResourceID:   packageCode + "@" + version,
			Status:       "published",
		})
	}

	platform.Success(c, gin.H{"package_code": packageCode, "version": version, "published": true})
}

func (h *PackageHandler) InstallPackage(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	userID := c.GetString("user_id")

	var req InstallPackageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		platform.APIError(c, apierror.ErrValidationFailed)
		return
	}

	result, err := h.svc.InstallPackage(c.Request.Context(), tenantID, userID, &req)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}

	if h.auditRepo != nil {
		h.auditRepo.InsertLog(c.Request.Context(), audit.AuditLogEntry{
			ActorUserID:  &userID,
			Action:       "agent_package.install",
			ResourceType: "agent_package",
			ResourceID:   req.PackageCode,
			Status:       "installed",
		})
	}

	platform.Success(c, result)
}

func (h *PackageHandler) UninstallPackage(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	packageCode := c.Param("code")

	if err := h.svc.UninstallPackage(c.Request.Context(), tenantID, packageCode); err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}

	if h.auditRepo != nil {
		userID := c.GetString("user_id")
		h.auditRepo.InsertLog(c.Request.Context(), audit.AuditLogEntry{
			ActorUserID:  &userID,
			Action:       "agent_package.uninstall",
			ResourceType: "agent_package",
			ResourceID:   packageCode,
			Status:       "uninstalled",
		})
	}

	platform.Success(c, gin.H{"package_code": packageCode, "uninstalled": true})
}

func (h *PackageHandler) UpdateInstallationStatus(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	packageCode := c.Param("code")

	var req UpdateInstallationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		platform.APIError(c, apierror.ErrValidationFailed)
		return
	}
	if req.Status == nil {
		platform.APIError(c, apierror.ErrValidationFailed)
		return
	}

	if err := h.svc.UpdateInstallationStatus(c.Request.Context(), tenantID, packageCode, *req.Status); err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}

	platform.Success(c, gin.H{"package_code": packageCode, "status": *req.Status})
}

func (h *PackageHandler) ListInstalled(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	status := c.Query("status")
	category := c.Query("category")
	q := c.Query("q")

	result, err := h.svc.ListInstalled(c.Request.Context(), tenantID, status, category, q)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	platform.Success(c, result)
}

func (h *PackageHandler) RegisterPackage(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	userID := c.GetString("user_id")

	var req RegisterPackageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		platform.APIError(c, apierror.ErrValidationFailed)
		return
	}

	reg, err := h.svc.RegisterPackage(c.Request.Context(), tenantID, userID, &req)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}

	if h.auditRepo != nil {
		detailBytes, _ := json.Marshal(map[string]string{
			"package_code": reg.PackageCode,
			"source_type":  reg.SourceType,
		})
		detailStr := string(detailBytes)
		h.auditRepo.InsertLog(c.Request.Context(), audit.AuditLogEntry{
			ActorUserID:  &userID,
			Action:       "agent_package.register",
			ResourceType: "agent_package_registration",
			ResourceID:   reg.PackageCode,
			Status:       reg.Status,
			DetailJSON:   &detailStr,
		})
	}

	platform.Success(c, reg)
}

func (h *PackageHandler) VerifyRegistration(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	packageCode := c.Param("code")

	if err := h.svc.VerifyRegistration(c.Request.Context(), tenantID, packageCode); err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}

	if h.auditRepo != nil {
		userID := c.GetString("user_id")
		h.auditRepo.InsertLog(c.Request.Context(), audit.AuditLogEntry{
			ActorUserID:  &userID,
			Action:       "agent_package.verify",
			ResourceType: "agent_package_registration",
			ResourceID:   packageCode,
			Status:       "verified",
		})
	}

	platform.Success(c, gin.H{"package_code": packageCode, "verified": true})
}

func (h *PackageHandler) RejectRegistration(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	packageCode := c.Param("code")

	if err := h.svc.RejectRegistration(c.Request.Context(), tenantID, packageCode); err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}

	if h.auditRepo != nil {
		userID := c.GetString("user_id")
		h.auditRepo.InsertLog(c.Request.Context(), audit.AuditLogEntry{
			ActorUserID:  &userID,
			Action:       "agent_package.reject",
			ResourceType: "agent_package_registration",
			ResourceID:   packageCode,
			Status:       "rejected",
		})
	}

	platform.Success(c, gin.H{"package_code": packageCode, "rejected": true})
}

func (h *PackageHandler) ListRegistrations(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	status := c.Query("status")
	sourceType := c.Query("source_type")

	result, err := h.svc.ListRegistrations(c.Request.Context(), tenantID, status, sourceType)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	platform.Success(c, result)
}

func (h *PackageHandler) GetRegistration(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	packageCode := c.Param("code")

	reg, err := h.svc.GetRegistrationDetail(c.Request.Context(), tenantID, packageCode)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	if reg == nil {
		platform.APIError(c, apierror.ErrResourceNotFound)
		return
	}
	platform.Success(c, reg)
}

func (h *PackageHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}