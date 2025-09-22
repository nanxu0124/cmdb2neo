package rca

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeProvider struct {
	contexts map[string]AlarmContext
	err      error
}

func (f *fakeProvider) ResolveContext(_ context.Context, event AlarmEvent) (AlarmContext, error) {
	if f.err != nil {
		return AlarmContext{}, f.err
	}
	ctx, ok := f.contexts[event.ID]
	if !ok {
		return AlarmContext{}, errors.New("not found")
	}
	return ctx, nil
}

type fakeStore struct {
	saved bool
}

func (f *fakeStore) Save(context.Context, string, Result) error {
	f.saved = true
	return nil
}

func TestNewAnalyzerRequiresProvider(t *testing.T) {
	if _, err := NewAnalyzer(nil, nil, Config{}); err == nil {
		t.Fatalf("expected error when provider missing")
	}
}

func TestAnalyzerAnalyzeValidatesInput(t *testing.T) {
	provider := &fakeProvider{contexts: map[string]AlarmContext{}}
	analyzer, err := NewAnalyzer(provider, nil, Config{})
	if err != nil {
		t.Fatalf("new analyzer: %v", err)
	}
	if _, err := analyzer.Analyze(context.Background(), "window", nil); err == nil {
		t.Fatalf("expected error for empty events")
	}
}

func TestCollectContexts(t *testing.T) {
	base := time.Now()
	events := []AlarmEvent{{ID: "e1", Occurred: base, NodeType: NodeTypeApp}}
	provider := &fakeProvider{contexts: map[string]AlarmContext{
		"e1": {
			App:            &Node{NodeRef: NodeRef{CMDBKey: "APP_1", Type: NodeTypeApp, Name: "app"}},
			VirtualMachine: &Node{NodeRef: NodeRef{CMDBKey: "VM_1", Type: NodeTypeVirtualMachine}},
		},
	}}
	analyzer, _ := NewAnalyzer(provider, nil, Config{})
	ecs, err := analyzer.collectContexts(context.Background(), events)
	if err != nil {
		t.Fatalf("collect contexts: %v", err)
	}
	if len(ecs) != 1 {
		t.Fatalf("expected 1 context, got %d", len(ecs))
	}
	if len(ecs[0].chain) != 2 {
		t.Fatalf("unexpected chain size %d", len(ecs[0].chain))
	}
}

func TestContextToSliceOrder(t *testing.T) {
	ctx := AlarmContext{
		App:            &Node{NodeRef: NodeRef{CMDBKey: "APP"}},
		VirtualMachine: &Node{NodeRef: NodeRef{CMDBKey: "VM"}},
		HostMachine:    &Node{NodeRef: NodeRef{CMDBKey: "HOST"}},
	}
	nodes := contextToSlice(ctx)
	if got := len(nodes); got != 3 {
		t.Fatalf("expected 3 nodes, got %d", got)
	}
	if nodes[0].CMDBKey != "APP" || nodes[1].CMDBKey != "VM" || nodes[2].CMDBKey != "HOST" {
		t.Fatalf("unexpected order: %+v", nodes)
	}
}

func TestEvaluateGeneratesCandidates(t *testing.T) {
	now := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	event := AlarmEvent{ID: "e1", Occurred: now, NodeType: NodeTypeApp}
	chain := []*Node{
		{NodeRef: NodeRef{CMDBKey: "APP", Type: NodeTypeApp}},
		{NodeRef: NodeRef{CMDBKey: "VM", Type: NodeTypeVirtualMachine}, ChildCounts: map[NodeType]int{NodeTypeApp: 1}},
		{NodeRef: NodeRef{CMDBKey: "HOST", Type: NodeTypeHostMachine}, ChildCounts: map[NodeType]int{NodeTypeVirtualMachine: 1}},
	}
	ecs := []eventContext{{event: event, chain: chain}}
	analyzer, _ := NewAnalyzer(&fakeProvider{}, nil, DefaultConfig())
	result := analyzer.evaluate(ecs)
	if len(result.Candidates) == 0 {
		t.Fatalf("expected candidates")
	}
	if len(result.Paths) == 0 {
		t.Fatalf("expected alarm paths")
	}
}

