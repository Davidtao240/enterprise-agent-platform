package experiment

import (
	"context"
	"fmt"
	"testing"
)

// fakeStore routerStore 测试替身(无数据库)。
type fakeStore struct {
	canary  *CanaryRelease
	shadow  *ShadowRule
	inserted []*ShadowExecution
}

func (f *fakeStore) FindActiveCanaryRelease(ctx context.Context, tenantID, app, graphKey string) (*CanaryRelease, error) {
	return f.canary, nil
}

func (f *fakeStore) FindActiveShadowRule(ctx context.Context, tenantID, app, graphKey string) (*ShadowRule, error) {
	return f.shadow, nil
}

func (f *fakeStore) InsertShadowExecution(ctx context.Context, exec *ShadowExecution) error {
	f.inserted = append(f.inserted, exec)
	return nil
}

func TestSampleHitDeterministic(t *testing.T) {
	runID := "run-abc-123"
	first := sampleHit(runID, "canary", 50)
	for i := 0; i < 10; i++ {
		if got := sampleHit(runID, "canary", 50); got != first {
			t.Fatalf("sampleHit not deterministic for %s: %v vs %v", runID, got, first)
		}
	}
	if !sampleHit(runID, "canary", 100) {
		t.Fatalf("percent=100 must always hit")
	}
	if sampleHit(runID, "canary", 0) {
		t.Fatalf("percent=0 must never hit")
	}
}

func TestSampleHitDistribution(t *testing.T) {
	// percent=50 时,1000 个不同 run_id 的命中率应在合理区间(确定性哈希,
	// 无随机波动;断言宽松区间防止 FNV 偏斜)。
	hits := 0
	for i := 0; i < 1000; i++ {
		if sampleHit(fmt.Sprintf("run-%d", i), "canary", 50) {
			hits++
		}
	}
	if hits < 400 || hits > 600 {
		t.Fatalf("sampleHit distribution skewed: %d/1000 hits at 50%%", hits)
	}
}

func TestResolveRoutePassthroughWithoutRules(t *testing.T) {
	router := NewRouter(&fakeStore{})
	route, err := router.ResolveRoute(context.Background(), "t1", "finance", "g1", "run-1")
	if err != nil {
		t.Fatalf("ResolveRoute: %v", err)
	}
	if route.ResolvedGraphKey != "g1" || route.CanaryReleaseID != "" || route.ShadowGraphKey != "" {
		t.Fatalf("expected passthrough, got %+v", route)
	}
}

func TestResolveRouteNilRepo(t *testing.T) {
	router := NewRouter(nil)
	route, err := router.ResolveRoute(context.Background(), "t1", "finance", "g1", "run-1")
	if err != nil {
		t.Fatalf("ResolveRoute: %v", err)
	}
	if route.ResolvedGraphKey != "g1" {
		t.Fatalf("nil repo must direct-route, got %+v", route)
	}
}

func TestResolveRouteCanaryFullStage(t *testing.T) {
	store := &fakeStore{canary: &CanaryRelease{
		ID: "rel-1", CandidateGraphKey: "g2", StagesJSON: "[1,5,20,100]", CurrentStageIndex: 3, // 100%
	}}
	route, err := NewRouter(store).ResolveRoute(context.Background(), "t1", "finance", "g1", "run-1")
	if err != nil {
		t.Fatalf("ResolveRoute: %v", err)
	}
	if route.ResolvedGraphKey != "g2" || route.CanaryReleaseID != "rel-1" {
		t.Fatalf("canary 100%% stage must route to candidate, got %+v", route)
	}
	if route.ShadowGraphKey != "" {
		t.Fatalf("no shadow rule configured, got %+v", route)
	}
}

