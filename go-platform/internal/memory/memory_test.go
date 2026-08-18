package memory

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
)

// fakeStore 内存版仓储,用于 Service 层单测。
type fakeStore struct {
	items map[string]*Memory
	seq   int
}

func newFakeStore() *fakeStore { return &fakeStore{items: map[string]*Memory{}} }

func (f *fakeStore) Create(_ context.Context, req *WriteRequest) (*Memory, error) {
	f.seq++
	m := &Memory{
		ID: "mem-" + strconv.Itoa(f.seq), TenantID: req.TenantID, Scope: Scope(req.Scope),
		ScopeID: req.ScopeID, ContentJSON: append(json.RawMessage(nil), req.Content...),
		ACL: req.ACL, CreatedBy: req.CreatedBy,
	}
	f.items[m.ID] = m
	return m, nil
}

func (f *fakeStore) ListActive(_ context.Context, _, _, _ string, _ int) ([]*Memory, error) {
	var out []*Memory
	for _, m := range f.items {
		if m.DeletedAt == nil {
			out = append(out, m)
		}
	}
	return out, nil
}

func (f *fakeStore) GetByID(_ context.Context, _, id string) (*Memory, error) {
	if m, ok := f.items[id]; ok {
		return m, nil
	}
	return nil, ErrMemoryNotFound
}

func (f *fakeStore) SoftDelete(_ context.Context, _, id string) error {
	if m, ok := f.items[id]; ok {
		m.DeletedAt = &m.CreatedAt
		return nil
	}
	return ErrMemoryNotFound
}

func TestServiceWriteValidatesScopeAndContent(t *testing.T) {
	svc := NewService(newFakeStore())
	ctx := context.Background()

	if _, err := svc.Write(ctx, &WriteRequest{
		TenantID: "t1", Scope: "galaxy", ScopeID: "s", Content: json.RawMessage(`{}`), CreatedBy: "u1",
	}); err == nil {
		t.Fatal("expected invalid scope error")
	}
	if _, err := svc.Write(ctx, &WriteRequest{
		TenantID: "t1", Scope: "user", ScopeID: "s", Content: json.RawMessage(`{invalid`), CreatedBy: "u1",
	}); err == nil {
		t.Fatal("expected invalid JSON error")
	}
	if _, err := svc.Write(ctx, &WriteRequest{
		TenantID: "t1", Scope: "user", ScopeID: "u1", Content: json.RawMessage(`{"k":1}`), CreatedBy: "u1",
	}); err != nil {
		t.Fatalf("valid write failed: %v", err)
	}
}

func TestACLAllows(t *testing.T) {
	cases := []struct {
		name    string
		acl     []string
		creator string
		viewer  string
		roles   []string
		want    bool
	}{
		{"private creator sees", nil, "u1", "u1", nil, true},
		{"private other denied", nil, "u1", "u2", nil, false},
		{"user entry match", []string{"user:u2"}, "u1", "u2", nil, true},
		{"user entry mismatch", []string{"user:u9"}, "u1", "u2", nil, false},
		{"role entry match", []string{"role:finance_manager"}, "u1", "u2", []string{"finance_manager"}, true},
		{"role entry mismatch", []string{"role:finance_manager"}, "u1", "u2", []string{"ops_viewer"}, false},
		{"mixed entry role hit", []string{"user:u9", "role:admin"}, "u1", "u2", []string{"admin"}, true},
		{"creator bypasses acl", []string{"role:admin"}, "u1", "u1", nil, false}, // acl 非空时按 acl 判定
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &Memory{ACL: tc.acl, CreatedBy: tc.creator}
			if got := ACLAllows(m, tc.viewer, tc.roles); got != tc.want {
				t.Fatalf("ACLAllows = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestQueryVisibleFiltersACL(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store)
	ctx := context.Background()

	_, _ = store.Create(ctx, &WriteRequest{
		TenantID: "t1", Scope: "team", ScopeID: "finance", CreatedBy: "alice",
		Content: json.RawMessage(`{"note":"private"}`),
	})
	_, _ = store.Create(ctx, &WriteRequest{
		TenantID: "t1", Scope: "team", ScopeID: "finance", CreatedBy: "alice",
		Content: json.RawMessage(`{"note":"managers only"}`), ACL: []string{"role:finance_manager"},
	})
	_, _ = store.Create(ctx, &WriteRequest{
		TenantID: "t1", Scope: "team", ScopeID: "finance", CreatedBy: "bob",
		Content: json.RawMessage(`{"note":"for bob"}`), ACL: []string{"user:bob"},
	})

	// bob 无 finance_manager 角色:看到自己的 + alice 私有的(不可见) → 仅 user:bob 条目
	got, err := svc.QueryVisible(ctx, &QueryRequest{
		TenantID: "t1", Scope: "team", ScopeID: "finance", ViewerID: "bob",
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(got) != 1 || string(got[0].ContentJSON) != `{"note":"for bob"}` {
		t.Fatalf("bob should see only his own entry, got %d", len(got))
	}

	// carol 有 finance_manager 角色:仅 role 条目
	got, err = svc.QueryVisible(ctx, &QueryRequest{
		TenantID: "t1", Scope: "team", ScopeID: "finance", ViewerID: "carol",
		ViewerRoles: []string{"finance_manager"},
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(got) != 1 || string(got[0].ContentJSON) != `{"note":"managers only"}` {
		t.Fatalf("carol should see only role-allowed entry, got %d", len(got))
	}
}

func TestSoftDeleteHidesEntry(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store)
	ctx := context.Background()

	m, _ := store.Create(ctx, &WriteRequest{
		TenantID: "t1", Scope: "user", ScopeID: "u1", CreatedBy: "u1",
		Content: json.RawMessage(`{"pref":"dark"}`),
	})
	if err := svc.Delete(ctx, "t1", m.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	got, _ := svc.QueryVisible(ctx, &QueryRequest{TenantID: "t1", Scope: "user", ScopeID: "u1", ViewerID: "u1"})
	if len(got) != 0 {
		t.Fatalf("deleted memory should be invisible, got %d", len(got))
	}
}
