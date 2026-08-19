package agent_gallery

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type packageRepository interface {
	CreateVersion(ctx context.Context, v *PackageVersion) error
	ListVersions(ctx context.Context, tenantID, packageCode string) ([]PackageVersion, error)
	GetVersion(ctx context.Context, tenantID, packageCode, version string) (*PackageVersion, error)
	SetCurrentVersion(ctx context.Context, tenantID, packageCode, version string) error
	DeprecateOtherVersions(ctx context.Context, tenantID, packageCode, keepVersion string) error

	CreateInstallation(ctx context.Context, inst *PackageInstallation) error
	GetInstallation(ctx context.Context, tenantID, packageCode string) (*PackageInstallation, error)
	UpdateInstallation(ctx context.Context, tenantID, packageCode string, updates map[string]any) error
	ListInstallations(ctx context.Context, tenantID, status string) ([]PackageInstallation, error)

	CreateRegistration(ctx context.Context, reg *PackageRegistration) error
	GetRegistration(ctx context.Context, tenantID, packageCode string) (*PackageRegistration, error)
	UpdateRegistration(ctx context.Context, tenantID, packageCode string, updates map[string]any) error
	ListRegistrations(ctx context.Context, tenantID, status, sourceType string) ([]PackageRegistration, error)
}

func (r *Repository) CreateVersion(ctx context.Context, v *PackageVersion) error {
	manifestJSON := "{}"
	if v.ManifestJSON != nil {
		b, _ := json.Marshal(v.ManifestJSON)
		manifestJSON = string(b)
	}
	return r.pool.QueryRow(ctx,
		`INSERT INTO agent_package_versions
		 (tenant_id, package_code, version, manifest_json, graph_key, graph_version, entry_type, status, is_current, created_by)
		 VALUES ($1,$2,$3,$4::jsonb,$5,$6,$7,$8,$9,$10)
		 RETURNING id, created_at, updated_at`,
		v.TenantID, v.PackageCode, v.Version, manifestJSON,
		v.GraphKey, v.GraphVersion, v.EntryType, v.Status, v.IsCurrent, v.CreatedBy,
	).Scan(&v.ID, &v.CreatedAt, &v.UpdatedAt)
}

func (r *Repository) ListVersions(ctx context.Context, tenantID, packageCode string) ([]PackageVersion, error) {
	q := `SELECT id, tenant_id, package_code, version, manifest_json::text,
		        graph_key, graph_version, entry_type, status, is_current, created_by, created_at, updated_at
		 FROM agent_package_versions
		 WHERE tenant_id = $1`
	args := []any{tenantID}
	argIdx := 2

	if packageCode != "" {
		q += fmt.Sprintf(" AND package_code = $%d", argIdx)
		args = append(args, packageCode)
		argIdx++
	}

	q += " ORDER BY package_code, version DESC"

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list versions: %w", err)
	}
	defer rows.Close()

	var versions []PackageVersion
	for rows.Next() {
		var v PackageVersion
		var manifestText string
		if err := rows.Scan(&v.ID, &v.TenantID, &v.PackageCode, &v.Version,
			&manifestText, &v.GraphKey, &v.GraphVersion, &v.EntryType,
			&v.Status, &v.IsCurrent, &v.CreatedBy, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan version: %w", err)
		}
		_ = json.Unmarshal([]byte(manifestText), &v.ManifestJSON)
		versions = append(versions, v)
	}
	return versions, nil
}

func (r *Repository) GetVersion(ctx context.Context, tenantID, packageCode, version string) (*PackageVersion, error) {
	var v PackageVersion
	var manifestText string
	err := r.pool.QueryRow(ctx,
		`SELECT id, tenant_id, package_code, version, manifest_json::text,
		        graph_key, graph_version, entry_type, status, is_current, created_by, created_at, updated_at
		 FROM agent_package_versions
		 WHERE tenant_id = $1 AND package_code = $2 AND version = $3`,
		tenantID, packageCode, version,
	).Scan(&v.ID, &v.TenantID, &v.PackageCode, &v.Version,
		&manifestText, &v.GraphKey, &v.GraphVersion, &v.EntryType,
		&v.Status, &v.IsCurrent, &v.CreatedBy, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get version: %w", err)
	}
	_ = json.Unmarshal([]byte(manifestText), &v.ManifestJSON)
	return &v, nil
}

