package agent

import (
	"context"
	"fmt"
	"testing"
)

// fakeExperimentRouter M5-C 路由决策替身:固定输出预设决策。
type fakeExperimentRouter struct {
	route ExperimentRoute
	err   error
	calls int
}

func (f *fakeExperimentRouter) ResolveRoute(ctx context.Context, tenantID, app, graphKey, runID string) (ExperimentRoute, error) {
	f.calls++
	return f.route, f.err
}

func (f *fakeExperimentRouter) RecordShadowExecution(ctx context.Context, tenantID, ruleID, primaryRunID, shadowRunID string) error {
	return nil
}

func experimentTestGraph(key string) *Graph {
	return &Graph{GraphKey: key, Version: "v1", BusinessAppCode: "finance", Status: "active"}
}

func TestResolveExperimentGraphWithoutRouter(t *testing.T) {
	g := &Gateway{repo: &fakeGatewayRepo{graph: experimentTestGraph("g1")}}
	route, graph, err := g.resolveExperimentGraph(context.Background(), &AgentRunPayload{GraphKey: "g1", BusinessAppCode: "finance"}, "run-1")
	if err != nil {
		t.Fatalf("resolveExperimentGraph: %v", err)
	}
	if route.ResolvedGraphKey != "g1" || graph.GraphKey != "g1" {
		t.Fatalf("no router must passthrough baseline, got route=%+v graph=%s", route, graph.GraphKey)
	}
}

func TestResolveExperimentGraphCanaryRedirect(t *testing.T) {
	g := &Gateway{repo: &fakeGatewayRepo{graphs: map[string]*Graph{
		"g1": experimentTestGraph("g1"),
		"g2": experimentTestGraph("g2"),
	}}}
	g.SetExperimentRouter(&fakeExperimentRouter{route: ExperimentRoute{
		ResolvedGraphKey: "g2", CanaryReleaseID: "rel-1",
	}})
	route, graph, err := g.resolveExperimentGraph(context.Background(), &AgentRunPayload{GraphKey: "g1", BusinessAppCode: "finance"}, "run-1")
	if err != nil {
		t.Fatalf("resolveExperimentGraph: %v", err)
	}
	if graph.GraphKey != "g2" || route.CanaryReleaseID != "rel-1" {
		t.Fatalf("canary hit must redirect to candidate, got graph=%s route=%+v", graph.GraphKey, route)
	}
}

func TestResolveExperimentGraphCanaryCandidateInactive(t *testing.T) {
	inactive := experimentTestGraph("g2")
	inactive.Status = "disabled"
	g := &Gateway{repo: &fakeGatewayRepo{graphs: map[string]*Graph{
		"g1": experimentTestGraph("g1"),
		"g2": inactive,
	}}}
	g.SetExperimentRouter(&fakeExperimentRouter{route: ExperimentRoute{ResolvedGraphKey: "g2"}})
	if _, _, err := g.resolveExperimentGraph(context.Background(), &AgentRunPayload{GraphKey: "g1", BusinessAppCode: "finance"}, "run-1"); err == nil {
		t.Fatalf("inactive candidate must be rejected")
	}
}

func TestResolveExperimentGraphCanaryCrossBusinessApp(t *testing.T) {
	foreign := experimentTestGraph("g2")
	foreign.BusinessAppCode = "hr"
	g := &Gateway{repo: &fakeGatewayRepo{graphs: map[string]*Graph{
		"g1": experimentTestGraph("g1"),
		"g2": foreign,
	}}}
	g.SetExperimentRouter(&fakeExperimentRouter{route: ExperimentRoute{ResolvedGraphKey: "g2"}})
	if _, _, err := g.resolveExperimentGraph(context.Background(), &AgentRunPayload{GraphKey: "g1", BusinessAppCode: "finance"}, "run-1"); err == nil {
		t.Fatalf("cross-business-app candidate must be rejected")
	}
}

func TestResolveExperimentGraphBaselineMissing(t *testing.T) {
	g := &Gateway{repo: &fakeGatewayRepo{graphErr: fmt.Errorf("not found")}}
	if _, _, err := g.resolveExperimentGraph(context.Background(), &AgentRunPayload{GraphKey: "missing"}, "run-1"); err == nil {
		t.Fatalf("missing baseline graph must fail even with router redirect")
	}
}

func TestResolveExperimentGraphRouterError(t *testing.T) {
	g := &Gateway{repo: &fakeGatewayRepo{graph: experimentTestGraph("g1")}}
	g.SetExperimentRouter(&fakeExperimentRouter{err: fmt.Errorf("db down")})
	if _, _, err := g.resolveExperimentGraph(context.Background(), &AgentRunPayload{GraphKey: "g1"}, "run-1"); err == nil {
		t.Fatalf("router error must propagate (fail-safe: no silent direct routing)")
	}
}

func TestResolveExperimentGraphSameKeyNoCandidateLookup(t *testing.T) {
	repo := &fakeGatewayRepo{graph: experimentTestGraph("g1")}
	g := &Gateway{repo: repo}
	router := &fakeExperimentRouter{route: ExperimentRoute{ResolvedGraphKey: "g1"}}
	g.SetExperimentRouter(router)
	route, graph, err := g.resolveExperimentGraph(context.Background(), &AgentRunPayload{GraphKey: "g1"}, "run-1")
	if err != nil {
		t.Fatalf("resolveExperimentGraph: %v", err)
	}
	if route.ResolvedGraphKey != "g1" || graph.GraphKey != "g1" {
		t.Fatalf("same-key route must keep baseline, got %+v", route)
	}
}
