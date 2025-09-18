package rca

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"cmdb2neo/internal/graph"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// GraphTopologyProvider 基于 Neo4j 图查询补全告警拓扑。
type GraphTopologyProvider struct {
	client graph.Reader
}

// NewGraphTopologyProvider 使用图查询客户端构建 provider。
func NewGraphTopologyProvider(client graph.Reader) *GraphTopologyProvider {
	return &GraphTopologyProvider{client: client}
}

// ResolveContext 根据事件类型选择对应的查询。

func (p *GraphTopologyProvider) ResolveContext(ctx context.Context, event AlarmEvent) (AlarmContext, error) {
	if p.client == nil {
		return AlarmContext{}, errors.New("graph client 未初始化")
	}

	switch event.NodeType {
	case NodeTypeApp:
		return p.resolveFromApp(ctx, event)
	case NodeTypeVirtualMachine:
		return p.resolveFromVM(ctx, event)
	case NodeTypeHostMachine:
		return p.resolveFromHost(ctx, event)
	case NodeTypePhysicalMachine:
		return p.resolveFromPhysical(ctx, event)
	case NodeTypeNetPartition:
		return p.resolveFromNetPartition(ctx, event)
	case NodeTypeIDC:
		return p.resolveFromIDC(ctx, event)
	default:
		return AlarmContext{}, fmt.Errorf("未支持的事件节点类型: %s", event.NodeType)
	}
}

func (p *GraphTopologyProvider) resolveFromApp(ctx context.Context, event AlarmEvent) (AlarmContext, error) {
	query := `
MATCH (app:App)
WHERE ($cmdb_key IS NOT NULL AND app.cmdb_key = $cmdb_key)
   OR ($ip IS NOT NULL AND app.ip = $ip)
   OR ($service IS NOT NULL AND app.name = $service)
OPTIONAL MATCH (app)-[:DEPLOYED_ON]->(vm:VirtualMachine)
OPTIONAL MATCH (vm)<-[:HOSTS_VM]-(host:HostMachine)
OPTIONAL MATCH (host)<-[:HAS_HOST]-(np:NetPartition)
OPTIONAL MATCH (np)-[:HAS_PHYSICAL]->(physical:PhysicalMachine)
OPTIONAL MATCH (np)<-[:HAS_PARTITION]-(idc:IDC)
RETURN app, vm, host, physical, np, idc,
       CASE WHEN vm IS NULL THEN 0 ELSE size((vm)<-[:DEPLOYED_ON]-(:App)) END AS vm_app_count,
       CASE WHEN host IS NULL THEN 0 ELSE size((host)-[:HOSTS_VM]->(:VirtualMachine)) END AS host_vm_count,
       CASE WHEN np IS NULL THEN 0 ELSE size((np)-[:HAS_HOST]->(:HostMachine)) END AS np_host_count,
       CASE WHEN np IS NULL THEN 0 ELSE size((np)-[:HAS_PHYSICAL]->(:PhysicalMachine)) END AS np_physical_count,
       CASE WHEN idc IS NULL THEN 0 ELSE size((idc)-[:HAS_PARTITION]->(:NetPartition)) END AS idc_np_count
ORDER BY coalesce(vm_app_count,0) DESC
LIMIT 1
`
	params := map[string]any{
		"cmdb_key": event.Attrs["cmdb_key"],
		"ip":       nullIfEmpty(event.IP),
		"service":  nullIfEmpty(event.Service),
	}
	return p.fetchContext(ctx, query, params)
}

func (p *GraphTopologyProvider) resolveFromVM(ctx context.Context, event AlarmEvent) (AlarmContext, error) {
	query := `
MATCH (vm:VirtualMachine)
WHERE ($cmdb_key IS NOT NULL AND vm.cmdb_key = $cmdb_key)
   OR ($ip IS NOT NULL AND vm.ip = $ip)
OPTIONAL MATCH (app:App)-[:DEPLOYED_ON]->(vm)
OPTIONAL MATCH (vm)<-[:HOSTS_VM]-(host:HostMachine)
OPTIONAL MATCH (host)<-[:HAS_HOST]-(np:NetPartition)
OPTIONAL MATCH (np)-[:HAS_PHYSICAL]->(physical:PhysicalMachine)
OPTIONAL MATCH (np)<-[:HAS_PARTITION]-(idc:IDC)
RETURN app, vm, host, physical, np, idc,
       CASE WHEN vm IS NULL THEN 0 ELSE size((vm)<-[:DEPLOYED_ON]-(:App)) END AS vm_app_count,
       CASE WHEN host IS NULL THEN 0 ELSE size((host)-[:HOSTS_VM]->(:VirtualMachine)) END AS host_vm_count,
       CASE WHEN np IS NULL THEN 0 ELSE size((np)-[:HAS_HOST]->(:HostMachine)) END AS np_host_count,
       CASE WHEN np IS NULL THEN 0 ELSE size((np)-[:HAS_PHYSICAL]->(:PhysicalMachine)) END AS np_physical_count,
       CASE WHEN idc IS NULL THEN 0 ELSE size((idc)-[:HAS_PARTITION]->(:NetPartition)) END AS idc_np_count
ORDER BY coalesce(vm_app_count,0) DESC
LIMIT 1
`
	params := map[string]any{
		"cmdb_key": event.Attrs["cmdb_key"],
		"ip":       nullIfEmpty(event.IP),
	}
	return p.fetchContext(ctx, query, params)
}

