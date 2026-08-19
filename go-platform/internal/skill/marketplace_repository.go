package skill

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MarketplaceRepository struct {
	pool *pgxpool.Pool
}

func NewMarketplaceRepository(pool *pgxpool.Pool) *MarketplaceRepository {
	return &MarketplaceRepository{pool: pool}
}

func (r *MarketplaceRepository) ListMarketplace(ctx context.Context, tenantID, userID, category, query, status string, page, pageSize int) ([]SkillMarketplaceItem, int, error) {
	countQ := `SELECT COUNT(*) FROM skill_registry WHERE 1=1`
	dataQ := `SELECT sr.id, sr.skill_code, sr.version, sr.name, sr.description, sr.category,
		        sr.icon, sr.tags::text, sr.author, sr.homepage_url, sr.status::text,
		        sr.is_current, sr.usage_count, sr.published_at,
		        si.status as install_status
		    FROM skill_registry sr
		    LEFT JOIN skill_installations si ON si.skill_id = sr.id AND si.tenant_id = $1 AND si.user_id = $2
		    WHERE 1=1`

	args := []any{tenantID, userID}
	argIdx := 3

	if category != "" {
		countQ += fmt.Sprintf(" AND category = $%d", argIdx)
		dataQ += fmt.Sprintf(" AND sr.category = $%d", argIdx)
		args = append(args, category)
		argIdx++
	}
	if status != "" {
		countQ += fmt.Sprintf(" AND status = $%d", argIdx)
		dataQ += fmt.Sprintf(" AND sr.status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}
	if query != "" {
		countQ += fmt.Sprintf(" AND (name ILIKE $%d OR description ILIKE $%d OR skill_code ILIKE $%d)", argIdx, argIdx, argIdx)
		dataQ += fmt.Sprintf(" AND (sr.name ILIKE $%d OR sr.description ILIKE $%d OR sr.skill_code ILIKE $%d)", argIdx, argIdx, argIdx)
		searchArg := "%" + query + "%"
		args = append(args, searchArg)
		argIdx++
	}

	var total int
	if err := r.pool.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count skills: %w", err)
	}

	dataQ += " ORDER BY sr.name ASC"
	if page > 0 && pageSize > 0 {
		offset := (page - 1) * pageSize
		dataQ += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
		args = append(args, pageSize, offset)
	}

	rows, err := r.pool.Query(ctx, dataQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list marketplace: %w", err)
	}
	defer rows.Close()

	var items []SkillMarketplaceItem
	for rows.Next() {
		var item SkillMarketplaceItem
		var tagsText string
		var installStatus *string
		if err := rows.Scan(&item.ID, &item.SkillCode, &item.Version, &item.Name,
			&item.Description, &item.Category, &item.Icon, &tagsText,
			&item.Author, &item.HomepageURL, &item.Status, &item.IsCurrent,
			&item.UsageCount, &item.PublishedAt, &installStatus); err != nil {
			return nil, 0, fmt.Errorf("scan marketplace item: %w", err)
		}
		_ = json.Unmarshal([]byte(tagsText), &item.Tags)

		if installStatus != nil {
			item.Installed = true
			item.InstallStatus = *installStatus
		}
		items = append(items, item)
	}
	return items, total, nil
}

func (r *MarketplaceRepository) InstallSkill(ctx context.Context, tenantID, userID, skillID, skillCode, version string) (*SkillInstallation, error) {
	id := uuid.NewString()
	var inst SkillInstallation
	err := r.pool.QueryRow(ctx,
		`INSERT INTO skill_installations
		 (id, tenant_id, user_id, skill_id, skill_code, installed_version, status)
		 VALUES ($1,$2,$3,$4,$5,$6,'active')
		 ON CONFLICT (tenant_id, user_id, skill_code) DO UPDATE SET
		   skill_id = EXCLUDED.skill_id,
		   installed_version = EXCLUDED.installed_version,
		   status = 'active',
		   updated_at = NOW()
		 RETURNING id, tenant_id, user_id, skill_id, skill_code, installed_version, status, installed_at, updated_at`,
		id, tenantID, userID, skillID, skillCode, version,
	).Scan(&inst.ID, &inst.TenantID, &inst.UserID, &inst.SkillID, &inst.SkillCode,
		&inst.InstalledVersion, &inst.Status, &inst.InstalledAt, &inst.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("install skill: %w", err)
	}
	return &inst, nil
}

func (r *MarketplaceRepository) GetInstallation(ctx context.Context, tenantID, userID, skillCode string) (*SkillInstallation, error) {
	var inst SkillInstallation
	err := r.pool.QueryRow(ctx,
		`SELECT id, tenant_id, user_id, skill_id, skill_code, installed_version, status, installed_at, updated_at
		 FROM skill_installations
		 WHERE tenant_id = $1 AND user_id = $2 AND skill_code = $3`,
		tenantID, userID, skillCode,
	).Scan(&inst.ID, &inst.TenantID, &inst.UserID, &inst.SkillID, &inst.SkillCode,
		&inst.InstalledVersion, &inst.Status, &inst.InstalledAt, &inst.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get installation: %w", err)
	}
	return &inst, nil
}

func (r *MarketplaceRepository) UpdateInstallation(ctx context.Context, tenantID, userID, skillCode, status string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE skill_installations SET status = $4, updated_at = NOW()
		 WHERE tenant_id = $1 AND user_id = $2 AND skill_code = $3`,
		tenantID, userID, skillCode, status)
	if err != nil {
		return fmt.Errorf("update installation: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("installation for skill %s not found", skillCode)
	}
	return nil
}

func (r *MarketplaceRepository) ListInstalledByUser(ctx context.Context, tenantID, userID string) ([]SkillInstallation, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, tenant_id, user_id, skill_id, skill_code, installed_version, status, installed_at, updated_at
		 FROM skill_installations
		 WHERE tenant_id = $1 AND user_id = $2 AND status = 'active'
		 ORDER BY installed_at DESC`,
		tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("list installed: %w", err)
	}
	defer rows.Close()

	var items []SkillInstallation
	for rows.Next() {
		var inst SkillInstallation
		if err := rows.Scan(&inst.ID, &inst.TenantID, &inst.UserID, &inst.SkillID, &inst.SkillCode,
			&inst.InstalledVersion, &inst.Status, &inst.InstalledAt, &inst.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan installation: %w", err)
		}
		items = append(items, inst)
	}
	return items, nil
}

