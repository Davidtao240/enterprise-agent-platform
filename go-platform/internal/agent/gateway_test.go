package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/enterprise-agent-platform/go-platform/internal/audit"
)

type fakeGatewayRepo struct {
	graph      *Graph
	graphErr   error
	policy     *DomainPolicy
	policyErr  error
	runCreated *AgentRunLog
	runStatus  string
	runOutput  *string
	runError   *string
}

func (f *fakeGatewayRepo) FindGraphByKey(ctx context.Context, graphKey string) (*Graph, error) {
	if f.graphErr != nil {
		return nil, f.graphErr
	}
	return f.graph, nil
}

func (f *fakeGatewayRepo) FindDomainPolicy(ctx context.Context, businessAppCode string) (*DomainPolicy, error) {
	if f.policyErr != nil {
		return nil, f.policyErr
	}
	if f.policy != nil {
		return f.policy, nil
	}
	return &DomainPolicy{
		BusinessAppCode:     businessAppCode,
		AllowedAgentDomains: `["finance","shared"]`,
		Status:              "active",
	}, nil
}

func (f *fakeGatewayRepo) CreateRunLog(ctx context.Context, log *AgentRunLog) error {
	copy := *log
	f.runCreated = &copy
	return nil
}

func (f *fakeGatewayRepo) UpdateRunLog(ctx context.Context, runID, status string, outputSummaryJSON, usageJSON, errorJSON *string, finishedAt *time.Time, durationMs *int) error {
	f.runStatus = status
	f.runOutput = outputSummaryJSON
	f.runError = errorJSON
	return nil
}

type fakeGatewayAuditRepo struct {
	entries []audit.AuditLogEntry
}

func (f *fakeGatewayAuditRepo) InsertLog(ctx context.Context, entry audit.AuditLogEntry) (string, time.Time, error) {
	f.entries = append(f.entries, entry)
	return "audit-1", time.Now(), nil
}

