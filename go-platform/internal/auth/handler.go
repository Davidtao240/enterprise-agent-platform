package auth

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/enterprise-agent-platform/go-platform/internal/audit"
	"github.com/enterprise-agent-platform/go-platform/internal/platform"
	"github.com/enterprise-agent-platform/go-platform/pkg/apierror"
	"github.com/gin-gonic/gin"
)

// Handler 处理 auth 相关的 HTTP 请求。
// 每个方法对应一个 API 端点，负责：
//  1. 解析请求参数
//  2. 调用 Service 层的业务逻辑
//  3. 将业务层错误映射为 HTTP 状态码
//  4. 通过 platform.Success / platform.APIError 返回标准 JSON 响应
type Handler struct {
	svc      *Service
	auditLog securityAuditLogger
}

// NewHandler 创建 Handler 实例。
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

type securityAuditLogger interface {
	InsertLog(ctx context.Context, entry audit.AuditLogEntry) (string, time.Time, error)
}

func (h *Handler) SetAuditLogger(repo securityAuditLogger) {
	h.auditLog = repo
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
		h.auditAuth(c, "", nil, "", "auth_login_failed", "failed", "validation_failed")
		platform.APIError(c, apierror.ErrValidationFailed)
		return
	}

	if req.Username == "" || len(req.Username) > 255 {
		h.auditAuth(c, "", nil, "", "auth_login_failed", "failed", "validation_failed")
		platform.APIError(c, apierror.ErrValidationFailed)
		return
	}
	if req.Password == "" || len(req.Password) > 1000 {
		h.auditAuth(c, "", nil, "", "auth_login_failed", "failed", "validation_failed")
		platform.APIError(c, apierror.ErrValidationFailed)
		return
	}

	resp, err := h.svc.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			h.auditAuth(c, "", nil, req.Username, "auth_login_failed", "failed", "invalid_credentials")
			platform.APIError(c, apierror.ErrInvalidCredentials)
			return
		}
		if errors.Is(err, ErrUserDisabled) {
			h.auditAuth(c, "", nil, req.Username, "auth_login_failed", "failed", "user_disabled")
			platform.APIError(c, apierror.ErrUserDisabled)
			return
		}
		h.auditAuth(c, "", nil, req.Username, "auth_login_failed", "failed", "internal_error")
		platform.APIError(c, apierror.ErrInternalError)
		return
	}

	h.auditAuth(c, resp.User.TenantID, &resp.User.ID, resp.User.ID, "auth_login_succeeded", "succeeded", "")
	platform.Success(c, resp)
}

func (h *Handler) auditAuth(c *gin.Context, tenantID string, actorUserID *string, resourceID, action, status, reason string) {
	if h.auditLog == nil {
		return
	}
	detailData := map[string]string{
		"method": c.Request.Method,
		"path":   c.FullPath(),
	}
	if reason != "" {
		detailData["reason"] = reason
	}
	detailBytes, err := json.Marshal(detailData)
	if err != nil {
		log.Printf("[auth] failed to marshal audit detail: %v", err)
		return
	}
	detail := string(detailBytes)
	_, _, err = h.auditLog.InsertLog(c.Request.Context(), audit.AuditLogEntry{
		TraceID:      c.GetHeader(platform.TraceIDHeader),
		TenantID:     tenantID,
		ActorUserID:  actorUserID,
		Action:       action,
		ResourceType: "auth",
		ResourceID:   resourceID,
		Status:       status,
		DetailJSON:   &detail,
	})
	if err != nil {
		log.Printf("[auth] failed to write audit log: %v", err)
	}
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

func (h *Handler) ListPermissionMatrix(c *gin.Context) {
	items, err := h.svc.repo.ListPermissionMatrix(c.Request.Context())
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	platform.Success(c, items)
}

func (h *Handler) ListUserRoles(c *gin.Context) {
	items, err := h.svc.repo.ListUserRoles(c.Request.Context())
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	platform.Success(c, items)
}
