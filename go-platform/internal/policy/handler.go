package policy

import (
	"github.com/enterprise-agent-platform/go-platform/internal/platform"
	"github.com/enterprise-agent-platform/go-platform/pkg/apierror"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	repo *Repository
}

func NewHandler(repo *Repository) *Handler {
	return &Handler{repo: repo}
}

func (h *Handler) ListDomainPolicies(c *gin.Context) {
	policies, err := h.repo.ListDomainPolicies(
		c.Request.Context(),
		c.Query("business_app_code"),
		c.Query("status"),
	)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	platform.Success(c, policies)
}
