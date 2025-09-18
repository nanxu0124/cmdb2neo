package rca

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"
)

// Analyzer 根因分析核心流程。
type Analyzer struct {
	provider TopologyProvider
	store    ResultStore
	config   Config
}

// NewAnalyzer 构建 Analyzer，store 允许为 nil 表示仅返回结果不持久化。
func NewAnalyzer(provider TopologyProvider, store ResultStore, cfg Config) (*Analyzer, error) {
	if provider == nil {
		return nil, errors.New("必须提供 TopologyProvider")
	}
	if len(cfg.Hierarchy) == 0 {
		cfg = DefaultConfig()
	}
	return &Analyzer{provider: provider, store: store, config: cfg}, nil
}

// Analyze 对一批告警事件执行根因分析。
func (a *Analyzer) Analyze(ctx context.Context, windowID string, events []AlarmEvent) (Result, error) {
	if len(events) == 0 {
		return Result{}, errors.New("空告警集合")
	}

	processed, err := a.collectContexts(ctx, events)
	if err != nil {
		return Result{}, err
	}

	result := a.evaluate(processed)

	if a.store != nil {
		if err := a.store.Save(ctx, windowID, result); err != nil {
			return Result{}, fmt.Errorf("持久化结果失败: %w", err)
		}
	}
	return result, nil
}

// eventContext 绑定原始事件与拓扑路径。
type eventContext struct {
	event AlarmEvent
	chain []*Node
}

// collectContexts 解析所有事件获取拓扑信息。
func (a *Analyzer) collectContexts(ctx context.Context, events []AlarmEvent) ([]eventContext, error) {
	result := make([]eventContext, 0, len(events))
	for _, evt := range events {
		ctx, err := a.provider.ResolveContext(ctx, evt)
		if err != nil {
			return nil, fmt.Errorf("解析告警 %s 失败: %w", evt.ID, err)
		}
		chain := contextToSlice(ctx)
		if len(chain) == 0 {
			continue
		}
		result = append(result, eventContext{event: evt, chain: chain})
	}
	return result, nil
}

func contextToSlice(ac AlarmContext) []*Node {
	ordered := []*Node{ac.App, ac.VirtualMachine, ac.HostMachine, ac.PhysicalMachine, ac.NetPartition, ac.IDC}
	out := make([]*Node, 0, len(ordered))
	for _, node := range ordered {
		if node != nil {
			clone := new(Node)
			*clone = *node
			out = append(out, clone)
		}
	}
	return out
}

// evaluate 对聚合后的事件执行自底向上的打分。
func (a *Analyzer) evaluate(events []eventContext) Result {
	nodeStates := buildNodeStates(events)
	windowStart, windowEnd := windowBounds(events)
	totalEvents := len(events)

	explainedNodes := make(map[string]bool)
	explainedEvents := make(map[string]string)

	candidates := make([]Candidate, 0)
	paths := make([]AlarmPath, 0)

	for _, level := range a.config.Hierarchy {
		layerCfg, ok := a.config.Layers[level]
		if !ok {
			layerCfg = LayerConfig{CoverageThreshold: 0.6, MinChildren: 1, Weights: ScoreWeights{Coverage: 0.6, TimeLead: 0.2, Impact: 0.15, Base: 0.05}}
		}

		nodes := filterStatesByType(nodeStates, level)
		sort.Slice(nodes, func(i, j int) bool { return nodes[i].NodeRef.CMDBKey < nodes[j].NodeRef.CMDBKey })
		for _, ns := range nodes {
			if explainedNodes[ns.NodeRef.CMDBKey] {
				continue
			}

			coverage, impactedChildren := ns.coverage()
			if ns.Node.Type == NodeTypeVirtualMachine && ns.childType() == NodeType("") {
				// 没有子节点信息，使用事件本身
				coverage = 1
			}

			if len(impactedChildren) < layerCfg.MinChildren {
				continue
			}
			if coverage < layerCfg.CoverageThreshold {
				continue
			}

			score := ns.computeScore(layerCfg.Weights, windowStart, windowEnd, totalEvents)
			candidates = append(candidates, Candidate{
				Node:       ns.NodeRef,
				Confidence: score.Normalized,
				Coverage:   coverage,
				Metrics:    score,
				Explained:  ns.eventIDs(),
			})

			explainedNodes[ns.NodeRef.CMDBKey] = true
			for childKey := range impactedChildren {
				explainedNodes[childKey] = true
			}
			for _, eid := range ns.eventIDs() {
				explainedEvents[eid] = ns.NodeRef.CMDBKey
			}

			paths = append(paths, ns.buildPath())
		}
	}

	unexplained := collectUnexplained(events, explainedEvents)

	return Result{
		Candidates:        candidates,
		Paths:             mergePaths(paths),
		UnexplainedEvents: unexplained,
	}
}

