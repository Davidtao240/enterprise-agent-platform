package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// Service 实现 auth 业务逻辑。
// 位于 Repository（数据层）和 Handler（HTTP 层）之间，
// 负责密码验证、JWT 签发/校验、用户信息组装。
type Service struct {
	repo      *Repository
	jwtSecret []byte        // JWT 签名密钥（字节形式，HMAC-SHA256）
	jwtExpiry time.Duration // Token 有效期
}

// NewService 创建 Service 实例。
// jwtSecret: 从 config.JWTSecret 传入
// jwtExpirationHours: 从 config.JWTExpirationHours 传入，转为 Duration
func NewService(repo *Repository, jwtSecret string, jwtExpirationHours int) *Service {
	return &Service{
		repo:      repo,
		jwtSecret: []byte(jwtSecret),
		jwtExpiry: time.Duration(jwtExpirationHours) * time.Hour,
	}
}

// ── 预定义错误 ──

var (
	// ErrInvalidCredentials 用户名或密码不匹配。
	// handler 中将其映射为 HTTP 401。
	ErrInvalidCredentials = errors.New("invalid username or password")

	// ErrUserDisabled 用户账号已被禁用。
	// handler 中将其映射为 HTTP 403。
	ErrUserDisabled = errors.New("user account is disabled")

	// ErrUserNotFound 用户不存在（用于 token 有效但用户已被删除的边界情况）。
	ErrUserNotFound = errors.New("user not found")
)

// Login 验证用户名密码 → 生成 JWT → 更新最后登录时间。
//
// 逻辑：
//  1. 查用户（用户名不存在 → 返回 ErrInvalidCredentials）
//  2. 检查账号状态（disabled → 返回 ErrUserDisabled）
//  3. bcrypt 验密（密码错误 → 返回 ErrInvalidCredentials）
//  4. 生成 JWT（sub=用户ID, username=用户名, exp=过期时间）
//  5. 更新 last_login_at
//  6. 返回 LoginResponse
//
// 注意：第 1 步和第 3 步返回相同的错误信息，
// 避免攻击者通过错误信息差异枚举有效用户名。
func (s *Service) Login(ctx context.Context, username, password string) (*LoginResponse, error) {
	// 1. 查用户
	user, err := s.repo.FindUserByUsername(ctx, username)
	if err != nil {
		return nil, ErrInvalidCredentials // 用户名不存在，不泄露具体原因
	}

	// 2. 检查账号状态
	if user.Status != "active" {
		return nil, ErrUserDisabled
	}

	// 3. bcrypt 验密：将明文密码与数据库中存储的哈希比对
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials // 密码错误，同样不泄露具体原因
	}

	// 4. 生成 JWT
	now := time.Now()
	expiresAt := now.Add(s.jwtExpiry)
	claims := jwt.MapClaims{
		"sub":      user.ID,          // JWT 标准字段：subject = 用户ID
		"username": user.Username,    // 自定义字段：方便中间件直接读取
		"iat":      now.Unix(),       // issued at = 签发时间
		"exp":      expiresAt.Unix(), // expiration = 过期时间
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return nil, fmt.Errorf("sign token: %w", err)
	}

	// 5. 更新最后登录时间（非关键路径，忽略错误）
	_ = s.repo.UpdateLastLogin(ctx, user.ID)

	// 6. 返回响应
	return &LoginResponse{
		AccessToken: signed,
		TokenType:   "Bearer",
		ExpiresIn:   int64(s.jwtExpiry.Seconds()),
		User: UserInfo{
			ID:          user.ID,
			Username:    user.Username,
			DisplayName: user.DisplayName,
		},
	}, nil
}

// GetMe 获取当前登录用户的完整信息（用户 + 角色 + 权限）。
//
// 调用时机：前端每次刷新页面或首次登录后调用 GET /api/v1/auth/me。
// 前端可将返回的 permissions 列表用于前端按钮级别的权限控制。
func (s *Service) GetMe(ctx context.Context, userID string) (*MeResponse, error) {
	// 查用户基本信息
	user, err := s.repo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	// 查用户拥有的角色
	roles, err := s.repo.FindRolesByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("find roles: %w", err)
	}

	// 查用户拥有的权限（通过角色间接获得）
	perms, err := s.repo.FindPermissionsByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("find permissions: %w", err)
	}

	// 组装角色列表
	roleInfos := make([]RoleInfo, len(roles))
	for i, r := range roles {
		roleInfos[i] = RoleInfo{Code: r.Code, Name: r.Name}
	}

	return &MeResponse{
		User: UserInfo{
			ID:          user.ID,
			Username:    user.Username,
			DisplayName: user.DisplayName,
		},
		Roles:       roleInfos,
		Permissions: perms,
	}, nil
}

// HasPermission 判断用户是否拥有指定权限码。
func (s *Service) HasPermission(ctx context.Context, userID, permission string) (bool, error) {
	perms, err := s.repo.FindPermissionsByUserID(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("find permissions: %w", err)
	}
	for _, p := range perms {
		if p == permission {
			return true, nil
		}
	}
	return false, nil
}

// ValidateToken 解析并验证 JWT，返回 token 中的用户 ID 和用户名。
//
// 由 AuthMiddleware 调用：
//  1. 从 Authorization 头提取 "Bearer <token>"
//  2. 调用此函数验证签名和有效期
//  3. 验证通过 → 将 userID/username 注入 gin.Context
//  4. 验证失败 → 返回 401
//
// 安全检查：
//   - 只接受 HMAC 签名算法，拒绝 "none" 等不安全的算法
//   - 自动验证 exp（过期）和 iat（签发时间）
func (s *Service) ValidateToken(tokenStr string) (userID string, username string, err error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		// 强制校验签名算法，防止 JWT "none algorithm" 攻击
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return "", "", fmt.Errorf("parse token: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return "", "", fmt.Errorf("invalid token claims")
	}

	userID, _ = claims["sub"].(string)
	username, _ = claims["username"].(string)
	return userID, username, nil
}

// GetBusinessApps 获取所有可用的业务入口。
// Phase 1 简单透传 Repository 查询结果，供前端 Dashboard 展示。
func (s *Service) GetBusinessApps(ctx context.Context) ([]BusinessApp, error) {
	return s.repo.FindBusinessApps(ctx)
}
