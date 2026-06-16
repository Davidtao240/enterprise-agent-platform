package auth

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository 封装所有 auth 相关的数据库查询。
// 通过 pgxpool 直接执行 SQL，不引入 ORM，保持查询透明可控。
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository 创建 Repository 实例。
// pool 由 main.go 在启动时创建并注入。
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// FindUserByUsername 根据登录名查询用户。
// 仅返回未软删除（deleted_at IS NULL）的用户。
// 用于登录时查找用户 → 验证密码。
func (r *Repository) FindUserByUsername(ctx context.Context, username string) (*User, error) {
	u := &User{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, username, display_name, password_hash, department_id, status, last_login_at, created_at, updated_at
		 FROM users WHERE username = $1 AND deleted_at IS NULL`, username,
	).Scan(&u.ID, &u.Username, &u.DisplayName, &u.PasswordHash, &u.DepartmentID, &u.Status, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

// FindUserByID 根据 UUID 查询用户。
// 用于 GET /me 时从 token 中的 user_id 反查用户信息。
func (r *Repository) FindUserByID(ctx context.Context, id string) (*User, error) {
	u := &User{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, username, display_name, password_hash, department_id, status, last_login_at, created_at, updated_at
		 FROM users WHERE id = $1 AND deleted_at IS NULL`, id,
	).Scan(&u.ID, &u.Username, &u.DisplayName, &u.PasswordHash, &u.DepartmentID, &u.Status, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

// FindRolesByUserID 查询用户拥有的角色列表。
// 通过 user_roles 关联表 JOIN roles 表获取。
func (r *Repository) FindRolesByUserID(ctx context.Context, userID string) ([]Role, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT r.id, r.code, r.name FROM roles r
		 JOIN user_roles ur ON ur.role_id = r.id
		 WHERE ur.user_id = $1 AND r.status = 'active' AND r.deleted_at IS NULL`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []Role
	for rows.Next() {
		var role Role
		if err := rows.Scan(&role.ID, &role.Code, &role.Name); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, nil
}

// FindPermissionsByUserID 查询用户拥有的所有权限编码（去重）。
//
// SQL 路径：users → user_roles → role_permissions → permissions
// 一个人可能有多个角色，每个角色有多个权限。
// 使用 DISTINCT 去重，因为多个角色可能共享同一权限。
//
// 返回的是权限编码列表，如 ["workflow:create", "file:upload"]，
// 后端中间件可根据此列表做权限校验。
func (r *Repository) FindPermissionsByUserID(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT DISTINCT p.code FROM permissions p
		 JOIN role_permissions rp ON rp.permission_id = p.id
		 JOIN user_roles ur ON ur.role_id = rp.role_id
		 WHERE ur.user_id = $1 AND p.deleted_at IS NULL`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var perms []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		perms = append(perms, code)
	}
	return perms, nil
}

// UpdateLastLogin 更新用户的最后登录时间。
// 每次登录成功时调用，用于安全审计。
func (r *Repository) UpdateLastLogin(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET last_login_at = $1, updated_at = $1 WHERE id = $2`,
		time.Now(), userID)
	return err
}

// ── Business App 查询（Phase 1 顺带实现，前端 Dashboard 需要） ──

// BusinessApp 对应 business_apps 表，表示一个业务入口。
type BusinessApp struct {
	Code        string `json:"code"`        // 业务编码，如 "finance"
	Name        string `json:"name"`        // 显示名称，如 "Finance Center"
	Description string `json:"description"` // 简介
	Icon        string `json:"icon"`        // 前端 Ant Design 图标名
	SortOrder   int    `json:"sort_order"`  // 显示顺序
	Status      string `json:"status"`      // active / disabled
}

// FindBusinessApps 查询所有 active 状态的业务入口，按 sort_order 排序。
// V1 只有 finance，但数据模型已支持多业务扩展。
func (r *Repository) FindBusinessApps(ctx context.Context) ([]BusinessApp, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT code, name, description, icon, sort_order, status FROM business_apps
		 WHERE status = 'active' AND deleted_at IS NULL ORDER BY sort_order`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var apps []BusinessApp
	for rows.Next() {
		var a BusinessApp
		if err := rows.Scan(&a.Code, &a.Name, &a.Description, &a.Icon, &a.SortOrder, &a.Status); err != nil {
			return nil, err
		}
		apps = append(apps, a)
	}
	return apps, nil
}
