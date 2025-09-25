package rcav2

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type Analyzer struct {
	provider TopologyProvider
	config   Config
}

func NewAnalyzer(provider TopologyProvider, cfg Config) (*Analyzer, error) {
	if provider == nil {
		return nil, fmt.Errorf("topology provider is required")
	}
	if len(cfg.Hierarchy) == 0 {
		cfg = DefaultConfig()
	}
	return &Analyzer{provider: provider, config: cfg}, nil
}

func (a *Analyzer) Analyze(ctx context.Context, events []AlarmEvent) (Result, error) {
	if len(events) == 0 {
		return Result{}, fmt.Errorf("empty alarms")
	}

	appOutages := a.computeAppOutages(ctx, events)

	contexts, err := a.resolveChains(ctx, events)
	if err != nil {
		return Result{}, err
	}

	nodeStates := buildNodeStates(contexts)
	candidates, paths := a.evaluate(nodeStates)
	unexplained := collectUnexplained(contexts, candidates)

	return Result{
		AppOutages:        appOutages,
		Candidates:        candidates,
		Paths:             paths,
		UnexplainedEvents: unexplained,
	}, nil
}

// Stage A -------------------------------------------------

type appGroup struct {
	AppName string
	IDC     string
	Events  []AlarmEvent
}

func (a *Analyzer) computeAppOutages(ctx context.Context, events []AlarmEvent) []AppOutage {
	threshold := a.config.AppOutageThreshold
	if threshold <= 0 {
		threshold = 0.6
	}

	groups := make(map[string]*appGroup)
	for _, evt := range events {
		if strings.TrimSpace(evt.AppName) == "" {
			continue
		}
		key := evt.AppName + "|" + evt.Datacenter
		grp, ok := groups[key]
		if !ok {
			grp = &appGroup{AppName: evt.AppName, IDC: evt.Datacenter}
			groups[key] = grp
		}
		grp.Events = append(grp.Events, evt)
	}

	outages := make([]AppOutage, 0, len(groups))
	for _, grp := range groups {
		if len(grp.Events) == 0 {
			continue
		}

		total, err := a.provider.ListAppInstances(ctx, grp.AppName, grp.IDC)
		if err != nil {
			continue
		}
		if total <= 0 {
			continue
		}

		nodes := collapseAlarmedNodes(grp.Events)
		if len(nodes) == 0 {
			continue
		}

		coverage := float64(len(nodes)) / float64(total)
		if coverage < threshold {
			continue
		}

		affected := make([]AppOutageNode, 0, len(nodes))
		for _, node := range nodes {
			affected = append(affected, node)
		}
		sort.Slice(affected, func(i, j int) bool {
			if affected[i].ServerType == affected[j].ServerType {
				return affected[i].IP < affected[j].IP
			}
			return affected[i].ServerType < affected[j].ServerType
		})

		outages = append(outages, AppOutage{
			AppName:       grp.AppName,
			Datacenter:    grp.IDC,
			TotalNodes:    total,
			AlarmedNodes:  len(nodes),
			Coverage:      coverage,
			Threshold:     threshold,
			AffectedNodes: affected,
		})
	}

	sort.Slice(outages, func(i, j int) bool {
		if outages[i].Coverage == outages[j].Coverage {
			if outages[i].AppName == outages[j].AppName {
				return outages[i].Datacenter < outages[j].Datacenter
			}
			return outages[i].AppName < outages[j].AppName
		}
		return outages[i].Coverage > outages[j].Coverage
	})

	return outages
}

func collapseAlarmedNodes(events []AlarmEvent) map[string]AppOutageNode {
	if len(events) == 0 {
		return nil
	}
	type nodeSummary struct {
		node  AppOutageNode
		rules map[string]struct{}
	}
	summaries := make(map[string]*nodeSummary)
	for _, evt := range events {
		key := normalizeEventKey(evt)
		summary, ok := summaries[key]
		if !ok {
			summary = &nodeSummary{
				node: AppOutageNode{
					ServerType: evt.ServerType,
					IP:         evt.IP,
					HostIP:     evt.HostIP,
					Partition:  evt.NetworkPartition,
				},
				rules: make(map[string]struct{}),
			}
			summaries[key] = summary
		}
		if evt.RuleName != "" {
			summary.rules[evt.RuleName] = struct{}{}
		}
	}
	result := make(map[string]AppOutageNode, len(summaries))
	for key, summary := range summaries {
		summary.node.RuleNames = sortedStrings(summary.rules)
		result[key] = summary.node
	}
	return result
}

func normalizeEventKey(evt AlarmEvent) string {
	switch evt.ServerType {
	case ServerTypeHost, ServerTypePhysical:
		if evt.IP != "" {
			return string(evt.ServerType) + ":" + evt.IP + ":" + evt.Datacenter
		}
		return string(evt.ServerType) + ":" + evt.HostIP + ":" + evt.Datacenter
	default:
		return string(evt.ServerType) + ":" + evt.IP + ":" + evt.Datacenter
	}
}

