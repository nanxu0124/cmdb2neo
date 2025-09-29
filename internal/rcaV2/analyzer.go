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

	candidates, paths, err := a.evaluate(topoIndex)
	if err != nil {
		return Result{}, err
	}

	res := Result{
		AppOutages: appOutages,
		Candidates: candidates,
		Paths:      paths,
	}
	res.Prompt = RenderPrompt(res, DefaultPromptOptions())
	return res, nil
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

func (a *Analyzer) evaluate(nodes map[string]*TopoNode) ([]Candidate, []AlarmPath, error) {

	// 只保留最上层的节点
	for _, v := range nodes {
		if v.Parent != nil {
			delete(nodes, v.NodeRef.Key)
		}
	}

	candidates := make([]Candidate, 0)
	paths := make([]AlarmPath, 0)
	for _, root := range nodes {
		a.postOrderEvaluate(root, &candidates, &paths)
	}

	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Confidence > candidates[j].Confidence })
	sort.Slice(paths, func(i, j int) bool { return paths[i].Candidate.Key < paths[j].Candidate.Key })
	return candidates, paths, nil
}

// postOrderEvaluate 后序遍历，从叶子节点开始处理
func (a *Analyzer) postOrderEvaluate(node *TopoNode, candidates *[]Candidate, paths *[]AlarmPath) {
	if node == nil {
		return
	}

	for _, child := range node.Children {
		a.postOrderEvaluate(child, candidates, paths)
	}

	layerCfg, ok := a.config.Layers[node.NodeRef.Type]
	if !ok {
		layerCfg = LayerConfig{CoverageThreshold: 0.6, MinChildren: 1, Weights: ScoreWeights{Coverage: 0.7}}
	}

	coverage := node.Coverage()

	if coverage > layerCfg.CoverageThreshold {
		// 满足条件，标记为候选根因
		score := node.ComputeScore(layerCfg.Weights)
		eventIds := collectEventIDs(node.Events)

		candidate := Candidate{
			Node:       node.NodeRef,
			Confidence: score.Normalized,
			Coverage:   coverage,
			Reason:     "TREE_POSTORDER",
			Metrics:    score,
			Explained:  eventIds,
		}

		*candidates = append(*candidates, candidate)
		*paths = append(*paths, buildPath(node))
		return
	} else {
		return
	}

}

func buildPath(node *TopoNode) AlarmPath {
	if node == nil {
		return AlarmPath{}
	}
	return AlarmPath{
		Candidate: node.NodeRef,
		Impacts:   collectImpacts(node),
	}
}

func collectImpacts(node *TopoNode) []PathImpact {
	if node == nil || len(node.Impacts) == 0 {
		return nil
	}

	keys := make([]string, 0, len(node.Impacts))
	for key := range node.Impacts {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	impacts := make([]PathImpact, 0, len(keys))
	for _, key := range keys {
		impact := node.Impacts[key]
		if impact == nil {
			continue
		}

		events := make([]AlarmEventRef, 0, len(impact.Events))
		for _, evt := range impact.Events {
			events = append(events, evt)
		}
		sort.Slice(events, func(i, j int) bool {
			if events[i].Occurred.Equal(events[j].Occurred) {
				return events[i].ID < events[j].ID
			}
			return events[i].Occurred.Before(events[j].Occurred)
		})

		var childImpacts []PathImpact
		if child, ok := node.Children[key]; ok && child != nil {
			childImpacts = collectImpacts(child)
		}

		impacts = append(impacts, PathImpact{
			Node:    impact.Node,
			Events:  events,
			Impacts: childImpacts,
		})
	}
	return impacts
}

func collectEventIDs(events map[string]AlarmEventRef) []string {
	ids := make([]string, 0, len(events))
	for id := range events {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
