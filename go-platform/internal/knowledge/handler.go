package knowledge

import (
	"log"
	"net/http"

	"github.com/enterprise-agent-platform/go-platform/internal/audit"
	"github.com/enterprise-agent-platform/go-platform/internal/platform"
	"github.com/enterprise-agent-platform/go-platform/pkg/apierror"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	svc       *Service
	auditRepo *audit.Repository
}

func NewHandler(svc *Service, auditRepo *audit.Repository) *Handler {
	return &Handler{svc: svc, auditRepo: auditRepo}
}

// ── Collection handlers ──────────────────────────────────

func (h *Handler) CreateCollection(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	userID := c.GetString("user_id")

	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		platform.APIError(c, apierror.ErrValidationFailed)
		return
	}

	coll, err := h.svc.CreateCollection(c.Request.Context(), tenantID, userID, req.Name, req.Description)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	platform.Success(c, coll)
}

func (h *Handler) ListCollections(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	items, err := h.svc.ListCollections(c.Request.Context(), tenantID)
	if err != nil {
		// 返回空列表而非样例数据：mock 数据会让前端显示虚假知识库，
		// 点进去后文档/检索全部不可用，掩盖真实故障
		log.Printf("[knowledge] ListCollections failed: %v", err)
		items = []Collection{}
	}
	platform.Success(c, gin.H{"items": items, "total": len(items)})
}

func (h *Handler) GetCollection(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	id := c.Param("id")

	coll, err := h.svc.GetCollection(c.Request.Context(), id, tenantID)
	if err != nil {
		platform.APIError(c, apierror.ErrResourceNotFound)
		return
	}
	platform.Success(c, coll)
}

func (h *Handler) DeleteCollection(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	id := c.Param("id")

	if err := h.svc.DeleteCollection(c.Request.Context(), id, tenantID); err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	platform.Success(c, gin.H{"id": id, "deleted": true})
}

// ── Document handlers ──────────────────────────────────

func (h *Handler) UploadDocument(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	userID := c.GetString("user_id")

	collectionID := c.Param("id")

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		platform.APIError(c, apierror.ErrValidationFailed)
		return
	}
	defer file.Close()

	doc, err := h.svc.UploadDocument(c.Request.Context(), tenantID, userID, collectionID, file, header.Filename)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}

	if h.auditRepo != nil {
		h.auditRepo.InsertLog(c.Request.Context(), audit.AuditLogEntry{
			ActorUserID:  &userID,
			Action:       "kb.document.upload",
			ResourceType: "knowledge_document",
			ResourceID:   doc.ID,
			Status:       "uploaded",
		})
	}

	platform.Success(c, doc)
}

func (h *Handler) ListDocuments(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	collectionID := c.Param("id")
	status := c.Query("status")

	items, err := h.svc.ListDocuments(c.Request.Context(), tenantID, collectionID, status)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	platform.Success(c, gin.H{"items": items, "total": len(items)})
}

func (h *Handler) DeleteDocument(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	id := c.Param("id")

	if err := h.svc.DeleteDocument(c.Request.Context(), id, tenantID); err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	platform.Success(c, gin.H{"id": id, "deleted": true})
}

// ── Search handler ──────────────────────────────────

func (h *Handler) Search(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	collectionID := c.Param("id")

	var req struct {
		Query    string  `json:"query" binding:"required"`
		TopK     int     `json:"top_k"`
		MinScore float64 `json:"min_score"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		platform.APIError(c, apierror.ErrValidationFailed)
		return
	}

	results, err := h.svc.Search(c.Request.Context(), tenantID, collectionID, req.Query, req.TopK, req.MinScore)
	if err != nil {
		log.Printf("[knowledge] Search failed (collection=%s): %v", collectionID, err)
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	platform.Success(c, gin.H{"items": results, "total": len(results)})
}

func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}