func (r *MarketplaceRepository) RecordUsage(ctx context.Context, tenantID, skillCode, skillVersion, userID, sessionID, toolCallID string) error {
	id := uuid.NewString()
	_, err := r.pool.Exec(ctx,
		`INSERT INTO skill_usage_events
		 (id, tenant_id, skill_code, skill_version, user_id, session_id, tool_call_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		id, tenantID, skillCode, skillVersion, userID, sessionID, toolCallID)
	if err != nil {
		return fmt.Errorf("record usage: %w", err)
	}

	_, err = r.pool.Exec(ctx,
		`UPDATE skill_registry SET usage_count = usage_count + 1 WHERE skill_code = $1 AND is_current = true`,
		skillCode)
	return err
}

func (r *MarketplaceRepository) UpdateSkillMetadata(ctx context.Context, skillID string, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}

	var args []any
	query := "UPDATE skill_registry SET updated_at = NOW()"

	if name, ok := updates["name"]; ok {
		query += fmt.Sprintf(", name = $%d", len(args)+1)
		args = append(args, name)
	}
	if desc, ok := updates["description"]; ok {
		query += fmt.Sprintf(", description = $%d", len(args)+1)
		args = append(args, desc)
	}
	if category, ok := updates["category"]; ok {
		query += fmt.Sprintf(", category = $%d", len(args)+1)
		args = append(args, category)
	}
	if icon, ok := updates["icon"]; ok {
		query += fmt.Sprintf(", icon = $%d", len(args)+1)
		args = append(args, icon)
	}
	if tags, ok := updates["tags"]; ok {
		b, _ := json.Marshal(tags)
		query += fmt.Sprintf(", tags = $%d::jsonb", len(args)+1)
		args = append(args, string(b))
	}
	if author, ok := updates["author"]; ok {
		query += fmt.Sprintf(", author = $%d", len(args)+1)
		args = append(args, author)
	}
	if homepage, ok := updates["homepage_url"]; ok {
		query += fmt.Sprintf(", homepage_url = $%d", len(args)+1)
		args = append(args, homepage)
	}

	idPos := len(args) + 1
	query += fmt.Sprintf(" WHERE id = $%d", idPos)
	args = append(args, skillID)

	tag, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("skill %s not found", skillID)
	}
	return nil
}

func (r *MarketplaceRepository) SetCurrentVersion(ctx context.Context, skillCode, version string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE skill_registry SET is_current = false WHERE skill_code = $1`,
		skillCode)
	if err != nil {
		return fmt.Errorf("clear current: %w", err)
	}
	_, err = r.pool.Exec(ctx,
		`UPDATE skill_registry SET is_current = true WHERE skill_code = $1 AND version = $2`,
		skillCode, version)
	if err != nil {
		return fmt.Errorf("set current: %w", err)
	}
	return nil
}