func sortedStrings(m map[string]struct{}) []string {
	if len(m) == 0 {
		return nil
	}
	res := make([]string, 0, len(m))
	for k := range m {
		res = append(res, k)
	}
	sort.Strings(res)
	return res
}

// Stage B -------------------------------------------------

type chainWithEvent struct {
	event   AlarmEvent
	eventID string
	chain   Chain
}

func (a *Analyzer) resolveChains(ctx context.Context, events []AlarmEvent) ([]*chainWithEvent, error) {
	result := make([]*chainWithEvent, 0, len(events))
	for _, evt := range events {
		chain, err := a.provider.ResolveChain(ctx, evt)
		if err != nil {
			return nil, fmt.Errorf("resolve chain for %s/%s failed: %w", evt.AppName, evt.IP, err)
		}
		result = append(result, &chainWithEvent{
			event:   evt,
			eventID: buildEventID(evt),
			chain:   chain,
		})
	}
	return result, nil
}

func buildEventID(evt AlarmEvent) string {
	return fmt.Sprintf("%s|%s|%s|%s|%s", evt.AppName, evt.ServerType, evt.Datacenter, evt.IP, evt.RuleName)
}

type nodeState struct {
	Node
	childImpacts map[string]*childImpact
	eventMap     map[string]AlarmEventRef
}

type childImpact struct {
	node   Node
	events map[string]AlarmEventRef
}

func buildNodeStates(contexts []*chainWithEvent) map[string]*nodeState {
	states := make(map[string]*nodeState)
	for _, ce := range contexts {
		ordered := chainToSlice(ce.chain)
		var parent *Node
		for _, node := range ordered {
			st := ensureState(states, *node)
			if st.eventMap == nil {
				st.eventMap = make(map[string]AlarmEventRef)
			}
			st.eventMap[ce.eventID] = AlarmEventRef{ID: ce.eventID, RuleName: ce.event.RuleName, NodeType: node.Type}

			if parent != nil {
				p := ensureState(states, *parent)
				p.addChildImpact(*node, ce)
			}
			parent = node
		}
	}
	return states
}

func chainToSlice(chain Chain) []*Node {
	ordered := []*Node{chain.App, chain.VirtualMachine, chain.HostMachine, chain.PhysicalMachine, chain.NetPartition, chain.IDC}
	result := make([]*Node, 0, len(ordered))
	for _, node := range ordered {
		if node != nil {
			clone := *node
			result = append(result, &clone)
		}
	}
	return result
}

func ensureState(states map[string]*nodeState, node Node) *nodeState {
	if st, ok := states[node.NodeRef.Key]; ok {
		return st
	}
	state := &nodeState{Node: node, childImpacts: make(map[string]*childImpact)}
	states[node.NodeRef.Key] = state
	return state
}

func (n *nodeState) addChildImpact(child Node, ce *chainWithEvent) {
	impact, ok := n.childImpacts[child.NodeRef.Key]
	if !ok {
		impact = &childImpact{node: child, events: make(map[string]AlarmEventRef)}
		n.childImpacts[child.NodeRef.Key] = impact
	}
	impact.events[ce.eventID] = AlarmEventRef{ID: ce.eventID, RuleName: ce.event.RuleName, NodeType: child.Type}
}

func (n *nodeState) coverage() (float64, map[string]struct{}) {
	if len(n.childImpacts) == 0 {
		return 0, nil
	}
	childType := n.childType()
	total := n.ChildCounts[childType]
	if total <= 0 {
		total = len(n.childImpacts)
	}
	cov := float64(len(n.childImpacts)) / float64(total)
	if cov > 1 {
		cov = 1
	}
	explained := make(map[string]struct{}, len(n.childImpacts))
	for key := range n.childImpacts {
		explained[key] = struct{}{}
	}
	return cov, explained
}

func (n *nodeState) childType() NodeType {
	for _, impact := range n.childImpacts {
		return impact.node.Type
	}
	return NodeType("")
}

func (n *nodeState) computeScore(weights ScoreWeights, totalEvents int) ScoreDetail {
	coverage, _ := n.coverage()
	impact := 0.0
	if totalEvents > 0 {
		impact = float64(len(n.eventMap)) / float64(totalEvents)
	}
	raw := weights.Base + weights.Coverage*coverage + weights.Impact*impact
	if raw < 0 {
		raw = 0
	}
	if raw > 1 {
		raw = 1
	}
	return ScoreDetail{
		Coverage:   coverage,
		Impact:     impact,
		Base:       weights.Base,
		RawScore:   raw,
		Normalized: raw,
	}
}

