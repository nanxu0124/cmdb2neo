package rcav1

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"cmdb2neo/internal/graph"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// TopologyProvider 提供拓扑链路和部署信息。
type TopologyProvider interface {
	ResolveChain(ctx context.Context, event AlarmEvent) (Chain, error)
	ListAppInstances(ctx context.Context, appName string, serverType ServerType, datacenter string) ([]Instance, error)
}

// GraphProvider 基于 Neo4j 的实现。
type GraphProvider struct {
	client graph.Reader
}

func NewGraphProvider(client graph.Reader) *GraphProvider {
	return &GraphProvider{client: client}
}

func (p *GraphProvider) ResolveChain(ctx context.Context, event AlarmEvent) (Chain, error) {
	switch event.ServerType {
	case ServerTypeHost:
		return p.resolveFromHost(ctx, event)
	case ServerTypePhysical:
		return p.resolveFromPhysical(ctx, event)
	default:
		return p.resolveFromAppOrVM(ctx, event)
	}
}

func (p *GraphProvider) ListAppInstances(ctx context.Context, appName string, serverType ServerType, datacenter string) ([]Instance, error) {
	var query string
	switch serverType {
	case ServerTypeHost:
		query = `
MATCH (app:App {name: $app})-[:DEPLOYED_ON]->(host:HostMachine)
MATCH (host)<-[:HAS_HOST]-(np:NetPartition)
MATCH (np)<-[:HAS_PARTITION]-(idc:IDC {name: $idc})
RETURN app, host, np, idc
`
	case ServerTypePhysical:
		query = `
MATCH (app:App {name: $app})-[:DEPLOYED_ON]->(phy:PhysicalMachine)
MATCH (np:NetPartition)-[:HAS_PHYSICAL]->(phy)
MATCH (np)<-[:HAS_PARTITION]-(idc:IDC {name: $idc})
RETURN app, phy AS host, np, idc
`
	default:
		query = `
MATCH (app:App {name: $app})-[:DEPLOYED_ON]->(vm:VirtualMachine)
MATCH (vm)<-[:HOSTS_VM]-(host:HostMachine)
MATCH (host)<-[:HAS_HOST]-(np:NetPartition)
MATCH (np)<-[:HAS_PARTITION]-(idc:IDC {name: $idc})
RETURN app, vm, host, np, idc
`
	}

	records, err := p.client.RunRead(ctx, query, map[string]any{
		"app": appName,
		"idc": datacenter,
	})
	if err != nil {
		return nil, err
	}
	instances := make([]Instance, 0, len(records))
	seen := make(map[string]bool)
	for _, record := range records {
		appNode, _ := nodeFromRecord(record, "app")
		targetNode, _ := nodeFromRecord(record, chooseTargetKey(serverType))
		npNode, _ := nodeFromRecord(record, "np")
		if appNode == nil || targetNode == nil {
			continue
		}
		key := targetNode.NodeRef.Key
		if seen[key] {
			continue
		}
		seen[key] = true
		partition := ""
		if npNode != nil {
			partition = firstNonEmpty(npNode.Props["name"], npNode.Props["partition"], npNode.Props["network_partion"])
		}
		hostIP := firstNonEmpty(targetNode.Props["host_ip"], targetNode.Props["ip"])
		instances = append(instances, Instance{
			AppName:    appName,
			ServerType: serverType,
			Datacenter: datacenter,
			IP:         firstNonEmpty(targetNode.Props["ip"], targetNode.Props["hostname"]),
			HostIP:     hostIP,
			Partition:  partition,
			NodeKey:    targetNode.NodeRef.Key,
		})
	}
	sort.Slice(instances, func(i, j int) bool { return instances[i].NodeKey < instances[j].NodeKey })
	return instances, nil
}

func chooseTargetKey(serverType ServerType) string {
	switch serverType {
	case ServerTypeHost:
		return "host"
	case ServerTypePhysical:
		return "host"
	default:
		return "vm"
	}
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
