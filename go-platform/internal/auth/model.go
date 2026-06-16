// Package auth 实现用户认证、JWT 鉴权和 RBAC 权限模型。
//
// 模块分工：
//   - model.go    → 请求/响应结构体 + 数据库模型
//   - repository.go → 数据库 CRUD（查用户、角色、权限）
//   - service.go   → 业务逻辑（密码验证、JWT 签发、token 校验）
//   - handler.go   → HTTP 处理器（login、me、business-apps）
//   - middleware.go → JWT 鉴权中间件（从请求头提取并验证 token）
//
// 鉴权流程：
//  1. POST /api/v1/auth/login → handler.Login → service.Login → repo 查用户 → bcrypt 验密 → 签发 JWT
//  2. 后续请求携带 Authorization: Bearer <token>
//  3. AuthMiddleware 解析 token → 将 user_id/username 注入 gin.Context
//  4. 各 handler 通过 c.GetString("user_id") 获取当前用户
package auth

import "time"

// ── 请求 / 响应体 ──

// LoginRequest 登录请求体。
// 前端 LoginPage 提交 { username, password }。
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse 登录成功响应，返回 JWT 和用户基本信息。
// 前端 auth store 将 token 存入 localStorage，后续请求自动带在 Authorization 头。
type LoginResponse struct {
	AccessToken string   `json:"access_token"` // JWT 令牌字符串
	TokenType   string   `json:"token_type"`   // 固定 "Bearer"
	ExpiresIn   int64    `json:"expires_in"`   // 有效秒数（默认 86400 = 24h）
	User        UserInfo `json:"user"`         // 用户基本信息
}

// UserInfo 对外暴露的用户身份信息（不含敏感字段）。
type UserInfo struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}

// MeResponse 当前用户完整信息，包含角色列表和权限编码列表。
// GET /api/v1/auth/me 的响应体。
type MeResponse struct {
	User        UserInfo   `json:"user"`
	Roles       []RoleInfo `json:"roles"`       // 用户拥有的角色
	Permissions []string   `json:"permissions"` // 用户拥有的权限编码，如 ["workflow:create", "file:upload"]
}

// RoleInfo 角色简要信息。
type RoleInfo struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// ── 数据库模型（对应 PostgreSQL 表） ──

// User 对应 users 表的一行。
// 包含密码哈希等敏感字段，仅内部使用，不暴露给前端。
type User struct {
	ID           string     // UUID
	Username     string     // 登录名，唯一
	DisplayName  string     // 显示名称
	PasswordHash string     // bcrypt 哈希
	DepartmentID *string    // 所属部门 UUID，可为空
	Status       string     // active / disabled
	LastLoginAt  *time.Time // 最近登录时间
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Role 对应 roles 表的一行。
type Role struct {
	ID   string
	Code string // 角色编码，如 "business_user"
	Name string // 角色名称，如 "Business User"
}
