package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRuntimeV2ClientStartResumeCancelContract(t *testing.T) {
	requests := make(chan *http.Request, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Clone(context.Background())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(RuntimeV2AcceptedResponse{
			ProtocolVersion: "2.0", RunID: "run-1", Status: "queued",
			AcceptedAt: time.Now().UTC(),
		})
	}))
	defer server.Close()
	client := NewRuntimeV2Client(server.URL, "service-token")

	if _, err := client.Start(context.Background(), &RuntimeV2StartRequest{
		ProtocolVersion: "2.0", RunID: "run-1", ThreadID: "thread-1", TraceID: "trace-1",
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := client.Resume(context.Background(), &RuntimeV2ResumeRequest{
		ProtocolVersion: "2.0", RunID: "run-1", InterruptID: "interrupt-1",
		ExpectedCheckpointVersion: 1, IdempotencyKey: "resume-1", ResumeInput: map[string]any{},
	}); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if _, err := client.Cancel(context.Background(), &RuntimeV2CancelRequest{
		ProtocolVersion: "2.0", RunID: "run-1", Reason: "test",
		RequestedBy: "user-1", IdempotencyKey: "cancel-1",
	}); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	wantPaths := []string{
		"/internal/v2/agent-runs",
		"/internal/v2/agent-runs/run-1/resume",
		"/internal/v2/agent-runs/run-1/cancel",
	}
	for _, wantPath := range wantPaths {
		request := <-requests
		if request.URL.Path != wantPath {
			t.Fatalf("path = %s, want %s", request.URL.Path, wantPath)
		}
		if request.Header.Get("X-Internal-Service-Token") != "service-token" {
			t.Fatal("Runtime V2 request did not carry service identity")
		}
	}
}
