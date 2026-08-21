package auth

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/enterprise-agent-platform/go-platform/internal/audit"
	"github.com/enterprise-agent-platform/go-platform/internal/platform"
	"github.com/enterprise-agent-platform/go-platform/pkg/apierror"
	"github.com/gin-gonic/gin"
)

type PermissionChecker interface {
	HasPermission(ctx context.Context, userID, permission string) (bool, error)
}

type MiddlewareAuditLogger interface {
	InsertLog(ctx context.Context, entry audit.AuditLogEntry) (string, time.Time, error)
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
	return AuthMiddlewareWithAudit(svc, nil)
}

func AuthMiddlewareWithAudit(svc *Service, auditLog MiddlewareAuditLogger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 提取 Authorization 头
		auth := c.GetHeader("Authorization")
		if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
			auditSecurityEvent(c, auditLog, nil, "auth_unauthorized", "denied", "missing_bearer_credential", "")
			platform.APIError(c, apierror.ErrUnauthorized)
			return
		}

		// 2. 去掉 "Bearer " 前缀，拿到纯 token
		token := strings.TrimPrefix(auth, "Bearer ")

		// 3. 验证 token 签名和有效期
		userID, username, tenantID, err := svc.ValidateToken(token)
		if err != nil {
			auditSecurityEvent(c, auditLog, nil, "auth_unauthorized", "denied", "invalid_bearer_credential", "")
			platform.APIError(c, apierror.ErrUnauthorized)
			return
		}

		// 4. 注入用户信息到上下文，后续 handler 通过 c.GetString 读取
		c.Set("user_id", userID)
		c.Set("username", username)
		c.Set("tenant_id", tenantID)

		// 5. 继续执行后续中间件和 handler
		c.Next()
	}
}

// RequirePermission 校验当前用户是否拥有指定权限码。
func RequirePermission(checker PermissionChecker, permission string) gin.HandlerFunc {
	return RequirePermissionWithAudit(checker, permission, nil)
}

func RequirePermissionWithAudit(checker PermissionChecker, permission string, auditLog MiddlewareAuditLogger) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")
		if userID == "" {
			auditSecurityEvent(c, auditLog, nil, "auth_unauthorized", "denied", "missing_user_context", permission)
			platform.APIError(c, apierror.ErrUnauthorized)
			return
		}
		ok, err := checker.HasPermission(c.Request.Context(), userID, permission)
		if err != nil {
			platform.APIError(c, apierror.ErrInternalError)
			return
		}
		if !ok {
			auditSecurityEvent(c, auditLog, &userID, "permission_denied", "denied", "missing_permission", permission)
			platform.APIError(c, apierror.ErrForbidden)
			return
		}
		c.Next()
	}
}

func auditSecurityEvent(c *gin.Context, auditLog MiddlewareAuditLogger, actorUserID *string, action, status, reason, permission string) {
	if auditLog == nil {
		return
	}
	path := c.FullPath()
	if path == "" {
		path = c.Request.URL.Path
	}
	detailData := map[string]string{
		"method": c.Request.Method,
		"path":   path,
		"reason": reason,
	}
	if permission != "" {
		detailData["permission"] = permission
	}
	detailBytes, err := json.Marshal(detailData)
	if err != nil {
		log.Printf("[auth] failed to marshal audit detail: %v", err)
		return
	}
	detail := string(detailBytes)
	_, _, err = auditLog.InsertLog(c.Request.Context(), audit.AuditLogEntry{
		TraceID:      c.GetHeader(platform.TraceIDHeader),
		TenantID:     c.GetString("tenant_id"),
		ActorUserID:  actorUserID,
		Action:       action,
		ResourceType: "http_request",
		ResourceID:   path,
		Status:       status,
		DetailJSON:   &detail,
	})
	if err != nil {
		log.Printf("[auth] failed to write audit log: %v", err)
	}
}
