package auth

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/enterprise-agent-platform/go-platform/internal/platform"
	"github.com/enterprise-agent-platform/go-platform/pkg/apierror"
)

// Handler 处理 auth 相关的 HTTP 请求。
// 每个方法对应一个 API 端点，负责：
//  1. 解析请求参数
//  2. 调用 Service 层的业务逻辑
//  3. 将业务层错误映射为 HTTP 状态码
//  4. 通过 platform.Success / platform.APIError 返回标准 JSON 响应
type Handler struct {
	svc *Service
}

// NewHandler 创建 Handler 实例。
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Login 处理 POST /api/v1/auth/login。
//
// 请求：{ "username": "...", "password": "..." }
// 成功 → 200 + { access_token, token_type, expires_in, user }
// 用户名/密码错误 → 401 UNAUTHORIZED
// 账号已禁用 → 403 FORBIDDEN
// 请求格式错误 → 400 VALIDATION_FAILED
func (h *Handler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		platform.APIError(c, apierror.ErrValidationFailed)
		return
	}

	resp, err := h.svc.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			// 自定义 401 响应，message 比 apierror.ErrUnauthorized 更具体
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"trace_id": c.GetHeader("X-Trace-Id"),
				"error":    gin.H{"code": "UNAUTHORIZED", "message": "Invalid username or password"},
			})
			return
		}
		if errors.Is(err, ErrUserDisabled) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"trace_id": c.GetHeader("X-Trace-Id"),
				"error":    gin.H{"code": "FORBIDDEN", "message": "User account is disabled"},
			})
			return
		}
		platform.APIError(c, apierror.ErrInternalError)
		return
	}

	platform.Success(c, resp)
}

// Me 处理 GET /api/v1/auth/me。
//
// 需要 JWT 鉴权（由 AuthMiddleware 前置处理）。
// 从 gin.Context 中读取中间件注入的 user_id，
// 查询用户完整信息（基本信息 + 角色 + 权限）并返回。
func (h *Handler) Me(c *gin.Context) {
	// user_id 由 AuthMiddleware 在鉴权成功后注入
	userID := c.GetString("user_id")
	if userID == "" {
		platform.APIError(c, apierror.ErrUnauthorized)
		return
	}

	resp, err := h.svc.GetMe(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			platform.APIError(c, apierror.ErrUnauthorized)
			return
		}
		platform.APIError(c, apierror.ErrInternalError)
		return
	}

	platform.Success(c, resp)
}

// GetBusinessApps 处理 GET /api/v1/business-apps。
//
// 需要 JWT 鉴权。
// 返回所有 active 状态的业务入口列表。
// V1 只返回 finance，但数据模型和 API 设计已预留多业务扩展。
func (h *Handler) GetBusinessApps(c *gin.Context) {
	apps, err := h.svc.GetBusinessApps(c.Request.Context())
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	platform.Success(c, apps)
}
