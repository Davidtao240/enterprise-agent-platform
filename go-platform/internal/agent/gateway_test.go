package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/enterprise-agent-platform/go-platform/internal/audit"
)

type fakeGatewayRepo struct {
	graph       *Graph
	graphErr    error
	policy      *DomainPolicy
	policyErr   error
	runCreated  *AgentRunLog
	existingRun *DurableRun
	runStatus   string
	runOutput   *string
	runError    *string
	leaseHeld   bool
	acquired    bool
	heartbeats  int
	acquireErr  error
	// M1-C-B 迟到结果拒绝:completeErr 非空时 CompleteV1Run 返回该错误;
	// completeOwner 记录最近一次完成写入携带的 lease owner。
	completeErr   error
	completeOwner string
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

func (f *fakeGatewayRepo) StartV1Run(ctx context.Context, start *V1DurableRunStart) (*DurableRun, bool, error) {
	if f.existingRun != nil {
		copy := *f.existingRun
		return &copy, false, nil
	}
	f.runCreated = &AgentRunLog{
		RunID:              start.RunID,
		TenantID:           start.TenantID,
		TraceID:            start.TraceID,
		WorkflowInstanceID: start.WorkflowInstanceID,
		NodeInstanceID:     start.NodeInstanceID,
		BusinessAppCode:    start.BusinessAppCode,
		GraphKey:           start.GraphKey,
		Status:             RunStatusRunning,
		StartedAt:          &start.StartedAt,
	}
	return &DurableRun{
		ID:        start.RunID,
		TenantID:  start.TenantID,
		GraphKey:  start.GraphKey,
		Status:    RunStatusRunning,
		Attempt:   start.Attempt,
		StartedAt: &start.StartedAt,
	}, true, nil
}

func (f *fakeGatewayRepo) CompleteV1Run(ctx context.Context, completion *V1DurableRunCompletion) error {
	f.completeOwner = completion.LeaseOwner
	if f.completeErr != nil {
		return f.completeErr
	}
	f.runStatus = completion.Status
	f.runOutput = completion.OutputSummaryJSON
	f.runError = completion.ErrorJSON
	return nil
}

func (f *fakeGatewayRepo) FindRun(ctx context.Context, tenantID, runID string) (*DurableRun, error) {
	if f.existingRun != nil {
		copy := *f.existingRun
		return &copy, nil
	}
	if f.runCreated != nil {
		return &DurableRun{
			ID:       runID,
			TenantID: tenantID,
			GraphKey: f.runCreated.GraphKey,
			Status:   RunStatusRunning,
			Attempt:  1,
		}, nil
	}
	return nil, ErrDurableRunNotFound
}

func (f *fakeGatewayRepo) AcquireRunLease(ctx context.Context, tenantID, runID string, attempt int, owner string, ttl time.Duration) (bool, error) {
	if f.acquireErr != nil {
		return false, f.acquireErr
	}
	if f.leaseHeld {
		return false, nil
	}
	f.acquired = true
	return true, nil
}

func (f *fakeGatewayRepo) HeartbeatRunLease(ctx context.Context, tenantID, runID string, attempt int, owner string, ttl time.Duration) (bool, error) {
	f.heartbeats++
	return true, nil
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
	gateway.durable = repo

	resp, err := gateway.Execute(context.Background(), &AgentRunPayload{
		TraceID:             "trace-1",
		BusinessAppCode:     "finance",
		WorkflowTemplateKey: "finance_operating_report",
		GraphKey:            "finance_operating_report_graph",
		WorkflowInstanceID:  "00000000-0000-0000-0000-000000000001",
		NodeInstanceID:      "00000000-0000-0000-0000-000000000002",
		Input:               map[string]any{"workflow_input": `{"file_id":"file.csv"}`},
		UserID:              "00000000-0000-0000-0000-000000000003",
		TenantID:            "00000000-0000-0000-0000-000000000010",
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
	if requestBody.RunID == "" || requestBody.RunID != resp.RunID {
		t.Fatalf("control-plane run_id was not propagated: request=%q response=%q", requestBody.RunID, resp.RunID)
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
	gateway.durable = repo

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
	gateway.durable = repo

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
	gateway.durable = repo

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
	gateway.durable = repo

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
	gateway.durable = repo
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
	gateway.durable = repo

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
	gateway.durable = repo
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

func TestGatewayDuplicateAttemptDoesNotCallAgentServiceAgain(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		http.Error(w, "duplicate attempt should not reach Python", http.StatusInternalServerError)
	}))
	defer server.Close()

	repo := &fakeGatewayRepo{
		graph: &Graph{
			GraphKey:        "finance_operating_report_graph",
			Version:         "1.0.0",
			BusinessAppCode: "finance",
			Status:          "active",
		},
		existingRun: &DurableRun{
			ID:       "00000000-0000-0000-0000-000000000099",
			TenantID: "00000000-0000-0000-0000-000000000010",
			GraphKey: "finance_operating_report_graph",
			Status:   RunStatusRunning,
			Attempt:  1,
		},
		// 存活 Worker 持有 lease:重复投递必须跳过,不得重驱动。
		leaseHeld: true,
	}
	gateway := NewGateway(nil, nil, server.URL, false)
	gateway.repo = repo
	gateway.durable = repo

	resp, err := gateway.Execute(context.Background(), &AgentRunPayload{
		TraceID:             "trace-duplicate",
		BusinessAppCode:     "finance",
		WorkflowTemplateKey: "finance_operating_report",
		GraphKey:            "finance_operating_report_graph",
		WorkflowInstanceID:  "00000000-0000-0000-0000-000000000001",
		NodeInstanceID:      "00000000-0000-0000-0000-000000000002",
		UserID:              "00000000-0000-0000-0000-000000000003",
		TenantID:            "00000000-0000-0000-0000-000000000010",
		Attempt:             1,
	})
	if err != nil {
		t.Fatalf("Execute duplicate: %v", err)
	}
	if !resp.Replayed || resp.RunID != repo.existingRun.ID || resp.Status != RunStatusRunning {
		t.Fatalf("unexpected replay response: %#v", resp)
	}
	if callCount != 0 {
		t.Fatalf("Python call count = %d, want 0", callCount)
	}
}

func TestGatewayExecuteTakesOverExpiredLeaseAndReDrives(t *testing.T) {
	callCount := 0
	var captured AgentRunRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		// 拉长调用窗口,让心跳有机会触发。
		time.Sleep(30 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(AgentRunResponse{
			RunID:    captured.RunID,
			GraphKey: "finance_operating_report_graph",
			Status:   RunStatusSucceeded,
			Output:   map[string]any{"ok": true},
		})
	}))
	defer server.Close()

	staleStarted := time.Now().Add(-2 * time.Hour)
	repo := &fakeGatewayRepo{
		graph: &Graph{
			GraphKey:        "finance_operating_report_graph",
			Version:         "1.0.0",
			BusinessAppCode: "finance",
			Status:          "active",
		},
		existingRun: &DurableRun{
			ID:        "00000000-0000-0000-0000-000000000099",
			TenantID:  "00000000-0000-0000-0000-000000000010",
			GraphKey:  "finance_operating_report_graph",
			Status:    RunStatusRunning,
			Attempt:   1,
			StartedAt: &staleStarted,
		},
		// lease 已过期/无主:重复投递可以接管并重驱动。
		leaseHeld: false,
	}
	gateway := NewGateway(nil, nil, server.URL, false)
	gateway.repo = repo
	gateway.durable = repo
	gateway.SetLeaseConfig(time.Minute, 5*time.Millisecond)

	resp, err := gateway.Execute(context.Background(), &AgentRunPayload{
		TraceID:             "trace-stale",
		BusinessAppCode:     "finance",
		WorkflowTemplateKey: "finance_operating_report",
		GraphKey:            "finance_operating_report_graph",
		WorkflowInstanceID:  "00000000-0000-0000-0000-000000000001",
		NodeInstanceID:      "00000000-0000-0000-0000-000000000002",
		UserID:              "00000000-0000-0000-0000-000000000003",
		TenantID:            "00000000-0000-0000-0000-000000000010",
		Attempt:             1,
	})
	if err != nil {
		t.Fatalf("Execute takeover: %v", err)
	}
	if resp.Replayed {
		t.Fatalf("lease takeover should re-drive, not replay: %#v", resp)
	}
	if resp.RunID != repo.existingRun.ID || resp.Status != RunStatusSucceeded {
		t.Fatalf("unexpected re-drive response: %#v", resp)
	}
	if callCount != 1 {
		t.Fatalf("Python call count = %d, want 1", callCount)
	}
	if captured.RunID != repo.existingRun.ID {
		t.Fatalf("re-drive request run_id = %q, want %q", captured.RunID, repo.existingRun.ID)
	}
	if !repo.acquired {
		t.Fatalf("takeover must acquire the run lease")
	}
	if repo.heartbeats == 0 {
		t.Fatalf("expected at least one heartbeat during the Python call")
	}
	if repo.runStatus != RunStatusSucceeded {
		t.Fatalf("CompleteV1Run status = %q, want succeeded", repo.runStatus)
	}
}

