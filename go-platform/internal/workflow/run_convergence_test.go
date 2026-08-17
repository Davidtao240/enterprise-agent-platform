package workflow

import (
	"context"
	"testing"
	"time"

	"github.com/enterprise-agent-platform/go-platform/internal/agent"
)

type fakeStaleRunLister struct {
	runs      []agent.StaleV1Run
	err       error
	gotBefore time.Time
}

func (f *fakeStaleRunLister) ListStaleRunningRuns(_ context.Context, staleBefore time.Time) ([]agent.StaleV1Run, error) {
	f.gotBefore = staleBefore
	return f.runs, f.err
}

func staleRun(attempt int) agent.StaleV1Run {
	return agent.StaleV1Run{
		RunID:              "run-1",
		TenantID:           "tenant-1",
		WorkflowInstanceID: "workflow-1",
		NodeInstanceID:     "node-agent",
		Attempt:            attempt,
		GraphKey:           "finance_operating_report_graph",
	}
}

func TestConvergenceScannerRequeuesStaleRunningNode(t *testing.T) {
	repo := &fakeWorkflowRepo{
		inst: testInstance(StatusRunning),
		nodes: map[string]NodeInstance{
			"node-agent": {
				ID: "node-agent", WorkflowInstanceID: "workflow-1", NodeKey: "agent_graph",
				NodeType: NodeTypeAgentGraph, Status: NodeStatusRunning, RetryCount: 0,
			},
		},
	}
	lister := &fakeStaleRunLister{runs: []agent.StaleV1Run{staleRun(1)}}
	enqueuer := &fakeNodeEnqueuer{}
	scanner := NewRunConvergenceScanner(lister, repo, enqueuer, nil, nil, 10*time.Minute)

	count, err := scanner.ScanOnce(context.Background())
	if err != nil {
		t.Fatalf("ScanOnce: %v", err)
	}
	if count != 1 {
		t.Fatalf("requeued = %d, want 1", count)
	}
	if len(enqueuer.got) != 1 {
		t.Fatalf("enqueued tasks = %d, want 1", len(enqueuer.got))
	}
	payload := enqueuer.got[0]
	if payload.WorkflowInstanceID != "workflow-1" || payload.NodeInstanceID != "node-agent" ||
		payload.Attempt != 1 || payload.NodeType != NodeTypeAgentGraph ||
		payload.GraphKey != "finance_operating_report_graph" || payload.TraceID != "trace-1" {
		t.Fatalf("unexpected requeued payload: %#v", payload)
	}
	wantBefore := time.Now().Add(-10 * time.Minute)
	if lister.gotBefore.Sub(wantBefore) > time.Minute || lister.gotBefore.Sub(wantBefore) < -time.Minute {
		t.Fatalf("staleBefore = %v, want ~%v", lister.gotBefore, wantBefore)
	}
}

func TestConvergenceScannerSkipsNonRunningContexts(t *testing.T) {
	tests := []struct {
		name          string
		nodeStatus    string
		nodeRetry     int
		runAttempt    int
		workflowState string
	}{
		{name: "node terminal", nodeStatus: NodeStatusSucceeded, nodeRetry: 0, runAttempt: 1, workflowState: StatusRunning},
		{name: "node advanced attempt", nodeStatus: NodeStatusRunning, nodeRetry: 1, runAttempt: 1, workflowState: StatusRunning},
		{name: "workflow not running", nodeStatus: NodeStatusRunning, nodeRetry: 0, runAttempt: 1, workflowState: StatusCancelled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeWorkflowRepo{
				inst: testInstance(tt.workflowState),
				nodes: map[string]NodeInstance{
					"node-agent": {
						ID: "node-agent", WorkflowInstanceID: "workflow-1", NodeKey: "agent_graph",
						NodeType: NodeTypeAgentGraph, Status: tt.nodeStatus, RetryCount: tt.nodeRetry,
					},
				},
			}
			lister := &fakeStaleRunLister{runs: []agent.StaleV1Run{staleRun(tt.runAttempt)}}
			enqueuer := &fakeNodeEnqueuer{}
			scanner := NewRunConvergenceScanner(lister, repo, enqueuer, nil, nil, 10*time.Minute)

			count, err := scanner.ScanOnce(context.Background())
			if err != nil {
				t.Fatalf("ScanOnce: %v", err)
			}
			if count != 0 || len(enqueuer.got) != 0 {
				t.Fatalf("requeued = %d tasks=%d, want 0/0", count, len(enqueuer.got))
			}
		})
	}
}

func TestConvergenceScannerRequiresConfiguration(t *testing.T) {
	scanner := NewRunConvergenceScanner(nil, nil, nil, nil, nil, 10*time.Minute)
	if _, err := scanner.ScanOnce(context.Background()); err == nil {
		t.Fatal("expected configuration error, got nil")
	}
}

type fakeAdvanceLister struct {
	advances []RunAdvance
}

func (f *fakeAdvanceLister) ListTerminalRunsNeedingAdvance(_ context.Context, limit int) ([]RunAdvance, error) {
	return f.advances, nil
}

func TestConvergenceScannerReconcilesTerminalRunWithRunningNode(t *testing.T) {
	repo := &fakeWorkflowRepo{
		inst: testInstance(StatusRunning),
		nodes: map[string]NodeInstance{
			"node-agent": {
				ID: "node-agent", WorkflowInstanceID: "workflow-1", NodeKey: "agent_graph",
				NodeType: NodeTypeAgentGraph, Status: NodeStatusRunning,
			},
		},
	}
	svc := &Service{repo: repo, engine: NewEngine()}
	scanner := NewRunConvergenceScanner(
		&fakeStaleRunLister{}, repo, &fakeNodeEnqueuer{}, svc,
		&fakeAdvanceLister{advances: []RunAdvance{
			{RunID: "run-1", TenantID: "tenant-1", WorkflowInstanceID: "workflow-1", NodeInstanceID: "node-agent", Status: "failed"},
		}},
		10*time.Minute,
	)

	if _, err := scanner.ScanOnce(context.Background()); err != nil {
		t.Fatalf("ScanOnce: %v", err)
	}
	if got := repo.nodeState["node-agent"]; got != NodeStatusFailed {
		t.Fatalf("reconciled node status = %q, want failed", got)
	}
}
