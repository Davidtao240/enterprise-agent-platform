package agent_gallery

import (
	"context"
	"encoding/json"
	"fmt"
)

func (s *Service) CreateVersion(ctx context.Context, tenantID string, v *PackageVersion) (*PackageVersion, error) {
	if v.Version == "" {
		return nil, fmt.Errorf("version is required")
	}
	if v.GraphKey == "" {
		return nil, fmt.Errorf("graph_key is required")
	}

	existing, err := s.repo.GetPackageByCode(ctx, tenantID, v.PackageCode)
	if err != nil {
		return nil, fmt.Errorf("check package: %w", err)
	}
	if existing == nil {
		return nil, fmt.Errorf("package %s does not exist, create package first", v.PackageCode)
	}

	if v.GraphVersion == "" {
		v.GraphVersion = "v1"
	}
	if v.EntryType == "" {
		v.EntryType = "conversation"
	}
	if v.Status == "" {
		v.Status = "draft"
	}

	v.TenantID = tenantID
	if err := s.repo.CreateVersion(ctx, v); err != nil {
		return nil, fmt.Errorf("create version: %w", err)
	}
	return v, nil
}

func (s *Service) ListVersions(ctx context.Context, tenantID, packageCode string) ([]PackageVersion, error) {
	return s.repo.ListVersions(ctx, tenantID, packageCode)
}

func (s *Service) PublishVersion(ctx context.Context, tenantID, packageCode, version string) error {
	v, err := s.repo.GetVersion(ctx, tenantID, packageCode, version)
	if err != nil {
		return fmt.Errorf("get version: %w", err)
	}
	if v == nil {
		return fmt.Errorf("version %s not found for package %s", version, packageCode)
	}
	if v.Status == "deprecated" {
		return fmt.Errorf("version %s is deprecated, cannot publish", version)
	}

	return s.repo.SetCurrentVersion(ctx, tenantID, packageCode, version)
}

func (s *Service) InstallPackage(ctx context.Context, tenantID, userID string, req *InstallPackageRequest) (*InstallationResponse, error) {
	existing, err := s.repo.GetPackageByCode(ctx, tenantID, req.PackageCode)
	if err != nil {
		return nil, fmt.Errorf("get package: %w", err)
	}
	if existing == nil {
		return nil, fmt.Errorf("package %s not found", req.PackageCode)
	}

	if req.Version == "" {
		req.Version = existing.GraphVersion
	}

	version, err := s.repo.GetVersion(ctx, tenantID, req.PackageCode, req.Version)
	if err != nil {
		return nil, fmt.Errorf("get version: %w", err)
	}
	if version == nil {
		return nil, fmt.Errorf("version %s not found for package %s, create version first", req.Version, req.PackageCode)
	}

	if version.Status != "published" || !version.IsCurrent {
		return nil, fmt.Errorf("version %s is not the current published version", req.Version)
	}

	inst := &PackageInstallation{
		TenantID:        tenantID,
		PackageCode:     req.PackageCode,
		InstalledVersion: req.Version,
		InstalledBy:     &userID,
		Status:          "active",
	}
	if err := s.repo.CreateInstallation(ctx, inst); err != nil {
		return nil, fmt.Errorf("create installation: %w", err)
	}

	listItem := AgentPackageListItem{
		ID:              existing.ID,
		PackageCode:     existing.PackageCode,
		Name:            existing.Name,
		Description:     existing.Description,
		Category:        existing.Category,
		BusinessAppCode: existing.BusinessAppCode,
		Icon:            existing.Icon,
		Status:          "published",
	}

	return &InstallationResponse{
		Installation: inst,
		Package:      &listItem,
		Version:      version,
	}, nil
}

func (s *Service) UninstallPackage(ctx context.Context, tenantID, packageCode string) error {
	updates := map[string]any{
		"status": "uninstalled",
	}
	return s.repo.UpdateInstallation(ctx, tenantID, packageCode, updates)
}

