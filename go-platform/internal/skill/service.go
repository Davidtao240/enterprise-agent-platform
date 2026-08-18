package skill

import (
	"context"
	"encoding/json"
	"fmt"
)

// Store Service 依赖仓储接口(便于测试替换)。
type Store interface {
	Create(ctx context.Context, req *CreateRequest) (*Skill, error)
	GetByID(ctx context.Context, id string) (*Skill, error)
	List(ctx context.Context, skillCode string) ([]*Skill, error)
	TransitionGuarded(ctx context.Context, id string, from, to Status, reviewer string) (*Skill, error)
	UpdateConfigIfDraft(ctx context.Context, id string, cfg json.RawMessage) (*Skill, error)
}

// Service M4-B Skill 生命周期服务。
type Service struct {
	store Store
}

// NewService 创建 Service。
func NewService(store Store) *Service {
	return &Service{store: store}
}

// Create 创建 draft 草稿。
func (s *Service) Create(ctx context.Context, req *CreateRequest) (*Skill, error) {
	if !json.Valid(req.ConfigJSON) {
		return nil, fmt.Errorf("skill config must be valid JSON")
	}
	return s.store.Create(ctx, req)
}

// Get / List 查询。
func (s *Service) Get(ctx context.Context, id string) (*Skill, error) {
	return s.store.GetByID(ctx, id)
}

func (s *Service) List(ctx context.Context, skillCode string) ([]*Skill, error) {
	return s.store.List(ctx, skillCode)
}

// transition 统一流转入口:先校验状态机,再守卫式更新。
func (s *Service) transition(ctx context.Context, id string, from, to Status, reviewer string) (*Skill, error) {
	if !CanTransition(from, to) {
		return nil, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, from, to)
	}
	return s.store.TransitionGuarded(ctx, id, from, to, reviewer)
}

// SubmitForReview draft -> review (内容锁定,等待审核)。
func (s *Service) SubmitForReview(ctx context.Context, id string) (*Skill, error) {
	return s.transition(ctx, id, StatusDraft, StatusReview, "")
}

// Publish review -> published (审核通过;reviewer 必填,职责分离)。
func (s *Service) Publish(ctx context.Context, id, reviewer string) (*Skill, error) {
	if reviewer == "" {
		return nil, fmt.Errorf("reviewer is required to publish")
	}
	return s.transition(ctx, id, StatusReview, StatusPublished, reviewer)
}

// Deprecate published -> deprecated (不可逆,保持旧 Run 兼容引用)。
func (s *Service) Deprecate(ctx context.Context, id string) (*Skill, error) {
	return s.transition(ctx, id, StatusPublished, StatusDeprecated, "")
}

// UpdateConfig 仅 draft 状态允许修改配置。
func (s *Service) UpdateConfig(ctx context.Context, id string, cfg json.RawMessage) (*Skill, error) {
	if !json.Valid(cfg) {
		return nil, fmt.Errorf("skill config must be valid JSON")
	}
	return s.store.UpdateConfigIfDraft(ctx, id, cfg)
}
