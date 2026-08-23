package tool

import (
	"net/http"
	"strings"

	"github.com/enterprise-agent-platform/go-platform/internal/audit"
	"github.com/enterprise-agent-platform/go-platform/internal/platform"
	"github.com/enterprise-agent-platform/go-platform/pkg/apierror"
	"github.com/gin-gonic/gin"
)

type SidecarHandler struct {
	svc       *SidecarService
	auditRepo *audit.Repository
}

func NewSidecarHandler(svc *SidecarService, auditRepo *audit.Repository) *SidecarHandler {
	return &SidecarHandler{svc: svc, auditRepo: auditRepo}
}

func (h *SidecarHandler) Register(c *gin.Context) {
	tenantID := c.GetString("tenant_id")

	var req struct {
		ConnectorCode string   `json:"connector_code" binding:"required"`
		Version       string   `json:"version" binding:"required"`
		SidecarURL    string   `json:"sidecar_url" binding:"required"`
		AuthToken     string   `json:"auth_token"`
		TimeoutMs     int      `json:"timeout_ms"`
		Capabilities  []string `json:"capabilities" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		platform.APIError(c, apierror.ErrValidationFailed)
		return
	}

	if !strings.HasPrefix(req.SidecarURL, "http://") && !strings.HasPrefix(req.SidecarURL, "https://") {
		platform.APIError(c, apierror.ErrValidationFailed)
		return
	}

	reg := &SidecarRegistration{
		TenantID:      tenantID,
		ConnectorCode: req.ConnectorCode,
		Version:       req.Version,
		SidecarURL:    req.SidecarURL,
		AuthToken:     req.AuthToken,
		TimeoutMs:     req.TimeoutMs,
		Capabilities:  req.Capabilities,
	}

	result, err := h.svc.RegisterSidecar(c.Request.Context(), reg)
	if err != nil {
		platform.APIError(c, &apierror.APIError{
			Code:    apierror.ErrValidationFailed.Code,
			Message: err.Error(),
			Status:  http.StatusBadRequest,
		})
		return
	}

	userID := c.GetString("user_id")
	if h.auditRepo != nil {
		h.auditRepo.InsertLog(c.Request.Context(), audit.AuditLogEntry{
			ActorUserID:  &userID,
			Action:       "sidecar.register",
			ResourceType: "sidecar",
			ResourceID:   req.ConnectorCode,
			Status:       "registered",
		})
	}

	platform.Success(c, result)
}

func (h *SidecarHandler) List(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	items, err := h.svc.ListSidecars(c.Request.Context(), tenantID)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	platform.Success(c, gin.H{"items": items, "total": len(items)})
}

func (h *SidecarHandler) Get(c *gin.Context) {
	id := c.Param("id")
	tenantID := c.GetString("tenant_id")
	reg, err := h.svc.GetSidecar(c.Request.Context(), tenantID, id)
	if err != nil {
		platform.APIError(c, apierror.ErrResourceNotFound)
		return
	}
	platform.Success(c, reg)
}

func (h *SidecarHandler) Deregister(c *gin.Context) {
	id := c.Param("id")
	tenantID := c.GetString("tenant_id")
	if err := h.svc.DeregisterSidecar(c.Request.Context(), tenantID, id); err != nil {
		platform.APIError(c, apierror.ErrResourceNotFound)
		return
	}

	userID := c.GetString("user_id")
	if h.auditRepo != nil {
		h.auditRepo.InsertLog(c.Request.Context(), audit.AuditLogEntry{
			ActorUserID:  &userID,
			Action:       "sidecar.deregister",
			ResourceType: "sidecar",
			ResourceID:   id,
			Status:       "deregistered",
		})
	}

	platform.Success(c, gin.H{"id": id, "deregistered": true})
}

func (h *SidecarHandler) HealthCheck(c *gin.Context) {
	id := c.Param("id")
	tenantID := c.GetString("tenant_id")
	reg, err := h.svc.HealthCheck(c.Request.Context(), tenantID, id)
	if err != nil {
		platform.APIError(c, apierror.ErrResourceNotFound)
		return
	}
	platform.Success(c, reg)
}

func (h *SidecarHandler) Validate(c *gin.Context) {
	id := c.Param("id")
	tenantID := c.GetString("tenant_id")
	reg, err := h.svc.GetSidecar(c.Request.Context(), tenantID, id)
	if err != nil {
		platform.APIError(c, apierror.ErrResourceNotFound)
		return
	}
	if err := h.svc.ValidateCapabilities(c.Request.Context(), reg); err != nil {
		platform.APIError(c, apierror.ErrValidationFailed)
		return
	}
	platform.Success(c, gin.H{"id": id, "valid": true})
}

func (h *SidecarHandler) Metadata(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	meta := h.svc.GetMetadata(tenantID)
	c.JSON(http.StatusOK, gin.H{"data": meta})
}

func (h *SidecarHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}