package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type fakeRuntimeControlRepo struct {
	activeRuns    []StaleV1Run
	interruptVers map[string]int64
}

func (f *fakeRuntimeControlRepo) ListActiveRunsForWorkflow(_ context.Context, tenantID, workflowInstanceID string) ([]StaleV1Run, error) {
	return f.activeRuns, nil
}

func (f *fakeRuntimeControlRepo) FindInterruptCheckpointVersion(_ context.Context, tenantID, runID, interruptID string) (int64, error) {
	return f.interruptVers[interruptID], nil
}

func TestRuntimeControllerResumeInterruptedRun(t *testing.T) {
	var got RuntimeV2ResumeRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(RuntimeV2AcceptedResponse{
			ProtocolVersion: "2.0", RunID: "run-1", Status: "running",
			AcceptedAt: time.Now().UTC(), CheckpointVersion: 3,
		})
	}))
	defer server.Close()

	repo := &fakeRuntimeControlRepo{interruptVers: map[string]int64{"interrupt-1": 3}}
	controller := NewRuntimeController(NewRuntimeV2Client(server.URL, "token"), repo)

	err := controller.ResumeInterruptedRun(context.Background(), "tenant-1", "run-1", "interrupt-1",
		map[string]any{"decision": "approved"})
	if err != nil {
		t.Fatalf("ResumeInterruptedRun: %v", err)
	}
	if got.RunID != "run-1" || got.InterruptID != "interrupt-1" || got.ExpectedCheckpointVersion != 3 ||
		got.IdempotencyKey != "approval:interrupt-1" || got.ResumeInput["decision"] != "approved" {
		t.Fatalf("unexpected resume request: %#v", got)
	}
}

func TestRuntimeControllerCancelWorkflowRuns(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(RuntimeV2AcceptedResponse{
			ProtocolVersion: "2.0", RunID: "run-x", Status: "cancelled",
			AcceptedAt: time.Now().UTC(),
		})
	}))
	defer server.Close()

	repo := &fakeRuntimeControlRepo{activeRuns: []StaleV1Run{
		{RunID: "run-1", TenantID: "tenant-1", WorkflowInstanceID: "workflow-1", NodeInstanceID: "node-1", Attempt: 1},
		{RunID: "run-2", TenantID: "tenant-1", WorkflowInstanceID: "workflow-1", NodeInstanceID: "node-2", Attempt: 1},
	}}
	controller := NewRuntimeController(NewRuntimeV2Client(server.URL, "token"), repo)

	if err := controller.CancelWorkflowRuns(context.Background(), "tenant-1", "workflow-1", "workflow cancelled", "user-1"); err != nil {
		t.Fatalf("CancelWorkflowRuns: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("cancel requests = %d, want 2", len(paths))
	}
}

func TestRuntimeControllerCancelNoopWithoutClient(t *testing.T) {
	controller := NewRuntimeController(nil, &fakeRuntimeControlRepo{})
	if err := controller.CancelWorkflowRuns(context.Background(), "t", "w", "r", "u"); err != nil {
		t.Fatalf("cancel without client should be no-op: %v", err)
	}
	if err := controller.ResumeInterruptedRun(context.Background(), "t", "r", "i", nil); err == nil {
		t.Fatal("resume without client should error")
	}
}
