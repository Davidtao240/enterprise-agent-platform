package conversation

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/enterprise-agent-platform/go-platform/internal/audit"
	"github.com/enterprise-agent-platform/go-platform/internal/platform"
	"github.com/enterprise-agent-platform/go-platform/pkg/apierror"
	"github.com/gin-gonic/gin"
)

type conversationService interface {
	CreateConversation(ctx context.Context, userID, tenantID string, req *CreateConversationRequest) (*Conversation, error)
	ListConversations(ctx context.Context, userID, tenantID, groupBy string) ([]ConversationGroup, error)
	GetConversation(ctx context.Context, id, tenantID string) (*Conversation, []ConversationMessage, error)
	UpdateConversation(ctx context.Context, id, tenantID string, req *UpdateConversationRequest) error
	SendMessage(ctx context.Context, userID, tenantID, conversationID string, req *SendMessageRequest) (*SendMessageResponse, error)
	AnswerClarification(ctx context.Context, userID, tenantID, conversationID string, req *AnswerClarificationRequest) error
	CancelRun(ctx context.Context, userID, tenantID, conversationID string) error
	GetSSEEvents(ctx context.Context, conversationID, tenantID string) (<-chan SSEEvent, error)
}

type Handler struct {
	svc      conversationService
	auditRepo *audit.Repository
	sseWriter *SSEWriter
}

func NewHandler(svc conversationService, auditRepo *audit.Repository, sseWriter *SSEWriter) *Handler {
	return &Handler{
		svc:       svc,
		auditRepo: auditRepo,
		sseWriter: sseWriter,
	}
}

func (h *Handler) CreateConversation(c *gin.Context) {
	var req CreateConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		platform.APIError(c, apierror.ErrValidationFailed)
		return
	}

	userID := c.GetString("user_id")
	tenantID := c.GetString("tenant_id")

	conv, err := h.svc.CreateConversation(c.Request.Context(), userID, tenantID, &req)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}

	title := ""
	if conv.Title != nil {
		title = *conv.Title
	}
	resp := CreateConversationResponse{
		ID:             conv.ID,
		ThreadID:       conv.ThreadID,
		AgentPackageCode: conv.AgentPackageCode,
		Title:          title,
		Status:         conv.Status,
	}

	if h.auditRepo != nil {
		h.auditCreateConversation(c, conv)
	}

	platform.Success(c, resp)
}

func (h *Handler) ListConversations(c *gin.Context) {
	userID := c.GetString("user_id")
	tenantID := c.GetString("tenant_id")
	groupBy := c.Query("group_by")

	groups, err := h.svc.ListConversations(c.Request.Context(), userID, tenantID, groupBy)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	platform.Success(c, groups)
}

func (h *Handler) GetConversation(c *gin.Context) {
	id := c.Param("id")
	tenantID := c.GetString("tenant_id")

	conv, msgs, err := h.svc.GetConversation(c.Request.Context(), id, tenantID)
	if err != nil {
		platform.APIError(c, apierror.ErrResourceNotFound)
		return
	}

	platform.Success(c, gin.H{
		"conversation": conv,
		"messages":     msgs,
	})
}

func (h *Handler) UpdateConversation(c *gin.Context) {
	id := c.Param("id")
	tenantID := c.GetString("tenant_id")

	var req UpdateConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		platform.APIError(c, apierror.ErrValidationFailed)
		return
	}

	if err := h.svc.UpdateConversation(c.Request.Context(), id, tenantID, &req); err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}

	platform.Success(c, gin.H{"id": id, "updated": true})
}

func (h *Handler) SendMessage(c *gin.Context) {
	id := c.Param("id")
	userID := c.GetString("user_id")
	tenantID := c.GetString("tenant_id")

	var req SendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		platform.APIError(c, apierror.ErrValidationFailed)
		return
	}

	resp, err := h.svc.SendMessage(c.Request.Context(), userID, tenantID, id, &req)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}

	platform.Success(c, resp)
}

func (h *Handler) AnswerClarification(c *gin.Context) {
	id := c.Param("id")
	userID := c.GetString("user_id")
	tenantID := c.GetString("tenant_id")

	var req AnswerClarificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		platform.APIError(c, apierror.ErrValidationFailed)
		return
	}

	if err := h.svc.AnswerClarification(c.Request.Context(), userID, tenantID, id, &req); err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}

	platform.Success(c, gin.H{"status": "submitted"})
}

func (h *Handler) CancelRun(c *gin.Context) {
	id := c.Param("id")
	userID := c.GetString("user_id")
	tenantID := c.GetString("tenant_id")

	if err := h.svc.CancelRun(c.Request.Context(), userID, tenantID, id); err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}

	platform.Success(c, gin.H{"status": "cancelled"})
}

func (h *Handler) Stream(c *gin.Context) {
	id := c.Param("id")
	tenantID := c.GetString("tenant_id")

	_, _, err := h.svc.GetConversation(c.Request.Context(), id, tenantID)
	if err != nil {
		platform.APIError(c, apierror.ErrResourceNotFound)
		return
	}

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	lastEventID := 0
	if lei := c.GetHeader("Last-Event-Id"); lei != "" {
		if n, err := strconv.Atoi(lei); err == nil {
			lastEventID = n
		}
	}

	h.sseWriter.Stream(c.Request.Context(), c.Writer, flusher, id, lastEventID)
}

func (h *Handler) auditCreateConversation(c *gin.Context, conv *Conversation) {
	userID := c.GetString("user_id")
	traceID := c.GetHeader(platform.TraceIDHeader)

	detailBytes, _ := json.Marshal(map[string]string{
		"conversation_id": conv.ID,
		"thread_id":       conv.ThreadID,
		"agent_package":   conv.AgentPackageCode,
	})
	detail := string(detailBytes)

	h.auditRepo.InsertLog(c.Request.Context(), audit.AuditLogEntry{
		TraceID:      traceID,
		ActorUserID:  &userID,
		Action:       "conversation_create",
		ResourceType: "conversation",
		ResourceID:   conv.ID,
		Status:       "created",
		DetailJSON:   &detail,
	})
}