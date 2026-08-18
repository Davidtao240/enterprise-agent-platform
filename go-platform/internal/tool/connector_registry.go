package tool

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── M3-A: connector_registry 仓储 ──
//
// Connector 注册与版本治理:(connector_code, version) 唯一,版本不可变。
// 运行时只信任注册表中的 release_stage/status,实现替换供应商不改核心。

// ErrConnectorNotFound 注册表中不存在指定的 Connector 版本。
var ErrConnectorNotFound = errors.New("connector not found in registry")

// ErrConnectorDeprecated 指定版本已弃用(存量 Binding 可继续,新建被拒)。
var ErrConnectorDeprecated = errors.New("connector version deprecated")

// RegistryEntry 对应 connector_registry 表。
type RegistryEntry struct {
	ID              string     `json:"id"`
	ConnectorCode   string     `json:"connector_code"`
	Version         string     `json:"version"`
	ConnectorType   string     `json:"connector_type"`
	CapabilitiesJSON string    `json:"capabilities_json"`
	AuthType        string     `json:"auth_type"`
	HealthCheckJSON *string    `json:"health_check_json,omitempty"`
	ReleaseStage    string     `json:"release_stage"`
	Status          string     `json:"status"` // draft / active / deprecated
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// ConnectorRegistryRepository 注册表访问。
type ConnectorRegistryRepository struct {
	pool *pgxpool.Pool
}

// NewConnectorRegistryRepository 创建注册表仓储。
func NewConnectorRegistryRepository(pool *pgxpool.Pool) *ConnectorRegistryRepository {
	return &ConnectorRegistryRepository{pool: pool}
}

const registrySelect = `SELECT id, connector_code, version, connector_type,
	capabilities_json::text, auth_type, health_check_json::text,
	release_stage, status, created_at, updated_at
	FROM connector_registry`

// Register 注册 Connector 版本(幂等:同 code+version 已存在时返回现有记录,
// 不修改 —— 版本不可变是契约的一部分)。
func (r *ConnectorRegistryRepository) Register(ctx context.Context, e *RegistryEntry) (*RegistryEntry, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO connector_registry
		 (connector_code, version, connector_type, capabilities_json, auth_type, health_check_json, release_stage, status)
		 VALUES ($1,$2,$3,$4::jsonb,$5,$6::jsonb,$7,$8)
		 ON CONFLICT (connector_code, version) DO NOTHING
		 RETURNING id, connector_code, version, connector_type,
		           capabilities_json::text, auth_type, health_check_json::text,
		           release_stage, status, created_at, updated_at`,
		e.ConnectorCode, e.Version, e.ConnectorType, e.CapabilitiesJSON, e.AuthType, e.HealthCheckJSON, e.ReleaseStage, e.Status,
	)
	entry, err := scanRegistryEntry(row)
	if errors.Is(err, pgx.ErrNoRows) {
		// 冲突:返回已注册的版本(幂等)
		return r.FindByCodeAndVersion(ctx, e.ConnectorCode, e.Version)
	}
	if err != nil {
		return nil, err
	}
	return entry, nil
}

// FindByCodeAndVersion 精确查找(code + version)。
func (r *ConnectorRegistryRepository) FindByCodeAndVersion(ctx context.Context, code, version string) (*RegistryEntry, error) {
	row := r.pool.QueryRow(ctx, registrySelect+` WHERE connector_code = $1 AND version = $2`, code, version)
	entry, err := scanRegistryEntry(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrConnectorNotFound
	}
	if err != nil {
		return nil, err
	}
	return entry, nil
}

// FindActiveByCode 按最新版本号取 active 版本(Binding 未固定版本时使用)。
func (r *ConnectorRegistryRepository) FindActiveByCode(ctx context.Context, code string) (*RegistryEntry, error) {
	row := r.pool.QueryRow(ctx, registrySelect+
		` WHERE connector_code = $1 AND status = 'active'
		  ORDER BY version DESC LIMIT 1`, code)
	entry, err := scanRegistryEntry(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrConnectorNotFound
	}
	if err != nil {
		return nil, err
	}
	return entry, nil
}

// List 全量注册表(管理端展示)。
func (r *ConnectorRegistryRepository) List(ctx context.Context) ([]*RegistryEntry, error) {
	rows, err := r.pool.Query(ctx, registrySelect+` ORDER BY connector_code, version DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*RegistryEntry
	for rows.Next() {
		e := &RegistryEntry{}
		if err := rows.Scan(&e.ID, &e.ConnectorCode, &e.Version, &e.ConnectorType,
			&e.CapabilitiesJSON, &e.AuthType, &e.HealthCheckJSON,
			&e.ReleaseStage, &e.Status, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

func scanRegistryEntry(row pgx.Row) (*RegistryEntry, error) {
	e := &RegistryEntry{}
	err := row.Scan(&e.ID, &e.ConnectorCode, &e.Version, &e.ConnectorType,
		&e.CapabilitiesJSON, &e.AuthType, &e.HealthCheckJSON,
		&e.ReleaseStage, &e.Status, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return e, nil
}