// nodeState 保存某节点聚合的告警信息。
type nodeState struct {
	Node
	childImpacts map[string]*childImpact
	eventMap     map[string]AlarmEvent
	earliest     time.Time
}

type childImpact struct {
	node     Node
	events   map[string]AlarmEvent
	earliest time.Time
}

func buildNodeStates(events []eventContext) map[string]*nodeState {
	states := make(map[string]*nodeState)
	for _, ec := range events {
		var child *Node
		for _, node := range ec.chain {
			st := ensureState(states, *node)
			if st.eventMap == nil {
				st.eventMap = make(map[string]AlarmEvent)
			}
			st.eventMap[ec.event.ID] = ec.event
			if st.earliest.IsZero() || ec.event.Occurred.Before(st.earliest) {
				st.earliest = ec.event.Occurred
			}

			if child != nil {
				st.addChildImpact(*child, ec.event)
			}
			child = node
		}
	}
	return states
}

func ensureState(states map[string]*nodeState, node Node) *nodeState {
	if st, ok := states[node.CMDBKey]; ok {
		return st
	}
	state := &nodeState{Node: node, childImpacts: make(map[string]*childImpact)}
	states[node.CMDBKey] = state
	return state
}

func (n *nodeState) addChildImpact(child Node, evt AlarmEvent) {
	imp, ok := n.childImpacts[child.CMDBKey]
	if !ok {
		imp = &childImpact{
			node:     child,
			events:   make(map[string]AlarmEvent),
			earliest: evt.Occurred,
		}
		n.childImpacts[child.CMDBKey] = imp
	}
	imp.events[evt.ID] = evt
	if evt.Occurred.Before(imp.earliest) {
		imp.earliest = evt.Occurred
	}
}

func (n *nodeState) coverage() (float64, map[string]struct{}) {
	if len(n.childImpacts) == 0 {
		return 0, nil
	}
	relevantChildType := n.childType()
	var total int
	if relevantChildType != "" && n.ChildCounts != nil {
		total = n.ChildCounts[relevantChildType]
	}
	if total == 0 {
		total = len(n.childImpacts)
	}
	impacted := len(n.childImpacts)
	coverage := float64(impacted) / float64(total)
	if coverage > 1 {
		coverage = 1
	}
	explained := make(map[string]struct{}, impacted)
	for key := range n.childImpacts {
		explained[key] = struct{}{}
	}
	return coverage, explained
}

func (n *nodeState) childType() NodeType {
	for _, impact := range n.childImpacts {
		return impact.node.Type
	}
	return NodeType("")
}

func (n *nodeState) computeScore(weights ScoreWeights, windowStart, windowEnd time.Time, totalEvents int) ScoreDetail {
	coverage, _ := n.coverage()
	eventCount := len(n.eventMap)
	impact := 0.0
	if totalEvents > 0 {
		impact = float64(eventCount) / float64(totalEvents)
	}

	lead := 0.0
	windowSpan := windowEnd.Sub(windowStart).Seconds()
	if windowSpan <= 0 {
		windowSpan = 1
	}
	if !n.earliest.IsZero() {
		lead = (windowEnd.Sub(n.earliest).Seconds()) / windowSpan
		if lead < 0 {
			lead = 0
		}
		if lead > 1 {
			lead = 1
		}
	}

	raw := weights.Base + weights.Coverage*coverage + weights.TimeLead*lead + weights.Impact*impact
	if raw < 0 {
		raw = 0
	}
	if raw > 1 {
		raw = 1
	}

	return ScoreDetail{
		Coverage:     coverage,
		TimeLead:     lead,
		Impact:       impact,
		Base:         weights.Base,
		RawScore:     raw,
		Normalized:   raw,
		WindowLength: windowSpan,
	}
}

