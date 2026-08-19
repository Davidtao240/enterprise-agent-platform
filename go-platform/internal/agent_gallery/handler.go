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

type galleryService interface {
	ListGallery(ctx context.Context, tenantID, category, businessAppCode, query string) (*GalleryResponse, error)
	GetPackageDetail(ctx context.Context, tenantID, packageCode string) (*AgentPackageDetail, error)
	CreatePackage(ctx context.Context, tenantID string, req *CreatePackageRequest) (*AgentPackage, error)
	UpdatePackage(ctx context.Context, tenantID, packageCode string, req *UpdatePackageRequest) error
}

type Handler struct {
	svc       galleryService
	auditRepo *audit.Repository
}

func NewHandler(svc galleryService, auditRepo *audit.Repository) *Handler {
	return &Handler{svc: svc, auditRepo: auditRepo}
}

func (h *Handler) ListGallery(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	category := c.Query("category")
	businessApp := c.Query("business_app")
	q := c.Query("q")

	resp, err := h.svc.ListGallery(c.Request.Context(), tenantID, category, businessApp, q)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	platform.Success(c, resp)
}

func (h *Handler) GetPackage(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	code := c.Param("code")

	detail, err := h.svc.GetPackageDetail(c.Request.Context(), tenantID, code)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	if detail == nil {
		platform.APIError(c, apierror.ErrResourceNotFound)
		return
	}
	platform.Success(c, detail)
}

func (h *Handler) CreatePackage(c *gin.Context) {
	tenantID := c.GetString("tenant_id")

	var req CreatePackageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		platform.APIError(c, apierror.ErrValidationFailed)
		return
	}

	pkg, err := h.svc.CreatePackage(c.Request.Context(), tenantID, &req)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}

	if h.auditRepo != nil {
		h.auditCreatePackage(c, pkg)
	}

	platform.Success(c, pkg)
}

func (h *Handler) UpdatePackage(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	code := c.Param("code")

	var req UpdatePackageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		platform.APIError(c, apierror.ErrValidationFailed)
		return
	}

	if err := h.svc.UpdatePackage(c.Request.Context(), tenantID, code, &req); err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}

	if req.Status != nil && h.auditRepo != nil {
		h.auditUpdatePackage(c, code, *req.Status)
	}

	platform.Success(c, gin.H{"package_code": code, "updated": true})
}

func (h *Handler) auditCreatePackage(c *gin.Context, pkg *AgentPackage) {
	userID := c.GetString("user_id")
	traceID := c.GetHeader(platform.TraceIDHeader)

	detailBytes, _ := json.Marshal(map[string]string{
		"package_code":  pkg.PackageCode,
		"graph_key":     pkg.GraphKey,
		"business_app":  pkg.BusinessAppCode,
	})
	detail := string(detailBytes)

	h.auditRepo.InsertLog(c.Request.Context(), audit.AuditLogEntry{
		TraceID:      traceID,
		ActorUserID:  &userID,
		Action:       "agent_package.create",
		ResourceType: "agent_package",
		ResourceID:   pkg.PackageCode,
		Status:       "created",
		DetailJSON:   &detail,
	})
}

func (h *Handler) auditUpdatePackage(c *gin.Context, code, newStatus string) {
	userID := c.GetString("user_id")
	traceID := c.GetHeader(platform.TraceIDHeader)

	detailBytes, _ := json.Marshal(map[string]string{
		"package_code": code,
		"new_status":   newStatus,
	})
	detail := string(detailBytes)

	status := "updated"
	if newStatus == "published" {
		status = "published"
	} else if newStatus == "disabled" {
		status = "disabled"
	}

	action := "agent_package.update"
	if newStatus == "published" {
		action = "agent_package.publish"
	} else if newStatus == "disabled" {
		action = "agent_package.disable"
	}

	h.auditRepo.InsertLog(c.Request.Context(), audit.AuditLogEntry{
		TraceID:      traceID,
		ActorUserID:  &userID,
		Action:       action,
		ResourceType: "agent_package",
		ResourceID:   code,
		Status:       status,
		DetailJSON:   &detail,
	})
}

func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}