func (a *Analyzer) evaluate(states map[string]*nodeState) ([]Candidate, []AlarmPath) {
	candidates := make([]Candidate, 0)
	paths := make([]AlarmPath, 0)
	totalEvents := countEvents(states)

	for _, level := range a.config.Hierarchy {
		layerCfg, ok := a.config.Layers[level]
		if !ok {
			layerCfg = LayerConfig{CoverageThreshold: 0.6, MinChildren: 1, Weights: ScoreWeights{Coverage: 0.7, Impact: 0.3}}
		}

		nodes := filterStates(states, level)
		sort.Slice(nodes, func(i, j int) bool { return nodes[i].NodeRef.Key < nodes[j].NodeRef.Key })
		for _, st := range nodes {
			coverage, explained := st.coverage()
			if len(explained) < layerCfg.MinChildren {
				continue
			}
			if coverage < layerCfg.CoverageThreshold {
				continue
			}

			score := st.computeScore(layerCfg.Weights, totalEvents)
			candidate := Candidate{
				Node:       st.NodeRef,
				Confidence: score.Normalized,
				Coverage:   coverage,
				Reason:     "TOPOLOGY",
				Metrics:    score,
				Explained:  collectEventIDs(st.eventMap),
			}
			candidates = append(candidates, candidate)
			paths = append(paths, buildPath(st))
		}
	}

	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Confidence > candidates[j].Confidence })
	return candidates, mergePaths(paths)
}

func filterStates(states map[string]*nodeState, t NodeType) []*nodeState {
	result := make([]*nodeState, 0)
	for _, st := range states {
		if st.NodeRef.Type == t {
			result = append(result, st)
		}
	}
	return result
}

func buildPath(state *nodeState) AlarmPath {
	impacts := make([]PathImpact, 0, len(state.childImpacts))
	for _, impact := range state.childImpacts {
		refs := collectEventMap(impact.events)
		impacts = append(impacts, PathImpact{Node: impact.node.NodeRef, Events: refs})
	}
	sort.Slice(impacts, func(i, j int) bool { return impacts[i].Node.Key < impacts[j].Node.Key })
	return AlarmPath{Candidate: state.NodeRef, Impacts: impacts}
}

func mergePaths(paths []AlarmPath) []AlarmPath {
	if len(paths) == 0 {
		return nil
	}
	merged := make(map[string]*AlarmPath)
	for _, path := range paths {
		key := path.Candidate.Key
		existing, ok := merged[key]
		if !ok {
			copy := path
			merged[key] = &copy
			continue
		}
		existing.Impacts = mergeImpacts(existing.Impacts, path.Impacts)
	}
	result := make([]AlarmPath, 0, len(merged))
	for _, path := range merged {
		sort.Slice(path.Impacts, func(i, j int) bool { return path.Impacts[i].Node.Key < path.Impacts[j].Node.Key })
		result = append(result, *path)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Candidate.Key < result[j].Candidate.Key })
	return result
}

func mergeImpacts(dst, src []PathImpact) []PathImpact {
	index := make(map[string]*PathImpact, len(dst))
	for i := range dst {
		index[dst[i].Node.Key] = &dst[i]
	}
	for _, imp := range src {
		if existing, ok := index[imp.Node.Key]; ok {
			existing.Events = mergeEventLists(existing.Events, imp.Events)
			continue
		}
		dst = append(dst, imp)
		index[imp.Node.Key] = &dst[len(dst)-1]
	}
	return dst
}

func mergeEventLists(a, b []AlarmEventRef) []AlarmEventRef {
	seen := make(map[string]AlarmEventRef, len(a)+len(b))
	for _, evt := range a {
		seen[evt.ID] = evt
	}
	for _, evt := range b {
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

func collectEventMap(events map[string]AlarmEventRef) []AlarmEventRef {
	refs := make([]AlarmEventRef, 0, len(events))
	for _, evt := range events {
		refs = append(refs, evt)
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].ID < refs[j].ID })
	return refs
}

func collectEventIDs(events map[string]AlarmEventRef) []string {
	ids := make([]string, 0, len(events))
	for id := range events {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func countEvents(states map[string]*nodeState) int {
	seen := make(map[string]struct{})
	for _, st := range states {
		for id := range st.eventMap {
			seen[id] = struct{}{}
		}
	}
	return len(seen)
}

func collectUnexplained(contexts []*chainWithEvent, candidates []Candidate) []AlarmEvent {
	explained := make(map[string]struct{})
	for _, cand := range candidates {
		for _, id := range cand.Explained {
			explained[id] = struct{}{}
		}
	}
	result := make([]AlarmEvent, 0)
	seen := make(map[string]struct{})
	for _, ce := range contexts {
		if _, ok := explained[ce.eventID]; ok {
			continue
		}
		if _, ok := seen[ce.eventID]; ok {
			continue
		}
		seen[ce.eventID] = struct{}{}
		result = append(result, ce.event)
	}
	return result
}
