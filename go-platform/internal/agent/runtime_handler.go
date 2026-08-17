package agent

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type runtimeEventApplier interface {
	ApplyRuntimeEvent(ctx context.Context, event *RuntimeEvent) (bool, error)
}

type RuntimeHandler struct {
	service runtimeEventApplier
	sink    RunEventSink
	runs    runFinder
}

type RuntimeEventEnvelope struct {
	ProtocolVersion   string         `json:"protocol_version" binding:"required"`
	EventID           string         `json:"event_id" binding:"required"`
	TenantID          string         `json:"tenant_id" binding:"required"`
	RunID             string         `json:"run_id" binding:"required"`
	Sequence          int64          `json:"sequence" binding:"required"`
	Attempt           int            `json:"attempt" binding:"required"`
	Type              string         `json:"type" binding:"required"`
	OccurredAt        time.Time      `json:"occurred_at" binding:"required"`
	Payload           map[string]any `json:"payload"`
	CheckpointVersion *int64         `json:"checkpoint_version,omitempty"`
}

func NewRuntimeHandler(service runtimeEventApplier) *RuntimeHandler {
	return &RuntimeHandler{service: service}
}

// SetEventSink wires workflow advancement for applied terminal/interrupt events.
func (h *RuntimeHandler) SetEventSink(sink RunEventSink, runs runFinder) {
	h.sink = sink
	h.runs = runs
}

// RequireInternalServiceToken authenticates service-to-service routes without
// accepting a user JWT as a substitute for runtime identity.
func RequireInternalServiceToken(expectedToken string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if expectedToken == "" {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"code": "SERVICE_AUTH_NOT_CONFIGURED", "message": "internal service authentication is not configured",
			})
			return
		}
		provided := strings.TrimSpace(c.GetHeader("X-Internal-Service-Token"))
		if subtle.ConstantTimeCompare([]byte(provided), []byte(expectedToken)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code": "SERVICE_AUTH_FAILED", "message": "invalid internal service identity",
			})
			return
		}
		c.Next()
	}
}

func (h *RuntimeHandler) ConsumeEvent(c *gin.Context) {
	var body RuntimeEventEnvelope
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_RUNTIME_EVENT", "message": err.Error()})
		return
	}
	if body.ProtocolVersion != "2.0" || body.Sequence <= 0 || body.Attempt <= 0 || body.OccurredAt.IsZero() {
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_RUNTIME_EVENT", "message": "protocol 2.0, positive sequence/attempt, and occurred_at are required"})
		return
	}
	if _, err := uuid.Parse(body.TenantID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_RUNTIME_EVENT", "message": "tenant_id must be a UUID"})
		return
	}
	if _, err := uuid.Parse(body.RunID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_RUNTIME_EVENT", "message": "run_id must be a UUID"})
		return
	}
	payload := body.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_RUNTIME_EVENT", "message": "payload must be valid JSON"})
		return
	}
	payloadJSON := string(payloadBytes)
	applied, err := h.service.ApplyRuntimeEvent(c.Request.Context(), &RuntimeEvent{
		EventID:           body.EventID,
		TenantID:          body.TenantID,
		RunID:             body.RunID,
		Sequence:          body.Sequence,
		Attempt:           body.Attempt,
		EventType:         body.Type,
		PayloadJSON:       &payloadJSON,
		CheckpointVersion: body.CheckpointVersion,
		OccurredAt:        body.OccurredAt,
	})
	if err != nil {
		status := http.StatusInternalServerError
		code := "RUNTIME_EVENT_APPLY_FAILED"
		switch {
		case errors.Is(err, ErrDurableRunNotFound):
			status, code = http.StatusNotFound, "RUN_NOT_FOUND"
		case errors.Is(err, ErrStaleRunAttempt):
			status, code = http.StatusConflict, "STALE_RUN_ATTEMPT"
		case errors.Is(err, ErrCheckpointVersionConflict):
			status, code = http.StatusConflict, "CHECKPOINT_VERSION_CONFLICT"
		case errors.Is(err, ErrInvalidRunTransition):
			status, code = http.StatusConflict, "RUN_INVALID_STATE"
		case errors.Is(err, ErrRuntimeEventConflict):
			status, code = http.StatusConflict, "RUNTIME_EVENT_CONFLICT"
		}
		c.JSON(status, gin.H{"code": code, "message": err.Error()})
		return
	}
	if applied {
		h.notifyRunEvent(c.Request.Context(), body)
	}
	c.JSON(http.StatusOK, gin.H{
		"protocol_version": "2.0",
		"event_id":         body.EventID,
		"applied":          applied,
	})
}
