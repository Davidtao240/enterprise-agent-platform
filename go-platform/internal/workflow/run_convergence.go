package workflow

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/enterprise-agent-platform/go-platform/internal/agent"
)

// RunConvergenceScanner 是 M1-C lease/heartbeat 之前的前导失联接管机制:
// 定期找出超过执行阈值仍未收敛的非终态 Durable Run,并把对应节点任务
// 重新入队。重新执行时 Gateway 会复用同一 Run 身份重驱动 Python
// (幂等:已完成则返回最终状态,未完成则从 Checkpoint 恢复)。
// M1-C-A 引入 lease/heartbeat 后,本扫描器应改为基于 lease 过期的接管。
type RunConvergenceScanner struct {
	runs          staleRunLister
	workflow      convergenceWorkflowLookup
	enqueuer      nodeEnqueuer
	advancer      runAdvancer
	advanceLister runAdvanceLister
	staleAfter    time.Duration
	interval      time.Duration
}

type staleRunLister interface {
	ListStaleRunningRuns(ctx context.Context, staleBefore time.Time) ([]agent.StaleV1Run, error)
}

type runAdvanceLister interface {
	ListTerminalRunsNeedingAdvance(ctx context.Context, limit int) ([]RunAdvance, error)
}

type runAdvancer interface {
	CompleteAgentRunNode(ctx context.Context, nodeInstanceID, runID, status string, outputJSON, errorJSON *string) error
}

type convergenceWorkflowLookup interface {
	FindNodeInstanceByID(ctx context.Context, id string) (*NodeInstance, error)
	FindInstanceByID(ctx context.Context, id string) (*Instance, error)
}

// convergenceScanInterval 是扫描周期。首次扫描在启动后立即执行。
const convergenceScanInterval = 60 * time.Second

func NewRunConvergenceScanner(
	runs staleRunLister,
	workflow convergenceWorkflowLookup,
	enqueuer nodeEnqueuer,
	advancer runAdvancer,
	advanceLister runAdvanceLister,
	staleAfter time.Duration,
) *RunConvergenceScanner {
	interval := convergenceScanInterval
	if staleAfter > 0 && staleAfter < interval {
		interval = staleAfter / 2
		if interval < 5*time.Second {
			interval = 5 * time.Second
		}
	}
	return &RunConvergenceScanner{
		runs:          runs,
		workflow:      workflow,
		enqueuer:      enqueuer,
		advancer:      advancer,
		advanceLister: advanceLister,
		staleAfter:    staleAfter,
		interval:      interval,
	}
}

// ScanOnce 执行一轮扫描,返回 (重入队节点数, 补偿推进节点数)。
func (s *RunConvergenceScanner) ScanOnce(ctx context.Context) (int, error) {
	if s.runs == nil || s.workflow == nil || s.enqueuer == nil || s.staleAfter <= 0 {
		return 0, fmt.Errorf("run convergence scanner is not configured")
	}
	requeued := s.scanStaleRuns(ctx)
	if err := s.scanTerminalRuns(ctx); err != nil {
		log.Printf("[convergence] reconcile terminal runs failed: %v", err)
	}
	return requeued, nil
}

// scanTerminalRuns 补偿推进"Run 已终态但节点仍 running"的节点
// (事件驱动完成中 sink 失败/进程崩溃后的最终一致恢复)。
func (s *RunConvergenceScanner) scanTerminalRuns(ctx context.Context) error {
	if s.advancer == nil || s.advanceLister == nil {
		return nil
	}
	advances, err := s.advanceLister.ListTerminalRunsNeedingAdvance(ctx, 100)
	if err != nil {
		return err
	}
	for _, a := range advances {
		if err := s.advancer.CompleteAgentRunNode(ctx, a.NodeInstanceID, a.RunID, a.Status, a.OutputSummaryJSON, a.ErrorJSON); err != nil {
			log.Printf("[convergence] advance node %s for terminal run %s failed: %v", a.NodeInstanceID, a.RunID, err)
			continue
		}
		log.Printf("[convergence] reconciled terminal run %s → node %s", a.RunID, a.NodeInstanceID)
	}
	return nil
}

func (s *RunConvergenceScanner) scanStaleRuns(ctx context.Context) int {
	staleBefore := time.Now().Add(-s.staleAfter)
	runs, err := s.runs.ListStaleRunningRuns(ctx, staleBefore)
	if err != nil {
		log.Printf("[convergence] list stale durable runs: %v", err)
		return 0
	}

	requeued := 0
	for _, run := range runs {
		node, err := s.workflow.FindNodeInstanceByID(ctx, run.NodeInstanceID)
		if err != nil {
			log.Printf("[convergence] skip stale run %s: find node: %v", run.RunID, err)
			continue
		}
		// 只接管"当前仍是本 attempt 且仍在执行"的节点;
		// 终态节点(已推进/已失败)与已进入新 attempt 的节点不得重复推进。
		if node.Status != NodeStatusRunning || node.RetryCount+1 != run.Attempt {
			log.Printf("[convergence] skip stale run %s: node status=%s retry=%d run attempt=%d",
				run.RunID, node.Status, node.RetryCount, run.Attempt)
			continue
		}
		inst, err := s.workflow.FindInstanceByID(ctx, run.WorkflowInstanceID)
		if err != nil {
			log.Printf("[convergence] skip stale run %s: find workflow: %v", run.RunID, err)
			continue
		}
		if inst.Status != StatusRunning {
			log.Printf("[convergence] skip stale run %s: workflow status=%s", run.RunID, inst.Status)
			continue
		}
		if err := s.enqueuer.EnqueueExecuteNode(&ExecuteNodePayload{
			WorkflowInstanceID: run.WorkflowInstanceID,
			NodeInstanceID:     run.NodeInstanceID,
			Attempt:            run.Attempt,
			NodeType:           node.NodeType,
			NodeKey:            node.NodeKey,
			GraphKey:           run.GraphKey,
			TraceID:            inst.TraceID,
		}); err != nil {
			log.Printf("[convergence] requeue stale run %s failed: %v", run.RunID, err)
			continue
		}
		log.Printf("[convergence] requeued node %s for stale durable run %s (attempt=%d)",
			run.NodeInstanceID, run.RunID, run.Attempt)
		requeued++
	}
	return requeued
}

// Start 启动周期扫描,直到 ctx 取消。首次扫描立即执行。
func (s *RunConvergenceScanner) Start(ctx context.Context) {
	if err := s.initialScan(ctx); err != nil {
		log.Printf("[convergence] initial scan failed: %v", err)
	}
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			count, err := s.ScanOnce(ctx)
			if err != nil {
				log.Printf("[convergence] scan failed: %v", err)
			} else if count > 0 {
				log.Printf("[convergence] requeued %d stale durable run node task(s)", count)
			}
		}
	}
}

func (s *RunConvergenceScanner) initialScan(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	count, err := s.ScanOnce(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		log.Printf("[convergence] initial scan requeued %d stale durable run node task(s)", count)
	}
	return nil
}