func TestGatewayExecuteReplaysTerminalRunEvenWhenStale(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		http.Error(w, "terminal run should not reach Python", http.StatusInternalServerError)
	}))
	defer server.Close()

	staleStarted := time.Now().Add(-2 * time.Hour)
	repo := &fakeGatewayRepo{
		graph: &Graph{
			GraphKey:        "finance_operating_report_graph",
			Version:         "1.0.0",
			BusinessAppCode: "finance",
			Status:          "active",
		},
		existingRun: &DurableRun{
			ID:        "00000000-0000-0000-0000-000000000099",
			TenantID:  "00000000-0000-0000-0000-000000000010",
			GraphKey:  "finance_operating_report_graph",
			Status:    RunStatusSucceeded,
			Attempt:   1,
			StartedAt: &staleStarted,
		},
	}
	gateway := NewGateway(nil, nil, server.URL, false)
	gateway.repo = repo
	gateway.durable = repo

	resp, err := gateway.Execute(context.Background(), &AgentRunPayload{
		TraceID:             "trace-terminal",
		BusinessAppCode:     "finance",
		WorkflowTemplateKey: "finance_operating_report",
		GraphKey:            "finance_operating_report_graph",
		WorkflowInstanceID:  "00000000-0000-0000-0000-000000000001",
		NodeInstanceID:      "00000000-0000-0000-0000-000000000002",
		UserID:              "00000000-0000-0000-0000-000000000003",
		TenantID:            "00000000-0000-0000-0000-000000000010",
		Attempt:             1,
	})
	if err != nil {
		t.Fatalf("Execute terminal run: %v", err)
	}
	if !resp.Replayed || resp.Status != RunStatusSucceeded {
		t.Fatalf("terminal run should be replayed: %#v", resp)
	}
	if callCount != 0 {
		t.Fatalf("Python call count = %d, want 0", callCount)
	}
}

