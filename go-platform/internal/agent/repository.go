package agent

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository 封装 Agent Registry、Graph Registry、Agent Run Logs、Approval Tasks 的数据库查询。
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository 创建 Repository 实例。
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// ── Agent Registry ──

// ListAgents 查询所有 active 状态的 Agent。
func (r *Repository) ListAgents(ctx context.Context) ([]Agent, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, agent_id, name, domain, reusable_scope,
		        capabilities_json, input_schema_json, output_schema_json,
		        endpoint, status, created_at, updated_at
		 FROM agent_registry WHERE status = 'active' AND deleted_at IS NULL
		 ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []Agent
	for rows.Next() {
		var a Agent
		if err := rows.Scan(&a.ID, &a.AgentID, &a.Name, &a.Domain, &a.ReusableScope,
			&a.CapabilitiesJSON, &a.InputSchemaJSON, &a.OutputSchemaJSON,
			&a.Endpoint, &a.Status, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		agents = append(agents, a)
	}
	return agents, nil
}

// FindAgentByID 根据 agent_id 查询单个 Agent。
func (r *Repository) FindAgentByID(ctx context.Context, agentID string) (*Agent, error) {
	a := &Agent{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, agent_id, name, domain, reusable_scope,
		        capabilities_json, input_schema_json, output_schema_json,
		        endpoint, status, created_at, updated_at
		 FROM agent_registry WHERE agent_id = $1 AND deleted_at IS NULL`, agentID,
	).Scan(&a.ID, &a.AgentID, &a.Name, &a.Domain, &a.ReusableScope,
		&a.CapabilitiesJSON, &a.InputSchemaJSON, &a.OutputSchemaJSON,
		&a.Endpoint, &a.Status, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return a, nil
}

// CreateAgent 插入一条 Agent 注册记录。
func (r *Repository) CreateAgent(ctx context.Context, a *Agent) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO agent_registry (agent_id, name, domain, reusable_scope, capabilities_json, input_schema_json, output_schema_json, endpoint, status)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		 RETURNING id, created_at, updated_at`,
		a.AgentID, a.Name, a.Domain, a.ReusableScope, a.CapabilitiesJSON, a.InputSchemaJSON, a.OutputSchemaJSON, a.Endpoint, a.Status,
	).Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)
}

// ── Graph Registry ──

// FindGraphByKey 根据 graph_key 查询注册的 Graph。
// Gateway 调用前必须验证 graph_key 存在且 active。
func (r *Repository) FindGraphByKey(ctx context.Context, graphKey string) (*Graph, error) {
	g := &Graph{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, graph_key, business_app_code, name, version, description, status, created_at, updated_at
		 FROM graph_registry WHERE graph_key = $1 AND deleted_at IS NULL`, graphKey,
	).Scan(&g.ID, &g.GraphKey, &g.BusinessAppCode, &g.Name, &g.Version, &g.Description, &g.Status, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return g, nil
}

// ── Domain Policy ──

// DomainPolicy 对应 domain_policies 表。
type DomainPolicy struct {
	BusinessAppCode        string
	AllowedAgentDomains    string // JSON array: ["finance","shared"]
	AllowedToolDomains     string // JSON array: ["finance","shared"]
	AllowSharedAgents      bool
	AllowSharedTools       bool
	HighRiskRequiresReview bool
	Status                 string
}

// FindDomainPolicy 查询某业务的域隔离策略。
func (r *Repository) FindDomainPolicy(ctx context.Context, businessAppCode string) (*DomainPolicy, error) {
	dp := &DomainPolicy{}
	err := r.pool.QueryRow(ctx,
		`SELECT business_app_code, allowed_agent_domains, allowed_tool_domains, allow_shared_agents, allow_shared_tools, high_risk_requires_review, status
		 FROM domain_policies WHERE business_app_code = $1 AND status = 'active' AND deleted_at IS NULL`, businessAppCode,
	).Scan(&dp.BusinessAppCode, &dp.AllowedAgentDomains, &dp.AllowedToolDomains, &dp.AllowSharedAgents, &dp.AllowSharedTools, &dp.HighRiskRequiresReview, &dp.Status)
	if err != nil {
		return nil, err
	}
	return dp, nil
}

// ── Agent Run Logs ──

// CreateRunLog 插入一条 Agent 执行日志。
func (r *Repository) CreateRunLog(ctx context.Context, log *AgentRunLog) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO agent_run_logs
		 (run_id, trace_id, workflow_instance_id, node_instance_id, business_app_code, graph_key, agent_id, status, input_summary_json, output_summary_json, usage_json, error_json, started_at, finished_at, duration_ms)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		 RETURNING id`,
		log.RunID, log.TraceID, log.WorkflowInstanceID, log.NodeInstanceID, log.BusinessAppCode, log.GraphKey,
		log.AgentID, log.Status, log.InputSummaryJSON, log.OutputSummaryJSON, log.UsageJSON, log.ErrorJSON,
		log.StartedAt, log.FinishedAt, log.DurationMs,
	).Scan(&log.ID)
}

