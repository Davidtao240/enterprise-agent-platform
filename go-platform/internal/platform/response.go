// Package platform 提供整个 Go 后端的共享工具。
//
// response.go 定义统一的 HTTP JSON 响应格式，
// 对应 API_CONTRACT.md 中规定的三种标准响应体：
//   - 成功（data 字段）
//   - 列表（data + pagination 字段）
//   - 错误（error 字段，含 code + message）
//
// 所有 handler 通过本包的 Success/List/APIError 函数构造响应，
// 保证前后端契约一致。
package platform

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/enterprise-agent-platform/go-platform/pkg/apierror"
)

// ── 标准响应体结构 ──

// successBody 对应 API 契约的 Common Success Response：
//
//	{ "trace_id": "...", "data": {...} }
type successBody struct {
	TraceID string `json:"trace_id"`
	Data    any    `json:"data"`
}

// listBody 对应 API 契约的 Common List Response：
//
//	{ "trace_id": "...", "data": [...], "pagination": {...} }
type listBody struct {
	TraceID    string `json:"trace_id"`
	Data       any    `json:"data"`
	Pagination any    `json:"pagination"`
}

// errorBody 对应 API 契约的 Common Error Response：
//
//	{ "trace_id": "...", "error": {"code": "...", "message": "..."} }
type errorBody struct {
	TraceID string `json:"trace_id"`
	Error   any    `json:"error"`
}

// Success 返回 200 + 标准成功响应体。
// data 可以是单个对象或列表（无需分页时使用）。
func Success(c *gin.Context, data any) {
	c.JSON(http.StatusOK, successBody{
		TraceID: getTraceID(c),
		Data:    data,
	})
}

// List 返回 200 + 带分页信息的列表响应体。
// 用于前端需要分页的数据接口（如审计日志、任务列表）。
func List(c *gin.Context, data any, page, pageSize, total int) {
	c.JSON(http.StatusOK, listBody{
		TraceID: getTraceID(c),
		Data:    data,
		Pagination: gin.H{
			"page":      page,
			"page_size": pageSize,
			"total":     total,
		},
	})
}

// APIError 返回错误响应（如 400/401/403/404/500）。
// 接收 pkg/apierror 中预定义的错误码对象，
// 自动填充 HTTP 状态码和错误体。
func APIError(c *gin.Context, e *apierror.APIError) {
	c.AbortWithStatusJSON(e.Status, errorBody{
		TraceID: getTraceID(c),
		Error: gin.H{
			"code":    e.Code,
			"message": e.Message,
		},
	})
}

// APIErrorWithMessage 返回错误响应，但使用自定义消息覆盖预定义错误的 Message 字段。
// 适用于需要根据业务上下文返回更具体错误信息的场景。
func APIErrorWithMessage(c *gin.Context, e *apierror.APIError, message string) {
	c.AbortWithStatusJSON(e.Status, errorBody{
		TraceID: getTraceID(c),
		Error: gin.H{
			"code":    e.Code,
			"message": message,
		},
	})
}

// getTraceID 从请求中提取 trace_id，用于分布式追踪。
// 优先从 X-Trace-Id 请求头获取，其次从 gin.Context 中读取（由中间件设置）。
func getTraceID(c *gin.Context) string {
	tid := c.GetHeader("X-Trace-Id")
	if tid != "" {
		return tid
	}
	tid = c.GetString("trace_id")
	if tid != "" {
		return tid
	}
	return ""
}