func (s *Service) UpdateInstallationStatus(ctx context.Context, tenantID, packageCode, status string) error {
	allowedStatuses := map[string]bool{"active": true, "disabled": true, "uninstalled": true}
	if !allowedStatuses[status] {
		return fmt.Errorf("invalid status: %s", status)
	}

	inst, err := s.repo.GetInstallation(ctx, tenantID, packageCode)
	if err != nil {
		return fmt.Errorf("get installation: %w", err)
	}
	if inst == nil {
		return fmt.Errorf("installation for %s not found", packageCode)
	}

	updates := map[string]any{"status": status}
	return s.repo.UpdateInstallation(ctx, tenantID, packageCode, updates)
}

func (s *Service) ListInstalled(ctx context.Context, tenantID, status, category, query string) (*InstalledListResponse, error) {
	installations, err := s.repo.ListInstallations(ctx, tenantID, status)
	if err != nil {
		return nil, fmt.Errorf("list installations: %w", err)
	}

	var items []InstallationResponse
	for _, inst := range installations {
		pkg, err := s.repo.GetPackageByCode(ctx, tenantID, inst.PackageCode)
		if err != nil {
			continue
		}
		if pkg == nil {
			continue
		}

		if category != "" && pkg.Category != category {
			continue
		}
		if query != "" {
			match := false
			if containsSubstring(pkg.Name, query) || containsSubstring(pkg.Description, query) {
				match = true
			}
			if !match {
				continue
			}
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

		version, _ := s.repo.GetVersion(ctx, tenantID, inst.PackageCode, inst.InstalledVersion)

		items = append(items, InstallationResponse{
			Installation: &inst,
			Package:      &listItem,
			Version:      version,
		})
	}

	return &InstalledListResponse{
		Items: items,
		Total: len(items),
	}, nil
}

func (s *Service) RegisterPackage(ctx context.Context, tenantID, userID string, req *RegisterPackageRequest) (*PackageRegistration, error) {
	if req.SourceType == "third_party" {
		if req.Signature == "" {
			return nil, fmt.Errorf("signature is required for third_party packages")
		}
	}

	manifestBytes, _ := json.Marshal(req.Manifest)
	var manifestJSON any
	_ = json.Unmarshal(manifestBytes, &manifestJSON)

	reg := &PackageRegistration{
		TenantID:     tenantID,
		PackageCode:  req.PackageCode,
		SourceType:   req.SourceType,
		SourceURL:    req.SourceURL,
		ManifestJSON: manifestJSON,
		Signature:    req.Signature,
		Verified:     req.SourceType == "official",
		Status:       "pending",
		RegisteredBy: &userID,
	}

	if req.VerifyOnly {
		reg.Status = "verified"
		reg.Verified = true
	}

	if err := s.repo.CreateRegistration(ctx, reg); err != nil {
		return nil, fmt.Errorf("register package: %w", err)
	}
	return reg, nil
}

func (s *Service) VerifyRegistration(ctx context.Context, tenantID, packageCode string) error {
	reg, err := s.repo.GetRegistration(ctx, tenantID, packageCode)
	if err != nil {
		return fmt.Errorf("get registration: %w", err)
	}
	if reg == nil {
		return fmt.Errorf("registration for %s not found", packageCode)
	}

	updates := map[string]any{
		"status":   "verified",
		"verified": true,
	}
	return s.repo.UpdateRegistration(ctx, tenantID, packageCode, updates)
}

func (s *Service) RejectRegistration(ctx context.Context, tenantID, packageCode string) error {
	reg, err := s.repo.GetRegistration(ctx, tenantID, packageCode)
	if err != nil {
		return fmt.Errorf("get registration: %w", err)
	}
	if reg == nil {
		return fmt.Errorf("registration for %s not found", packageCode)
	}

	updates := map[string]any{
		"status":   "rejected",
		"verified": false,
	}
	return s.repo.UpdateRegistration(ctx, tenantID, packageCode, updates)
}

func (s *Service) ListRegistrations(ctx context.Context, tenantID, status, sourceType string) (*RegistrationListResponse, error) {
	items, err := s.repo.ListRegistrations(ctx, tenantID, status, sourceType)
	if err != nil {
		return nil, fmt.Errorf("list registrations: %w", err)
	}
	return &RegistrationListResponse{
		Items: items,
		Total: len(items),
	}, nil
}

func (s *Service) GetRegistrationDetail(ctx context.Context, tenantID, packageCode string) (*PackageRegistration, error) {
	return s.repo.GetRegistration(ctx, tenantID, packageCode)
}

func containsSubstring(s, substr string) bool {
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}