func (p *GraphTopologyProvider) resolveFromHost(ctx context.Context, event AlarmEvent) (AlarmContext, error) {
	query := `
MATCH (host:HostMachine)
WHERE ($cmdb_key IS NOT NULL AND host.cmdb_key = $cmdb_key)
   OR ($ip IS NOT NULL AND host.ip = $ip)
OPTIONAL MATCH (host)-[:HOSTS_VM]->(vm:VirtualMachine)
OPTIONAL MATCH (app:App)-[:DEPLOYED_ON]->(vm)
OPTIONAL MATCH (host)<-[:HAS_HOST]-(np:NetPartition)
OPTIONAL MATCH (np)-[:HAS_PHYSICAL]->(physical:PhysicalMachine)
OPTIONAL MATCH (np)<-[:HAS_PARTITION]-(idc:IDC)
RETURN app, vm, host, physical, np, idc,
       CASE WHEN vm IS NULL THEN 0 ELSE size((vm)<-[:DEPLOYED_ON]-(:App)) END AS vm_app_count,
       CASE WHEN host IS NULL THEN 0 ELSE size((host)-[:HOSTS_VM]->(:VirtualMachine)) END AS host_vm_count,
       CASE WHEN np IS NULL THEN 0 ELSE size((np)-[:HAS_HOST]->(:HostMachine)) END AS np_host_count,
       CASE WHEN np IS NULL THEN 0 ELSE size((np)-[:HAS_PHYSICAL]->(:PhysicalMachine)) END AS np_physical_count,
       CASE WHEN idc IS NULL THEN 0 ELSE size((idc)-[:HAS_PARTITION]->(:NetPartition)) END AS idc_np_count
ORDER BY coalesce(host_vm_count,0) DESC
LIMIT 1
`
	params := map[string]any{
		"cmdb_key": event.Attrs["cmdb_key"],
		"ip":       nullIfEmpty(event.IP),
	}
	return p.fetchContext(ctx, query, params)
}

func (p *GraphTopologyProvider) resolveFromPhysical(ctx context.Context, event AlarmEvent) (AlarmContext, error) {
	query := `
MATCH (physical:PhysicalMachine)
WHERE ($cmdb_key IS NOT NULL AND physical.cmdb_key = $cmdb_key)
   OR ($ip IS NOT NULL AND physical.ip = $ip)
OPTIONAL MATCH (np:NetPartition)-[:HAS_PHYSICAL]->(physical)
OPTIONAL MATCH (np)<-[:HAS_PARTITION]-(idc:IDC)
RETURN null AS app, null AS vm, null AS host, physical, np, idc,
       0 AS vm_app_count,
       0 AS host_vm_count,
       CASE WHEN np IS NULL THEN 0 ELSE size((np)-[:HAS_HOST]->(:HostMachine)) END AS np_host_count,
       CASE WHEN np IS NULL THEN 0 ELSE size((np)-[:HAS_PHYSICAL]->(:PhysicalMachine)) END AS np_physical_count,
       CASE WHEN idc IS NULL THEN 0 ELSE size((idc)-[:HAS_PARTITION]->(:NetPartition)) END AS idc_np_count
LIMIT 1
`
	params := map[string]any{
		"cmdb_key": event.Attrs["cmdb_key"],
		"ip":       nullIfEmpty(event.IP),
	}
	return p.fetchContext(ctx, query, params)
}

func (p *GraphTopologyProvider) resolveFromNetPartition(ctx context.Context, event AlarmEvent) (AlarmContext, error) {
	query := `
MATCH (np:NetPartition)
WHERE ($cmdb_key IS NOT NULL AND np.cmdb_key = $cmdb_key)
   OR ($name IS NOT NULL AND np.name = $name)
OPTIONAL MATCH (np)<-[:HAS_PARTITION]-(idc:IDC)
RETURN null AS app, null AS vm, null AS host, null AS physical, np, idc,
       0 AS vm_app_count,
       0 AS host_vm_count,
       CASE WHEN np IS NULL THEN 0 ELSE size((np)-[:HAS_HOST]->(:HostMachine)) END AS np_host_count,
       CASE WHEN np IS NULL THEN 0 ELSE size((np)-[:HAS_PHYSICAL]->(:PhysicalMachine)) END AS np_physical_count,
       CASE WHEN idc IS NULL THEN 0 ELSE size((idc)-[:HAS_PARTITION]->(:NetPartition)) END AS idc_np_count
LIMIT 1
`
	params := map[string]any{
		"cmdb_key": event.Attrs["cmdb_key"],
		"name":     nullIfEmpty(event.Service),
	}
	return p.fetchContext(ctx, query, params)
}

