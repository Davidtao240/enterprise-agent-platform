package governance

import (
	"encoding/json"
	"time"
)

const (
	StatusDraft           = "draft"
	StatusPendingApproval = "pending_approval"
	StatusPublished       = "published"
	StatusDeprecated      = "deprecated"
)

type ConfigurationVersion struct {
	ID              string          `json:"id"`
	TenantID        string          `json:"tenant_id"`
	ResourceType    string          `json:"resource_type"`
	ResourceKey     string          `json:"resource_key"`
	Version         string          `json:"version"`
	LifecycleStatus string          `json:"lifecycle_status"`
	SnapshotJSON    json.RawMessage `json:"snapshot_json"`
	ChangeSummary   string          `json:"change_summary"`
	CreatedBy       string          `json:"created_by"`
	ApprovedBy      *string         `json:"approved_by,omitempty"`
	ApprovedAt      *time.Time      `json:"approved_at,omitempty"`
	PublishedAt     *time.Time      `json:"published_at,omitempty"`
	DeprecatedAt    *time.Time      `json:"deprecated_at,omitempty"`
	TraceID         string          `json:"trace_id"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type CreateRequest struct {
	ResourceType  string          `json:"resource_type" binding:"required"`
	ResourceKey   string          `json:"resource_key" binding:"required"`
	Version       string          `json:"version" binding:"required"`
	SnapshotJSON  json.RawMessage `json:"snapshot_json" binding:"required"`
	ChangeSummary string          `json:"change_summary"`
}

func IsSupportedResourceType(resourceType string) bool {
	switch resourceType {
	case "business_app", "workflow_template", "agent", "tool", "domain_policy":
		return true
	default:
		return false
	}
}
