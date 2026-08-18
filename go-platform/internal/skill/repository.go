package skill

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrSkillNotFound Skill 不存在。
var ErrSkillNotFound = errors.New("skill not found")

// ErrDuplicateVersion (skill_code, version) 已存在。
var ErrDuplicateVersion = errors.New("skill code+version already exists")

// isUniqueViolation 判定 PostgreSQL 唯一约束冲突。
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

const skillSelect = `SELECT id, skill_code, version, status, config_json,
	created_by, reviewed_by, published_at, created_at, updated_at
	FROM skill_registry`

// Repository 封装 skill_registry 表访问。
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository 创建 Repository。
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Create 创建 draft 草稿。
func (r *Repository) Create(ctx context.Context, req *CreateRequest) (*Skill, error) {
	id := uuid.NewString()
	now := time.Now().UTC()
	row := r.pool.QueryRow(ctx, `
		INSERT INTO skill_registry (id, skill_code, version, status, config_json, created_by, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$7)
		RETURNING id, skill_code, version, status, config_json,
			created_by, reviewed_by, published_at, created_at, updated_at`,
		id, req.SkillCode, req.Version, StatusDraft, req.ConfigJSON, req.CreatedBy, now,
	)
	s, err := scanSkill(row)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrDuplicateVersion
		}
		return nil, err
	}
	return s, nil
}

// GetByID 按 ID 查找。
func (r *Repository) GetByID(ctx context.Context, id string) (*Skill, error) {
	row := r.pool.QueryRow(ctx, skillSelect+" WHERE id = $1", id)
	return scanSkill(row)
}

// List 按 skill_code 列出全部版本(可空 = 全部)。
func (r *Repository) List(ctx context.Context, skillCode string) ([]*Skill, error) {
	q := skillSelect
	args := []any{}
	if skillCode != "" {
		q += " WHERE skill_code = $1"
		args = append(args, skillCode)
	}
	q += " ORDER BY skill_code, created_at DESC"
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Skill
	for rows.Next() {
		s, err := scanSkill(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// TransitionGuarded 守卫式状态流转:仅当当前状态为 from 时更新为 to,
// rows=0 表示不存在或状态不符(ErrInvalidTransition/ErrSkillNotFound)。
// to=published 时落 reviewed_by + published_at。
func (r *Repository) TransitionGuarded(ctx context.Context, id string, from, to Status, reviewer string) (*Skill, error) {
	now := time.Now().UTC()
	var (
		s    Skill
		cfg  []byte
		rows pgx.Row
	)
	if to == StatusPublished {
		rows = r.pool.QueryRow(ctx, `
			UPDATE skill_registry
			SET status = $2, reviewed_by = $3, published_at = $4, updated_at = $4
			WHERE id = $1 AND status = $5
			RETURNING id, skill_code, version, status, config_json,
				created_by, reviewed_by, published_at, created_at, updated_at`,
			id, to, reviewer, now, from)
	} else {
		rows = r.pool.QueryRow(ctx, `
			UPDATE skill_registry
			SET status = $2, updated_at = $3
			WHERE id = $1 AND status = $4
			RETURNING id, skill_code, version, status, config_json,
				created_by, reviewed_by, published_at, created_at, updated_at`,
			id, to, now, from)
	}
	if err := rows.Scan(&s.ID, &s.SkillCode, &s.Version, &s.Status, &cfg,
		&s.CreatedBy, &s.ReviewedBy, &s.PublishedAt, &s.CreatedAt, &s.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// 区分不存在与状态不符
			if _, gerr := r.GetByID(ctx, id); gerr != nil {
				return nil, ErrSkillNotFound
			}
			return nil, ErrInvalidTransition
		}
		return nil, err
	}
	s.ConfigJSON = append(json.RawMessage(nil), cfg...)
	return &s, nil
}

// UpdateConfigIfDraft 仅 draft 状态允许修改配置。
func (r *Repository) UpdateConfigIfDraft(ctx context.Context, id string, cfg json.RawMessage) (*Skill, error) {
	row := r.pool.QueryRow(ctx, `
		UPDATE skill_registry SET config_json = $2, updated_at = now()
		WHERE id = $1 AND status = $3
		RETURNING id, skill_code, version, status, config_json,
			created_by, reviewed_by, published_at, created_at, updated_at`,
		id, cfg, StatusDraft)
	s, err := scanSkill(row)
	if err != nil {
		if errors.Is(err, ErrSkillNotFound) {
			if _, gerr := r.GetByID(ctx, id); gerr != nil {
				return nil, ErrSkillNotFound
			}
			return nil, ErrInvalidTransition
		}
		return nil, err
	}
	return s, nil
}

func scanSkill(row pgx.Row) (*Skill, error) {
	var (
		s   Skill
		cfg []byte
	)
	if err := row.Scan(&s.ID, &s.SkillCode, &s.Version, &s.Status, &cfg,
		&s.CreatedBy, &s.ReviewedBy, &s.PublishedAt, &s.CreatedAt, &s.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSkillNotFound
		}
		return nil, err
	}
	s.ConfigJSON = append(json.RawMessage(nil), cfg...)
	return &s, nil
}
