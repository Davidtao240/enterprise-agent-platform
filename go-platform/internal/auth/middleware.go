package auth

import (
	"context"
	"strings"

	"github.com/enterprise-agent-platform/go-platform/internal/platform"
	"github.com/enterprise-agent-platform/go-platform/pkg/apierror"
	"github.com/gin-gonic/gin"
)

type PermissionChecker interface {
	HasPermission(ctx context.Context, userID, permission string) (bool, error)
}

// AuthMiddleware 是 Gin 中间件，拦截所有需要鉴权的路由。
//
// 工作流程：
//  1. 从请求头获取 Authorization 字段
//  2. 检查是否以 "Bearer " 开头（标准 JWT 格式）
//  3. 提取 token → 调用 Service.ValidateToken 验证
//  4. 验证通过 → 将 user_id 和 username 注入 gin.Context
//  5. 验证失败 → 直接返回 401，不进入后续 handler
//
// 使用方式（在 main.go 中）：
//
//	protected := v1.Group("")
//	protected.Use(AuthMiddleware(svc))
//	{
//	    protected.GET("/auth/me", authHandler.Me)
//	}
//
// 之后所有 handler 都可以通过 c.GetString("user_id") 获取当前用户。
func AuthMiddleware(svc *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 提取 Authorization 头
		auth := c.GetHeader("Authorization")
		if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
			platform.APIError(c, apierror.ErrUnauthorized)
			return
		}

		// 2. 去掉 "Bearer " 前缀，拿到纯 token
		token := strings.TrimPrefix(auth, "Bearer ")

		// 3. 验证 token 签名和有效期
		userID, username, err := svc.ValidateToken(token)
		if err != nil {
			platform.APIError(c, apierror.ErrUnauthorized)
			return
		}

		// 4. 注入用户信息到上下文，后续 handler 通过 c.GetString 读取
		c.Set("user_id", userID)
		c.Set("username", username)

		// 5. 继续执行后续中间件和 handler
		c.Next()
	}
}

// RequirePermission 校验当前用户是否拥有指定权限码。
func RequirePermission(checker PermissionChecker, permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")
		if userID == "" {
			platform.APIError(c, apierror.ErrUnauthorized)
			return
		}
		ok, err := checker.HasPermission(c.Request.Context(), userID, permission)
		if err != nil {
			platform.APIError(c, apierror.ErrInternalError)
			return
		}
		if !ok {
			platform.APIError(c, apierror.ErrForbidden)
			return
		}
		c.Next()
	}
}
