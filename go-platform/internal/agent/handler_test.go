package agent

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type fakeHandlerRepo struct {
	task           *ApprovalTask
	updatedID      string
	updatedStatus  string
	updatedComment string
	updatedBy      string
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
	f.updatedID = id
	f.updatedStatus = status
	f.updatedComment = comment
	f.updatedBy = decisionBy
	return nil
}

type fakeApprovalWorkflow struct {
	nodeID   string
	decision string
	userID   string
	comment  string
}

func (f *fakeApprovalWorkflow) CompleteHumanReviewNode(ctx context.Context, nodeInstanceID, decision, userID, comment string) error {
	f.nodeID = nodeInstanceID
	f.decision = decision
	f.userID = userID
	f.comment = comment
	return nil
}

func TestApproveTaskUpdatesDecisionAndAdvancesWorkflow(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &fakeHandlerRepo{task: &ApprovalTask{
		ID:             "approval-1",
		NodeInstanceID: "node-review-1",
		Status:         "pending",
	}}
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

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", resp.Code, resp.Body.String())
	}
	if repo.updatedID != "approval-1" || repo.updatedStatus != "approved" || repo.updatedBy != "reviewer-1" {
		t.Fatalf("approval update mismatch: %#v", repo)
	}
	if workflow.nodeID != "node-review-1" || workflow.decision != "approved" || workflow.comment != "looks good" {
		t.Fatalf("workflow was not advanced correctly: %#v", workflow)
	}
}
