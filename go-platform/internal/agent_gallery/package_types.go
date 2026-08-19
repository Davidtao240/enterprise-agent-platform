package agent_gallery

import "time"

type PackageVersion struct {
    ID              string    `json:"id"`
    TenantID        string    `json:"tenant_id"`
    PackageCode     string    `json:"package_code"`
    Version         string    `json:"version"`
    ManifestJSON    any       `json:"manifest"`
    GraphKey        string    `json:"graph_key"`
    GraphVersion    string    `json:"graph_version"`
    EntryType       string    `json:"entry_type"`
    Status          string    `json:"status"`
    IsCurrent       bool      `json:"is_current"`
    CreatedBy       *string   `json:"created_by,omitempty"`
    CreatedAt       time.Time `json:"created_at"`
    UpdatedAt       time.Time `json:"updated_at"`
}

type PackageInstallation struct {
    ID             string    `json:"id"`
    TenantID       string    `json:"tenant_id"`
    PackageCode    string    `json:"package_code"`
    InstalledVersion string   `json:"installed_version"`
    InstalledAt    time.Time `json:"installed_at"`
    InstalledBy    *string   `json:"installed_by,omitempty"`
    Status         string    `json:"status"`
    UpdatedAt      time.Time `json:"updated_at"`
}

type PackageRegistration struct {
    ID             string    `json:"id"`
    TenantID       string    `json:"tenant_id"`
    PackageCode    string    `json:"package_code"`
    SourceType     string    `json:"source_type"`
    SourceURL      string    `json:"source_url,omitempty"`
    ManifestJSON   any       `json:"manifest"`
    Signature      string    `json:"signature,omitempty"`
    Verified       bool      `json:"verified"`
    Status         string    `json:"status"`
    RegisteredBy   *string   `json:"registered_by,omitempty"`
    CreatedAt      time.Time `json:"created_at"`
    UpdatedAt      time.Time `json:"updated_at"`
}

type PackageManifest struct {
    PackageCode     string         `json:"package_code"`
    Name            string         `json:"name"`
    Version         string         `json:"version"`
    Description     string         `json:"description"`
    Category        string         `json:"category"`
    BusinessAppCode string         `json:"business_app_code"`
    GraphKey        string         `json:"graph_key"`
    GraphVersion    string         `json:"graph_version"`
    EntryType       string         `json:"entry_type"`
    Icon            string         `json:"icon"`
    Capabilities    map[string]any `json:"capabilities"`
    SamplePrompts   []string       `json:"sample_prompts"`
    Author          string         `json:"author"`
    License         string         `json:"license"`
    MinPlatformVersion string     `json:"min_platform_version"`
}

type InstallPackageRequest struct {
    PackageCode    string `json:"package_code" binding:"required"`
    Version        string `json:"version"`
    GraphKey       string `json:"graph_key" binding:"required"`
    GraphVersion   string `json:"graph_version"`
    EntryType      string `json:"entry_type"`
}

type RegisterPackageRequest struct {
    PackageCode  string         `json:"package_code" binding:"required"`
    SourceType   string         `json:"source_type" binding:"required,oneof=official third_party marketplace"`
    SourceURL    string         `json:"source_url"`
    Manifest     PackageManifest `json:"manifest" binding:"required"`
    Signature    string         `json:"signature"`
    VerifyOnly   bool           `json:"verify_only"`
}

type UpdateInstallationRequest struct {
    Status *string `json:"status" binding:"omitempty,oneof=active disabled uninstalled"`
}

type ListInstalledRequest struct {
    Status   string `form:"status"`
    Category string `form:"category"`
    Query    string `form:"q"`
}

type ListRegistrationsRequest struct {
    Status     string `form:"status"`
    SourceType string `form:"source_type"`
}

type InstallationResponse struct {
    Installation  *PackageInstallation  `json:"installation"`
    Package       *AgentPackageListItem `json:"package,omitempty"`
    Version       *PackageVersion       `json:"version,omitempty"`
}

type InstalledListResponse struct {
    Items []InstallationResponse `json:"items"`
    Total int                    `json:"total"`
}

type RegistrationListResponse struct {
    Items []PackageRegistration `json:"items"`
    Total int                   `json:"total"`
}

type VersionListResponse struct {
    Items []PackageVersion `json:"items"`
    Total int              `json:"total"`
}