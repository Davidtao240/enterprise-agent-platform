package agent_gallery

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) ListPublishedPackages(ctx context.Context, tenantID, category, businessAppCode, query string) ([]AgentPackageListItem, error) {
	q := `SELECT p.id, p.package_code, p.name, p.description, p.category, p.business_app_code,
		        p.icon, p.status,
		        COALESCE(SUM(s.conversation_count), 0) AS usage_count,
		        MAX(c.last_message_at) AS last_used_at
		 FROM agent_packages p
		 LEFT JOIN agent_package_usage_stats s ON s.package_code = p.package_code
		   AND s.tenant_id = p.tenant_id
		   AND s.stat_date >= CURRENT_DATE - INTERVAL '30 days'
		 LEFT JOIN conversations c ON c.agent_package_code = p.package_code
		   AND c.tenant_id = p.tenant_id
		   AND c.deleted_at IS NULL
		 WHERE p.tenant_id = $1 AND p.status = 'published' AND p.deleted_at IS NULL`

	args := []any{tenantID}
	argIdx := 2

	if category != "" {
		q += fmt.Sprintf(" AND p.category = $%d", argIdx)
		args = append(args, category)
		argIdx++
	}
	if businessAppCode != "" {
		q += fmt.Sprintf(" AND p.business_app_code = $%d", argIdx)
		args = append(args, businessAppCode)
		argIdx++
	}
	if query != "" {
		q += fmt.Sprintf(" AND (p.name ILIKE $%d OR p.description ILIKE $%d)", argIdx, argIdx)
		args = append(args, "%"+query+"%")
		argIdx++
	}

	q += ` GROUP BY p.id, p.package_code, p.name, p.description, p.category, p.business_app_code, p.icon, p.status
		 ORDER BY p.name ASC`

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list packages: %w", err)
	}
	defer rows.Close()

	var packages []AgentPackageListItem
	for rows.Next() {
		var p AgentPackageListItem
		if err := rows.Scan(&p.ID, &p.PackageCode, &p.Name, &p.Description,
			&p.Category, &p.BusinessAppCode, &p.Icon, &p.Status,
			&p.UsageCount, &p.LastUsedAt); err != nil {
			return nil, fmt.Errorf("scan package: %w", err)
		}
		packages = append(packages, p)
	}
	return packages, nil
}

func (r *Repository) GetPackageByCode(ctx context.Context, tenantID, packageCode string) (*AgentPackage, error) {
	pkg := &AgentPackage{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, tenant_id, package_code, name, description, category, business_app_code,
		        graph_key, graph_version, entry_type, icon, capabilities_json::text,
		        sample_prompts_json::text, status, published_at, created_at, updated_at
		 FROM agent_packages
		 WHERE tenant_id = $1 AND package_code = $2 AND deleted_at IS NULL`,
		tenantID, packageCode,
	).Scan(&pkg.ID, &pkg.TenantID, &pkg.PackageCode, &pkg.Name, &pkg.Description,
		&pkg.Category, &pkg.BusinessAppCode, &pkg.GraphKey, &pkg.GraphVersion,
		&pkg.EntryType, &pkg.Icon, &pkg.CapabilitiesJSON, &pkg.SamplePromptsJSON,
		&pkg.Status, &pkg.PublishedAt, &pkg.CreatedAt, &pkg.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get package: %w", err)
	}
	return pkg, nil
}

func (r *Repository) GetPackageStats(ctx context.Context, tenantID, packageCode string) (int, []UsageDay, error) {
	var totalConversations int
	err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(conversation_count), 0)
		 FROM agent_package_usage_stats
		 WHERE tenant_id = $1 AND package_code = $2`,
		tenantID, packageCode,
	).Scan(&totalConversations)
	if err != nil {
		return 0, nil, fmt.Errorf("get total stats: %w", err)
	}

	rows, err := r.pool.Query(ctx,
		`SELECT stat_date, conversation_count, message_count
		 FROM agent_package_usage_stats
		 WHERE tenant_id = $1 AND package_code = $2
		   AND stat_date >= CURRENT_DATE - INTERVAL '7 days'
		 ORDER BY stat_date DESC
		 LIMIT 7`,
		tenantID, packageCode,
	)
	if err != nil {
		return totalConversations, nil, fmt.Errorf("get daily stats: %w", err)
	}
	defer rows.Close()

	var days []UsageDay
	for rows.Next() {
		var d UsageDay
		var date time.Time
		if err := rows.Scan(&date, &d.ConversationCount, &d.MessageCount); err != nil {
			return totalConversations, nil, fmt.Errorf("scan daily stats: %w", err)
		}
		d.Date = date.Format("2006-01-02")
		days = append(days, d)
	}
	return totalConversations, days, nil
}

func (r *Repository) CreatePackage(ctx context.Context, pkg *AgentPackage) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO agent_packages
		 (tenant_id, package_code, name, description, category, business_app_code,
		  graph_key, graph_version, entry_type, icon, capabilities_json, sample_prompts_json, status)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb,$12::jsonb,'draft')
		 RETURNING id, created_at, updated_at`,
		pkg.TenantID, pkg.PackageCode, pkg.Name, pkg.Description, pkg.Category,
		pkg.BusinessAppCode, pkg.GraphKey, pkg.GraphVersion, pkg.EntryType, pkg.Icon,
		pkg.CapabilitiesJSON, pkg.SamplePromptsJSON,
	).Scan(&pkg.ID, &pkg.CreatedAt, &pkg.UpdatedAt)
}

func (r *Repository) UpdatePackage(ctx context.Context, tenantID, packageCode string, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}

	var args []any
	query := "UPDATE agent_packages SET updated_at = now()"

	if name, ok := updates["name"]; ok {
		query += fmt.Sprintf(", name = $%d", len(args)+1)
		args = append(args, name)
	}
	if desc, ok := updates["description"]; ok {
		query += fmt.Sprintf(", description = $%d", len(args)+1)
		args = append(args, desc)
	}
	if icon, ok := updates["icon"]; ok {
		query += fmt.Sprintf(", icon = $%d", len(args)+1)
		args = append(args, icon)
	}
	if status, ok := updates["status"]; ok {
		query += fmt.Sprintf(", status = $%d", len(args)+1)
		args = append(args, status)
		if status == "published" {
			query += fmt.Sprintf(", published_at = $%d", len(args)+1)
			args = append(args, "NOW()")
		}
	}
	if caps, ok := updates["capabilities_json"]; ok {
		b, _ := json.Marshal(caps)
		query += fmt.Sprintf(", capabilities_json = $%d::jsonb", len(args)+1)
		args = append(args, string(b))
	}
	if prompts, ok := updates["sample_prompts_json"]; ok {
		b, _ := json.Marshal(prompts)
		query += fmt.Sprintf(", sample_prompts_json = $%d::jsonb", len(args)+1)
		args = append(args, string(b))
	}

	idPos := len(args) + 1
	codePos := len(args) + 2
	query += fmt.Sprintf(" WHERE tenant_id = $%d AND package_code = $%d AND deleted_at IS NULL", idPos, codePos)
	args = append(args, tenantID, packageCode)

	tag, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("package %s not found", packageCode)
	}
	return nil
}

func (r *Repository) CheckGraphKeyExists(ctx context.Context, graphKey string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM agents WHERE graph_key = $1 AND status = 'active')`,
		graphKey,
	).Scan(&exists)
	return exists, err
}