func (n *nodeState) eventIDs() []string {
	ids := make([]string, 0, len(n.eventMap))
	for id := range n.eventMap {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (n *nodeState) buildPath() AlarmPath {
	impacts := make([]PathImpact, 0, len(n.childImpacts))
	for _, child := range n.childImpacts {
		events := make([]AlarmEventRef, 0, len(child.events))
		for _, evt := range child.events {
			events = append(events, AlarmEventRef{ID: evt.ID, NodeType: evt.NodeType, Occurred: evt.Occurred})
		}
		sort.Slice(events, func(i, j int) bool { return events[i].ID < events[j].ID })
		impacts = append(impacts, PathImpact{Node: child.node.NodeRef, Events: events})
	}
	sort.Slice(impacts, func(i, j int) bool { return impacts[i].Node.CMDBKey < impacts[j].Node.CMDBKey })
	return AlarmPath{Candidate: n.NodeRef, Impacts: impacts}
}

func filterStatesByType(states map[string]*nodeState, t NodeType) []*nodeState {
	result := make([]*nodeState, 0)
	for _, st := range states {
		if st.Node.Type == t {
			result = append(result, st)
		}
	}
	return result
}

func mergePaths(paths []AlarmPath) []AlarmPath {
	if len(paths) == 0 {
		return nil
	}
	group := make(map[string]*AlarmPath)
	for _, path := range paths {
		key := path.Candidate.CMDBKey
		if _, ok := group[key]; !ok {
			pathCopy := path
			group[key] = &pathCopy
			continue
		}
		existing := group[key]
		existing.Impacts = mergeImpacts(existing.Impacts, path.Impacts)
	}

	result := make([]AlarmPath, 0, len(group))
	for _, path := range group {
		sort.Slice(path.Impacts, func(i, j int) bool { return path.Impacts[i].Node.CMDBKey < path.Impacts[j].Node.CMDBKey })
		result = append(result, *path)
	}
	return result
}

func mergeImpacts(dst, src []PathImpact) []PathImpact {
	index := make(map[string]PathImpact, len(dst)+len(src))
	for _, imp := range dst {
		index[imp.Node.CMDBKey] = imp
	}
	for _, imp := range src {
		if existing, ok := index[imp.Node.CMDBKey]; ok {
			existing.Events = mergeEventRefs(existing.Events, imp.Events)
			index[imp.Node.CMDBKey] = existing
			continue
		}
		index[imp.Node.CMDBKey] = imp
	}
	result := make([]PathImpact, 0, len(index))
	for _, imp := range index {
		result = append(result, imp)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Node.CMDBKey < result[j].Node.CMDBKey })
	return result
}

func mergeEventRefs(dst, src []AlarmEventRef) []AlarmEventRef {
	seen := make(map[string]AlarmEventRef, len(dst)+len(src))
	for _, evt := range dst {
		seen[evt.ID] = evt
	}
	for _, evt := range src {
		if _, ok := seen[evt.ID]; !ok {
			seen[evt.ID] = evt
		}
	}
	result := make([]AlarmEventRef, 0, len(seen))
	for _, evt := range seen {
		result = append(result, evt)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func windowBounds(events []eventContext) (time.Time, time.Time) {
	if len(events) == 0 {
		return time.Time{}, time.Time{}
	}
	start := events[0].event.Occurred
	end := events[0].event.Occurred
	for _, ec := range events {
		if ec.event.Occurred.Before(start) {
			start = ec.event.Occurred
		}
		if ec.event.Occurred.After(end) {
			end = ec.event.Occurred
		}
	}
	return start, end
}

func collectUnexplained(events []eventContext, explained map[string]string) []AlarmEvent {
	result := make([]AlarmEvent, 0)
	seen := make(map[string]bool)
	for _, ec := range events {
		if seen[ec.event.ID] {
			continue
		}
		seen[ec.event.ID] = true
		if _, ok := explained[ec.event.ID]; ok {
			continue
		}
		result = append(result, ec.event)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}