// UpdateRunLog 更新执行日志（agent 完成后调用）。
func (r *Repository) UpdateRunLog(ctx context.Context, runID, status string, outputSummaryJSON, usageJSON, errorJSON *string, finishedAt *time.Time, durationMs *int) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE agent_run_logs SET status = $2, output_summary_json = $3, usage_json = $4, error_json = $5, finished_at = $6, duration_ms = $7
		 WHERE run_id = $1`,
		runID, status, outputSummaryJSON, usageJSON, errorJSON, finishedAt, durationMs)
	return err
}

// ListRunLogs 分页查询 Agent 执行日志。
func (r *Repository) ListRunLogs(ctx context.Context, workflowInstanceID, graphKey string, page, pageSize int) ([]AgentRunLog, int, error) {
	where := "WHERE 1=1"
	args := []any{}
	argIdx := 1

	if workflowInstanceID != "" {
		where += " AND workflow_instance_id = $" + strconv.Itoa(argIdx)
		args = append(args, workflowInstanceID)
		argIdx++
	}
	if graphKey != "" {
		where += " AND graph_key = $" + strconv.Itoa(argIdx)
		args = append(args, graphKey)
		argIdx++
	}

	var total int
	if err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM agent_run_logs "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	query := "SELECT id, run_id, trace_id, workflow_instance_id, node_instance_id, business_app_code, graph_key, agent_id, status, input_summary_json, output_summary_json, usage_json, error_json, started_at, finished_at, duration_ms FROM agent_run_logs " +
		where + " ORDER BY created_at DESC LIMIT $" + strconv.Itoa(argIdx) + " OFFSET $" + strconv.Itoa(argIdx+1)
	args = append(args, pageSize, offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []AgentRunLog
	for rows.Next() {
		var l AgentRunLog
		if err := rows.Scan(&l.ID, &l.RunID, &l.TraceID, &l.WorkflowInstanceID, &l.NodeInstanceID, &l.BusinessAppCode, &l.GraphKey, &l.AgentID, &l.Status, &l.InputSummaryJSON, &l.OutputSummaryJSON, &l.UsageJSON, &l.ErrorJSON, &l.StartedAt, &l.FinishedAt, &l.DurationMs); err != nil {
			return nil, 0, err
		}
		logs = append(logs, l)
	}
	return logs, total, nil
}

// ── Approval Tasks ──

// CreateApprovalTask 创建一条审批任务。
func (r *Repository) LatestRunOutput(ctx context.Context, workflowInstanceID string) (*string, error) {
	var output *string
	err := r.pool.QueryRow(ctx,
		`SELECT output_summary_json::text
		 FROM agent_run_logs
		 WHERE workflow_instance_id = $1 AND output_summary_json IS NOT NULL
		 ORDER BY created_at DESC
		 LIMIT 1`, workflowInstanceID,
	).Scan(&output)
	if err != nil {
		return nil, err
	}
	return output, nil
}

func (r *Repository) CreateApprovalTask(ctx context.Context, task *ApprovalTask) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO approval_tasks
		 (workflow_instance_id, node_instance_id, business_app_code, title, status, assignee_role)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 RETURNING id, created_at, updated_at`,
		task.WorkflowInstanceID, task.NodeInstanceID, task.BusinessAppCode, task.Title, task.Status, task.AssigneeRole,
	).Scan(&task.ID, &task.CreatedAt, &task.UpdatedAt)
}