func TestResolveRouteCanaryZeroStage(t *testing.T) {
	store := &fakeStore{canary: &CanaryRelease{
		ID: "rel-1", CandidateGraphKey: "g2", StagesJSON: "[1,5,20,100]", CurrentStageIndex: 0, // 1%
	}}
	router := NewRouter(store)
	// 1% 阶梯下,绝大多数 run 应直投基线。
	redirected := 0
	for i := 0; i < 1000; i++ {
		route, err := router.ResolveRoute(context.Background(), "t1", "finance", "g1", fmt.Sprintf("run-%d", i))
		if err != nil {
			t.Fatalf("ResolveRoute: %v", err)
		}
		if route.ResolvedGraphKey == "g2" {
			redirected++
		}
	}
	if redirected > 50 {
		t.Fatalf("1%% stage redirected too many runs: %d/1000", redirected)
	}
}

func TestResolveRouteCanaryStageIndexOutOfBounds(t *testing.T) {
	store := &fakeStore{canary: &CanaryRelease{
		ID: "rel-1", CandidateGraphKey: "g2", StagesJSON: "[1,5,20,100]", CurrentStageIndex: 9,
	}}
	route, err := NewRouter(store).ResolveRoute(context.Background(), "t1", "finance", "g1", "run-1")
	if err != nil {
		t.Fatalf("ResolveRoute: %v", err)
	}
	if route.ResolvedGraphKey != "g1" {
		t.Fatalf("out-of-bounds stage must converge to baseline (percent=0), got %+v", route)
	}
}

func TestResolveRouteShadowFullPercent(t *testing.T) {
	store := &fakeStore{shadow: &ShadowRule{
		ID: "rule-1", ShadowGraphKey: "g2", TrafficPercent: 100,
	}}
	route, err := NewRouter(store).ResolveRoute(context.Background(), "t1", "finance", "g1", "run-1")
	if err != nil {
		t.Fatalf("ResolveRoute: %v", err)
	}
	if route.ShadowGraphKey != "g2" || route.ShadowRuleID != "rule-1" {
		t.Fatalf("shadow 100%% must always sample, got %+v", route)
	}
	if route.ResolvedGraphKey != "g1" || route.CanaryReleaseID != "" {
		t.Fatalf("shadow sampling must not change primary routing, got %+v", route)
	}
}

func TestResolveRouteCanaryAndShadowIndependent(t *testing.T) {
	store := &fakeStore{
		canary: &CanaryRelease{ID: "rel-1", CandidateGraphKey: "g2", StagesJSON: "[100]", CurrentStageIndex: 0},
		shadow: &ShadowRule{ID: "rule-1", ShadowGraphKey: "g3", TrafficPercent: 100},
	}
	route, err := NewRouter(store).ResolveRoute(context.Background(), "t1", "finance", "g1", "run-1")
	if err != nil {
		t.Fatalf("ResolveRoute: %v", err)
	}
	if route.ResolvedGraphKey != "g2" || route.ShadowGraphKey != "g3" {
		t.Fatalf("canary and shadow must coexist independently, got %+v", route)
	}
}

func TestRecordShadowExecution(t *testing.T) {
	store := &fakeStore{}
	router := NewRouter(store)
	if err := router.RecordShadowExecution(context.Background(), "t1", "rule-1", "primary-1", "shadow-1"); err != nil {
		t.Fatalf("RecordShadowExecution: %v", err)
	}
	if len(store.inserted) != 1 || store.inserted[0].PrimaryRunID != "primary-1" || store.inserted[0].ShadowRunID != "shadow-1" {
		t.Fatalf("unexpected executions recorded: %+v", store.inserted)
	}
	// nil repo 时为安全空操作。
	if err := NewRouter(nil).RecordShadowExecution(context.Background(), "t1", "rule-1", "p", "s"); err != nil {
		t.Fatalf("nil repo RecordShadowExecution: %v", err)
	}
}

func TestCurrentStagePercent(t *testing.T) {
	c := &CanaryRelease{StagesJSON: "[1,5,20,100]"}
	c.CurrentStageIndex = 2
	if got := c.CurrentStagePercent(); got != 20 {
		t.Fatalf("CurrentStagePercent = %d, want 20", got)
	}
	c.CurrentStageIndex = -1
	if got := c.CurrentStagePercent(); got != 0 {
		t.Fatalf("negative index must yield 0, got %d", got)
	}
}
