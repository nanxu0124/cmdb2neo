package rcav1

import (
	"context"
	"fmt"
	"sort"
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

	chains, err := a.resolveChains(ctx, events)
	if err != nil {
		return Result{}, err
	}

	stageA := a.detectAppOutage(ctx, chains)

	nodeStates := buildNodeStates(chains, stageA)
	result := a.evaluate(nodeStates, stageA)
	result.UnexplainedEvents = collectUnexplained(chains, result.Candidates)
	return result, nil
}

type chainWithEvent struct {
	event   AlarmEvent
	eventID string
	chain   Chain
	seed    bool
	reason  string
}

type outageGroup struct {
	Key        string
	AppName    string
	ServerType ServerType
	IDC        string
	Events     []*chainWithEvent
	Expected   map[string]Instance
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

func (a *Analyzer) detectAppOutage(ctx context.Context, chains []*chainWithEvent) map[string]stageAnomaly {
	groups := make(map[string]*outageGroup)
	for _, ce := range chains {
		key := fmt.Sprintf("%s|%s|%s", ce.event.AppName, ce.event.ServerType, ce.event.Datacenter)
		grp, ok := groups[key]
		if !ok {
			grp = &outageGroup{
				Key:        key,
				AppName:    ce.event.AppName,
				ServerType: ce.event.ServerType,
				IDC:        ce.event.Datacenter,
				Events:     []*chainWithEvent{},
				Expected:   map[string]Instance{},
			}
			groups[key] = grp
		}
		grp.Events = append(grp.Events, ce)
	}

	anomalies := make(map[string]stageAnomaly)
	for _, grp := range groups {
		instances, err := a.provider.ListAppInstances(ctx, grp.AppName, grp.ServerType, grp.IDC)
		if err != nil {
			continue
		}
		expected := make(map[string]Instance)
		for _, inst := range instances {
			expected[normalizeNodeKey(inst)] = inst
		}
		grp.Expected = expected
		if len(expected) == 0 {
			continue
		}

		alarmed := make(map[string]*chainWithEvent)
		for _, ce := range grp.Events {
			key := normalizeNodeKey(Instance{
				ServerType: grp.ServerType,
				IP:         ce.event.IP,
				HostIP:     ce.event.HostIP,
			})
			alarmed[key] = ce
		}

		fullOutage := len(alarmed) == len(expected)
		if !fullOutage && a.config.RequireFullMatch {
			continue
		}

		for key, inst := range expected {
			ce := alarmed[key]
			if ce == nil {
				continue
			}
			node := pickNodeFromChain(ce.chain, inst.ServerType)
			if node == nil {
				continue
			}
			anomalies[node.NodeRef.Key] = stageAnomaly{
				Node:    *node,
				Reason:  "APP_FULL_OUTAGE",
				EventID: ce.eventID,
			}
		}
	}

	return anomalies
}

type stageAnomaly struct {
	Node    Node
	Reason  string
	EventID string
}

func pickNodeFromChain(chain Chain, serverType ServerType) *Node {
	switch serverType {
	case ServerTypeHost:
		return chain.HostMachine
	case ServerTypePhysical:
		if chain.PhysicalMachine != nil {
			return chain.PhysicalMachine
		}
		return chain.HostMachine
	default:
		if chain.VirtualMachine != nil {
			return chain.VirtualMachine
		}
		return chain.HostMachine
	}
}

func normalizeNodeKey(inst Instance) string {
	switch inst.ServerType {
	case ServerTypeHost, ServerTypePhysical:
		if inst.IP != "" {
			return string(inst.ServerType) + ":" + inst.IP
		}
		return string(inst.ServerType) + ":" + inst.HostIP
	default:
		return string(inst.ServerType) + ":" + inst.IP
	}
}

func buildEventID(evt AlarmEvent) string {
	return fmt.Sprintf("%s|%s|%s|%s|%s", evt.AppName, evt.ServerType, evt.Datacenter, evt.IP, evt.RuleName)
}

// ---------------- Stage B evaluation ----------------

type nodeState struct {
	Node
	childImpacts map[string]*childImpact
	eventMap     map[string]AlarmEventRef
}

type childImpact struct {
	node   Node
	events map[string]AlarmEventRef
}

func buildNodeStates(chains []*chainWithEvent, seeds map[string]stageAnomaly) map[string]*nodeState {
	states := make(map[string]*nodeState)
	for _, ce := range chains {
		ordered := chainToSlice(ce)
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

func chainToSlice(ce *chainWithEvent) []*Node {
	chain := ce.chain
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

func (a *Analyzer) evaluate(states map[string]*nodeState, seeds map[string]stageAnomaly) Result {
	candidates := make([]Candidate, 0)
	paths := make([]AlarmPath, 0)
	totalEvents := countEvents(states)
	explainedEvents := make(map[string]struct{})

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
			reason := "TOPOLOGY"
			if seed, ok := seeds[st.NodeRef.Key]; ok && seed.Reason != "" {
				reason = seed.Reason
			}

			candidate := Candidate{
				Node:       st.NodeRef,
				Confidence: score.Normalized,
				Coverage:   coverage,
				Reason:     reason,
				Metrics:    score,
				Explained:  collectEventIDs(st.eventMap),
			}
			candidates = append(candidates, candidate)
			paths = append(paths, buildPath(st))

			for id := range st.eventMap {
				explainedEvents[id] = struct{}{}
			}
		}
	}

	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Confidence > candidates[j].Confidence })
	return Result{Candidates: candidates, Paths: mergePaths(paths)}
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
