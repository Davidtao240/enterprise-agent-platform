package workflow

import (
	"context"
	"testing"

	"github.com/enterprise-agent-platform/go-platform/internal/agent"
)

type fakeApprovalCreator struct {
	tasks []*agent.ApprovalTask
}

func (f *fakeApprovalCreator) CreateApprovalTask(_ context.Context, task *agent.ApprovalTask) error {
	f.tasks = append(f.tasks, task)
	return nil
}

func runningAgentNode(instanceID string) *Service {
	repo := &fakeWorkflowRepo{
		inst: testInstance(StatusRunning),
		nodes: map[string]NodeInstance{
			"node-agent": {
				ID: "node-agent", WorkflowInstanceID: "workflow-1", NodeKey: "agent_graph",
				NodeType: NodeTypeAgentGraph, Status: NodeStatusRunning,
			},
		},
	}
	return &Service{repo: repo, engine: NewEngine()}
}

func TestCompleteAgentRunNodeFailedPath(t *testing.T) {
	svc := runningAgentNode("workflow-1")
	if err := svc.CompleteAgentRunNode(context.Background(), "node-agent", "run-1", "failed", nil, nil); err != nil {
		t.Fatalf("CompleteAgentRunNode failed: %v", err)
	}
	repo := svc.repo.(*fakeWorkflowRepo)
	if got := repo.nodeState["node-agent"]; got != NodeStatusFailed {
		t.Fatalf("node status = %q, want failed", got)
	}
}

func TestCompleteAgentRunNodeIdempotentOnTerminalNode(t *testing.T) {
	repo := &fakeWorkflowRepo{
		inst: testInstance(StatusRunning),
		nodes: map[string]NodeInstance{
			"node-agent": {
				ID: "node-agent", WorkflowInstanceID: "workflow-1", NodeKey: "agent_graph",
				NodeType: NodeTypeAgentGraph, Status: NodeStatusSucceeded,
			},
		},
	}
	svc := &Service{repo: repo, engine: NewEngine()}
	if err := svc.CompleteAgentRunNode(context.Background(), "node-agent", "run-1", "succeeded", nil, nil); err != nil {
		t.Fatalf("idempotent completion must not error: %v", err)
	}
	if len(repo.nodeState) != 0 {
		t.Fatalf("terminal node state changed: %#v", repo.nodeState)
	}
}

func TestRunEventBridgeInterruptCreatesApprovalAndWaits(t *testing.T) {
	svc := runningAgentNode("workflow-1")
	approvals := &fakeApprovalCreator{}
	bridge := NewRunEventBridge(svc, approvals)

	err := bridge.OnRunInterrupted(context.Background(), "tenant-1", "run-1", "workflow-1", "node-agent", "interrupt-1", "human")
	if err != nil {
		t.Fatalf("OnRunInterrupted: %v", err)
	}
	repo := svc.repo.(*fakeWorkflowRepo)
	if got := repo.nodeState["node-agent"]; got != NodeStatusWaitingReview {
		t.Fatalf("node status = %q, want waiting_review", got)
	}
	if repo.instanceState[len(repo.instanceState)-1] != StatusWaitingReview {
		t.Fatalf("last instance state = %q, want waiting_review", repo.instanceState)
	}
	if len(approvals.tasks) != 1 {
		t.Fatalf("approval tasks = %d, want 1", len(approvals.tasks))
	}
	task := approvals.tasks[0]
	if task.DurableRunID == nil || *task.DurableRunID != "run-1" || task.InterruptID == nil || *task.InterruptID != "interrupt-1" {
		t.Fatalf("approval task not linked to runtime interrupt: %#v", task)
	}

	// 幂等:节点已 waiting_review,重复中断不重复建审批。
	if err := bridge.OnRunInterrupted(context.Background(), "tenant-1", "run-1", "workflow-1", "node-agent", "interrupt-1", "human"); err != nil {
		t.Fatalf("duplicate interrupt must not error: %v", err)
	}
	if len(approvals.tasks) != 1 {
		t.Fatalf("duplicate interrupt created extra approval tasks: %d", len(approvals.tasks))
	}
}
