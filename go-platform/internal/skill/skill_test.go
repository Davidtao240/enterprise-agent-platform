package skill

import (
	"context"
	"encoding/json"
	"testing"
)

// fakeStore 内存版仓储,用于 Service 层单测。
type fakeStore struct {
	items map[string]*Skill
}

func newFakeStore() *fakeStore { return &fakeStore{items: map[string]*Skill{}} }

func (f *fakeStore) Create(_ context.Context, req *CreateRequest) (*Skill, error) {
	for _, s := range f.items {
		if s.SkillCode == req.SkillCode && s.Version == req.Version {
			return nil, ErrDuplicateVersion
		}
	}
	s := &Skill{
		ID: "sk-1", SkillCode: req.SkillCode, Version: req.Version, Status: StatusDraft,
		ConfigJSON: append(json.RawMessage(nil), req.ConfigJSON...), CreatedBy: req.CreatedBy,
	}
	f.items[s.ID] = s
	return s, nil
}

func (f *fakeStore) GetByID(_ context.Context, id string) (*Skill, error) {
	if s, ok := f.items[id]; ok {
		return s, nil
	}
	return nil, ErrSkillNotFound
}

func (f *fakeStore) List(_ context.Context, _ string) ([]*Skill, error) {
	var out []*Skill
	for _, s := range f.items {
		out = append(out, s)
	}
	return out, nil
}

func (f *fakeStore) TransitionGuarded(_ context.Context, id string, from, to Status, reviewer string) (*Skill, error) {
	s, ok := f.items[id]
	if !ok {
		return nil, ErrSkillNotFound
	}
	if s.Status != from {
		return nil, ErrInvalidTransition
	}
	s.Status = to
	if to == StatusPublished {
		s.ReviewedBy = &reviewer
	}
	return s, nil
}

func (f *fakeStore) UpdateConfigIfDraft(_ context.Context, id string, cfg json.RawMessage) (*Skill, error) {
	s, ok := f.items[id]
	if !ok {
		return nil, ErrSkillNotFound
	}
	if s.Status != StatusDraft {
		return nil, ErrInvalidTransition
	}
	s.ConfigJSON = append(json.RawMessage(nil), cfg...)
	return s, nil
}

func TestSkillStateMachineTable(t *testing.T) {
	cases := []struct {
		from, to Status
		want     bool
	}{
		{StatusDraft, StatusReview, true},
		{StatusReview, StatusPublished, true},
		{StatusPublished, StatusDeprecated, true},
		{StatusDraft, StatusPublished, false},   // 跳过 review
		{StatusReview, StatusDeprecated, false}, // 未发布不可废弃
		{StatusDeprecated, StatusDraft, false},  // 废弃不可逆
		{StatusPublished, StatusDraft, false},   // 发布不可回退
		{StatusReview, StatusDraft, false},      // Spec 未定义驳回路径
	}
	for _, tc := range cases {
		if got := CanTransition(tc.from, tc.to); got != tc.want {
			t.Errorf("CanTransition(%s -> %s) = %v, want %v", tc.from, tc.to, got, tc.want)
		}
	}
}

func TestSkillLifecycleHappyPath(t *testing.T) {
	svc := NewService(newFakeStore())
	ctx := context.Background()

	s, err := svc.Create(ctx, &CreateRequest{
		SkillCode: "finance_report_gen", Version: "1.0.0",
		ConfigJSON: json.RawMessage(`{"prompt":"..."}`), CreatedBy: "alice",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// draft 可改配置
	if _, err = svc.UpdateConfig(ctx, s.ID, json.RawMessage(`{"prompt":"v2"}`)); err != nil {
		t.Fatalf("update draft config: %v", err)
	}

	if _, err = svc.SubmitForReview(ctx, s.ID); err != nil {
		t.Fatalf("submit: %v", err)
	}
	// review 后配置锁定
	if _, err = svc.UpdateConfig(ctx, s.ID, json.RawMessage(`{"prompt":"hack"}`)); err == nil {
		t.Fatal("config update should be rejected after submit")
	}

	pub, err := svc.Publish(ctx, s.ID, "bob")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if pub.Status != StatusPublished || pub.ReviewedBy == nil || *pub.ReviewedBy != "bob" {
		t.Fatalf("published state wrong: %+v", pub)
	}
	// published 后配置不可改
	if _, err = svc.UpdateConfig(ctx, s.ID, json.RawMessage(`{"prompt":"hack"}`)); err == nil {
		t.Fatal("config update should be rejected after publish")
	}

	dep, err := svc.Deprecate(ctx, s.ID)
	if err != nil {
		t.Fatalf("deprecate: %v", err)
	}
	if dep.Status != StatusDeprecated {
		t.Fatalf("expected deprecated, got %s", dep.Status)
	}
	// deprecated 终态不可再流转
	if _, err = svc.Deprecate(ctx, s.ID); err == nil {
		t.Fatal("deprecated is terminal; second deprecate must fail")
	}
}

func TestPublishRequiresReviewer(t *testing.T) {
	svc := NewService(newFakeStore())
	ctx := context.Background()
	s, _ := svc.Create(ctx, &CreateRequest{
		SkillCode: "s", Version: "1", ConfigJSON: json.RawMessage(`{}`), CreatedBy: "a",
	})
	_, _ = svc.SubmitForReview(ctx, s.ID)
	if _, err := svc.Publish(ctx, s.ID, ""); err == nil {
		t.Fatal("publish without reviewer must fail")
	}
}

func TestDuplicateVersionRejected(t *testing.T) {
	svc := NewService(newFakeStore())
	ctx := context.Background()
	req := &CreateRequest{SkillCode: "dup", Version: "1.0", ConfigJSON: json.RawMessage(`{}`), CreatedBy: "a"}
	if _, err := svc.Create(ctx, req); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, err := svc.Create(ctx, req); err != ErrDuplicateVersion {
		t.Fatalf("expected ErrDuplicateVersion, got %v", err)
	}
}
