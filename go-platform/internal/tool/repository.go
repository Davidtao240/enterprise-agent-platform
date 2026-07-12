package tool

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository 封装 Tool Registry 和 Agent-Tool Permission 的数据库查询。
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository 创建 Repository 实例。
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// ListTools 查询所有 active 状态的 Tool。
func (r *Repository) ListTools(ctx context.Context, domain, riskLevel, isShared, status string) ([]Tool, error) {
	where := "WHERE deleted_at IS NULL"
	args := []any{}
	argIdx := 1
	if domain != "" {
		where += " AND domain = $" + strconv.Itoa(argIdx)
		args = append(args, domain)
		argIdx++
	}
	if riskLevel != "" {
		where += " AND risk_level = $" + strconv.Itoa(argIdx)
		args = append(args, riskLevel)
		argIdx++
	}
	if isShared != "" {
		where += " AND is_shared = $" + strconv.Itoa(argIdx)
		args = append(args, isShared)
		argIdx++
	}
	if status != "" {
		where += " AND status = $" + strconv.Itoa(argIdx)
		args = append(args, status)
	} else {
		where += " AND status = 'active'"
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id, tool_id, name, domain, risk_level, is_shared, input_schema_json, output_schema_json, status, created_at, updated_at
		 FROM tool_registry `+where+` ORDER BY name`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tools []Tool
	for rows.Next() {
		var t Tool
		if err := rows.Scan(&t.ID, &t.ToolID, &t.Name, &t.Domain, &t.RiskLevel, &t.IsShared, &t.InputSchemaJSON, &t.OutputSchemaJSON, &t.Status, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		tools = append(tools, t)
	}
	return tools, nil
}

// FindPermissionsByAgent 查询某 Agent 在某业务下的所有 Tool 权限。
func (r *Repository) FindPermissionsByAgent(ctx context.Context, agentID, businessAppCode string) ([]AgentToolPermission, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT agent_id, tool_id, business_app_code, status
		 FROM agent_tool_permissions
		 WHERE agent_id = $1 AND business_app_code = $2 AND status = 'active'`,
		agentID, businessAppCode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var perms []AgentToolPermission
	for rows.Next() {
		var p AgentToolPermission
		if err := rows.Scan(&p.AgentID, &p.ToolID, &p.BusinessAppCode, &p.Status); err != nil {
			return nil, err
		}
		perms = append(perms, p)
	}
	return perms, nil
}
