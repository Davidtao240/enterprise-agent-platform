package agent

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/enterprise-agent-platform/go-platform/internal/trace"
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
	tracer  traceSink
}

// traceSink M5-A: L2/L5/L6 Trace 事件投递 (由 internal/trace.Recorder 实现)。
type traceSink interface {
	Record(ctx context.Context, events ...*trace.Event)
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

// SetTraceSink wires M5-A trace recording for applied runtime events (L2/L5/L6).
func (h *RuntimeHandler) SetTraceSink(tracer traceSink) { h.tracer = tracer }

// recordTraceEvent maps an applied runtime event to a six-layer trace event.
// run.queued/started/succeeded/failed/cancelled -> L2; interrupted/resumed -> L6;
// checkpoint.saved -> L5. Best-effort: failures are logged, never returned.
func (h *RuntimeHandler) recordTraceEvent(ctx context.Context, body RuntimeEventEnvelope) {
	if h.tracer == nil {
		return
	}
	var layer, eventType string
	metadata := map[string]any{"run_id": body.RunID, "attempt": body.Attempt}
	switch body.Type {
	case RuntimeEventRunQueued:
		layer, eventType = trace.LayerRun, "queued"
	case RuntimeEventRunStarted:
		layer, eventType = trace.LayerRun, "start"
	case RuntimeEventRunSucceeded:
		layer, eventType = trace.LayerRun, "end"
	case RuntimeEventRunFailed:
		layer, eventType = trace.LayerRun, "error"
	case RuntimeEventRunCancelled:
		layer, eventType = trace.LayerRun, "end"
		metadata["cancelled"] = true
	case RuntimeEventRunInterrupted:
		layer, eventType = trace.LayerInterrupt, "interrupted"
	case RuntimeEventRunResumed:
		layer, eventType = trace.LayerInterrupt, "resumed"
	case RuntimeEventCheckpointSaved:
		layer, eventType = trace.LayerCheckpnt, "saved"
	default:
		return
	}
	if body.CheckpointVersion != nil {
		metadata["checkpoint_version"] = *body.CheckpointVersion
	}
	// trace_id 优先使用 Run 关联的工作流级 TraceID(与 tool_calls 一致);
	// 查询失败时退化为 run_id,保证事件不丢。
	traceID := body.RunID
	if h.runs != nil {
		if run, err := h.runs.FindDurableRunByIDForTenant(ctx, body.TenantID, body.RunID); err == nil && run.TraceID != "" {
			traceID = run.TraceID
		}
	}
	h.tracer.Record(ctx, &trace.Event{
		TraceID:   traceID,
		Layer:     layer,
		EventType: eventType,
		TenantID:  body.TenantID,
		Timestamp: body.OccurredAt,
		Metadata:  metadata,
	})
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
		h.recordTraceEvent(c.Request.Context(), body)
	}
	c.JSON(http.StatusOK, gin.H{
		"protocol_version": "2.0",
		"event_id":         body.EventID,
		"applied":          applied,
	})
}
