package workflow

import (
	"context"
	"fmt"
	"time"

	"github.com/enterprise-agent-platform/go-platform/internal/agent"
)

type agentApprovalCreator interface {
	CreateApprovalTask(ctx context.Context, task *agent.ApprovalTask) error
}

// RunEventBridge 实现 agent.RunEventSink,把 Durable Run 的终态/中断事件
// 推进到对应 Workflow 节点(Workflow 事件驱动完成)。它位于 workflow 包,
// 由 main.go 注入到 agent.RuntimeHandler 以避免 agent→workflow 的导入环。
type RunEventBridge struct {
	svc       *Service
	approvals agentApprovalCreator
}

func NewRunEventBridge(svc *Service, approvals agentApprovalCreator) *RunEventBridge {
	return &RunEventBridge{svc: svc, approvals: approvals}
}

func (b *RunEventBridge) OnRunSucceeded(ctx context.Context, tenantID, runID, workflowInstanceID, nodeInstanceID string, outputJSON *string) error {
	return b.svc.CompleteAgentRunNode(ctx, nodeInstanceID, runID, "succeeded", outputJSON, nil)
}

func (b *RunEventBridge) OnRunFailed(ctx context.Context, tenantID, runID, workflowInstanceID, nodeInstanceID string, errorJSON *string) error {
	return b.svc.CompleteAgentRunNode(ctx, nodeInstanceID, runID, "failed", nil, errorJSON)
}

func (b *RunEventBridge) OnRunCancelled(ctx context.Context, tenantID, runID, workflowInstanceID, nodeInstanceID string) error {
	return b.svc.CompleteAgentRunNode(ctx, nodeInstanceID, runID, "cancelled", nil, nil)
}

// OnRunInterrupted 把 Runtime 中断转为审批任务并让节点进入 waiting_review。
// 幂等:节点已不在 running(已中断/已推进)时直接返回。审批通过后由
// Resume 控制面恢复 Run,Run 的后续终态事件再推进节点。
func (b *RunEventBridge) OnRunInterrupted(ctx context.Context, tenantID, runID, workflowInstanceID, nodeInstanceID, interruptID, kind string) error {
	if b.approvals == nil {
		return fmt.Errorf("approval creator is not configured")
	}
	node, err := b.svc.repo.FindNodeInstanceByID(ctx, nodeInstanceID)
	if err != nil {
		return fmt.Errorf("find interrupted node: %w", err)
	}
	if node.Status != NodeStatusRunning {
		return nil // 幂等:节点已推进或已处于等待态。
	}
	inst, err := b.svc.repo.FindInstanceByID(ctx, node.WorkflowInstanceID)
	if err != nil {
		return fmt.Errorf("find instance: %w", err)
	}

	task := &agent.ApprovalTask{
		WorkflowInstanceID: node.WorkflowInstanceID,
		NodeInstanceID:     nodeInstanceID,
		BusinessAppCode:    inst.BusinessAppCode,
		Title:              fmt.Sprintf("运行时复核:%s", inst.Title),
		Status:             "pending",
		DurableRunID:       &runID,
		InterruptID:        &interruptID,
	}
	if err := b.approvals.CreateApprovalTask(ctx, task); err != nil {
		return fmt.Errorf("create runtime interrupt approval: %w", err)
	}

	now := time.Now()
	if err := b.svc.repo.UpdateNodeStatus(ctx, nodeInstanceID, NodeStatusWaitingReview, nil, &now); err != nil {
		return fmt.Errorf("mark interrupted node waiting: %w", err)
	}
	if err := b.svc.repo.UpdateInstanceStatus(ctx, node.WorkflowInstanceID, StatusWaitingReview, nil, nil); err != nil {
		return fmt.Errorf("mark workflow waiting: %w", err)
	}
	b.svc.auditLog(ctx, inst.CreatedBy, inst.BusinessAppCode, inst.TraceID, "agent_run_interrupted", nodeInstanceID, StatusWaitingReview, nil)
	return nil
}
