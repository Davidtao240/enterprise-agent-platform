package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type fakeRuntimeEventApplier struct {
	event   *RuntimeEvent
	applied bool
	err     error
}

func (f *fakeRuntimeEventApplier) ApplyRuntimeEvent(_ context.Context, event *RuntimeEvent) (bool, error) {
	f.event = event
	return f.applied, f.err
}

func TestRuntimeEventRouteRequiresServiceIdentityAndAppliesEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeRuntimeEventApplier{applied: true}
	router := gin.New()
	group := router.Group("/internal/v2")
	group.Use(RequireInternalServiceToken("test-service-token"))
	group.POST("/runtime-events", NewRuntimeHandler(service).ConsumeEvent)

	payload := RuntimeEventEnvelope{
		ProtocolVersion: "2.0",
		EventID:         "event-1",
		TenantID:        "00000000-0000-0000-0000-000000000010",
		RunID:           "00000000-0000-0000-0000-000000000020",
		Sequence:        1,
		Attempt:         1,
		Type:            RuntimeEventRunStarted,
		OccurredAt:      time.Now().UTC(),
		Payload:         map[string]any{},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	unauthorized := httptest.NewRequest(http.MethodPost, "/internal/v2/runtime-events", bytes.NewReader(body))
	unauthorized.Header.Set("Content-Type", "application/json")
	unauthorizedRecorder := httptest.NewRecorder()
	router.ServeHTTP(unauthorizedRecorder, unauthorized)
	if unauthorizedRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want 401", unauthorizedRecorder.Code)
	}

	authorized := httptest.NewRequest(http.MethodPost, "/internal/v2/runtime-events", bytes.NewReader(body))
	authorized.Header.Set("Content-Type", "application/json")
	authorized.Header.Set("X-Internal-Service-Token", "test-service-token")
	authorizedRecorder := httptest.NewRecorder()
	router.ServeHTTP(authorizedRecorder, authorized)
	if authorizedRecorder.Code != http.StatusOK {
		t.Fatalf("authorized status = %d body=%s", authorizedRecorder.Code, authorizedRecorder.Body.String())
	}
	if service.event == nil || service.event.EventID != payload.EventID || service.event.TenantID != payload.TenantID {
		t.Fatalf("event was not mapped to trusted envelope: %#v", service.event)
	}
}

func TestRuntimeEventRouteMapsAttemptConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeRuntimeEventApplier{err: ErrStaleRunAttempt}
	router := gin.New()
	router.POST("/internal/v2/runtime-events", NewRuntimeHandler(service).ConsumeEvent)
	body := `{"protocol_version":"2.0","event_id":"event-1","tenant_id":"00000000-0000-0000-0000-000000000010","run_id":"00000000-0000-0000-0000-000000000020","sequence":1,"attempt":1,"type":"run.started","occurred_at":"2026-08-16T10:00:00Z","payload":{}}`
	request := httptest.NewRequest(http.MethodPost, "/internal/v2/runtime-events", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d body=%s, want 409", recorder.Code, recorder.Body.String())
	}
}

func TestRuntimeEventRouteRejectsNonUUIDIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeRuntimeEventApplier{applied: true}
	router := gin.New()
	router.POST("/internal/v2/runtime-events", NewRuntimeHandler(service).ConsumeEvent)

	body := `{"protocol_version":"2.0","event_id":"event-1","tenant_id":"not-a-uuid","run_id":"00000000-0000-0000-0000-000000000020","sequence":1,"attempt":1,"type":"run.started","occurred_at":"2026-08-16T10:00:00Z","payload":{}}`
	request := httptest.NewRequest(http.MethodPost, "/internal/v2/runtime-events", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", recorder.Code, recorder.Body.String())
	}
}
