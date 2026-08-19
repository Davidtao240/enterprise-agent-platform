package skill

import (
	"context"
	"fmt"
)

type marketplaceRepository interface {
	ListMarketplace(ctx context.Context, tenantID, userID, category, query, status string, page, pageSize int) ([]SkillMarketplaceItem, int, error)
	InstallSkill(ctx context.Context, tenantID, userID, skillID, skillCode, version string) (*SkillInstallation, error)
	GetInstallation(ctx context.Context, tenantID, userID, skillCode string) (*SkillInstallation, error)
	UpdateInstallation(ctx context.Context, tenantID, userID, skillCode, status string) error
	ListInstalledByUser(ctx context.Context, tenantID, userID string) ([]SkillInstallation, error)
	RecordUsage(ctx context.Context, tenantID, skillCode, skillVersion, userID, sessionID, toolCallID string) error
	UpdateSkillMetadata(ctx context.Context, skillID string, updates map[string]any) error
	SetCurrentVersion(ctx context.Context, skillCode, version string) error
}

type MarketplaceService struct {
	store    Store
	repo     marketplaceRepository
}

func NewMarketplaceService(store Store, repo marketplaceRepository) *MarketplaceService {
	return &MarketplaceService{store: store, repo: repo}
}

func (s *MarketplaceService) ListMarketplace(ctx context.Context, tenantID, userID, category, query, status string, page, pageSize int) (*MarketplaceResponse, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}

	items, total, err := s.repo.ListMarketplace(ctx, tenantID, userID, category, query, status, page, pageSize)
	if err != nil {
		return nil, fmt.Errorf("list marketplace: %w", err)
	}
	return &MarketplaceResponse{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *MarketplaceService) InstallSkill(ctx context.Context, tenantID, userID string, req *InstallSkillRequest) (*SkillInstallation, error) {
	skills, err := s.store.List(ctx, req.SkillCode)
	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}
	if len(skills) == 0 {
		return nil, fmt.Errorf("skill %s not found", req.SkillCode)
	}

	var targetSkill *Skill
	if req.Version != "" {
		for _, sk := range skills {
			if sk.Version == req.Version {
				targetSkill = sk
				break
			}
		}
		if targetSkill == nil {
			return nil, fmt.Errorf("version %s not found for skill %s", req.Version, req.SkillCode)
		}
	} else {
		for _, sk := range skills {
			if sk.Status == StatusPublished {
				targetSkill = sk
				break
			}
		}
		if targetSkill == nil {
			return nil, fmt.Errorf("no published version found for skill %s", req.SkillCode)
		}
	}

	if targetSkill.Status != StatusPublished {
		return nil, fmt.Errorf("skill %s@%s is not published (status: %s)", req.SkillCode, targetSkill.Version, targetSkill.Status)
	}

	return s.repo.InstallSkill(ctx, tenantID, userID, targetSkill.ID, targetSkill.SkillCode, targetSkill.Version)
}

func (s *MarketplaceService) UninstallSkill(ctx context.Context, tenantID, userID, skillCode string) error {
	return s.repo.UpdateInstallation(ctx, tenantID, userID, skillCode, "uninstalled")
}

func (s *MarketplaceService) UpdateInstallationStatus(ctx context.Context, tenantID, userID, skillCode, status string) error {
	return s.repo.UpdateInstallation(ctx, tenantID, userID, skillCode, status)
}

func (s *MarketplaceService) ListInstalled(ctx context.Context, tenantID, userID string) ([]SkillInstallation, error) {
	return s.repo.ListInstalledByUser(ctx, tenantID, userID)
}

func (s *MarketplaceService) RecordSkillUsage(ctx context.Context, tenantID, skillCode, skillVersion, userID, sessionID, toolCallID string) error {
	return s.repo.RecordUsage(ctx, tenantID, skillCode, skillVersion, userID, sessionID, toolCallID)
}

func (s *MarketplaceService) UpdateSkillMetadata(ctx context.Context, skillID string, req *UpdateSkillMetadataRequest) error {
	updates := map[string]any{}

	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Category != nil {
		updates["category"] = *req.Category
	}
	if req.Icon != nil {
		updates["icon"] = *req.Icon
	}
	if req.Tags != nil {
		updates["tags"] = req.Tags
	}
	if req.Author != nil {
		updates["author"] = *req.Author
	}
	if req.HomepageURL != nil {
		updates["homepage_url"] = *req.HomepageURL
	}

	return s.repo.UpdateSkillMetadata(ctx, skillID, updates)
}

func (s *MarketplaceService) PublishNewVersion(ctx context.Context, skillCode, version string) error {
	return s.repo.SetCurrentVersion(ctx, skillCode, version)
}