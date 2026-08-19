package agent_gallery

import (
	"context"
	"encoding/json"
	"fmt"
)

type galleryRepository interface {
	ListPublishedPackages(ctx context.Context, tenantID, category, businessAppCode, query string) ([]AgentPackageListItem, error)
	GetPackageByCode(ctx context.Context, tenantID, packageCode string) (*AgentPackage, error)
	GetPackageStats(ctx context.Context, tenantID, packageCode string) (int, []UsageDay, error)
	CreatePackage(ctx context.Context, pkg *AgentPackage) error
	UpdatePackage(ctx context.Context, tenantID, packageCode string, updates map[string]any) error
	CheckGraphKeyExists(ctx context.Context, graphKey string) (bool, error)

	CreateVersion(ctx context.Context, v *PackageVersion) error
	ListVersions(ctx context.Context, tenantID, packageCode string) ([]PackageVersion, error)
	GetVersion(ctx context.Context, tenantID, packageCode, version string) (*PackageVersion, error)
	SetCurrentVersion(ctx context.Context, tenantID, packageCode, version string) error
	DeprecateOtherVersions(ctx context.Context, tenantID, packageCode, keepVersion string) error

	CreateInstallation(ctx context.Context, inst *PackageInstallation) error
	GetInstallation(ctx context.Context, tenantID, packageCode string) (*PackageInstallation, error)
	UpdateInstallation(ctx context.Context, tenantID, packageCode string, updates map[string]any) error
	ListInstallations(ctx context.Context, tenantID, status string) ([]PackageInstallation, error)

	CreateRegistration(ctx context.Context, reg *PackageRegistration) error
	GetRegistration(ctx context.Context, tenantID, packageCode string) (*PackageRegistration, error)
	UpdateRegistration(ctx context.Context, tenantID, packageCode string, updates map[string]any) error
	ListRegistrations(ctx context.Context, tenantID, status, sourceType string) ([]PackageRegistration, error)
}

type Service struct {
	repo galleryRepository
}

func NewService(repo galleryRepository) *Service {
	return &Service{repo: repo}
}

func (s *Service) ListGallery(ctx context.Context, tenantID, category, businessAppCode, query string) (*GalleryResponse, error) {
	packages, err := s.repo.ListPublishedPackages(ctx, tenantID, category, businessAppCode, query)
	if err != nil {
		return nil, fmt.Errorf("list gallery: %w", err)
	}
	return &GalleryResponse{
		Packages: packages,
		Total:    len(packages),
	}, nil
}

func (s *Service) GetPackageDetail(ctx context.Context, tenantID, packageCode string) (*AgentPackageDetail, error) {
	pkg, err := s.repo.GetPackageByCode(ctx, tenantID, packageCode)
	if err != nil {
		return nil, fmt.Errorf("get package: %w", err)
	}
	if pkg == nil {
		return nil, nil
	}

	listItem := AgentPackageListItem{
		ID:              pkg.ID,
		PackageCode:     pkg.PackageCode,
		Name:            pkg.Name,
		Description:     pkg.Description,
		Category:        pkg.Category,
		BusinessAppCode: pkg.BusinessAppCode,
		Icon:            pkg.Icon,
		Status:          pkg.Status,
	}

	var capabilities any
	if pkg.CapabilitiesJSON != nil {
		_ = json.Unmarshal([]byte(*pkg.CapabilitiesJSON), &capabilities)
	}

	var samplePrompts []string
	if pkg.SamplePromptsJSON != nil {
		_ = json.Unmarshal([]byte(*pkg.SamplePromptsJSON), &samplePrompts)
	}

	totalConversations, recent7Days, err := s.repo.GetPackageStats(ctx, tenantID, packageCode)
	if err != nil {
		return nil, fmt.Errorf("get stats: %w", err)
	}

	return &AgentPackageDetail{
		AgentPackageListItem: listItem,
		Capabilities:         capabilities,
		SamplePrompts:        samplePrompts,
		TotalConversations:   totalConversations,
		Recent7Days:          recent7Days,
	}, nil
}

func (s *Service) CreatePackage(ctx context.Context, tenantID string, req *CreatePackageRequest) (*AgentPackage, error) {
	exists, err := s.repo.CheckGraphKeyExists(ctx, req.GraphKey)
	if err != nil {
		return nil, fmt.Errorf("check graph_key: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("graph_key %s does not exist or is not active", req.GraphKey)
	}

	capsJSON := "{}"
	if req.CapabilitiesJSON != nil {
		b, _ := json.Marshal(req.CapabilitiesJSON)
		capsJSON = string(b)
	}

	promptsJSON := "[]"
	if req.SamplePrompts != nil {
		b, _ := json.Marshal(req.SamplePrompts)
		promptsJSON = string(b)
	}

	entryType := req.EntryType
	if entryType == "" {
		entryType = "conversation"
	}

	graphVersion := req.GraphVersion
	if graphVersion == "" {
		graphVersion = "v1"
	}

	pkg := &AgentPackage{
		TenantID:          tenantID,
		PackageCode:       req.PackageCode,
		Name:              req.Name,
		Description:       req.Description,
		Category:          req.Category,
		BusinessAppCode:   req.BusinessAppCode,
		GraphKey:          req.GraphKey,
		GraphVersion:      graphVersion,
		EntryType:         entryType,
		Icon:              req.Icon,
		CapabilitiesJSON:   &capsJSON,
		SamplePromptsJSON: &promptsJSON,
		Status:            "draft",
	}

	if err := s.repo.CreatePackage(ctx, pkg); err != nil {
		return nil, fmt.Errorf("create package: %w", err)
	}
	return pkg, nil
}

var validTransitions = map[string]map[string]bool{
	"draft":    {"published": true, "disabled": true},
	"published": {"disabled": true},
	"disabled":  {"published": true},
}

func (s *Service) UpdatePackage(ctx context.Context, tenantID, packageCode string, req *UpdatePackageRequest) error {
	updates := map[string]any{}

	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Icon != nil {
		updates["icon"] = *req.Icon
	}
	if req.CapabilitiesJSON != nil {
		updates["capabilities_json"] = req.CapabilitiesJSON
	}
	if req.SamplePrompts != nil {
		updates["sample_prompts_json"] = req.SamplePrompts
	}

	if req.Status != nil {
		pkg, err := s.repo.GetPackageByCode(ctx, tenantID, packageCode)
		if err != nil {
			return fmt.Errorf("get package for transition: %w", err)
		}
		if pkg == nil {
			return fmt.Errorf("package %s not found", packageCode)
		}

		allowed, ok := validTransitions[pkg.Status]
		if !ok || !allowed[*req.Status] {
			return fmt.Errorf("invalid_transition: %s -> %s", pkg.Status, *req.Status)
		}

		if *req.Status == "published" {
			exists, err := s.repo.CheckGraphKeyExists(ctx, pkg.GraphKey)
			if err != nil {
				return fmt.Errorf("check graph_key: %w", err)
			}
			if !exists {
				return fmt.Errorf("graph_key %s is not active, cannot publish", pkg.GraphKey)
			}
		}

		updates["status"] = *req.Status
	}

	return s.repo.UpdatePackage(ctx, tenantID, packageCode, updates)
}