// TestGatewayLateSuccessRejectedAfterLeaseTakeover 验证 M1-C-B 迟到成功拒绝:
// Python 调用期间 lease 被接管,成功结果写入被 ErrLeaseNotHeld 拒绝后,
// Execute 不得报错,而是回放 Run 当前(非终态)状态让 Worker 跳过推进。
func TestGatewayLateSuccessRejectedAfterLeaseTakeover(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(AgentRunResponse{
			RunID:    "00000000-0000-0000-0000-000000000099",
			GraphKey: "finance_operating_report_graph",
			Status:   RunStatusSucceeded,
			Output:   map[string]any{"stale": true},
		})
	}))
	defer server.Close()

	repo := &fakeGatewayRepo{
		graph: &Graph{
			GraphKey:        "finance_operating_report_graph",
			Version:         "1.0.0",
			BusinessAppCode: "finance",
			Status:          "active",
		},
		existingRun: &DurableRun{
			ID:        "00000000-0000-0000-0000-000000000099",
			TenantID:  "00000000-0000-0000-0000-000000000010",
			GraphKey:  "finance_operating_report_graph",
			Status:    RunStatusRunning,
			Attempt:   1,
			StartedAt: func() *time.Time { t := time.Now().Add(-time.Hour); return &t }(),
		},
		leaseHeld:   false, // 本执行者已接管过期 lease 并重驱动
		completeErr: fmt.Errorf("%w: run taken over", ErrLeaseNotHeld),
	}
	gateway := NewGateway(nil, nil, server.URL, false)
	gateway.repo = repo
	gateway.durable = repo

	resp, err := gateway.Execute(context.Background(), &AgentRunPayload{
		TraceID:             "trace-late-success",
		BusinessAppCode:     "finance",
		WorkflowTemplateKey: "finance_operating_report",
		GraphKey:            "finance_operating_report_graph",
		WorkflowInstanceID:  "00000000-0000-0000-0000-000000000001",
		NodeInstanceID:      "00000000-0000-0000-0000-000000000002",
		UserID:              "00000000-0000-0000-0000-000000000003",
		TenantID:            "00000000-0000-0000-0000-000000000010",
		Attempt:             1,
	})
	if err != nil {
		t.Fatalf("late success must not surface as error: %v", err)
	}
	if !resp.Replayed || resp.Status != RunStatusRunning {
		t.Fatalf("late success should replay current non-terminal state: %#v", resp)
	}
	if resp.Output != nil {
		if _, stale := resp.Output["stale"]; stale {
			t.Fatalf("late success output must be discarded: %#v", resp.Output)
		}
	}
	if repo.completeOwner == "" {
		t.Fatal("completion must carry the executor lease owner")
	}
}