func (p *GraphTopologyProvider) resolveFromIDC(ctx context.Context, event AlarmEvent) (AlarmContext, error) {
	query := `
MATCH (idc:IDC)
WHERE ($cmdb_key IS NOT NULL AND idc.cmdb_key = $cmdb_key)
   OR ($name IS NOT NULL AND idc.name = $name)
RETURN null AS app, null AS vm, null AS host, null AS physical, null AS np, idc,
       0 AS vm_app_count,
       0 AS host_vm_count,
       0 AS np_host_count,
       0 AS np_physical_count,
       CASE WHEN idc IS NULL THEN 0 ELSE size((idc)-[:HAS_PARTITION]->(:NetPartition)) END AS idc_np_count
LIMIT 1
`
	params := map[string]any{
		"cmdb_key": event.Attrs["cmdb_key"],
		"name":     nullIfEmpty(event.Service),
	}
	return p.fetchContext(ctx, query, params)
}

func (p *GraphTopologyProvider) fetchContext(ctx context.Context, query string, params map[string]any) (AlarmContext, error) {
	records, err := p.client.RunRead(ctx, query, params)
	if err != nil {
		return AlarmContext{}, err
	}
	if len(records) == 0 {
		return AlarmContext{}, errors.New("未在图中找到对应节点")
	}
	rec := records[0]

	ctxResult := AlarmContext{}

	if node, err := nodeFromRecord(rec, "app"); err != nil {
		return AlarmContext{}, err
	} else if node != nil {
		ctxResult.App = node
	}
	if node, err := nodeFromRecord(rec, "vm"); err != nil {
		return AlarmContext{}, err
	} else if node != nil {
		setChildCount(node, NodeTypeApp, rec["vm_app_count"])
		ctxResult.VirtualMachine = node
	}
	if node, err := nodeFromRecord(rec, "host"); err != nil {
		return AlarmContext{}, err
	} else if node != nil {
		setChildCount(node, NodeTypeVirtualMachine, rec["host_vm_count"])
		ctxResult.HostMachine = node
	}
	if node, err := nodeFromRecord(rec, "physical"); err != nil {
		return AlarmContext{}, err
	} else if node != nil {
		ctxResult.PhysicalMachine = node
	}
	if node, err := nodeFromRecord(rec, "np"); err != nil {
		return AlarmContext{}, err
	} else if node != nil {
		setChildCount(node, NodeTypeHostMachine, rec["np_host_count"])
		setChildCount(node, NodeTypePhysicalMachine, rec["np_physical_count"])
		ctxResult.NetPartition = node
	}
	if node, err := nodeFromRecord(rec, "idc"); err != nil {
		return AlarmContext{}, err
	} else if node != nil {
		setChildCount(node, NodeTypeNetPartition, rec["idc_np_count"])
		ctxResult.IDC = node
	}

	if ctxResult.HostMachine != nil && ctxResult.PhysicalMachine != nil {
		ctxResult.PhysicalMachine = nil
	}

	return ctxResult, nil
}

func nodeFromRecord(record map[string]any, key string) (*Node, error) {
	val, ok := record[key]
	if !ok || val == nil {
		return nil, nil
	}
	node, ok := val.(neo4j.Node)
	if !ok {
		return nil, fmt.Errorf("字段 %s 不是 Neo4j 节点: %T", key, val)
	}
	labels := node.Labels()
	typ := inferNodeType(labels)

	props := make(map[string]any, len(node.Props))
	for k, v := range node.Props {
		props[k] = v
	}

	name := toString(props["name"])
	if name == "" {
		name = toString(props["hostname"])
	}

	return &Node{
		NodeRef: NodeRef{
			CMDBKey: toString(props["cmdb_key"]),
			Type:    typ,
			Name:    name,
			Labels:  append([]string(nil), labels...),
			Props:   props,
		},
		ChildCounts: make(map[NodeType]int),
	}, nil
}

func setChildCount(node *Node, childType NodeType, raw any) {
	if node == nil || childType == NodeType("") {
		return
	}
	count := toInt(raw)
	if count <= 0 {
		return
	}
	if node.ChildCounts == nil {
		node.ChildCounts = make(map[NodeType]int)
	}
	node.ChildCounts[childType] = count
}

func inferNodeType(labels []string) NodeType {
	for _, label := range labels {
		switch NodeType(label) {
		case NodeTypeApp, NodeTypeVirtualMachine, NodeTypeHostMachine, NodeTypePhysicalMachine, NodeTypeNetPartition, NodeTypeIDC:
			return NodeType(label)
		}
	}
	if len(labels) > 0 {
		return NodeType(labels[0])
	}
	return NodeType("")
}

func nullIfEmpty(val string) any {
	if strings.TrimSpace(val) == "" {
		return nil
	}
	return val
}

func toString(val any) string {
	switch v := val.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return ""
	}
}

func toInt(val any) int {
	switch v := val.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}
