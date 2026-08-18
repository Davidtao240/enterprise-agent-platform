package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// Service M4-A Memory 服务:写入校验 + ACL 过滤检索。
type Service struct {
	store Store
}

// Store Service 依赖仓储接口(便于测试替换)。
type Store interface {
	Create(ctx context.Context, req *WriteRequest) (*Memory, error)
	ListActive(ctx context.Context, tenantID, scope, scopeID string, limit int) ([]*Memory, error)
	GetByID(ctx context.Context, tenantID, id string) (*Memory, error)
	SoftDelete(ctx context.Context, tenantID, id string) error
}

// NewService 创建 Service。
func NewService(store Store) *Service {
	return &Service{store: store}
}

// Write 写入记忆:scope 合法性 + content 非空 JSON 校验。
func (s *Service) Write(ctx context.Context, req *WriteRequest) (*Memory, error) {
	if !ValidScope(req.Scope) {
		return nil, fmt.Errorf("invalid memory scope: %q", req.Scope)
	}
	if !json.Valid(req.Content) {
		return nil, errors.New("memory content must be valid JSON")
	}
	return s.store.Create(ctx, req)
}

// QueryVisible 检索对 viewer 可见的记忆:
// Repository 已过滤软删除/过期,此处追加 ACL 过滤。
func (s *Service) QueryVisible(ctx context.Context, req *QueryRequest) ([]*Memory, error) {
	if !ValidScope(req.Scope) {
		return nil, fmt.Errorf("invalid memory scope: %q", req.Scope)
	}
	items, err := s.store.ListActive(ctx, req.TenantID, req.Scope, req.ScopeID, 100)
	if err != nil {
		return nil, err
	}
	visible := make([]*Memory, 0, len(items))
	for _, m := range items {
		if ACLAllows(m, req.ViewerID, req.ViewerRoles) {
			visible = append(visible, m)
		}
	}
	return visible, nil
}

// Delete 软删除记忆。
func (s *Service) Delete(ctx context.Context, tenantID, id string) error {
	return s.store.SoftDelete(ctx, tenantID, id)
}

// ACLAllows 判定 viewer 是否可读该记忆。
//
// 规则(Spec MEMORY_AND_CONTEXT.md §2.2):
//   - acl 为空/[] (私有): 仅 created_by == viewerID 可见
//   - 否则: acl 中出现 "user:<viewerID>" 或任一 "role:<viewerRole>" 即可见
func ACLAllows(m *Memory, viewerID string, viewerRoles []string) bool {
	if len(m.ACL) == 0 {
		return m.CreatedBy == viewerID
	}
	roles := make(map[string]bool, len(viewerRoles))
	for _, r := range viewerRoles {
		roles[r] = true
	}
	for _, entry := range m.ACL {
		const userPrefix = "user:"
		const rolePrefix = "role:"
		if len(entry) > len(userPrefix) && entry[:len(userPrefix)] == userPrefix {
			if entry[len(userPrefix):] == viewerID {
				return true
			}
		}
		if len(entry) > len(rolePrefix) && entry[:len(rolePrefix)] == rolePrefix {
			if roles[entry[len(rolePrefix):]] {
				return true
			}
		}
	}
	return false
}
