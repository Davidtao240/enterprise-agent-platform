package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type fakeGatewayRepo struct {
	graph      *Graph
	runCreated *AgentRunLog
	runStatus  string
	runOutput  *string
}

func (f *fakeGatewayRepo) FindGraphByKey(ctx context.Context, graphKey string) (*Graph, error) {
	return f.graph, nil
}

func (f *fakeGatewayRepo) FindDomainPolicy(ctx context.Context, businessAppCode string) (*DomainPolicy, error) {
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
	return nil
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
	gateway := NewGateway(nil, nil, server.URL)
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
