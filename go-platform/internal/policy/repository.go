package policy

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) ListDomainPolicies(ctx context.Context, businessAppCode, status string) ([]DomainPolicy, error) {
	where := "WHERE deleted_at IS NULL"
	args := []any{}
	if businessAppCode != "" {
		where += " AND business_app_code = $" + strconv.Itoa(len(args)+1)
		args = append(args, businessAppCode)
	}
	if status != "" {
		where += " AND status = $" + strconv.Itoa(len(args)+1)
		args = append(args, status)
	}

	rows, err := r.pool.Query(ctx,
		`SELECT id, business_app_code, allowed_agent_domains::text, allowed_tool_domains::text,
		        allow_shared_agents, allow_shared_tools, high_risk_requires_review,
		        status, created_at, updated_at
		 FROM domain_policies `+where+` ORDER BY business_app_code`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var policies []DomainPolicy
	for rows.Next() {
		var item DomainPolicy
		if err := rows.Scan(
			&item.ID,
			&item.BusinessAppCode,
			&item.AllowedAgentDomainsJSON,
			&item.AllowedToolDomainsJSON,
			&item.AllowSharedAgents,
			&item.AllowSharedTools,
			&item.HighRiskRequiresReview,
			&item.Status,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		policies = append(policies, item)
	}
	return policies, rows.Err()
}

// FindAllowedDomains 实现 tool.DomainPolicyProvider 接口:
// 查询指定业务域允许的工具域列表,用于跨域调用校验。
func (r *Repository) FindAllowedDomains(ctx context.Context, businessAppCode string) ([]string, error) {
	var allowedDomainsJSON string
	err := r.pool.QueryRow(ctx,
		`SELECT allowed_tool_domains::text
		 FROM domain_policies
		 WHERE business_app_code = $1 AND status = 'active' AND deleted_at IS NULL
		 LIMIT 1`,
		businessAppCode,
	).Scan(&allowedDomainsJSON)
	if err != nil {
		return nil, nil
	}

	var domains []string
	if err := json.Unmarshal([]byte(allowedDomainsJSON), &domains); err != nil {
		return nil, nil
	}
	return domains, nil
}
