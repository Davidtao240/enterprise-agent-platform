package policy

import (
	"context"
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