func (r *Repository) SetCurrentVersion(ctx context.Context, tenantID, packageCode, version string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE agent_package_versions SET is_current = false, status = 'deprecated', updated_at = NOW()
		 WHERE tenant_id = $1 AND package_code = $2 AND is_current = true`,
		tenantID, packageCode)
	if err != nil {
		return fmt.Errorf("deprecate current versions: %w", err)
	}
	_ = tag

	tag, err = r.pool.Exec(ctx,
		`UPDATE agent_package_versions SET is_current = true, status = 'published', updated_at = NOW()
		 WHERE tenant_id = $1 AND package_code = $2 AND version = $3`,
		tenantID, packageCode, version)
	if err != nil {
		return fmt.Errorf("set current version: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("version %s not found for package %s", version, packageCode)
	}
	return nil
}

func (r *Repository) DeprecateOtherVersions(ctx context.Context, tenantID, packageCode, keepVersion string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE agent_package_versions SET status = 'deprecated', is_current = false, updated_at = NOW()
		 WHERE tenant_id = $1 AND package_code = $2 AND version != $3`,
		tenantID, packageCode, keepVersion)
	return err
}

func (r *Repository) CreateInstallation(ctx context.Context, inst *PackageInstallation) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO agent_package_installations
		 (tenant_id, package_code, installed_version, installed_by, status)
		 VALUES ($1,$2,$3,$4,'active')
		 ON CONFLICT (tenant_id, package_code) DO UPDATE SET
		   installed_version = EXCLUDED.installed_version,
		   updated_at = NOW(),
		   status = 'active'
		 RETURNING id, installed_at, updated_at`,
		inst.TenantID, inst.PackageCode, inst.InstalledVersion, inst.InstalledBy,
	).Scan(&inst.ID, &inst.InstalledAt, &inst.UpdatedAt)
}

func (r *Repository) GetInstallation(ctx context.Context, tenantID, packageCode string) (*PackageInstallation, error) {
	var inst PackageInstallation
	err := r.pool.QueryRow(ctx,
		`SELECT id, tenant_id, package_code, installed_version, installed_at, installed_by, status, updated_at
		 FROM agent_package_installations
		 WHERE tenant_id = $1 AND package_code = $2`,
		tenantID, packageCode,
	).Scan(&inst.ID, &inst.TenantID, &inst.PackageCode, &inst.InstalledVersion,
		&inst.InstalledAt, &inst.InstalledBy, &inst.Status, &inst.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get installation: %w", err)
	}
	return &inst, nil
}

func (r *Repository) UpdateInstallation(ctx context.Context, tenantID, packageCode string, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}

	var args []any
	query := "UPDATE agent_package_installations SET updated_at = NOW()"

	if status, ok := updates["status"]; ok {
		query += fmt.Sprintf(", status = $%d", len(args)+1)
		args = append(args, status)
	}
	if version, ok := updates["installed_version"]; ok {
		query += fmt.Sprintf(", installed_version = $%d", len(args)+1)
		args = append(args, version)
	}

	idPos := len(args) + 1
	codePos := len(args) + 2
	query += fmt.Sprintf(" WHERE tenant_id = $%d AND package_code = $%d", idPos, codePos)
	args = append(args, tenantID, packageCode)

	tag, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("installation for %s not found", packageCode)
	}
	return nil
}

func (r *Repository) ListInstallations(ctx context.Context, tenantID, status string) ([]PackageInstallation, error) {
	q := `SELECT id, tenant_id, package_code, installed_version, installed_at, installed_by, status, updated_at
		 FROM agent_package_installations
		 WHERE tenant_id = $1`
	args := []any{tenantID}
	argIdx := 2

	if status != "" {
		q += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}

	q += " ORDER BY installed_at DESC"

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list installations: %w", err)
	}
	defer rows.Close()

	var items []PackageInstallation
	for rows.Next() {
		var inst PackageInstallation
		if err := rows.Scan(&inst.ID, &inst.TenantID, &inst.PackageCode, &inst.InstalledVersion,
			&inst.InstalledAt, &inst.InstalledBy, &inst.Status, &inst.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan installation: %w", err)
		}
		items = append(items, inst)
	}
	return items, nil
}

func (r *Repository) CreateRegistration(ctx context.Context, reg *PackageRegistration) error {
	manifestJSON := "{}"
	if reg.ManifestJSON != nil {
		b, _ := json.Marshal(reg.ManifestJSON)
		manifestJSON = string(b)
	}
	return r.pool.QueryRow(ctx,
		`INSERT INTO agent_package_registrations
		 (tenant_id, package_code, source_type, source_url, manifest_json, signature, verified, status, registered_by)
		 VALUES ($1,$2,$3,$4,$5::jsonb,$6,$7,$8,$9)
		 ON CONFLICT (tenant_id, package_code) DO UPDATE SET
		   source_type = EXCLUDED.source_type,
		   source_url = EXCLUDED.source_url,
		   manifest_json = EXCLUDED.manifest_json,
		   updated_at = NOW()
		 RETURNING id, created_at, updated_at`,
		reg.TenantID, reg.PackageCode, reg.SourceType, reg.SourceURL,
		manifestJSON, reg.Signature, reg.Verified, reg.Status, reg.RegisteredBy,
	).Scan(&reg.ID, &reg.CreatedAt, &reg.UpdatedAt)
}

func (r *Repository) GetRegistration(ctx context.Context, tenantID, packageCode string) (*PackageRegistration, error) {
	var reg PackageRegistration
	var manifestText string
	err := r.pool.QueryRow(ctx,
		`SELECT id, tenant_id, package_code, source_type, source_url, manifest_json::text,
		        signature, verified, status, registered_by, created_at, updated_at
		 FROM agent_package_registrations
		 WHERE tenant_id = $1 AND package_code = $2`,
		tenantID, packageCode,
	).Scan(&reg.ID, &reg.TenantID, &reg.PackageCode, &reg.SourceType, &reg.SourceURL,
		&manifestText, &reg.Signature, &reg.Verified, &reg.Status, &reg.RegisteredBy,
		&reg.CreatedAt, &reg.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get registration: %w", err)
	}
	_ = json.Unmarshal([]byte(manifestText), &reg.ManifestJSON)
	return &reg, nil
}

func (r *Repository) UpdateRegistration(ctx context.Context, tenantID, packageCode string, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}

	var args []any
	query := "UPDATE agent_package_registrations SET updated_at = NOW()"

	if status, ok := updates["status"]; ok {
		query += fmt.Sprintf(", status = $%d", len(args)+1)
		args = append(args, status)
	}
	if verified, ok := updates["verified"]; ok {
		query += fmt.Sprintf(", verified = $%d", len(args)+1)
		args = append(args, verified)
	}

	idPos := len(args) + 1
	codePos := len(args) + 2
	query += fmt.Sprintf(" WHERE tenant_id = $%d AND package_code = $%d", idPos, codePos)
	args = append(args, tenantID, packageCode)

	tag, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("registration for %s not found", packageCode)
	}
	return nil
}

func (r *Repository) ListRegistrations(ctx context.Context, tenantID, status, sourceType string) ([]PackageRegistration, error) {
	q := `SELECT id, tenant_id, package_code, source_type, source_url, manifest_json::text,
		        signature, verified, status, registered_by, created_at, updated_at
		 FROM agent_package_registrations
		 WHERE tenant_id = $1`
	args := []any{tenantID}
	argIdx := 2

	if status != "" {
		q += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}
	if sourceType != "" {
		q += fmt.Sprintf(" AND source_type = $%d", argIdx)
		args = append(args, sourceType)
		argIdx++
	}

	q += " ORDER BY created_at DESC"

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list registrations: %w", err)
	}
	defer rows.Close()

	var items []PackageRegistration
	for rows.Next() {
		var reg PackageRegistration
		var manifestText string
		if err := rows.Scan(&reg.ID, &reg.TenantID, &reg.PackageCode, &reg.SourceType, &reg.SourceURL,
			&manifestText, &reg.Signature, &reg.Verified, &reg.Status, &reg.RegisteredBy,
			&reg.CreatedAt, &reg.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan registration: %w", err)
		}
		_ = json.Unmarshal([]byte(manifestText), &reg.ManifestJSON)
		items = append(items, reg)
	}
	return items, nil
}