package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/enterprise-agent-platform/go-platform/internal/agent"
	"github.com/enterprise-agent-platform/go-platform/internal/audit"
)

type fakeWorkflowRepo struct {
	inst          *Instance
	tmpl          *Template
	nodes         map[string]NodeInstance
	nodesByFlow   []NodeInstance
	instanceState []string
	nodeState     map[string]string
	nodeErrors    map[string]string
}

func (f *fakeWorkflowRepo) FindTemplatesByBusinessApp(ctx context.Context, businessAppCode string) ([]Template, error) {
	return nil, nil
}

func (f *fakeWorkflowRepo) FindTemplateByBusinessAndKey(ctx context.Context, businessAppCode, templateKey string) (*Template, error) {
	return f.tmpl, nil
}

func (f *fakeWorkflowRepo) FindInstanceByID(ctx context.Context, id string) (*Instance, error) {
	return f.inst, nil
}

func (f *fakeWorkflowRepo) ListInstances(ctx context.Context, businessAppCode, status, createdBy string, page, pageSize int) ([]Instance, int, error) {
	return nil, 0, nil
}

func (f *fakeWorkflowRepo) FindNodeInstancesByWorkflow(ctx context.Context, workflowInstanceID string) ([]NodeInstance, error) {
	if f.nodesByFlow != nil {
		return f.nodesByFlow, nil
	}
	nodes := make([]NodeInstance, 0, len(f.nodes))
	for _, node := range f.nodes {
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func (f *fakeWorkflowRepo) FindNodeInstanceByID(ctx context.Context, id string) (*NodeInstance, error) {
	node, ok := f.nodes[id]
	if !ok {
		return nil, errors.New("node not found")
	}
	return &node, nil
}

func (f *fakeWorkflowRepo) CreateInstance(ctx context.Context, inst *Instance) error {
	return nil
}

func (f *fakeWorkflowRepo) CreateNodeInstances(ctx context.Context, workflowInstanceID string, nodes []TemplateNode) error {
	return nil
}

func (f *fakeWorkflowRepo) UpdateInstanceStatus(ctx context.Context, id, status string, startedAt, finishedAt *time.Time) error {
	f.instanceState = append(f.instanceState, status)
	if f.inst != nil {
		f.inst.Status = status
	}
	return nil
}

func (f *fakeWorkflowRepo) UpdateInstanceOutput(ctx context.Context, id string, outputJSON string) error {
	return nil
}

func (f *fakeWorkflowRepo) UpdateNodeStatus(ctx context.Context, id, status string, startedAt, finishedAt *time.Time) error {
	if f.nodeState == nil {
		f.nodeState = map[string]string{}
	}
	f.nodeState[id] = status
	node := f.nodes[id]
	node.Status = status
	f.nodes[id] = node
	return nil
}

func (f *fakeWorkflowRepo) UpdateNodeInput(ctx context.Context, id string, inputJSON string) error {
	return nil
}

func (f *fakeWorkflowRepo) UpdateNodeOutput(ctx context.Context, id string, outputJSON string) error {
	return nil
}

func (f *fakeWorkflowRepo) UpdateNodeError(ctx context.Context, id string, errorJSON string) error {
	if f.nodeErrors == nil {
		f.nodeErrors = map[string]string{}
	}
	f.nodeErrors[id] = errorJSON
	node := f.nodes[id]
	node.RetryCount++
	f.nodes[id] = node
	return nil
}

func (f *fakeWorkflowRepo) FindPendingNodeByWorkflow(ctx context.Context, workflowInstanceID string) (*NodeInstance, error) {
	return nil, nil
}

func (f *fakeWorkflowRepo) CancelPendingNodes(ctx context.Context, workflowInstanceID string) error {
	return nil
}

type fakeNodeEnqueuer struct {
	err error
	got []*ExecuteNodePayload
}

type fakeWorkflowAuditLogger struct {
	entries []audit.AuditLogEntry
}

func (f *fakeWorkflowAuditLogger) InsertLog(ctx context.Context, entry audit.AuditLogEntry) (string, time.Time, error) {
	f.entries = append(f.entries, entry)
	return "audit-1", time.Now(), nil
}

func (f *fakeNodeEnqueuer) EnqueueExecuteNode(payload *ExecuteNodePayload) error {
	f.got = append(f.got, payload)
	return f.err
}

type fakeWorkerAgentRepo struct {
	createErr error
}

func (f *fakeWorkerAgentRepo) CreateApprovalTask(ctx context.Context, task *agent.ApprovalTask) error {
	return f.createErr
}

func (f *fakeWorkerAgentRepo) LatestRunOutput(ctx context.Context, workflowInstanceID string) (*string, error) {
	return nil, nil
}

func testTemplate(def string) *Template {
	return &Template{
		BusinessAppCode:     "finance",
		WorkflowTemplateKey: "finance_operating_report",
		DefinitionJSON:      def,
	}
}

func testInstance(status string) *Instance {
	return &Instance{
		ID:                  "workflow-1",
		BusinessAppCode:     "finance",
		WorkflowTemplateKey: "finance_operating_report",
		GraphKey:            "finance_operating_report_graph",
		Title:               "Report",
		Status:              status,
		TraceID:             "trace-1",
		CreatedBy:           "user-1",
	}
}

func TestStartWorkflowReturnsErrorAndMarksFailedWhenEntryEnqueueFails(t *testing.T) {
	repo := &fakeWorkflowRepo{
		inst: testInstance(StatusDraft),
		tmpl: testTemplate(`{
			"nodes":[{"id":"upload","type":"file_upload","name":"Upload"}],
			"edges":[]
		}`),
		nodes: map[string]NodeInstance{
			"node-upload": {ID: "node-upload", WorkflowInstanceID: "workflow-1", NodeKey: "upload", NodeType: NodeTypeFileUpload, Status: NodeStatusPending},
		},
		nodesByFlow: []NodeInstance{{ID: "node-upload", WorkflowInstanceID: "workflow-1", NodeKey: "upload", NodeType: NodeTypeFileUpload, Status: NodeStatusPending}},
	}
	enqueuer := &fakeNodeEnqueuer{err: errors.New("redis down")}
	svc := &Service{repo: repo, engine: NewEngine(), worker: enqueuer}

	resp, err := svc.StartWorkflow(context.Background(), "user-1", "workflow-1")
	if err == nil {
		t.Fatal("expected enqueue error, got nil")
	}
	if resp != nil {
		t.Fatalf("expected nil response on enqueue failure, got %#v", resp)
	}
	if got := repo.instanceState[len(repo.instanceState)-1]; got != StatusFailed {
		t.Fatalf("last instance status = %s, want failed; all states=%v", got, repo.instanceState)
	}
}

func TestOnNodeCompletedReturnsErrorAndMarksNextNodeFailedWhenEnqueueFails(t *testing.T) {
	repo := &fakeWorkflowRepo{
		inst: testInstance(StatusRunning),
		tmpl: testTemplate(`{
			"nodes":[
				{"id":"upload","type":"file_upload","name":"Upload"},
				{"id":"agent_graph","type":"agent_graph","name":"Analyze","max_retries":1}
			],
			"edges":[{"from":"upload","to":"agent_graph","when":"succeeded"}]
		}`),
		nodes: map[string]NodeInstance{
			"node-upload": {ID: "node-upload", WorkflowInstanceID: "workflow-1", NodeKey: "upload", NodeType: NodeTypeFileUpload, Status: NodeStatusSucceeded},
			"node-agent":  {ID: "node-agent", WorkflowInstanceID: "workflow-1", NodeKey: "agent_graph", NodeType: NodeTypeAgentGraph, Status: NodeStatusPending, MaxRetries: 1},
		},
		nodesByFlow: []NodeInstance{
			{ID: "node-upload", WorkflowInstanceID: "workflow-1", NodeKey: "upload", NodeType: NodeTypeFileUpload, Status: NodeStatusSucceeded},
			{ID: "node-agent", WorkflowInstanceID: "workflow-1", NodeKey: "agent_graph", NodeType: NodeTypeAgentGraph, Status: NodeStatusPending, MaxRetries: 1},
		},
	}
	enqueuer := &fakeNodeEnqueuer{err: errors.New("redis down")}
	svc := &Service{repo: repo, engine: NewEngine(), worker: enqueuer}

	err := svc.OnNodeCompleted(context.Background(), "node-upload", EdgeWhenSucceeded)
	if err == nil {
		t.Fatal("expected enqueue error, got nil")
	}
	if got := repo.nodeState["node-agent"]; got != NodeStatusFailed {
		t.Fatalf("next node status = %s, want failed", got)
	}
	if got := repo.instanceState[len(repo.instanceState)-1]; got != StatusFailed {
		t.Fatalf("last instance status = %s, want failed; all states=%v", got, repo.instanceState)
	}
	if !strings.Contains(repo.nodeErrors["node-agent"], "enqueue next node") {
		t.Fatalf("node error did not record enqueue failure: %s", repo.nodeErrors["node-agent"])
	}
}

func TestHandleHumanReviewFailsNodeWhenApprovalTaskCreationFails(t *testing.T) {
	repo := &fakeWorkflowRepo{
		inst: testInstance(StatusRunning),
		tmpl: testTemplate(`{
			"nodes":[{"id":"human_review","type":"human_review","name":"Review","role":"business_reviewer","max_retries":1}],
			"edges":[]
		}`),
		nodes: map[string]NodeInstance{
			"node-review": {ID: "node-review", WorkflowInstanceID: "workflow-1", NodeKey: "human_review", NodeType: NodeTypeHumanReview, Status: NodeStatusRunning, MaxRetries: 1},
		},
	}
	svc := &Service{repo: repo, engine: NewEngine()}
	worker := &Worker{
		svc:       svc,
		agentRepo: &fakeWorkerAgentRepo{createErr: errors.New("db insert failed")},
	}

	err := worker.handleHumanReview(context.Background(), &ExecuteNodePayload{
		WorkflowInstanceID: "workflow-1",
		NodeInstanceID:     "node-review",
		NodeKey:            "human_review",
	})
	if err == nil {
		t.Fatal("expected approval task creation error, got nil")
	}
	if got := repo.nodeState["node-review"]; got != NodeStatusFailed {
		t.Fatalf("review node status = %s, want failed", got)
	}
	for _, status := range repo.instanceState {
		if status == StatusWaitingReview {
			t.Fatalf("workflow must not enter waiting_review when approval task creation fails: %v", repo.instanceState)
		}
	}
}

func TestRetryNodeWritesAuditLog(t *testing.T) {
	repo := &fakeWorkflowRepo{
		inst: testInstance(StatusFailed),
		nodes: map[string]NodeInstance{
			"node-agent": {
				ID:                 "node-agent",
				WorkflowInstanceID: "workflow-1",
				NodeKey:            "agent_graph",
				NodeType:           NodeTypeAgentGraph,
				Status:             NodeStatusFailed,
				RetryCount:         0,
				MaxRetries:         2,
			},
		},
	}
	enqueuer := &fakeNodeEnqueuer{}
	auditLog := &fakeWorkflowAuditLogger{}
	svc := &Service{repo: repo, auditRepo: auditLog, engine: NewEngine(), worker: enqueuer}

	_, err := svc.RetryNode(context.Background(), "user-1", "workflow-1", "node-agent")
	if err != nil {
		t.Fatalf("RetryNode: %v", err)
	}

	found := false
	for _, entry := range auditLog.entries {
		if entry.Action == "workflow_node_retried" && entry.ActorUserID != nil && *entry.ActorUserID == "user-1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected workflow_node_retried audit entry, got %#v", auditLog.entries)
	}
}
