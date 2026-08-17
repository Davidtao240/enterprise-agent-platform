package agent

import (
	"context"
	"fmt"
	"log"
)

// RuntimeController 是 Go 控制面主动操作 Durable Run 的入口
// (Cancel/Resume),供 Workflow 引擎与审批决策调用。它依赖 RuntimeV2Client
// 与仓储;未配置 Runtime V2 时,Cancel 视为无操作(不存在 V2 Run)。
type RuntimeController struct {
	client *RuntimeV2Client
	repo   runtimeControlRepository
}

type runtimeControlRepository interface {
	ListActiveRunsForWorkflow(ctx context.Context, tenantID, workflowInstanceID string) ([]StaleV1Run, error)
	FindInterruptCheckpointVersion(ctx context.Context, tenantID, runID, interruptID string) (int64, error)
}

func NewRuntimeController(client *RuntimeV2Client, repo runtimeControlRepository) *RuntimeController {
	return &RuntimeController{client: client, repo: repo}
}

// CancelWorkflowRuns 取消工作流下所有活动 Run(best-effort:单个失败不阻断)。
func (c *RuntimeController) CancelWorkflowRuns(ctx context.Context, tenantID, workflowInstanceID, reason, requestedBy string) error {
	if c.client == nil || c.repo == nil {
		return nil // Runtime V2 未配置时无 Run 可取消。
	}
	runs, err := c.repo.ListActiveRunsForWorkflow(ctx, tenantID, workflowInstanceID)
	if err != nil {
		return fmt.Errorf("list active runs: %w", err)
	}
	for _, run := range runs {
		_, err := c.client.Cancel(ctx, &RuntimeV2CancelRequest{
			ProtocolVersion: "2.0",
			RunID:           run.RunID,
			Reason:          reason,
			RequestedBy:     requestedBy,
			IdempotencyKey:  "cancel:" + run.RunID,
		})
		if err != nil {
			log.Printf("[runtime] cancel run %s failed: %v", run.RunID, err)
		}
	}
	return nil
}

// ResumeInterruptedRun 以中断对应的 Checkpoint 版本恢复 Run。
// idempotency_key 稳定绑定 interrupt,重复 Resume 幂等。
func (c *RuntimeController) ResumeInterruptedRun(ctx context.Context, tenantID, runID, interruptID string, resumeInput map[string]any) error {
	if c.client == nil || c.repo == nil {
		return fmt.Errorf("runtime V2 is not configured")
	}
	version, err := c.repo.FindInterruptCheckpointVersion(ctx, tenantID, runID, interruptID)
	if err != nil {
		return fmt.Errorf("find interrupt checkpoint version: %w", err)
	}
	_, err = c.client.Resume(ctx, &RuntimeV2ResumeRequest{
		ProtocolVersion:           "2.0",
		RunID:                     runID,
		InterruptID:               interruptID,
		ExpectedCheckpointVersion: version,
		IdempotencyKey:            "approval:" + interruptID,
		ResumeInput:               resumeInput,
	})
	return err
}
