package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"reflect"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type rowScanner interface {
	Scan(dest ...any) error
}

func (r *Repository) StartV1RunTx(ctx context.Context, start *V1DurableRunStart) (*DurableRun, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("begin V1 durable run: %w", err)
	}
	defer tx.Rollback(ctx)

	thread := &AgentThread{}
	err = tx.QueryRow(ctx,
		`INSERT INTO agent_threads
			 (tenant_id, created_by, business_app_code, workflow_instance_id, title, status)
		 VALUES ($1,$2,$3,$4,$5,'active')
		 ON CONFLICT (tenant_id, workflow_instance_id) WHERE workflow_instance_id IS NOT NULL
		 DO UPDATE SET updated_at = agent_threads.updated_at
		 RETURNING id, tenant_id, created_by, business_app_code, workflow_instance_id, title, status, created_at, updated_at`,
		start.TenantID, start.CreatedBy, start.BusinessAppCode, start.WorkflowInstanceID, start.ThreadTitle,
	).Scan(&thread.ID, &thread.TenantID, &thread.CreatedBy, &thread.BusinessAppCode, &thread.WorkflowInstanceID,
		&thread.Title, &thread.Status, &thread.CreatedAt, &thread.UpdatedAt)
	if err != nil {
		return nil, false, fmt.Errorf("ensure workflow thread: %w", err)
	}

	tag, err := tx.Exec(ctx,
		`INSERT INTO agent_runs
			 (id, thread_id, tenant_id, trace_id, workflow_instance_id, node_instance_id,
			  graph_key, graph_version, configuration_snapshot_json, status, attempt)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'queued',$10)
		 ON CONFLICT (tenant_id, node_instance_id, attempt) WHERE node_instance_id IS NOT NULL
		 DO NOTHING`,
		start.RunID, thread.ID, start.TenantID, start.TraceID, start.WorkflowInstanceID, start.NodeInstanceID,
		start.GraphKey, start.GraphVersion, start.ConfigurationSnapshotJSON, start.Attempt,
	)
	if err != nil {
		return nil, false, fmt.Errorf("create durable run: %w", err)
	}

	if tag.RowsAffected() == 0 {
		run, findErr := findDurableRun(
			tx.QueryRow(ctx, durableRunSelect+` WHERE tenant_id = $1 AND node_instance_id = $2 AND attempt = $3`,
				start.TenantID, start.NodeInstanceID, start.Attempt),
		)
		if findErr != nil {
			return nil, false, fmt.Errorf("find existing durable run attempt: %w", findErr)
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, false, fmt.Errorf("commit existing durable run lookup: %w", err)
		}
		return run, false, nil
	}

	queuedEventID := uuid.NewString()
	startedEventID := uuid.NewString()
	if _, err := tx.Exec(ctx,
		`INSERT INTO runtime_events
			 (event_id, tenant_id, run_id, sequence, attempt, event_type, payload_json, occurred_at, consumed_at)
		 VALUES
			 ($1,$2,$3,1,$4,$5,'{}',$6,$6),
			 ($7,$2,$3,2,$4,$8,'{}',$6,$6)`,
		queuedEventID, start.TenantID, start.RunID, start.Attempt, RuntimeEventRunQueued, start.StartedAt,
		startedEventID, RuntimeEventRunStarted,
	); err != nil {
		return nil, false, fmt.Errorf("create initial runtime events: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`UPDATE agent_runs
		 SET status = 'running', started_at = $4, updated_at = $4
		 WHERE tenant_id = $1 AND id = $2 AND attempt = $3 AND status = 'queued'`,
		start.TenantID, start.RunID, start.Attempt, start.StartedAt,
	); err != nil {
		return nil, false, fmt.Errorf("start durable run: %w", err)
	}

	stepName := "legacy_v1_graph_execution"
	if _, err := tx.Exec(ctx,
		`INSERT INTO agent_run_steps
			 (tenant_id, run_id, sequence, attempt, step_type, name, status, input_summary_json, started_at)
		 VALUES ($1,$2,1,$3,$4,$5,$6,$7,$8)`,
		start.TenantID, start.RunID, start.Attempt, StepTypeSystem, stepName, StepStatusRunning,
		start.InputSummaryJSON, start.StartedAt,
	); err != nil {
		return nil, false, fmt.Errorf("create V1 bridge step: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO agent_run_logs
			 (run_id, durable_run_id, tenant_id, trace_id, workflow_instance_id, node_instance_id,
			  business_app_code, graph_key, status, input_summary_json, started_at)
		 VALUES ($1::varchar,$1::uuid,$2,$3,$4,$5,$6,$7,'running',$8,$9)`,
		start.RunID, start.TenantID, start.TraceID, start.WorkflowInstanceID, start.NodeInstanceID,
		start.BusinessAppCode, start.GraphKey, start.InputSummaryJSON, start.StartedAt,
	); err != nil {
		return nil, false, fmt.Errorf("create V1 compatibility run log: %w", err)
	}

	run, err := findDurableRun(tx.QueryRow(ctx, durableRunSelect+` WHERE tenant_id = $1 AND id = $2`, start.TenantID, start.RunID))
	if err != nil {
		return nil, false, fmt.Errorf("load started durable run: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("commit V1 durable run: %w", err)
	}
	return run, true, nil
}

func (r *Repository) StartV2RunTx(ctx context.Context, start *V2DurableRunStart) (*DurableRun, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("begin V2 durable run: %w", err)
	}
	defer tx.Rollback(ctx)

	thread := &AgentThread{}
	err = tx.QueryRow(ctx,
		`INSERT INTO agent_threads
			 (tenant_id, created_by, business_app_code, workflow_instance_id, title, status)
		 VALUES ($1,$2,$3,$4,$5,'active')
		 ON CONFLICT (tenant_id, workflow_instance_id) WHERE workflow_instance_id IS NOT NULL
		 DO UPDATE SET updated_at = agent_threads.updated_at
		 RETURNING id, tenant_id, created_by, business_app_code, workflow_instance_id, title, status, created_at, updated_at`,
		start.TenantID, start.CreatedBy, start.BusinessAppCode, start.WorkflowInstanceID, start.ThreadTitle,
	).Scan(&thread.ID, &thread.TenantID, &thread.CreatedBy, &thread.BusinessAppCode, &thread.WorkflowInstanceID,
		&thread.Title, &thread.Status, &thread.CreatedAt, &thread.UpdatedAt)
	if err != nil {
		return nil, false, fmt.Errorf("ensure V2 workflow thread: %w", err)
	}

	tag, err := tx.Exec(ctx,
		`INSERT INTO agent_runs
			 (id, thread_id, tenant_id, trace_id, workflow_instance_id, node_instance_id,
			  graph_key, graph_version, configuration_snapshot_json, status, attempt, budget_json)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'queued',$10,$11)
		 ON CONFLICT (tenant_id, node_instance_id, attempt) WHERE node_instance_id IS NOT NULL
		 DO NOTHING`,
		start.RunID, thread.ID, start.TenantID, start.TraceID, start.WorkflowInstanceID, start.NodeInstanceID,
		start.GraphKey, start.GraphVersion, start.ConfigurationSnapshotJSON, start.Attempt, start.BudgetJSON,
	)
	if err != nil {
		return nil, false, fmt.Errorf("create V2 durable run: %w", err)
	}

	created := tag.RowsAffected() == 1
	if created {
		// agent_run_logs 兼容摘要(M2-A):V2 异步路径同样维护兼容行,保证
		// 归档输出(LatestRunOutput)与审批详情视图在两条执行路径下同形。
		// 终态摘要由 ApplyRuntimeEventTx 在 run.succeeded/failed 事件时补齐。
		if _, err := tx.Exec(ctx,
			`INSERT INTO agent_run_logs
				 (run_id, durable_run_id, tenant_id, trace_id, workflow_instance_id, node_instance_id,
				  business_app_code, graph_key, status, started_at)
			 VALUES ($1::varchar,$1::uuid,$2,$3,$4,$5,$6,$7,'running',$8)`,
			start.RunID, start.TenantID, start.TraceID, start.WorkflowInstanceID, start.NodeInstanceID,
			start.BusinessAppCode, start.GraphKey, time.Now().UTC(),
		); err != nil {
			return nil, false, fmt.Errorf("create V2 compatibility run log: %w", err)
		}
	}
	var run *DurableRun
	if created {
		run, err = findDurableRun(tx.QueryRow(ctx,
			durableRunSelect+` WHERE tenant_id = $1 AND id = $2`, start.TenantID, start.RunID))
	} else {
		run, err = findDurableRun(tx.QueryRow(ctx,
			durableRunSelect+` WHERE tenant_id = $1 AND node_instance_id = $2 AND attempt = $3`,
			start.TenantID, start.NodeInstanceID, start.Attempt))
	}
	if err != nil {
		return nil, false, fmt.Errorf("load V2 durable run: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("commit V2 durable run: %w", err)
	}
	return run, created, nil
}

func (r *Repository) CompleteV1RunTx(ctx context.Context, completion *V1DurableRunCompletion) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin V1 durable completion: %w", err)
	}
	defer tx.Rollback(ctx)

	run, err := findDurableRun(tx.QueryRow(ctx,
		durableRunSelect+` WHERE tenant_id = $1 AND id = $2 FOR UPDATE`, completion.TenantID, completion.RunID))
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrDurableRunNotFound
	}
	if err != nil {
		return fmt.Errorf("lock durable run: %w", err)
	}
	if run.Attempt != completion.Attempt {
		return fmt.Errorf("%w: current=%d received=%d", ErrStaleRunAttempt, run.Attempt, completion.Attempt)
	}

	// 迟到结果检查(M1-C-B):
	// 1. Run 非终态 + lease 被他人持有 → 拒绝 (防止接管者执行被旧执行者覆盖)
	// 2. Run 已终态 + lease 被他人持有 → 同状态幂等放行但记录日志 (观测性)
	if completion.LeaseOwner != "" && run.LeaseOwner != nil && *run.LeaseOwner != completion.LeaseOwner {
		if !isTerminalRunStatus(run.Status) {
			return fmt.Errorf("%w: run=%s attempt=%d owner=%q held_by=%q",
				ErrLeaseNotHeld, completion.RunID, completion.Attempt,
				completion.LeaseOwner, *run.LeaseOwner)
		}
	}

	if run.Status == completion.Status {
		// 同状态幂等重放:不改变 status,但通过 COALESCE 合并
		// output/error 数据(防止接管者写入的数据被旧执行者丢弃)。
		if _, err := tx.Exec(ctx,
			`UPDATE agent_runs
			 SET output_summary_json = COALESCE($4::jsonb, output_summary_json),
			     error_json = COALESCE($5::jsonb, error_json),
			     updated_at = $6
			 WHERE tenant_id = $1 AND id = $2 AND attempt = $3`,
			completion.TenantID, completion.RunID, completion.Attempt,
			completion.OutputSummaryJSON, completion.ErrorJSON, completion.FinishedAt,
		); err != nil {
			return fmt.Errorf("merge duplicate V1 completion data: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE agent_run_logs
			 SET output_summary_json = COALESCE($3::jsonb, output_summary_json),
			     error_json = COALESCE($4::jsonb, error_json),
			     updated_at = $5
			 WHERE tenant_id = $1 AND durable_run_id = $2`,
			completion.TenantID, completion.RunID,
			completion.OutputSummaryJSON, completion.ErrorJSON, completion.FinishedAt,
		); err != nil {
			return fmt.Errorf("merge duplicate V1 compat log data: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit duplicate V1 completion: %w", err)
		}
		return nil
	}

	if !CanTransitionRun(run.Status, completion.Status) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidRunTransition, run.Status, completion.Status)
	}

	if _, err := tx.Exec(ctx,
		`UPDATE agent_runs
		 SET status = $4, output_summary_json = $5, error_json = $6,
		     finished_at = $7, updated_at = $7
		 WHERE tenant_id = $1 AND id = $2 AND attempt = $3`,
		completion.TenantID, completion.RunID, completion.Attempt, completion.Status,
		completion.OutputSummaryJSON, completion.ErrorJSON, completion.FinishedAt,
	); err != nil {
		return fmt.Errorf("complete durable run: %w", err)
	}

	stepStatus, stepEventType, runEventType := terminalEventTypes(completion.Status)
	if _, err := tx.Exec(ctx,
		`UPDATE agent_run_steps
		 SET status = $4, output_summary_json = $5, usage_json = $6, error_json = $7,
		     finished_at = $8, updated_at = $8
		 WHERE tenant_id = $1 AND run_id = $2 AND attempt = $3 AND sequence = 1`,
		completion.TenantID, completion.RunID, completion.Attempt, stepStatus,
		completion.OutputSummaryJSON, completion.UsageJSON, completion.ErrorJSON, completion.FinishedAt,
	); err != nil {
		return fmt.Errorf("complete V1 bridge step: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO runtime_events
			 (event_id, tenant_id, run_id, sequence, attempt, event_type, payload_json, occurred_at, consumed_at)
		 VALUES
			 ($1,$2,$3,3,$4,$5,'{}',$6,$6),
			 ($7,$2,$3,4,$4,$8,'{}',$6,$6)`,
		uuid.NewString(), completion.TenantID, completion.RunID, completion.Attempt,
		stepEventType, completion.FinishedAt, uuid.NewString(), runEventType,
	); err != nil {
		return fmt.Errorf("create terminal runtime events: %w", err)
	}

	tag, err := tx.Exec(ctx,
		`UPDATE agent_run_logs
		 SET status = $4, output_summary_json = $5, usage_json = $6, error_json = $7,
		     finished_at = $8, duration_ms = $9
		 WHERE tenant_id = $1 AND durable_run_id = $2 AND run_id = $3`,
		completion.TenantID, completion.RunID, completion.RunID, completion.Status,
		completion.OutputSummaryJSON, completion.UsageJSON, completion.ErrorJSON,
		completion.FinishedAt, completion.DurationMs,
	)
	if err != nil {
		return fmt.Errorf("update V1 compatibility run log: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("update V1 compatibility run log: expected one row, updated %d", tag.RowsAffected())
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit V1 durable completion: %w", err)
	}
	return nil
}

func (r *Repository) FindDurableRunByIDForTenant(ctx context.Context, tenantID, runID string) (*DurableRun, error) {
	run, err := findDurableRun(r.pool.QueryRow(ctx, durableRunSelect+` WHERE tenant_id = $1 AND id = $2`, tenantID, runID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrDurableRunNotFound
	}
	if err != nil {
		return nil, err
	}
	return run, nil
}

// ListStaleRunningRuns 返回失联接管候选:lease 已过期的非终态 Run,以及
// 从未取得 lease 且超过年龄阈值的 Run(升级前的存量数据)。收敛扫描器据此
// 重新入队节点任务;Run 重驱动以同一 Run 身份幂等恢复。
func (r *Repository) ListStaleRunningRuns(ctx context.Context, staleBefore time.Time) ([]StaleV1Run, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, tenant_id, workflow_instance_id, node_instance_id, attempt, graph_key
		 FROM agent_runs
		 WHERE status IN ('queued','running')
		   AND workflow_instance_id IS NOT NULL
		   AND node_instance_id IS NOT NULL
		   AND (
		         (lease_expires_at IS NOT NULL AND lease_expires_at < now())
		      OR (lease_expires_at IS NULL AND started_at IS NOT NULL AND started_at < $1)
		   )
		 ORDER BY started_at ASC
		 LIMIT 100`,
		staleBefore,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	runs := make([]StaleV1Run, 0, 8)
	for rows.Next() {
		var run StaleV1Run
		if err := rows.Scan(&run.RunID, &run.TenantID, &run.WorkflowInstanceID,
			&run.NodeInstanceID, &run.Attempt, &run.GraphKey); err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return runs, nil
}

// AcquireRunLease 以单条条件 UPDATE 原子获取 lease(M1-C-A)。
// 仅当 Run 非终态且"无主、已过期、或持有者正是 owner"时成功。
func (r *Repository) AcquireRunLease(ctx context.Context, tenantID, runID string, attempt int, owner string, ttl time.Duration) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE agent_runs
		 SET lease_owner = $4,
		     lease_expires_at = now() + $5,
		     heartbeat_at = now(),
		     updated_at = now()
		 WHERE tenant_id = $1 AND id = $2 AND attempt = $3
		   AND status IN ('queued','running')
		   AND (lease_owner = $4 OR lease_owner IS NULL OR lease_expires_at IS NULL OR lease_expires_at < now())`,
		tenantID, runID, attempt, owner, ttl,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// HeartbeatRunLease 延长 lease 并记录心跳;owner 不匹配(已被接管)时返回 false。
func (r *Repository) HeartbeatRunLease(ctx context.Context, tenantID, runID string, attempt int, owner string, ttl time.Duration) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE agent_runs
		 SET heartbeat_at = now(),
		     lease_expires_at = now() + $5,
		     updated_at = now()
		 WHERE tenant_id = $1 AND id = $2 AND attempt = $3 AND lease_owner = $4
		   AND status IN ('queued','running')`,
		tenantID, runID, attempt, owner, ttl,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// ListActiveRunsForWorkflow 返回工作流下仍处于活动(可取消)状态的 Run。
func (r *Repository) ListActiveRunsForWorkflow(ctx context.Context, tenantID, workflowInstanceID string) ([]StaleV1Run, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, tenant_id, workflow_instance_id, node_instance_id, attempt, graph_key
		 FROM agent_runs
		 WHERE tenant_id = $1 AND workflow_instance_id = $2
		   AND status IN ('queued','running','waiting_human','waiting_external')
		 ORDER BY created_at ASC`,
		tenantID, workflowInstanceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	runs := make([]StaleV1Run, 0, 4)
	for rows.Next() {
		var run StaleV1Run
		if err := rows.Scan(&run.RunID, &run.TenantID, &run.WorkflowInstanceID,
			&run.NodeInstanceID, &run.Attempt, &run.GraphKey); err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return runs, nil
}

// FindInterruptCheckpointVersion 返回中断对应的 Checkpoint 版本,用于 Resume
// 的版本校验。
func (r *Repository) FindInterruptCheckpointVersion(ctx context.Context, tenantID, runID, interruptID string) (int64, error) {
	var version int64
	err := r.pool.QueryRow(ctx,
		`SELECT checkpoint_version FROM agent_interrupts
		 WHERE tenant_id = $1 AND run_id = $2 AND id = $3`,
		tenantID, runID, interruptID,
	).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrDurableRunNotFound
	}
	if err != nil {
		return 0, err
	}
	return version, nil
}

func (r *Repository) TransitionDurableRun(ctx context.Context, tenantID, runID string, attempt int, fromStatuses []string, toStatus string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE agent_runs
		 SET status = $5::varchar,
		     started_at = CASE WHEN $5::varchar = 'running' THEN COALESCE(started_at, now()) ELSE started_at END,
		     finished_at = CASE WHEN $5::varchar IN ('succeeded','failed','cancelled') THEN now() ELSE finished_at END,
		     updated_at = now()
		 WHERE tenant_id = $1 AND id = $2 AND attempt = $3 AND status = ANY($4::varchar[])`,
		tenantID, runID, attempt, fromStatuses, toStatus,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrInvalidRunTransition
	}
	return nil
}

func (r *Repository) ApplyRuntimeEventTx(ctx context.Context, event *RuntimeEvent, fromStatuses []string, toStatus string) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin runtime event: %w", err)
	}
	defer tx.Rollback(ctx)

	run, err := findDurableRun(tx.QueryRow(ctx,
		durableRunSelect+` WHERE tenant_id = $1 AND id = $2 FOR UPDATE`, event.TenantID, event.RunID))
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrDurableRunNotFound
	}
	if err != nil {
		return false, fmt.Errorf("lock runtime event run: %w", err)
	}
	if run.Attempt != event.Attempt {
		return false, fmt.Errorf("%w: current=%d received=%d", ErrStaleRunAttempt, run.Attempt, event.Attempt)
	}
	duplicate, err := findMatchingRuntimeEvent(ctx, tx, event)
	if err != nil {
		return false, err
	}
	if duplicate {
		if err := tx.Commit(ctx); err != nil {
			return false, fmt.Errorf("commit duplicate runtime event: %w", err)
		}
		return false, nil
	}
	if event.CheckpointVersion != nil && run.CheckpointVersion != nil && *event.CheckpointVersion < *run.CheckpointVersion {
		return false, fmt.Errorf("%w: current=%d received=%d", ErrCheckpointVersionConflict, *run.CheckpointVersion, *event.CheckpointVersion)
	}

	consumedAt := time.Now()
	tag, err := tx.Exec(ctx,
		`INSERT INTO runtime_events
			 (event_id, tenant_id, run_id, sequence, attempt, event_type, payload_json,
			  checkpoint_version, occurred_at, consumed_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		 ON CONFLICT DO NOTHING`,
		event.EventID, event.TenantID, event.RunID, event.Sequence, event.Attempt, event.EventType,
		event.PayloadJSON, event.CheckpointVersion, event.OccurredAt, consumedAt,
	)
	if err != nil {
		return false, fmt.Errorf("insert runtime event: %w", err)
	}
	if tag.RowsAffected() == 0 {
		duplicate, duplicateErr := findMatchingRuntimeEvent(ctx, tx, event)
		if duplicateErr != nil {
			return false, duplicateErr
		}
		if !duplicate {
			return false, ErrRuntimeEventConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return false, fmt.Errorf("commit duplicate runtime event: %w", err)
		}
		return false, nil
	}
	if err := applyRuntimeControlMetadata(ctx, tx, event); err != nil {
		return false, err
	}

	// 终态事件载荷落库(M2-A):run.succeeded 携带 {output,usage},run.failed
	// 携带 {error}。V1 桥路径事件 payload 为空,COALESCE 保持原值不受影响。
	outputSummary, usageJSON, errorJSON := terminalEventPayloadFields(event)

	if toStatus != "" {
		if !containsStatus(fromStatuses, run.Status) {
			return false, fmt.Errorf("%w: %s -> %s", ErrInvalidRunTransition, run.Status, toStatus)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE agent_runs
			 SET status = $4::varchar,
			     checkpoint_version = CASE
			       WHEN $5::bigint IS NULL THEN checkpoint_version
			       ELSE GREATEST(COALESCE(checkpoint_version, 0), $5)
			     END,
			     started_at = CASE WHEN $4::varchar = 'running' THEN COALESCE(started_at, $6) ELSE started_at END,
			     finished_at = CASE WHEN $4::varchar IN ('succeeded','failed','cancelled') THEN $6 ELSE finished_at END,
			     output_summary_json = COALESCE($8::jsonb, output_summary_json),
			     usage_json = COALESCE($9::jsonb, usage_json),
			     error_json = COALESCE($10::jsonb, error_json),
			     updated_at = $7
			 WHERE tenant_id = $1 AND id = $2 AND attempt = $3`,
			event.TenantID, event.RunID, event.Attempt, toStatus, event.CheckpointVersion, event.OccurredAt, consumedAt,
			outputSummary, usageJSON, errorJSON,
		); err != nil {
			return false, fmt.Errorf("apply runtime event transition: %w", err)
		}
		// agent_run_logs 兼容摘要同步(M2-A):V2 异步路径下归档输出与审批
		// 详情仍从 agent_run_logs 读取;终态事件到达时补齐摘要与耗时。
		if isTerminalRunStatus(toStatus) {
			if _, err := tx.Exec(ctx,
				`UPDATE agent_run_logs
				 SET status = $3::varchar,
				     output_summary_json = COALESCE($4::jsonb, output_summary_json),
				     usage_json = COALESCE($5::jsonb, usage_json),
				     error_json = COALESCE($6::jsonb, error_json),
				     finished_at = $7,
				     duration_ms = COALESCE(EXTRACT(EPOCH FROM ($7 - started_at)) * 1000, duration_ms)
				 WHERE tenant_id = $1 AND durable_run_id = $2`,
				event.TenantID, event.RunID, toStatus,
				outputSummary, usageJSON, errorJSON, event.OccurredAt,
			); err != nil {
				return false, fmt.Errorf("update compat run log for runtime event: %w", err)
			}
		}
	} else if event.CheckpointVersion != nil {
		if _, err := tx.Exec(ctx,
			`UPDATE agent_runs
			 SET checkpoint_version = GREATEST(COALESCE(checkpoint_version, 0), $4), updated_at = $5
			 WHERE tenant_id = $1 AND id = $2 AND attempt = $3`,
			event.TenantID, event.RunID, event.Attempt, *event.CheckpointVersion, consumedAt,
		); err != nil {
			return false, fmt.Errorf("apply runtime checkpoint version: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit runtime event: %w", err)
	}
	event.ConsumedAt = &consumedAt
	return true, nil
}

// terminalEventPayloadFields 从终态 Runtime Event payload 提取输出/用量/错误
// 载荷(M2-A)。run.succeeded 事件携带与 V1 envelope 同形的 {output,usage};
// run.failed 携带 {error}。payload 为空或键缺失时返回 nil(调用方 COALESCE
// 保留原值),V1 桥路径的空 payload 事件不受影响。
func terminalEventPayloadFields(event *RuntimeEvent) (outputSummary, usageJSON, errorJSON *string) {
	if event.PayloadJSON == nil || *event.PayloadJSON == "" {
		return nil, nil, nil
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(*event.PayloadJSON), &payload); err != nil {
		log.Printf("[durable] terminal event %s for run %s has malformed payload: %v",
			event.EventType, event.RunID, err)
		return nil, nil, nil
	}
	pick := func(key string) *string {
		raw, ok := payload[key]
		if !ok || string(raw) == "null" {
			return nil
		}
		value := string(raw)
		return &value
	}
	switch event.EventType {
	case RuntimeEventRunSucceeded:
		output := pick("output")
		usage := pick("usage")
		if output == nil && usage == nil {
			log.Printf("[durable] run.succeeded event for run %s has no output/usage in payload; "+
				"agent may not be sending output data", event.RunID)
		}
		return output, usage, nil
	case RuntimeEventRunFailed:
		errField := pick("error")
		if errField == nil {
			log.Printf("[durable] run.failed event for run %s has no error in payload; "+
				"agent may not be sending error details", event.RunID)
		}
		return nil, nil, errField
	default:
		return nil, nil, nil
	}
}

func findMatchingRuntimeEvent(ctx context.Context, tx pgx.Tx, event *RuntimeEvent) (bool, error) {
	var eventID, tenantID, runID, eventType string
	var sequence int64
	var attempt int
	var checkpointVersion *int64
	var payloadJSON *string
	err := tx.QueryRow(ctx,
		`SELECT event_id, tenant_id, run_id, sequence, attempt, event_type,
		        checkpoint_version, payload_json::text
		 FROM runtime_events
		 WHERE (event_id = $1 OR (run_id = $2 AND sequence = $3))
		   AND tenant_id = $4
		 ORDER BY CASE WHEN event_id = $1 THEN 0 ELSE 1 END
		 LIMIT 1`, event.EventID, event.RunID, event.Sequence, event.TenantID,
	).Scan(&eventID, &tenantID, &runID, &sequence, &attempt, &eventType, &checkpointVersion, &payloadJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load duplicate runtime event: %w", err)
	}
	if tenantID != event.TenantID || runID != event.RunID || sequence != event.Sequence ||
		attempt != event.Attempt || eventType != event.EventType ||
		!sameOptionalInt64(checkpointVersion, event.CheckpointVersion) ||
		!sameJSON(payloadJSON, event.PayloadJSON) {
		return false, ErrRuntimeEventConflict
	}
	return true, nil
}

func sameOptionalInt64(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func sameJSON(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	var leftValue, rightValue any
	if json.Unmarshal([]byte(*left), &leftValue) != nil || json.Unmarshal([]byte(*right), &rightValue) != nil {
		return *left == *right
	}
	return reflect.DeepEqual(leftValue, rightValue)
}

type runtimeControlPayload struct {
	Backend        string         `json:"backend"`
	CheckpointRef  string         `json:"checkpoint_ref"`
	StateHash      *string        `json:"state_hash"`
	InterruptID    string         `json:"interrupt_id"`
	Kind           string         `json:"kind"`
	ResumeSchema   map[string]any `json:"resume_schema"`
	IdempotencyKey *string        `json:"idempotency_key"`
	StepID         string         `json:"step_id"`
	StepSequence   int64          `json:"step_sequence"`
	StepType       string         `json:"step_type"`
	Name           *string        `json:"name"`
	Status         string         `json:"status"`
	Error          map[string]any `json:"error"`
}

func applyRuntimeControlMetadata(ctx context.Context, tx pgx.Tx, event *RuntimeEvent) error {
	payload := runtimeControlPayload{}
	if event.PayloadJSON != nil {
		if err := json.Unmarshal([]byte(*event.PayloadJSON), &payload); err != nil {
			return fmt.Errorf("%w: invalid control event payload", ErrRuntimeEventConflict)
		}
	}

	switch event.EventType {
	case RuntimeEventStepStarted:
		if _, err := uuid.Parse(payload.StepID); err != nil || payload.StepSequence <= 0 ||
			!validRuntimeStepType(payload.StepType) {
			return fmt.Errorf("%w: step.started requires UUID step_id, sequence, and valid step_type", ErrRuntimeEventConflict)
		}
		tag, err := tx.Exec(ctx,
			`INSERT INTO agent_run_steps
				 (id, tenant_id, run_id, sequence, attempt, step_type, name, status, started_at)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,'running',$8)
			 ON CONFLICT DO NOTHING`,
			payload.StepID, event.TenantID, event.RunID, payload.StepSequence, event.Attempt,
			payload.StepType, payload.Name, event.OccurredAt,
		)
		if err != nil {
			return fmt.Errorf("start runtime step: %w", err)
		}
		if tag.RowsAffected() == 0 {
			var existingID, status string
			if err := tx.QueryRow(ctx,
				`SELECT id, status FROM agent_run_steps
				 WHERE tenant_id = $1 AND run_id = $2 AND sequence = $3`,
				event.TenantID, event.RunID, payload.StepSequence,
			).Scan(&existingID, &status); err != nil || existingID != payload.StepID || status != StepStatusRunning {
				return ErrRuntimeEventConflict
			}
		}

	case RuntimeEventStepCompleted:
		if _, err := uuid.Parse(payload.StepID); err != nil || payload.StepSequence <= 0 {
			return fmt.Errorf("%w: step.completed requires UUID step_id and sequence", ErrRuntimeEventConflict)
		}
		tag, err := tx.Exec(ctx,
			`UPDATE agent_run_steps
			 SET status = 'succeeded', finished_at = $6, updated_at = $6
			 WHERE tenant_id = $1 AND run_id = $2 AND id = $3 AND sequence = $4
			   AND attempt = $5 AND status = 'running'`,
			event.TenantID, event.RunID, payload.StepID, payload.StepSequence, event.Attempt, event.OccurredAt,
		)
		if err != nil {
			return fmt.Errorf("complete runtime step: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return ErrRuntimeEventConflict
		}

	case RuntimeEventStepFailed:
		if _, err := uuid.Parse(payload.StepID); err != nil || payload.StepSequence <= 0 {
			return fmt.Errorf("%w: step.failed requires UUID step_id and sequence", ErrRuntimeEventConflict)
		}
		stepStatus := StepStatusFailed
		if payload.Status == StepStatusCancelled {
			stepStatus = StepStatusCancelled
		}
		var errorJSON any
		if len(payload.Error) > 0 {
			errorJSON = payload.Error
		}
		tag, err := tx.Exec(ctx,
			`UPDATE agent_run_steps
			 SET status = $6::varchar, error_json = $7, finished_at = $8, updated_at = $8
			 WHERE tenant_id = $1 AND run_id = $2 AND id = $3 AND sequence = $4
			   AND attempt = $5 AND status = 'running'`,
			event.TenantID, event.RunID, payload.StepID, payload.StepSequence, event.Attempt,
			stepStatus, errorJSON, event.OccurredAt,
		)
		if err != nil {
			return fmt.Errorf("fail runtime step: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return ErrRuntimeEventConflict
		}

	case RuntimeEventCheckpointSaved:
		if event.CheckpointVersion == nil || *event.CheckpointVersion <= 0 || payload.Backend == "" || payload.CheckpointRef == "" {
			return fmt.Errorf("%w: checkpoint event requires version, backend, and checkpoint_ref", ErrRuntimeEventConflict)
		}
		tag, err := tx.Exec(ctx,
			`INSERT INTO agent_checkpoints
				 (tenant_id, run_id, version, backend, checkpoint_ref, state_hash, created_at)
			 VALUES ($1,$2,$3,$4,$5,$6,$7)
			 ON CONFLICT (run_id, version) DO NOTHING`,
			event.TenantID, event.RunID, *event.CheckpointVersion, payload.Backend,
			payload.CheckpointRef, payload.StateHash, event.OccurredAt,
		)
		if err != nil {
			return fmt.Errorf("index runtime checkpoint: %w", err)
		}
		if tag.RowsAffected() == 0 {
			var backend, checkpointRef string
			if err := tx.QueryRow(ctx,
				`SELECT backend, checkpoint_ref FROM agent_checkpoints
				 WHERE tenant_id = $1 AND run_id = $2 AND version = $3`,
				event.TenantID, event.RunID, *event.CheckpointVersion,
			).Scan(&backend, &checkpointRef); err != nil || backend != payload.Backend || checkpointRef != payload.CheckpointRef {
				return ErrRuntimeEventConflict
			}
		}

	case RuntimeEventRunInterrupted:
		if event.CheckpointVersion == nil || *event.CheckpointVersion <= 0 || payload.InterruptID == "" {
			return fmt.Errorf("%w: interrupt event requires interrupt_id and checkpoint version", ErrRuntimeEventConflict)
		}
		if payload.Kind == "" {
			payload.Kind = "human"
		}
		if payload.Kind != "human" && payload.Kind != "external" && payload.Kind != "input_required" {
			return fmt.Errorf("%w: unsupported interrupt kind %q", ErrRuntimeEventConflict, payload.Kind)
		}
		resumeSchema, err := json.Marshal(payload.ResumeSchema)
		if err != nil {
			return fmt.Errorf("marshal interrupt resume schema: %w", err)
		}
		tag, err := tx.Exec(ctx,
			`INSERT INTO agent_interrupts
				 (id, tenant_id, run_id, checkpoint_version, kind, status, resume_schema_json, created_at, updated_at)
			 VALUES ($1,$2,$3,$4,$5,'pending',$6,$7,$7)
			 ON CONFLICT (id) DO NOTHING`,
			payload.InterruptID, event.TenantID, event.RunID, *event.CheckpointVersion,
			payload.Kind, resumeSchema, event.OccurredAt,
		)
		if err != nil {
			return fmt.Errorf("index runtime interrupt: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrRuntimeEventConflict
		}

	case RuntimeEventRunResumed:
		if payload.InterruptID == "" {
			return fmt.Errorf("%w: resumed event requires interrupt_id", ErrRuntimeEventConflict)
		}
		tag, err := tx.Exec(ctx,
			`UPDATE agent_interrupts
			 SET status = 'resumed', resume_idempotency_key = $4,
			     resumed_at = $5, updated_at = $5
			 WHERE tenant_id = $1 AND run_id = $2 AND id = $3 AND status = 'pending'`,
			event.TenantID, event.RunID, payload.InterruptID, payload.IdempotencyKey, event.OccurredAt,
		)
		if err != nil {
			return fmt.Errorf("resume runtime interrupt: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return ErrRuntimeEventConflict
		}

	case RuntimeEventRunCancelled, RuntimeEventRunFailed, RuntimeEventRunSucceeded:
		if _, err := tx.Exec(ctx,
			`UPDATE agent_interrupts SET status = 'cancelled', updated_at = $3
			 WHERE tenant_id = $1 AND run_id = $2 AND status = 'pending'`,
			event.TenantID, event.RunID, event.OccurredAt,
		); err != nil {
			return fmt.Errorf("cancel pending runtime interrupts: %w", err)
		}

	default:
		log.Printf("[durable] applyRuntimeControlMetadata: unhandled event type %s for run %s",
			event.EventType, event.RunID)
	}
	return nil
}

func validRuntimeStepType(stepType string) bool {
	switch stepType {
	case StepTypeModel, StepTypeTool, StepTypeCheckpoint, StepTypeInterrupt, StepTypeSystem:
		return true
	default:
		return false
	}
}

const durableRunSelect = `SELECT id, thread_id, tenant_id, trace_id, workflow_instance_id, node_instance_id,
	parent_run_id, graph_key, graph_version, configuration_snapshot_json::text, status, attempt,
	checkpoint_version, lease_owner, lease_expires_at, heartbeat_at,
	budget_json::text, output_summary_json::text, usage_json::text, error_json::text,
	started_at, finished_at, created_at, updated_at
	FROM agent_runs`

func findDurableRun(row rowScanner) (*DurableRun, error) {
	run := &DurableRun{}
	if err := row.Scan(
		&run.ID, &run.ThreadID, &run.TenantID, &run.TraceID, &run.WorkflowInstanceID, &run.NodeInstanceID,
		&run.ParentRunID, &run.GraphKey, &run.GraphVersion, &run.ConfigurationSnapshotJSON, &run.Status, &run.Attempt,
		&run.CheckpointVersion, &run.LeaseOwner, &run.LeaseExpiresAt, &run.HeartbeatAt,
		&run.BudgetJSON, &run.OutputSummaryJSON, &run.UsageJSON, &run.ErrorJSON, &run.StartedAt, &run.FinishedAt,
		&run.CreatedAt, &run.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return run, nil
}

func terminalEventTypes(status string) (string, string, string) {
	switch status {
	case RunStatusSucceeded:
		return StepStatusSucceeded, RuntimeEventStepCompleted, RuntimeEventRunSucceeded
	case RunStatusCancelled:
		return StepStatusCancelled, RuntimeEventStepFailed, RuntimeEventRunCancelled
	default:
		return StepStatusFailed, RuntimeEventStepFailed, RuntimeEventRunFailed
	}
}

func containsStatus(statuses []string, status string) bool {
	for _, candidate := range statuses {
		if candidate == status {
			return true
		}
	}
	return false
}
