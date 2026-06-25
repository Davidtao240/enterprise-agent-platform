package agent

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/enterprise-agent-platform/go-platform/internal/audit"
	"github.com/gin-gonic/gin"
)

type fakeHandlerRepo struct {
	task           *ApprovalTask
	updatedID      string
	updatedStatus  string
	updatedComment string
	updatedBy      string
	updateErr      error
	completeErr    error
}

func (f *fakeHandlerRepo) ListAgents(ctx context.Context) ([]Agent, error) {
	return nil, nil
}

func (f *fakeHandlerRepo) CreateAgent(ctx context.Context, a *Agent) error {
	return nil
}

func (f *fakeHandlerRepo) ListRunLogs(ctx context.Context, workflowInstanceID, graphKey string, page, pageSize int) ([]AgentRunLog, int, error) {
	return nil, 0, nil
}

func (f *fakeHandlerRepo) ListApprovalTasks(ctx context.Context, status, businessAppCode, workflowInstanceID string, page, pageSize int) ([]ApprovalTaskView, int, error) {
	return nil, 0, nil
}

func (f *fakeHandlerRepo) GetApprovalTaskView(ctx context.Context, id string) (*ApprovalTaskView, error) {
	return nil, nil
}

func (f *fakeHandlerRepo) FindApprovalByID(ctx context.Context, id string) (*ApprovalTask, error) {
	return f.task, nil
}

func (f *fakeHandlerRepo) UpdateApprovalDecision(ctx context.Context, id, status, comment, decisionBy string) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	f.updatedID = id
	f.updatedStatus = status
	f.updatedComment = comment
	f.updatedBy = decisionBy
	return nil
}

func (f *fakeHandlerRepo) CompleteApprovalAndWorkflowDecision(ctx context.Context, id, status, comment, decisionBy string) (*ApprovalTask, error) {
	if f.completeErr != nil {
		return nil, f.completeErr
	}
	if err := f.UpdateApprovalDecision(ctx, id, status, comment, decisionBy); err != nil {
		return nil, err
	}
	return f.task, nil
}

type fakeApprovalWorkflow struct {
	nodeID           string
	decision         string
	userID           string
	comment          string
	err              error
	continueNodeID   string
	continueDecision string
	continueErr      error
}

func (f *fakeApprovalWorkflow) CompleteHumanReviewNode(ctx context.Context, nodeInstanceID, decision, userID, comment string) error {
	if f.err != nil {
		return f.err
	}
	f.nodeID = nodeInstanceID
	f.decision = decision
	f.userID = userID
	f.comment = comment
	return nil
}

func (f *fakeApprovalWorkflow) ContinueAfterHumanReviewNode(ctx context.Context, nodeInstanceID, decision, userID, comment string) error {
	if f.continueErr != nil {
		return f.continueErr
	}
	f.continueNodeID = nodeInstanceID
	f.continueDecision = decision
	f.userID = userID
	f.comment = comment
	return nil
}

type fakeHandlerAuditRepo struct {
	entries []audit.AuditLogEntry
}

func (f *fakeHandlerAuditRepo) InsertLog(ctx context.Context, entry audit.AuditLogEntry) (string, time.Time, error) {
	f.entries = append(f.entries, entry)
	return "audit-1", time.Now(), nil
}

func TestApproveTaskUpdatesDecisionAndAdvancesWorkflow(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &fakeHandlerRepo{task: &ApprovalTask{
		ID:                 "approval-1",
		WorkflowInstanceID: "workflow-1",
		NodeInstanceID:     "node-review-1",
		BusinessAppCode:    "finance",
		Status:             "pending",
	}}
	workflow := &fakeApprovalWorkflow{}
	auditRepo := &fakeHandlerAuditRepo{}
	handler := &Handler{repo: repo}
	handler.auditRepo = auditRepo
	handler.SetWorkflowService(workflow)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", "reviewer-1")
		c.Next()
	})
	router.POST("/approval-tasks/:id/approve", handler.ApproveTask)

	req := httptest.NewRequest(http.MethodPost, "/approval-tasks/approval-1/approve", bytes.NewBufferString(`{"comment":"looks good"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", resp.Code, resp.Body.String())
	}
	if repo.updatedID != "approval-1" || repo.updatedStatus != "approved" || repo.updatedBy != "reviewer-1" {
		t.Fatalf("approval update mismatch: %#v", repo)
	}
	if workflow.continueNodeID != "node-review-1" || workflow.continueDecision != "approved" || workflow.comment != "looks good" {
		t.Fatalf("workflow was not continued correctly: %#v", workflow)
	}
	if len(auditRepo.entries) != 1 {
		t.Fatalf("expected one approval audit entry, got %#v", auditRepo.entries)
	}
	entry := auditRepo.entries[0]
	if entry.Action != "approval_approved" || entry.BusinessAppCode == nil || *entry.BusinessAppCode != "finance" {
		t.Fatalf("approval audit mismatch: %#v", entry)
	}
}

func TestApproveTaskDoesNotContinueWorkflowWhenAtomicDecisionFails(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &fakeHandlerRepo{task: &ApprovalTask{
		ID:                 "approval-1",
		WorkflowInstanceID: "workflow-1",
		NodeInstanceID:     "node-review-1",
		BusinessAppCode:    "finance",
		Status:             "pending",
	}}
	repo.completeErr = errors.New("transaction failed")
	workflow := &fakeApprovalWorkflow{}
	handler := &Handler{repo: repo}
	handler.SetWorkflowService(workflow)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", "reviewer-1")
		c.Next()
	})
	router.POST("/approval-tasks/:id/approve", handler.ApproveTask)

	req := httptest.NewRequest(http.MethodPost, "/approval-tasks/approval-1/approve", bytes.NewBufferString(`{"comment":"looks good"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body=%s", resp.Code, resp.Body.String())
	}
	if workflow.continueNodeID != "" {
		t.Fatalf("workflow should not continue when atomic decision fails: %#v", workflow)
	}
}
