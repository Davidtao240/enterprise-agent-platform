package workflow

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository 封装 workflow 相关的数据库查询。
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository 创建 Repository 实例。
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// ── 模板查询 ──

// FindTemplateByBusinessAndKey 根据 business_app_code + template_key 查询 active 状态的模板。
// 每次创建实例时调用，确保使用最新 active 版本。
func (r *Repository) FindTemplateByBusinessAndKey(ctx context.Context, businessAppCode, templateKey string) (*Template, error) {
	t := &Template{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, business_app_code, workflow_template_key, name, version, graph_key, definition_json, status, created_at, updated_at
		 FROM workflow_templates
		 WHERE business_app_code = $1 AND workflow_template_key = $2 AND status = 'active' AND deleted_at IS NULL
		 ORDER BY created_at DESC LIMIT 1`,
		businessAppCode, templateKey,
	).Scan(&t.ID, &t.BusinessAppCode, &t.WorkflowTemplateKey, &t.Name, &t.Version, &t.GraphKey, &t.DefinitionJSON, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return t, nil
}

// FindTemplatesByBusinessApp 查询某业务下所有 active 状态的模板列表。
// GET /api/v1/business-apps/{code}/workflow-templates
func (r *Repository) FindTemplatesByBusinessApp(ctx context.Context, businessAppCode string) ([]Template, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, business_app_code, workflow_template_key, name, version, graph_key, definition_json, status, created_at, updated_at
		 FROM workflow_templates
		 WHERE business_app_code = $1 AND status = 'active' AND deleted_at IS NULL
		 ORDER BY name`,
		businessAppCode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var templates []Template
	for rows.Next() {
		var t Template
		if err := rows.Scan(&t.ID, &t.BusinessAppCode, &t.WorkflowTemplateKey, &t.Name, &t.Version, &t.GraphKey, &t.DefinitionJSON, &t.Status, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		templates = append(templates, t)
	}
	return templates, nil
}

// ── 实例 CRUD ──

// CreateInstance 插入一条 workflow_instance 记录。
func (r *Repository) CreateInstance(ctx context.Context, inst *Instance) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO workflow_instances
		 (business_app_code, workflow_template_id, workflow_template_key, workflow_template_version,
		  graph_key, title, status, input_json, created_by, trace_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		 RETURNING id, created_at, updated_at`,
		inst.BusinessAppCode, inst.WorkflowTemplateID, inst.WorkflowTemplateKey, inst.WorkflowTemplateVersion,
		inst.GraphKey, inst.Title, inst.Status, inst.InputJSON, inst.CreatedBy, inst.TraceID,
	).Scan(&inst.ID, &inst.CreatedAt, &inst.UpdatedAt)
}

// FindInstanceByID 根据 UUID 查询实例。
func (r *Repository) FindInstanceByID(ctx context.Context, id string) (*Instance, error) {
	inst := &Instance{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, business_app_code, workflow_template_id, workflow_template_key, workflow_template_version,
		        graph_key, title, status, input_json, output_json, created_by,
		        started_at, finished_at, trace_id, created_at, updated_at
		 FROM workflow_instances WHERE id = $1 AND deleted_at IS NULL`, id,
	).Scan(&inst.ID, &inst.BusinessAppCode, &inst.WorkflowTemplateID, &inst.WorkflowTemplateKey,
		&inst.WorkflowTemplateVersion, &inst.GraphKey, &inst.Title, &inst.Status,
		&inst.InputJSON, &inst.OutputJSON, &inst.CreatedBy,
		&inst.StartedAt, &inst.FinishedAt, &inst.TraceID, &inst.CreatedAt, &inst.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return inst, nil
}

// ListInstances 按条件分页查询实例列表。
// 支持的过滤条件：business_app_code, status, created_by
func (r *Repository) ListInstances(ctx context.Context, businessAppCode, status, createdBy string, page, pageSize int) ([]Instance, int, error) {
	// 构建动态查询条件
	where := "WHERE deleted_at IS NULL"
	args := []any{}
	argIdx := 1

	if businessAppCode != "" {
		where += fmt.Sprintf(" AND business_app_code = $%d", argIdx)
		args = append(args, businessAppCode)
		argIdx++
	}
	if status != "" {
		where += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}
	if createdBy != "" {
		where += fmt.Sprintf(" AND created_by = $%d", argIdx)
		args = append(args, createdBy)
		argIdx++
	}

	// 查询总数
	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM workflow_instances %s", where)
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// 查询分页数据
	offset := (page - 1) * pageSize
	dataQuery := fmt.Sprintf(
		`SELECT id, business_app_code, workflow_template_id, workflow_template_key, workflow_template_version,
		        graph_key, title, status, input_json, output_json, created_by,
		        started_at, finished_at, trace_id, created_at, updated_at
		 FROM workflow_instances %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		where, argIdx, argIdx+1)
	args = append(args, pageSize, offset)

	rows, err := r.pool.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var instances []Instance
	for rows.Next() {
		var inst Instance
		if err := rows.Scan(&inst.ID, &inst.BusinessAppCode, &inst.WorkflowTemplateID, &inst.WorkflowTemplateKey,
			&inst.WorkflowTemplateVersion, &inst.GraphKey, &inst.Title, &inst.Status,
			&inst.InputJSON, &inst.OutputJSON, &inst.CreatedBy,
			&inst.StartedAt, &inst.FinishedAt, &inst.TraceID, &inst.CreatedAt, &inst.UpdatedAt); err != nil {
			return nil, 0, err
		}
		instances = append(instances, inst)
	}
	return instances, total, nil
}

// UpdateInstanceStatus 更新实例状态。
// 可同时设置 started_at 或 finished_at。
func (r *Repository) UpdateInstanceStatus(ctx context.Context, id, status string, startedAt, finishedAt *time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE workflow_instances SET status = $2, started_at = COALESCE($3, started_at),
		 finished_at = COALESCE($4, finished_at), updated_at = now()
		 WHERE id = $1`,
		id, status, startedAt, finishedAt)
	return err
}

// UpdateInstanceOutput 更新实例的最终输出 JSON。
func (r *Repository) UpdateInstanceOutput(ctx context.Context, id string, outputJSON string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE workflow_instances SET output_json = $2, updated_at = now() WHERE id = $1`,
		id, outputJSON)
	return err
}

// ── 节点实例 CRUD ──

// CreateNodeInstances 批量创建节点实例（从模板 definition_json 初始化）。
func (r *Repository) CreateNodeInstances(ctx context.Context, workflowInstanceID string, nodes []TemplateNode) error {
	for _, node := range nodes {
		maxRetries := node.MaxRetries
		if maxRetries == 0 && node.Type == NodeTypeAgentGraph {
			maxRetries = 3 // agent_graph 节点默认最多重试 3 次
		}
		_, err := r.pool.Exec(ctx,
			`INSERT INTO workflow_node_instances
			 (workflow_instance_id, node_key, node_type, name, status, max_retries)
			 VALUES ($1,$2,$3,$4,$5,$6)`,
			workflowInstanceID, node.ID, node.Type, node.Name, NodeStatusPending, maxRetries)
		if err != nil {
			return fmt.Errorf("insert node %s: %w", node.ID, err)
		}
	}
	return nil
}

// FindNodeInstancesByWorkflow 查询某个工作流实例的所有节点。
func (r *Repository) FindNodeInstancesByWorkflow(ctx context.Context, workflowInstanceID string) ([]NodeInstance, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, workflow_instance_id, node_key, node_type, name, status,
		        input_json, output_json, error_json, retry_count, max_retries,
		        started_at, finished_at, created_at, updated_at
		 FROM workflow_node_instances
		 WHERE workflow_instance_id = $1 AND deleted_at IS NULL
		 ORDER BY created_at`, workflowInstanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []NodeInstance
	for rows.Next() {
		var n NodeInstance
		if err := rows.Scan(&n.ID, &n.WorkflowInstanceID, &n.NodeKey, &n.NodeType, &n.Name, &n.Status,
			&n.InputJSON, &n.OutputJSON, &n.ErrorJSON, &n.RetryCount, &n.MaxRetries,
			&n.StartedAt, &n.FinishedAt, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

// FindNodeInstanceByID 根据 UUID 查询单个节点实例。
func (r *Repository) FindNodeInstanceByID(ctx context.Context, id string) (*NodeInstance, error) {
	n := &NodeInstance{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, workflow_instance_id, node_key, node_type, name, status,
		        input_json, output_json, error_json, retry_count, max_retries,
		        started_at, finished_at, created_at, updated_at
		 FROM workflow_node_instances WHERE id = $1 AND deleted_at IS NULL`, id,
	).Scan(&n.ID, &n.WorkflowInstanceID, &n.NodeKey, &n.NodeType, &n.Name, &n.Status,
		&n.InputJSON, &n.OutputJSON, &n.ErrorJSON, &n.RetryCount, &n.MaxRetries,
		&n.StartedAt, &n.FinishedAt, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return n, nil
}

// UpdateNodeStatus 更新节点状态。
func (r *Repository) UpdateNodeStatus(ctx context.Context, id, status string, startedAt, finishedAt *time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE workflow_node_instances SET status = $2,
		 started_at = COALESCE($3, started_at),
		 finished_at = COALESCE($4, finished_at),
		 updated_at = now()
		 WHERE id = $1`,
		id, status, startedAt, finishedAt)
	return err
}

// UpdateNodeInput 设置节点输入 JSON。
func (r *Repository) UpdateNodeInput(ctx context.Context, id string, inputJSON string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE workflow_node_instances SET input_json = $2, updated_at = now() WHERE id = $1`,
		id, inputJSON)
	return err
}

// UpdateNodeOutput 设置节点输出 JSON。
func (r *Repository) UpdateNodeOutput(ctx context.Context, id string, outputJSON string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE workflow_node_instances SET output_json = $2, updated_at = now() WHERE id = $1`,
		id, outputJSON)
	return err
}

// UpdateNodeError 记录节点执行失败的详细信息。
func (r *Repository) UpdateNodeError(ctx context.Context, id string, errorJSON string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE workflow_node_instances SET error_json = $2, retry_count = retry_count + 1, updated_at = now() WHERE id = $1`,
		id, errorJSON)
	return err
}

// FindPendingNodeByWorkflow 查询某工作流实例中第一个 pending 状态的节点。
// 用于 worker 调度：每完成一个节点，找下一个 pending 节点执行。
func (r *Repository) FindPendingNodeByWorkflow(ctx context.Context, workflowInstanceID string) (*NodeInstance, error) {
	n := &NodeInstance{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, workflow_instance_id, node_key, node_type, name, status,
		        input_json, output_json, error_json, retry_count, max_retries,
		        started_at, finished_at, created_at, updated_at
		 FROM workflow_node_instances
		 WHERE workflow_instance_id = $1 AND status = 'pending' AND deleted_at IS NULL
		 ORDER BY created_at LIMIT 1`, workflowInstanceID,
	).Scan(&n.ID, &n.WorkflowInstanceID, &n.NodeKey, &n.NodeType, &n.Name, &n.Status,
		&n.InputJSON, &n.OutputJSON, &n.ErrorJSON, &n.RetryCount, &n.MaxRetries,
		&n.StartedAt, &n.FinishedAt, &n.CreatedAt, &n.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil // 没有待执行节点
	}
	if err != nil {
		return nil, err
	}
	return n, nil
}

// CancelPendingNodes 取消工作流实例中所有 pending 状态的节点。
// 工作流被取消时调用。
func (r *Repository) CancelPendingNodes(ctx context.Context, workflowInstanceID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE workflow_node_instances SET status = $2, updated_at = now()
		 WHERE workflow_instance_id = $1 AND status IN ('pending','running','waiting_review')`,
		workflowInstanceID, NodeStatusCancelled)
	return err
}