func TestGatewayExecuteCallsAgentServiceAndUpdatesRunLog(t *testing.T) {
	var requestBody AgentRunRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/v1/agent-runs" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(AgentRunResponse{
			RunID:    "python-run-1",
			GraphKey: "finance_operating_report_graph",
			Status:   "succeeded",
			Output: map[string]any{
				"summary": "ok",
			},
			Usage: &AgentUsage{Model: "mock", PromptTokens: 1, CompletionTokens: 1},
		})
	}))
	defer server.Close()

	repo := &fakeGatewayRepo{
		graph: &Graph{
			GraphKey:        "finance_operating_report_graph",
			BusinessAppCode: "finance",
			Status:          "active",
		},
	}
	gateway := NewGateway(nil, nil, server.URL, false)
	gateway.repo = repo

	resp, err := gateway.Execute(context.Background(), &AgentRunPayload{
		TraceID:             "trace-1",
		BusinessAppCode:     "finance",
		WorkflowTemplateKey: "finance_operating_report",
		GraphKey:            "finance_operating_report_graph",
		WorkflowInstanceID:  "00000000-0000-0000-0000-000000000001",
		NodeInstanceID:      "00000000-0000-0000-0000-000000000002",
		Input:               map[string]any{"workflow_input": `{"file_id":"file.csv"}`},
		UserID:              "00000000-0000-0000-0000-000000000003",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if resp.Status != "succeeded" {
		t.Fatalf("status = %s, want succeeded", resp.Status)
	}
	if requestBody.GraphKey != "finance_operating_report_graph" || requestBody.BusinessAppCode != "finance" {
		t.Fatalf("bad request body: %#v", requestBody)
	}
	if repo.runCreated == nil || repo.runCreated.Status != "running" {
		t.Fatalf("run log was not created as running: %#v", repo.runCreated)
	}
	if repo.runStatus != "succeeded" {
		t.Fatalf("updated run status = %s, want succeeded", repo.runStatus)
	}
	if repo.runOutput == nil || *repo.runOutput == "" {
		t.Fatal("expected output summary JSON to be persisted")
	}
}

// TestGatewayGraphNotFound tests that a missing graph_key returns an error.
func TestGatewayGraphNotFound(t *testing.T) {
	repo := &fakeGatewayRepo{
		graphErr: errors.New("no rows"),
	}
	gateway := NewGateway(nil, nil, "http://unused:8000", false)
	gateway.repo = repo

	_, err := gateway.Execute(context.Background(), &AgentRunPayload{
		GraphKey:        "nonexistent_graph",
		BusinessAppCode: "finance",
	})
	if err == nil {
		t.Fatal("expected error for missing graph_key, got nil")
	}
}

// TestGatewayGraphNotActive tests that a disabled graph is rejected.
func TestGatewayGraphNotActive(t *testing.T) {
	repo := &fakeGatewayRepo{
		graph: &Graph{
			GraphKey:        "disabled_graph",
			BusinessAppCode: "finance",
			Status:          "disabled",
		},
	}
	gateway := NewGateway(nil, nil, "http://unused:8000", false)
	gateway.repo = repo

	_, err := gateway.Execute(context.Background(), &AgentRunPayload{
		GraphKey:        "disabled_graph",
		BusinessAppCode: "finance",
	})
	if err == nil {
		t.Fatal("expected error for disabled graph, got nil")
	}
}

// TestGatewayDomainPolicyViolation tests cross-domain call rejection.
func TestGatewayDomainPolicyViolation(t *testing.T) {
	repo := &fakeGatewayRepo{
		graph: &Graph{
			GraphKey:        "hr_graph",
			BusinessAppCode: "hr",
			Status:          "active",
		},
		policy: &DomainPolicy{
			BusinessAppCode:     "finance",
			AllowedAgentDomains: `["finance","shared"]`,
			Status:              "active",
		},
	}
	gateway := NewGateway(nil, nil, "http://unused:8000", false)
	gateway.repo = repo

	// finance app trying to call hr graph → should be rejected
	_, err := gateway.Execute(context.Background(), &AgentRunPayload{
		GraphKey:        "hr_graph",
		BusinessAppCode: "finance",
	})
	if err == nil {
		t.Fatal("expected domain policy violation error, got nil")
	}
}

// TestGatewayAgentServiceFailure tests handling of Python service errors.
func TestGatewayAgentServiceFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(AgentRunResponse{
			RunID:    "python-run-err",
			GraphKey: "finance_operating_report_graph",
			Status:   "failed",
			Error:    &AgentRunError{Code: "GRAPH_EXECUTION_FAILED", Message: "LLM timeout"},
		})
	}))
	defer server.Close()

	repo := &fakeGatewayRepo{
		graph: &Graph{
			GraphKey:        "finance_operating_report_graph",
			BusinessAppCode: "finance",
			Status:          "active",
		},
	}
	gateway := NewGateway(nil, nil, server.URL, false)
	gateway.repo = repo

	resp, err := gateway.Execute(context.Background(), &AgentRunPayload{
		TraceID:            "trace-err",
		BusinessAppCode:    "finance",
		GraphKey:           "finance_operating_report_graph",
		WorkflowInstanceID: "00000000-0000-0000-0000-000000000001",
		NodeInstanceID:     "00000000-0000-0000-0000-000000000002",
	})
	if err != nil {
		t.Fatalf("Execute should not return transport error for agent failure: %v", err)
	}
	if resp.Status != "failed" {
		t.Fatalf("status = %s, want failed", resp.Status)
	}
	if resp.Error == nil || resp.Error.Code != "GRAPH_EXECUTION_FAILED" {
		t.Fatalf("expected GRAPH_EXECUTION_FAILED error, got %#v", resp.Error)
	}
	if repo.runStatus != "failed" {
		t.Fatalf("run log status = %s, want failed", repo.runStatus)
	}
}

