package audit

import "time"

// AuditLog 对应 audit_logs 表，记录所有关键业务操作。
type AuditLog struct {
	ID               string    `json:"id"`
	TraceID          string    `json:"trace_id"`
	ActorUserID      *string   `json:"actor_user_id"`
	BusinessAppCode  *string   `json:"business_app_code"`
	Action           string    `json:"action"`
	ResourceType     string    `json:"resource_type"`
	ResourceID       string    `json:"resource_id"`
	Status           string    `json:"status"`
	DetailJSON       *string   `json:"detail_json"`
	IPAddress        *string   `json:"ip_address"`
	UserAgent        *string   `json:"user_agent"`
	CreatedAt        time.Time `json:"created_at"`
}

// AuditLogEntry 写审计日志时的输入参数（不含自动生成字段）。
type AuditLogEntry struct {
	TraceID         string
	ActorUserID     *string
	BusinessAppCode *string
	Action          string
	ResourceType    string
	ResourceID      string
	Status          string
	DetailJSON      *string
	IPAddress       *string
	UserAgent       *string
}

// ListResponse 审计日志列表项，精简字段以减少传输量。
type ListResponse struct {
	ID              string  `json:"id"`
	TraceID         string  `json:"trace_id"`
	ActorUserID     *string `json:"actor_user_id"`
	BusinessAppCode *string `json:"business_app_code"`
	Action          string  `json:"action"`
	ResourceType    string  `json:"resource_type"`
	ResourceID      string  `json:"resource_id"`
	Status          string  `json:"status"`
	DetailJSON      *string `json:"detail_json"`
	CreatedAt       string  `json:"created_at"`
}
