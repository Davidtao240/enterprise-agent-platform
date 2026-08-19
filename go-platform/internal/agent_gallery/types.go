package agent_gallery

import "time"

type AgentPackageStatus string

const (
	StatusDraft      AgentPackageStatus = "draft"
	StatusPublished  AgentPackageStatus = "published"
	StatusDisabled   AgentPackageStatus = "disabled"
)

type AgentPackage struct {
	ID               string    `json:"id"`
	TenantID         string    `json:"tenant_id"`
	PackageCode      string    `json:"package_code"`
	Name             string    `json:"name"`
	Description      string    `json:"description"`
	Category         string    `json:"category"`
	BusinessAppCode  string    `json:"business_app_code"`
	GraphKey         string    `json:"graph_key"`
	GraphVersion     string    `json:"graph_version"`
	EntryType        string    `json:"entry_type"`
	Icon             string    `json:"icon"`
	CapabilitiesJSON *string   `json:"capabilities_json,omitempty"`
	SamplePromptsJSON *string  `json:"sample_prompts_json,omitempty"`
	Status           string    `json:"status"`
	PublishedAt      *time.Time `json:"published_at,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type AgentPackageListItem struct {
	ID              string    `json:"id"`
	PackageCode     string    `json:"package_code"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	Category        string    `json:"category"`
	BusinessAppCode string    `json:"business_app_code"`
	Icon            string    `json:"icon"`
	Status          string    `json:"status"`
	UsageCount      int       `json:"usage_count"`
	LastUsedAt      *time.Time `json:"last_used_at,omitempty"`
}

type AgentPackageDetail struct {
	AgentPackageListItem
	Capabilities   any    `json:"capabilities"`
	SamplePrompts  []string `json:"sample_prompts"`
	TotalConversations int  `json:"total_conversations"`
	Recent7Days   []UsageDay `json:"recent_7_days"`
}

type UsageDay struct {
	Date             string `json:"date"`
	ConversationCount int   `json:"conversation_count"`
	MessageCount     int   `json:"message_count"`
}

type CreatePackageRequest struct {
	PackageCode     string `json:"package_code" binding:"required"`
	Name            string `json:"name" binding:"required"`
	Description     string `json:"description"`
	Category        string `json:"category" binding:"required,oneof=general departmental"`
	BusinessAppCode string `json:"business_app_code" binding:"required"`
	GraphKey        string `json:"graph_key" binding:"required"`
	GraphVersion    string `json:"graph_version"`
	EntryType       string `json:"entry_type"`
	Icon            string `json:"icon"`
	CapabilitiesJSON any   `json:"capabilities_json"`
	SamplePrompts   []string `json:"sample_prompts"`
}

type UpdatePackageRequest struct {
	Name             *string `json:"name"`
	Description      *string `json:"description"`
	Icon             *string `json:"icon"`
	CapabilitiesJSON any     `json:"capabilities_json"`
	SamplePrompts    []string `json:"sample_prompts"`
	Status           *string  `json:"status"`
}

type ListGalleryRequest struct {
	Category        string `form:"category"`
	BusinessAppCode string `form:"business_app"`
	Query           string `form:"q"`
}

type GalleryResponse struct {
	Packages []AgentPackageListItem `json:"packages"`
	Total    int                    `json:"total"`
}