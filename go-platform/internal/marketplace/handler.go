package marketplace

import (
	"github.com/enterprise-agent-platform/go-platform/internal/platform"
	"github.com/enterprise-agent-platform/go-platform/pkg/apierror"
	"github.com/gin-gonic/gin"
)

type Handler struct{}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) ListItems(c *gin.Context) {
	platform.Success(c, gin.H{
		"packages": []any{},
		"total":    0,
	})
}

func (h *Handler) GetItem(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		platform.APIError(c, apierror.ErrValidationFailed)
		return
	}
	platform.Success(c, gin.H{
		"code": code,
	})
}

func (h *Handler) InstallItem(c *gin.Context) {
	platform.Success(c, gin.H{"status": "installed"})
}

func (h *Handler) UninstallItem(c *gin.Context) {
	platform.Success(c, gin.H{"status": "uninstalled"})
}

func (h *Handler) RateItem(c *gin.Context) {
	platform.Success(c, gin.H{"status": "rated"})
}

func (h *Handler) ListReviews(c *gin.Context) {
	platform.Success(c, gin.H{"reviews": []any{}})
}

func (h *Handler) ListConnectors(c *gin.Context) {
	platform.Success(c, gin.H{
		"packages": []any{},
		"total":    0,
	})
}

func (h *Handler) InstallConnector(c *gin.Context) {
	platform.Success(c, gin.H{"status": "installed"})
}

func (h *Handler) UninstallConnector(c *gin.Context) {
	platform.Success(c, gin.H{"status": "uninstalled"})
}