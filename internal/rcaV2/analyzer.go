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

	topoIndex := make(map[string]*TopoNode)
	records := make([]*eventRecord, 0, len(events))
	for _, evt := range events {
		resolved, err := a.provider.ResolveEvent(ctx, evt)
		if err != nil {
			return Result{}, fmt.Errorf("resolve topology for %s/%s failed: %w", evt.AppName, evt.IP, err)
		}
		rec := &eventRecord{event: evt, eventID: buildEventID(evt)}
		records = append(records, rec)

		var child *TopoNode
		for _, node := range resolved {
			topo := ensureTopoNode(topoIndex, node)
			nodeRef := AlarmEventRef{ID: rec.eventID, RuleName: evt.RuleName, NodeType: node.NodeRef.Type, Occurred: evt.OccurredAt}
			topo.AddEvent(rec.eventID, nodeRef)
			if child != nil {
				topo.AttachChild(child)
				impactRef := AlarmEventRef{ID: rec.eventID, RuleName: evt.RuleName, NodeType: child.NodeRef.Type, Occurred: evt.OccurredAt}
				topo.AddImpact(child, impactRef)
			}
			child = topo
		}
	}

	candidates, paths, explained := a.evaluate(topoIndex)
	unexplained := collectUnexplained(records, explained)

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

type eventRecord struct {
	event   AlarmEvent
	eventID string
}

func buildEventID(evt AlarmEvent) string {
	return fmt.Sprintf("%s|%s|%s|%s|%s", evt.AppName, evt.ServerType, evt.Datacenter, evt.IP, evt.RuleName)
}

func ensureTopoNode(index map[string]*TopoNode, node Node) *TopoNode {
	if existing, ok := index[node.NodeRef.Key]; ok {
		// 合并 ChildCounts 以防后续查询补充基线
		if existing.ChildCounts == nil {
			existing.ChildCounts = make(map[NodeType]int)
		}
		for k, v := range node.ChildCounts {
			if v <= 0 {
				continue
			}
			existing.ChildCounts[k] = v
		}
		return existing
	}
	topo := NewTopoNode(node)
	if topo.ChildCounts == nil {
		topo.ChildCounts = make(map[NodeType]int)
	}
	index[node.NodeRef.Key] = topo
	return topo
}

func (a *Analyzer) evaluate(nodes map[string]*TopoNode) ([]Candidate, []AlarmPath, map[string]struct{}) {
	totalEvents := countEvents(nodes)
	nodesByType := make(map[NodeType][]*TopoNode)
	for _, n := range nodes {
		nodesByType[n.NodeRef.Type] = append(nodesByType[n.NodeRef.Type], n)
	}

	candidates := make([]Candidate, 0)
	paths := make([]AlarmPath, 0)
	explained := make(map[string]struct{})

	for _, level := range a.config.Hierarchy {
		sameType := nodesByType[level]
		if len(sameType) == 0 {
			continue
		}
		layerCfg, ok := a.config.Layers[level]
		if !ok {
			layerCfg = LayerConfig{CoverageThreshold: 0.6, MinChildren: 1, Weights: ScoreWeights{Coverage: 0.7, Impact: 0.3}}
		}
		sort.Slice(sameType, func(i, j int) bool { return sameType[i].NodeRef.Key < sameType[j].NodeRef.Key })

		for _, node := range sameType {
			if len(node.Events) == 0 {
				continue
			}
			coverage, activeChildren := node.Coverage()
			childCount := len(activeChildren)
			if childCount >= layerCfg.MinChildren && coverage >= layerCfg.CoverageThreshold {
				// 达标，允许继续向上扩散
				continue
			}

			score := node.ComputeScore(layerCfg.Weights, totalEvents)
			eventIDs := collectEventIDs(node.Events)
			candidate := Candidate{
				Node:       node.NodeRef,
				Confidence: score.Normalized,
				Coverage:   coverage,
				Reason:     "TOPOLOGY",
				Metrics:    score,
				Explained:  eventIDs,
			}
			candidates = append(candidates, candidate)
			paths = append(paths, buildPath(node))
			for _, id := range eventIDs {
				explained[id] = struct{}{}
			}
			node.SuppressUpwards(node.Events)
			for id := range node.Events {
				delete(node.Events, id)
			}
		}
	}

	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Confidence > candidates[j].Confidence })
	return candidates, mergePaths(paths), explained
}

func buildPath(node *TopoNode) AlarmPath {
	impacts := make([]PathImpact, 0, len(node.Impacts))
	for _, impact := range node.Impacts {
		if impact == nil || len(impact.Events) == 0 {
			continue
		}
		refs := collectEventMap(impact.Events)
		impacts = append(impacts, PathImpact{Node: impact.Node, Events: refs})
	}
	sort.Slice(impacts, func(i, j int) bool { return impacts[i].Node.Key < impacts[j].Node.Key })
	return AlarmPath{Candidate: node.NodeRef, Impacts: impacts}
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

func countEvents(nodes map[string]*TopoNode) int {
	seen := make(map[string]struct{})
	for _, n := range nodes {
		for id := range n.Events {
			seen[id] = struct{}{}
		}
	}
	return len(seen)
}

func collectUnexplained(records []*eventRecord, explained map[string]struct{}) []AlarmEvent {
	result := make([]AlarmEvent, 0)
	seen := make(map[string]struct{})
	for _, rec := range records {
		if _, ok := explained[rec.eventID]; ok {
			continue
		}
		if _, ok := seen[rec.eventID]; ok {
			continue
		}
		seen[rec.eventID] = struct{}{}
		result = append(result, rec.event)
	}
	return result
}
