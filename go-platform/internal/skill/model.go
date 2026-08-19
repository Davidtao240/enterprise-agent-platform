package skill

import (
	"encoding/json"
	"errors"
	"time"
)

// Status Skill 生命周期状态。
type Status string

const (
	StatusDraft      Status = "draft"
	StatusReview     Status = "review"
	StatusPublished  Status = "published"
	StatusDeprecated Status = "deprecated"
)

// ErrInvalidTransition 非法状态流转。
var ErrInvalidTransition = errors.New("invalid skill status transition")

// CanTransition 状态机转换表 (Spec §4.1):
// draft -> review -> published -> deprecated;废弃不可逆。
var transitions = map[Status][]Status{
	StatusDraft:     {StatusReview},
	StatusReview:    {StatusPublished},
	StatusPublished: {StatusDeprecated},
}

func CanTransition(from, to Status) bool {
	for _, next := range transitions[from] {
		if next == to {
			return true
		}
	}
	return false
}

type Skill struct {
	ID          string          `json:"id"`
	SkillCode   string          `json:"skill_code"`
	Version     string          `json:"version"`
	Status      Status          `json:"status"`
	ConfigJSON  json.RawMessage `json:"config_json"`
	CreatedBy   string          `json:"created_by"`
	ReviewedBy  *string         `json:"reviewed_by,omitempty"`
	PublishedAt *time.Time      `json:"published_at,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type CreateRequest struct {
	SkillCode  string          `json:"skill_code" binding:"required"`
	Version    string          `json:"version" binding:"required"`
	ConfigJSON json.RawMessage `json:"config_json" binding:"required"`
	CreatedBy  string          `json:"created_by" binding:"required"`
}

// ── M8-B: Skill Marketplace Models ──

type SkillMarketplaceItem struct {
	ID              string    `json:"id"`
	SkillCode       string    `json:"skill_code"`
	Version         string    `json:"version"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	Category        string    `json:"category"`
	Icon            string    `json:"icon"`
	Tags            []string  `json:"tags"`
	Author          string    `json:"author"`
	HomepageURL     string    `json:"homepage_url"`
	Status          string    `json:"status"`
	IsCurrent       bool      `json:"is_current"`
	UsageCount      int       `json:"usage_count"`
	PublishedAt     *time.Time `json:"published_at,omitempty"`
	Installed       bool      `json:"installed"`
	InstalledVersion string    `json:"installed_version,omitempty"`
	InstallStatus   string    `json:"install_status,omitempty"`
}

type SkillInstallation struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenant_id"`
	UserID          string    `json:"user_id"`
	SkillID         string    `json:"skill_id"`
	SkillCode       string    `json:"skill_code"`
	InstalledVersion string    `json:"installed_version"`
	Status          string    `json:"status"`
	InstalledAt     time.Time `json:"installed_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type SkillUsageEvent struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	SkillCode   string    `json:"skill_code"`
	SkillVersion string   `json:"skill_version"`
	UserID      *string   `json:"user_id,omitempty"`
	SessionID   string    `json:"session_id"`
	ToolCallID  string    `json:"tool_call_id"`
	UsedAt      time.Time `json:"used_at"`
}

type MarketplaceListRequest struct {
	Category string `form:"category"`
	Query    string `form:"q"`
	Status   string `form:"status"`
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
}

type MarketplaceResponse struct {
	Items    []SkillMarketplaceItem `json:"items"`
	Total    int                    `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"page_size"`
}

type InstallSkillRequest struct {
	SkillCode string `json:"skill_code" binding:"required"`
	Version   string `json:"version"`
}

type UpdateInstallationRequest struct {
	Status *string `json:"status" binding:"omitempty,oneof=active disabled uninstalled"`
}

type UpdateSkillMetadataRequest struct {
	Name        *string  `json:"name"`
	Description *string  `json:"description"`
	Category    *string  `json:"category"`
	Icon        *string  `json:"icon"`
	Tags        []string `json:"tags"`
	Author      *string  `json:"author"`
	HomepageURL *string  `json:"homepage_url"`
}