func TestBuildNodeStates(t *testing.T) {
	now := time.Now()
	child := &Node{NodeRef: NodeRef{CMDBKey: "APP", Type: NodeTypeApp}}
	parent := &Node{NodeRef: NodeRef{CMDBKey: "VM", Type: NodeTypeVirtualMachine}, ChildCounts: map[NodeType]int{NodeTypeApp: 2}}
	ec := eventContext{event: AlarmEvent{ID: "evt", NodeType: NodeTypeApp, Occurred: now}, chain: []*Node{child, parent}}
	states := buildNodeStates([]eventContext{ec})
	st := states[parent.CMDBKey]
	cov, impacted := st.coverage()
	if cov <= 0 {
		t.Fatalf("coverage should be positive")
	}
	if len(impacted) != 1 {
		t.Fatalf("expected impacted child recorded")
	}
	if st.childType() != NodeTypeApp {
		t.Fatalf("child type mismatch")
	}
	score := st.computeScore(ScoreWeights{Coverage: 1}, now, now.Add(time.Minute), 1)
	if score.Coverage <= 0 {
		t.Fatalf("score coverage zero")
	}
	ids := st.eventIDs()
	if len(ids) != 1 || ids[0] != "evt" {
		t.Fatalf("event ids incorrect: %v", ids)
	}
	path := st.buildPath()
	if len(path.Impacts) != 1 {
		t.Fatalf("expected single impact")
	}
}

func TestFilterStatesMergeHelpers(t *testing.T) {
	n1 := &nodeState{Node: Node{NodeRef: NodeRef{CMDBKey: "A", Type: NodeTypeHostMachine}}}
	n2 := &nodeState{Node: Node{NodeRef: NodeRef{CMDBKey: "B", Type: NodeTypeVirtualMachine}}}
	states := map[string]*nodeState{"A": n1, "B": n2}
	filtered := filterStatesByType(states, NodeTypeHostMachine)
	if len(filtered) != 1 || filtered[0] != n1 {
		t.Fatalf("filter failed")
	}

	pathA := AlarmPath{Candidate: NodeRef{CMDBKey: "P"}, Impacts: []PathImpact{{Node: NodeRef{CMDBKey: "C1"}}}}
	pathB := AlarmPath{Candidate: NodeRef{CMDBKey: "P"}, Impacts: []PathImpact{{Node: NodeRef{CMDBKey: "C2"}}}}
	merged := mergePaths([]AlarmPath{pathA, pathB})
	if len(merged) != 1 || len(merged[0].Impacts) != 2 {
		t.Fatalf("merge paths failed: %+v", merged)
	}

	imp1 := []PathImpact{{Node: NodeRef{CMDBKey: "N1"}, Events: []AlarmEventRef{{ID: "1"}}}}
	imp2 := []PathImpact{{Node: NodeRef{CMDBKey: "N1"}, Events: []AlarmEventRef{{ID: "2"}}}}
	combined := mergeImpacts(imp1, imp2)
	if len(combined) != 1 || len(combined[0].Events) != 2 {
		t.Fatalf("merge impacts failed")
	}

	refs := mergeEventRefs([]AlarmEventRef{{ID: "1"}}, []AlarmEventRef{{ID: "1"}, {ID: "2"}})
	if len(refs) != 2 {
		t.Fatalf("expected deduplicated events")
	}
}

func TestWindowBoundsAndUnexplained(t *testing.T) {
	now := time.Now()
	ecs := []eventContext{
		{event: AlarmEvent{ID: "e1", Occurred: now}, chain: []*Node{}},
		{event: AlarmEvent{ID: "e2", Occurred: now.Add(time.Minute)}, chain: []*Node{}},
	}
	start, end := windowBounds(ecs)
	if !start.Equal(now) || !end.Equal(now.Add(time.Minute)) {
		t.Fatalf("window bounds mismatch")
	}

	explained := map[string]string{"e1": "node"}
	remaining := collectUnexplained(ecs, explained)
	if len(remaining) != 1 || remaining[0].ID != "e2" {
		t.Fatalf("expected only unexplained event e2")
	}
}
