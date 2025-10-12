package rcav2

import (
	"context"
	"fmt"
	"strings"

	"cmdb2neo/internal/graph"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// TopologyProvider 提供拓扑链路和部署信息。
type TopologyProvider interface {
	ListAppInstances(ctx context.Context, appName string, datacenter string) (int, error)
	ResolveEvent(ctx context.Context, event AlarmEvent) ([]Node, error)
}

// GraphProvider 基于 Neo4j 的实现。
type GraphProvider struct {
	client graph.Reader
}

func NewGraphProvider(client graph.Reader) *GraphProvider {
	return &GraphProvider{client: client}
}

func (p *GraphProvider) ResolveEvent(ctx context.Context, event AlarmEvent) ([]Node, error) {
	var chain Chain
	var err error
	switch event.ServerType {
	case ServerTypeHost:
		chain, err = p.resolveFromHost(ctx, event)
	case ServerTypePhysical:
		chain, err = p.resolveFromPhysical(ctx, event)
	default:
		chain, err = p.resolveFromAppOrVM(ctx, event)
	}
	if err != nil {
		return nil, err
	}
	return chainToNodes(chain), nil
}

func (p *GraphProvider) ListAppInstances(ctx context.Context, appName string, datacenter string) (int, error) {
	queries := []string{
		`
MATCH (app:App {name: $app})-[:DEPLOYED_ON]->(vm:VirtualMachine)
MATCH (vm)<-[:HOSTS_VM]-(host:HostMachine)
MATCH (host)<-[:HAS_HOST]-(np:NetPartition)<-[:HAS_PARTITION]-(idc:IDC {name: $idc})
RETURN COUNT(DISTINCT vm) AS total
`,
		`
MATCH (app:App {name: $app})-[:DEPLOYED_ON]->(host:HostMachine)
MATCH (host)<-[:HAS_HOST]-(np:NetPartition)<-[:HAS_PARTITION]-(idc:IDC {name: $idc})
RETURN COUNT(DISTINCT host) AS total
`,
		`
MATCH (app:App {name: $app})-[:DEPLOYED_ON]->(phy:PhysicalMachine)
MATCH (np:NetPartition)-[:HAS_PHYSICAL]->(phy)
MATCH (np)<-[:HAS_PARTITION]-(idc:IDC {name: $idc})
RETURN COUNT(DISTINCT phy) AS total
`,
	}

	total := 0
	params := map[string]any{"app": appName, "idc": datacenter}
	for _, query := range queries {
		records, err := p.client.RunRead(ctx, query, params)
		if err != nil {
			return 0, err
		}
		for _, record := range records {
			switch v := record["total"].(type) {
			case int64:
				total += int(v)
			case int:
				total += v
			}
		}
	}
	return total, nil
}

func (p *GraphProvider) resolveFromAppOrVM(ctx context.Context, event AlarmEvent) (Chain, error) {
	query := `
MATCH (app:App)
WHERE app.name = $name
OPTIONAL MATCH (app)-[:DEPLOYED_ON]->(vm:VirtualMachine)
OPTIONAL MATCH (vm)<-[:HOSTS_VM]-(host:HostMachine)
OPTIONAL MATCH (host)<-[:HAS_HOST]-(np:NetPartition)
OPTIONAL MATCH (np)<-[:HAS_PARTITION]-(idc:IDC)
RETURN app, vm, host, null AS physical, np, idc,
       CASE WHEN vm IS NULL THEN 0 ELSE size((vm)<-[:DEPLOYED_ON]-(:App)) END AS vm_app_count,
       CASE WHEN host IS NULL THEN 0 ELSE size((host)-[:HOSTS_VM]->(:VirtualMachine)) END AS host_vm_count,
       CASE WHEN np IS NULL THEN 0 ELSE size((np)-[:HAS_HOST]->(:HostMachine)) END AS np_host_count,
       CASE WHEN np IS NULL THEN 0 ELSE size((np)-[:HAS_PHYSICAL]->(:PhysicalMachine)) END AS np_physical_count,
       CASE WHEN idc IS NULL THEN 0 ELSE size((idc)-[:HAS_PARTITION]->(:NetPartition)) END AS idc_np_count
ORDER BY idc.name = $idc DESC
LIMIT 1
`
	records, err := p.client.RunRead(ctx, query, map[string]any{
		"name": event.AppName,
		"idc":  event.Datacenter,
	})
	if err != nil {
		return Chain{}, err
	}
	if len(records) == 0 {
		return Chain{}, fmt.Errorf("app %s not found", event.AppName)
	}
	return chainFromRecord(records[0])
}

func (p *GraphProvider) resolveFromHost(ctx context.Context, event AlarmEvent) (Chain, error) {
	query := `
MATCH (host:HostMachine)
WHERE host.ip = $ip
OPTIONAL MATCH (app:App)-[:DEPLOYED_ON]->(host)
OPTIONAL MATCH (host)<-[:HAS_HOST]-(np:NetPartition)
OPTIONAL MATCH (np)<-[:HAS_PARTITION]-(idc:IDC)
RETURN app, null AS vm, host, null AS physical, np, idc,
       0 AS vm_app_count,
       CASE WHEN host IS NULL THEN 0 ELSE size((host)-[:HOSTS_VM]->(:VirtualMachine)) END AS host_vm_count,
       CASE WHEN np IS NULL THEN 0 ELSE size((np)-[:HAS_HOST]->(:HostMachine)) END AS np_host_count,
       CASE WHEN np IS NULL THEN 0 ELSE size((np)-[:HAS_PHYSICAL]->(:PhysicalMachine)) END AS np_physical_count,
       CASE WHEN idc IS NULL THEN 0 ELSE size((idc)-[:HAS_PARTITION]->(:NetPartition)) END AS idc_np_count
LIMIT 1
`
	records, err := p.client.RunRead(ctx, query, map[string]any{"ip": event.IP})
	if err != nil {
		return Chain{}, err
	}
	if len(records) == 0 {
		return Chain{}, fmt.Errorf("host %s not found", event.IP)
	}
	return chainFromRecord(records[0])
}

func (p *GraphProvider) resolveFromPhysical(ctx context.Context, event AlarmEvent) (Chain, error) {
	query := `
MATCH (phy:PhysicalMachine)
WHERE phy.ip = $ip
OPTIONAL MATCH (app:App)-[:DEPLOYED_ON]->(phy)
OPTIONAL MATCH (np:NetPartition)-[:HAS_PHYSICAL]->(phy)
OPTIONAL MATCH (np)<-[:HAS_PARTITION]-(idc:IDC)
RETURN app, null AS vm, null AS host, phy AS physical, np, idc,
       0 AS vm_app_count,
       0 AS host_vm_count,
       CASE WHEN np IS NULL THEN 0 ELSE size((np)-[:HAS_HOST]->(:HostMachine)) END AS np_host_count,
       CASE WHEN np IS NULL THEN 0 ELSE size((np)-[:HAS_PHYSICAL]->(:PhysicalMachine)) END AS np_physical_count,
       CASE WHEN idc IS NULL THEN 0 ELSE size((idc)-[:HAS_PARTITION]->(:NetPartition)) END AS idc_np_count
LIMIT 1
`
	records, err := p.client.RunRead(ctx, query, map[string]any{"ip": event.IP})
	if err != nil {
		return Chain{}, err
	}
	if len(records) == 0 {
		return Chain{}, fmt.Errorf("physical %s not found", event.IP)
	}
	return chainFromRecord(records[0])
}

func chainFromRecord(record map[string]any) (Chain, error) {
	chain := Chain{}

	if node, err := nodeFromRecord(record, "app"); err != nil {
		return Chain{}, err
	} else {
		chain.App = node
	}
	if node, err := nodeFromRecord(record, "vm"); err != nil {
		return Chain{}, err
	} else {
		chain.VirtualMachine = node
	}
	if node, err := nodeFromRecord(record, "host"); err != nil {
		return Chain{}, err
	} else {
		chain.HostMachine = node
	}
	if node, err := nodeFromRecord(record, "physical"); err != nil {
		return Chain{}, err
	} else {
		chain.PhysicalMachine = node
	}
	if node, err := nodeFromRecord(record, "np"); err != nil {
		return Chain{}, err
	} else {
		chain.NetPartition = node
	}
	if node, err := nodeFromRecord(record, "idc"); err != nil {
		return Chain{}, err
	} else {
		chain.IDC = node
	}

	setChildCount(chain.VirtualMachine, NodeTypeApp, record["vm_app_count"])
	setChildCount(chain.HostMachine, NodeTypeVirtualMachine, record["host_vm_count"])
	setChildCount(chain.NetPartition, NodeTypeHostMachine, record["np_host_count"])
	setChildCount(chain.NetPartition, NodeTypePhysicalMachine, record["np_physical_count"])
	setChildCount(chain.IDC, NodeTypeNetPartition, record["idc_np_count"])

	if chain.HostMachine != nil && chain.PhysicalMachine != nil {
		chain.PhysicalMachine = nil
	}
	return chain, nil
}

func setChildCount(node *Node, childType NodeType, raw any) {
	if node == nil || childType == NodeType("") {
		return
	}
	value := intValue(raw)
	if value <= 0 {
		return
	}
	if node.ChildCounts == nil {
		node.ChildCounts = make(map[NodeType]int)
	}
	node.ChildCounts[childType] = value
}

func intValue(raw any) int {
	switch v := raw.(type) {
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

func chainToNodes(chain Chain) []Node {
	ordered := []*Node{chain.App, chain.VirtualMachine, chain.HostMachine, chain.PhysicalMachine, chain.NetPartition, chain.IDC}
	nodes := make([]Node, 0, len(ordered))
	for _, ptr := range ordered {
		if ptr == nil {
			continue
		}
		nodes = append(nodes, *ptr)
	}
	return nodes
}

func nodeFromRecord(record map[string]any, key string) (*Node, error) {
	val, ok := record[key]
	if !ok || val == nil {
		return nil, nil
	}
	node, ok := val.(neo4j.Node)
	if !ok {
		return nil, fmt.Errorf("field %s is not neo4j node", key)
	}
	propsCopy := make(map[string]any, len(node.Props))
	for k, v := range node.Props {
		propsCopy[k] = v
	}
	labels := node.Labels
	typeName := inferNodeType(labels)
	name := firstNonEmpty(propsCopy["name"], propsCopy["hostname"], propsCopy["cmdb_key"], propsCopy["ip"])
	partition := firstNonEmpty(propsCopy["network_partion"], propsCopy["partition"], propsCopy["name"])
	key = firstNonEmpty(propsCopy["cmdb_key"])
	if key == "" {
		if ip := firstNonEmpty(propsCopy["ip"]); ip != "" {
			key = fmt.Sprintf("%s:%s", typeName, ip)
		} else {
			key = fmt.Sprintf("%s:%d", typeName, node.Id)
		}
	}
	return &Node{
		NodeRef: NodeRef{
			Key:       key,
			Type:      typeName,
			Name:      name,
			IDC:       firstNonEmpty(propsCopy["idc"]),
			Partition: firstNonEmpty(partition),
			Labels:    append([]string(nil), labels...),
			Props:     propsCopy,
		},
		ChildCounts: make(map[NodeType]int),
	}, nil
}

func inferNodeType(labels []string) NodeType {
	for _, lb := range labels {
		switch NodeType(lb) {
		case NodeTypeApp, NodeTypeVirtualMachine, NodeTypeHostMachine, NodeTypePhysicalMachine, NodeTypeNetPartition, NodeTypeIDC:
			return NodeType(lb)
		}
	}
	if len(labels) > 0 {
		return NodeType(labels[0])
	}
	return NodeType("")
}

func firstNonEmpty(values ...any) string {
	for _, v := range values {
		if str, ok := v.(string); ok && strings.TrimSpace(str) != "" {
			return str
		}
	}
	return ""
}