func TestGatewayAgentServiceHTTPFailureRecordsRunLog(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream failed", http.StatusBadGateway)
	}))
	defer server.Close()

	repo := &fakeGatewayRepo{
		graph: &Graph{
			GraphKey:        "finance_operating_report_graph",
			BusinessAppCode: "finance",
			Status:          "active",
		},
	}
	auditRepo := &fakeGatewayAuditRepo{}
	gateway := NewGateway(nil, nil, server.URL, false)
	gateway.repo = repo
	gateway.auditRepo = auditRepo

	_, err := gateway.Execute(context.Background(), &AgentRunPayload{
		TraceID:            "trace-http-failed",
		BusinessAppCode:    "finance",
		GraphKey:           "finance_operating_report_graph",
		WorkflowInstanceID: "00000000-0000-0000-0000-000000000001",
		NodeInstanceID:     "00000000-0000-0000-0000-000000000002",
		UserID:             "00000000-0000-0000-0000-000000000003",
	})
	if err == nil {
		t.Fatal("expected HTTP failure error, got nil")
	}
	if repo.runStatus != "failed" {
		t.Fatalf("run log status = %s, want failed", repo.runStatus)
	}
	if repo.runError == nil || *repo.runError == "" {
		t.Fatal("expected error_json to be recorded")
	}
	actions := map[string]bool{}
	for _, entry := range auditRepo.entries {
		actions[entry.Action] = true
	}
	if !actions["agent_run_started"] || !actions["agent_run_failed"] {
		t.Fatalf("expected started and failed audit entries, got %#v", auditRepo.entries)
	}
}

func TestGatewayAgentServiceDecodeFailureRecordsRunLog(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"run_id":`))
	}))
	defer server.Close()

	repo := &fakeGatewayRepo{
		graph: &Graph{
			GraphKey:        "finance_operating_report_graph",
			BusinessAppCode: "finance",
			Status:          "active",
		},
	}
	gateway := NewGateway(nil, nil, server.URL, false)
	gateway.repo = repo

	_, err := gateway.Execute(context.Background(), &AgentRunPayload{
		TraceID:            "trace-decode-failed",
		BusinessAppCode:    "finance",
		GraphKey:           "finance_operating_report_graph",
		WorkflowInstanceID: "00000000-0000-0000-0000-000000000001",
		NodeInstanceID:     "00000000-0000-0000-0000-000000000002",
		UserID:             "00000000-0000-0000-0000-000000000003",
	})
	if err == nil {
		t.Fatal("expected decode failure error, got nil")
	}
	if repo.runStatus != "failed" {
		t.Fatalf("run log status = %s, want failed", repo.runStatus)
	}
	if repo.runError == nil || *repo.runError == "" {
		t.Fatal("expected error_json to be recorded")
	}
}

// TestGatewayDomainPolicyMissing_LooseMode tests that missing policy allows execution in loose mode.
func TestGatewayDomainPolicyMissing_LooseMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(AgentRunResponse{
			RunID:    "run-loose",
			GraphKey: "finance_operating_report_graph",
			Status:   "succeeded",
			Output:   map[string]any{},
		})
	}))
	defer server.Close()

	repo := &fakeGatewayRepo{
		graph: &Graph{
			GraphKey:        "finance_operating_report_graph",
			BusinessAppCode: "finance",
			Status:          "active",
		},
		policyErr: errors.New("no rows"),
	}
	gateway := NewGateway(nil, nil, server.URL, false)
	gateway.repo = repo
	// strictPolicy defaults to false (loose mode)

	resp, err := gateway.Execute(context.Background(), &AgentRunPayload{
		GraphKey:        "finance_operating_report_graph",
		BusinessAppCode: "finance",
	})
	if err != nil {
		t.Fatalf("loose mode should allow missing policy: %v", err)
	}
	if resp.Status != "succeeded" {
		t.Fatalf("status = %s, want succeeded", resp.Status)
	}
}