// TestGatewayLateFailureRejectedAfterLeaseTakeover 验证 M1-C-B 迟到失败拒绝:
// lease 被接管后,原执行者的失败(即使 Python 真的返回 5xx)不得把 Run
// 标记为 failed,也不得向 Worker 返回错误触发 OnNodeFailed 回退节点。
func TestGatewayLateFailureRejectedAfterLeaseTakeover(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "simulated python outage", http.StatusInternalServerError)
	}))
	defer server.Close()

	repo := &fakeGatewayRepo{
		graph: &Graph{
			GraphKey:        "finance_operating_report_graph",
			Version:         "1.0.0",
			BusinessAppCode: "finance",
			Status:          "active",
		},
		existingRun: &DurableRun{
			ID:        "00000000-0000-0000-0000-000000000099",
			TenantID:  "00000000-0000-0000-0000-000000000010",
			GraphKey:  "finance_operating_report_graph",
			Status:    RunStatusRunning,
			Attempt:   1,
			StartedAt: func() *time.Time { t := time.Now().Add(-time.Hour); return &t }(),
		},
		leaseHeld:   false,
		completeErr: fmt.Errorf("%w: run taken over", ErrLeaseNotHeld),
	}
	gateway := NewGateway(nil, nil, server.URL, false)
	gateway.repo = repo
	gateway.durable = repo

	resp, err := gateway.Execute(context.Background(), &AgentRunPayload{
		TraceID:             "trace-late-failure",
		BusinessAppCode:     "finance",
		WorkflowTemplateKey: "finance_operating_report",
		GraphKey:            "finance_operating_report_graph",
		WorkflowInstanceID:  "00000000-0000-0000-0000-000000000001",
		NodeInstanceID:      "00000000-0000-0000-0000-000000000002",
		UserID:              "00000000-0000-0000-0000-000000000003",
		TenantID:            "00000000-0000-0000-0000-000000000010",
		Attempt:             1,
	})
	if err != nil {
		t.Fatalf("late failure must not surface as error (would regress node): %v", err)
	}
	if !resp.Replayed || resp.Status != RunStatusRunning {
		t.Fatalf("late failure should replay current non-terminal state: %#v", resp)
	}
	if repo.runStatus == RunStatusFailed {
		t.Fatal("late failure must not mark the run failed while a takeover executor owns it")
	}
}