// FindApprovalByNode 根据节点实例 ID 查询审批任务。
func (r *Repository) FindApprovalByNode(ctx context.Context, nodeInstanceID string) (*ApprovalTask, error) {
	task := &ApprovalTask{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, workflow_instance_id, node_instance_id, business_app_code, title, status,
		        assignee_role, assignee_user_id, decision_by, decision_comment, decided_at, created_at, updated_at
		 FROM approval_tasks WHERE node_instance_id = $1`, nodeInstanceID,
	).Scan(&task.ID, &task.WorkflowInstanceID, &task.NodeInstanceID, &task.BusinessAppCode, &task.Title, &task.Status,
		&task.AssigneeRole, &task.AssigneeUserID, &task.DecisionBy, &task.DecisionComment, &task.DecidedAt, &task.CreatedAt, &task.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return task, nil
}

func (r *Repository) FindApprovalByID(ctx context.Context, id string) (*ApprovalTask, error) {
	task := &ApprovalTask{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, workflow_instance_id, node_instance_id, business_app_code, title, status,
		        assignee_role, assignee_user_id, decision_by, decision_comment, decided_at, created_at, updated_at
		 FROM approval_tasks WHERE id = $1`, id,
	).Scan(&task.ID, &task.WorkflowInstanceID, &task.NodeInstanceID, &task.BusinessAppCode, &task.Title, &task.Status,
		&task.AssigneeRole, &task.AssigneeUserID, &task.DecisionBy, &task.DecisionComment, &task.DecidedAt, &task.CreatedAt, &task.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return task, nil
}

func (r *Repository) ListApprovalTasks(ctx context.Context, status, businessAppCode, workflowInstanceID string, page, pageSize int) ([]ApprovalTaskView, int, error) {
	where := "WHERE 1=1"
	args := []any{}
	argIdx := 1

	if status != "" {
		where += " AND at.status = $" + strconv.Itoa(argIdx)
		args = append(args, status)
		argIdx++
	}
	if businessAppCode != "" {
		where += " AND at.business_app_code = $" + strconv.Itoa(argIdx)
		args = append(args, businessAppCode)
		argIdx++
	}
	if workflowInstanceID != "" {
		where += " AND at.workflow_instance_id = $" + strconv.Itoa(argIdx)
		args = append(args, workflowInstanceID)
		argIdx++
	}

	var total int
	if err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM approval_tasks at "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	query := `SELECT at.id, at.workflow_instance_id, at.node_instance_id, at.business_app_code,
	                 at.title, at.status, at.assignee_role, at.assignee_user_id,
	                 at.decision_by, at.decision_comment, at.decided_at, at.created_at, at.updated_at,
	                 wi.title, wi.status, wni.status,
	                 arl.output_summary_json::text, arl.status, arl.finished_at
	          FROM approval_tasks at
	          JOIN workflow_instances wi ON wi.id = at.workflow_instance_id
	          JOIN workflow_node_instances wni ON wni.id = at.node_instance_id
	          LEFT JOIN LATERAL (
	              SELECT output_summary_json, status, finished_at
	              FROM agent_run_logs
	              WHERE workflow_instance_id = at.workflow_instance_id
	              ORDER BY created_at DESC
	              LIMIT 1
	          ) arl ON true ` + where + `
	          ORDER BY at.created_at DESC LIMIT $` + strconv.Itoa(argIdx) + ` OFFSET $` + strconv.Itoa(argIdx+1)
	args = append(args, pageSize, offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var tasks []ApprovalTaskView
	for rows.Next() {
		var v ApprovalTaskView
		if err := rows.Scan(&v.ID, &v.WorkflowInstanceID, &v.NodeInstanceID, &v.BusinessAppCode,
			&v.Title, &v.Status, &v.AssigneeRole, &v.AssigneeUserID,
			&v.DecisionBy, &v.DecisionComment, &v.DecidedAt, &v.CreatedAt, &v.UpdatedAt,
			&v.WorkflowTitle, &v.WorkflowStatus, &v.NodeStatus,
			&v.AgentOutputJSON, &v.AgentRunStatus, &v.AgentRunFinishedAt); err != nil {
			return nil, 0, err
		}
		tasks = append(tasks, v)
	}
	return tasks, total, nil
}

func (r *Repository) GetApprovalTaskView(ctx context.Context, id string) (*ApprovalTaskView, error) {
	var v ApprovalTaskView
	err := r.pool.QueryRow(ctx,
		`SELECT at.id, at.workflow_instance_id, at.node_instance_id, at.business_app_code,
		        at.title, at.status, at.assignee_role, at.assignee_user_id,
		        at.decision_by, at.decision_comment, at.decided_at, at.created_at, at.updated_at,
		        wi.title, wi.status, wni.status,
		        arl.output_summary_json::text, arl.status, arl.finished_at
		 FROM approval_tasks at
		 JOIN workflow_instances wi ON wi.id = at.workflow_instance_id
		 JOIN workflow_node_instances wni ON wni.id = at.node_instance_id
		 LEFT JOIN LATERAL (
		     SELECT output_summary_json, status, finished_at
		     FROM agent_run_logs
		     WHERE workflow_instance_id = at.workflow_instance_id
		     ORDER BY created_at DESC
		     LIMIT 1
		 ) arl ON true
		 WHERE at.id = $1`, id,
	).Scan(&v.ID, &v.WorkflowInstanceID, &v.NodeInstanceID, &v.BusinessAppCode,
		&v.Title, &v.Status, &v.AssigneeRole, &v.AssigneeUserID,
		&v.DecisionBy, &v.DecisionComment, &v.DecidedAt, &v.CreatedAt, &v.UpdatedAt,
		&v.WorkflowTitle, &v.WorkflowStatus, &v.NodeStatus,
		&v.AgentOutputJSON, &v.AgentRunStatus, &v.AgentRunFinishedAt)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// UpdateApprovalDecision 记录审批决定（通过/驳回）。
func (r *Repository) UpdateApprovalDecision(ctx context.Context, id, status, comment, decisionBy string) error {
	now := time.Now()
	_, err := r.pool.Exec(ctx,
		`UPDATE approval_tasks SET status = $2, decision_by = $3, decision_comment = $4, decided_at = $5, updated_at = $5
			 WHERE id = $1`,
		id, status, decisionBy, comment, now)
	return err
}

func (r *Repository) CompleteApprovalAndWorkflowDecision(ctx context.Context, id, status, comment, decisionBy string) (*ApprovalTask, error) {
	var nodeStatus, instanceStatus string
	switch status {
	case "approved":
		nodeStatus = "succeeded"
		instanceStatus = "approved"
	case "rejected":
		nodeStatus = "failed"
		instanceStatus = "rejected"
	default:
		return nil, fmt.Errorf("unsupported approval decision status: %s", status)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	task := &ApprovalTask{}
	err = tx.QueryRow(ctx,
		`SELECT at.id, at.workflow_instance_id, at.node_instance_id, wi.trace_id, at.business_app_code, at.title, at.status,
		        at.assignee_role, at.assignee_user_id, at.decision_by, at.decision_comment, at.decided_at, at.created_at, at.updated_at
		 FROM approval_tasks at
		 JOIN workflow_instances wi ON wi.id = at.workflow_instance_id
		 WHERE at.id = $1 FOR UPDATE`, id,
	).Scan(&task.ID, &task.WorkflowInstanceID, &task.NodeInstanceID, &task.WorkflowTraceID, &task.BusinessAppCode, &task.Title, &task.Status,
		&task.AssigneeRole, &task.AssigneeUserID, &task.DecisionBy, &task.DecisionComment, &task.DecidedAt, &task.CreatedAt, &task.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if task.Status != "pending" {
		return nil, fmt.Errorf("approval task %s is not pending: %s", id, task.Status)
	}

	now := time.Now()
	if _, err := tx.Exec(ctx,
		`UPDATE workflow_node_instances
		 SET status = $2, finished_at = $3, updated_at = $3
		 WHERE id = $1`,
		task.NodeInstanceID, nodeStatus, now,
	); err != nil {
		return nil, err
	}

	if status == "rejected" {
		if _, err := tx.Exec(ctx,
			`UPDATE workflow_instances
			 SET status = $2, finished_at = $3, updated_at = $3
			 WHERE id = $1`,
			task.WorkflowInstanceID, instanceStatus, now,
		); err != nil {
			return nil, err
		}
	} else {
		if _, err := tx.Exec(ctx,
			`UPDATE workflow_instances
			 SET status = $2, updated_at = $3
			 WHERE id = $1`,
			task.WorkflowInstanceID, instanceStatus, now,
		); err != nil {
			return nil, err
		}
	}

	if _, err := tx.Exec(ctx,
		`UPDATE approval_tasks
		 SET status = $2, decision_by = $3, decision_comment = $4, decided_at = $5, updated_at = $5
		 WHERE id = $1`,
		id, status, decisionBy, comment, now,
	); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	task.Status = status
	task.DecisionBy = &decisionBy
	task.DecisionComment = &comment
	task.DecidedAt = &now
	task.UpdatedAt = now
	return task, nil
}
