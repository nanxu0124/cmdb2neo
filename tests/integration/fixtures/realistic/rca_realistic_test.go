package realistic

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	"cmdb2neo/internal/rca"
)

func TestRealisticFixtureRootCause(t *testing.T) {
	fixture := loadFixtureData(t)
	provider := &fixtureProvider{data: fixture}
	events := loadFixtureEvents(t)

	cfg := rca.DefaultConfig()
	cfg.Hierarchy = []rca.NodeType{rca.NodeTypeVirtualMachine, rca.NodeTypeHostMachine, rca.NodeTypeNetPartition}
	cfg.Layers[rca.NodeTypeVirtualMachine] = rca.LayerConfig{
		CoverageThreshold: 0.6,
		MinChildren:       1,
		Weights:           rca.ScoreWeights{Coverage: 0.6, TimeLead: 0.2, Impact: 0.15, Base: 0.05},
	}
	cfg.Layers[rca.NodeTypeHostMachine] = rca.LayerConfig{
		CoverageThreshold: 0.5,
		MinChildren:       1,
		Weights:           rca.ScoreWeights{Coverage: 0.55, TimeLead: 0.2, Impact: 0.2, Base: 0.05},
	}
	cfg.Layers[rca.NodeTypeNetPartition] = rca.LayerConfig{
		CoverageThreshold: 0.7,
		MinChildren:       1,
		Weights:           rca.ScoreWeights{Coverage: 0.5, TimeLead: 0.25, Impact: 0.2, Base: 0.05},
	}

	analyzer, err := rca.NewAnalyzer(provider, nil, cfg)
	if err != nil {
		t.Fatalf("new analyzer: %v", err)
	}

	result, err := analyzer.Analyze(context.Background(), "window-realistic", events)
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}

	if len(result.Candidates) < 3 {
		t.Fatalf("expected at least 3 candidates, got %d", len(result.Candidates))
	}

	hostCand, ok := findCandidate(result.Candidates, "HM_4002")
	if !ok {
		t.Fatalf("host HM_4002 candidate missing")
	}
	if diff := math.Abs(hostCand.Coverage - 0.75); diff > 0.01 {
		t.Fatalf("host coverage expect 0.75, got %.2f", hostCand.Coverage)
	}
	if len(hostCand.Explained) < 4 {
		t.Fatalf("host should explain at least 4 events, got %d", len(hostCand.Explained))
	}

	for _, vmKey := range []string{"VM_5002", "VM_5003", "VM_5004"} {
		vmCand, ok := findCandidate(result.Candidates, vmKey)
		if !ok {
			t.Fatalf("vm candidate %s missing", vmKey)
		}
		if vmCand.Coverage < 0.99 {
			t.Fatalf("vm %s coverage expect 1, got %.2f", vmKey, vmCand.Coverage)
		}
	}

	hostPath, ok := findPath(result.Paths, "HM_4002")
	if !ok {
		t.Fatalf("host path missing")
	}
	impacted := make([]string, 0, len(hostPath.Impacts))
	for _, imp := range hostPath.Impacts {
		impacted = append(impacted, imp.Node.CMDBKey)
	}
	sort.Strings(impacted)
	expectedImpacted := []string{"VM_5002", "VM_5003", "VM_5004"}
	if !equalStrings(impacted, expectedImpacted) {
		t.Fatalf("host impacted mismatch: got %v expect %v", impacted, expectedImpacted)
	}

	if len(result.UnexplainedEvents) != 0 {
		t.Fatalf("expected no unexplained events, got %d", len(result.UnexplainedEvents))
	}
}

// ---------- fixture loading ----------

type fixtureData struct {
	idcByName     map[string]idcFixture
	idcByKey      map[string]idcFixture
	npByName      map[string]npFixture
	npByKey       map[string]npFixture
	hostByIP      map[string]hostFixture
	hostByKey     map[string]hostFixture
	vmByIP        map[string]vmFixture
	vmByKey       map[string]vmFixture
	appByIP       map[string]appFixture
	appByName     map[string]appFixture
	hostVMs       map[string][]string
	vmApps        map[string][]string
	npHosts       map[string][]string
	npPhysicals   map[string][]string
	idcPartitions map[string][]string
}

type idcFixture struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Location string `json:"location"`
}

type npFixture struct {
	ID   int    `json:"id"`
	IDC  string `json:"idc"`
	Name string `json:"Name"`
	CIDR string `json:"CIDR"`
}

type hostFixture struct {
	ID               int    `json:"id"`
	IDC              string `json:"idc"`
	NetworkPartition string `json:"network_partition"`
	ServerType       int    `json:"server_type"`
	IP               string `json:"ip"`
	HostName         string `json:"host_name"`
}

type physicalFixture struct {
	ID               int    `json:"id"`
	IDC              string `json:"idc"`
	NetworkPartition string `json:"network_partition"`
	ServerType       int    `json:"server_type"`
	IP               string `json:"ip"`
	HostName         string `json:"host_name"`
}

type vmFixture struct {
	ID               int    `json:"id"`
	IDC              string `json:"idc"`
	NetworkPartition string `json:"network_partition"`
	ServerType       int    `json:"server_type"`
	IP               string `json:"ip"`
	HostName         string `json:"host_name"`
	HostIP           string `json:"host_ip"`
}

type appFixture struct {
	ID   int    `json:"id"`
	IP   string `json:"ip"`
	Name string `json:"name"`
}

type alarmFixture struct {
	ID         string            `json:"id"`
	Source     string            `json:"source"`
	Priority   string            `json:"priority"`
	NodeType   string            `json:"node_type"`
	IP         string            `json:"ip"`
	HostIP     string            `json:"host_ip"`
	Service    string            `json:"service"`
	Occurred   string            `json:"occurred_at"`
	Attributes map[string]string `json:"attributes"`
}

func loadFixtureData(t *testing.T) *fixtureData {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime caller failed for ")
	}
	dir := filepath.Dir(file)

	idcs := readJSON[idcFixture](t, filepath.Join(dir, "idc.json"))
	nps := readJSON[npFixture](t, filepath.Join(dir, "network_partition.json"))
	hosts := readJSON[hostFixture](t, filepath.Join(dir, "host_machine.json"))
	physicals := readJSON[physicalFixture](t, filepath.Join(dir, "physical_machine.json"))
	vms := readJSON[vmFixture](t, filepath.Join(dir, "virtual_machine.json"))
	apps := readJSON[appFixture](t, filepath.Join(dir, "app.json"))

	data := &fixtureData{
		idcByName:     make(map[string]idcFixture),
		idcByKey:      make(map[string]idcFixture),
		npByName:      make(map[string]npFixture),
		npByKey:       make(map[string]npFixture),
		hostByIP:      make(map[string]hostFixture),
		hostByKey:     make(map[string]hostFixture),
		vmByIP:        make(map[string]vmFixture),
		vmByKey:       make(map[string]vmFixture),
		appByIP:       make(map[string]appFixture),
		appByName:     make(map[string]appFixture),
		hostVMs:       make(map[string][]string),
		vmApps:        make(map[string][]string),
		npHosts:       make(map[string][]string),
		npPhysicals:   make(map[string][]string),
		idcPartitions: make(map[string][]string),
	}

	for _, idc := range idcs {
		key := fmt.Sprintf("IDC_%d", idc.ID)
		data.idcByName[idc.Name] = idc
		data.idcByKey[key] = idc
	}
	for _, np := range nps {
		key := fmt.Sprintf("NP_%d", np.ID)
		data.npByName[np.Name] = np
		data.npByKey[key] = np
		if idc, ok := data.idcByName[np.IDC]; ok {
			idcKey := fmt.Sprintf("IDC_%d", idc.ID)
			data.idcPartitions[idcKey] = append(data.idcPartitions[idcKey], key)
		}
	}
	for _, host := range hosts {
		key := fmt.Sprintf("HM_%d", host.ID)
		data.hostByIP[host.IP] = host
		data.hostByKey[key] = host
		if np, ok := data.npByName[host.NetworkPartition]; ok {
			npKey := fmt.Sprintf("NP_%d", np.ID)
			data.npHosts[npKey] = append(data.npHosts[npKey], key)
		}
	}
	for _, phy := range physicals {
		if np, ok := data.npByName[phy.NetworkPartition]; ok {
			npKey := fmt.Sprintf("NP_%d", np.ID)
			phyKey := fmt.Sprintf("PM_%d", phy.ID)
			data.npPhysicals[npKey] = append(data.npPhysicals[npKey], phyKey)
		}
	}
	for _, vm := range vms {
		key := fmt.Sprintf("VM_%d", vm.ID)
		data.vmByIP[vm.IP] = vm
		data.vmByKey[key] = vm
		hostKey := hostKeyByIP(data, vm.HostIP)
		if hostKey != "" {
			data.hostVMs[hostKey] = append(data.hostVMs[hostKey], key)
		}
	}
	for _, app := range apps {
		data.appByIP[app.IP] = app
		data.appByName[app.Name] = app
		if vm, ok := data.vmByIP[app.IP]; ok {
			vmKey := fmt.Sprintf("VM_%d", vm.ID)
			appKey := fmt.Sprintf("APP_%d", app.ID)
			data.vmApps[vmKey] = append(data.vmApps[vmKey], appKey)
		}
	}

	return data
}

func hostKeyByIP(data *fixtureData, ip string) string {
	if host, ok := data.hostByIP[ip]; ok {
		return fmt.Sprintf("HM_%d", host.ID)
	}
	return ""
}

func readJSON[T any](t *testing.T, path string) []T {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s failed: %v", path, err)
	}
	var items []T
	if err := json.Unmarshal(raw, &items); err != nil {
		t.Fatalf("unmarshal %s failed: %v", path, err)
	}
	return items
}

func loadFixtureEvents(t *testing.T) []rca.AlarmEvent {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime caller failed for ")
	}
	dir := filepath.Dir(file)

	path := filepath.Join(dir, "alarm_events.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read events failed: %v", err)
	}
	var fixtures []alarmFixture
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatalf("unmarshal events failed: %v", err)
	}
	events := make([]rca.AlarmEvent, 0, len(fixtures))
	for _, f := range fixtures {
		ts, err := time.Parse(time.RFC3339, f.Occurred)
		if err != nil {
			t.Fatalf("parse time %s: %v", f.Occurred, err)
		}
		events = append(events, rca.AlarmEvent{
			ID:       f.ID,
			Source:   f.Source,
			Priority: f.Priority,
			NodeType: rca.NodeType(f.NodeType),
			IP:       f.IP,
			HostIP:   f.HostIP,
			Service:  f.Service,
			Occurred: ts,
			Attrs:    f.Attributes,
		})
	}
	return events
}

// ---------- Topology Provider backed by fixtures ----------

type fixtureProvider struct {
	data *fixtureData
}

func (p *fixtureProvider) ResolveContext(_ context.Context, event rca.AlarmEvent) (rca.AlarmContext, error) {
	switch event.NodeType {
	case rca.NodeTypeApp:
		return p.resolveApp(event)
	case rca.NodeTypeHostMachine:
		return p.resolveHost(event)
	default:
		return rca.AlarmContext{}, fmt.Errorf("unsupported node type %s", event.NodeType)
	}
}

func (p *fixtureProvider) resolveApp(event rca.AlarmEvent) (rca.AlarmContext, error) {
	app, err := p.findApp(event)
	if err != nil {
		return rca.AlarmContext{}, err
	}
	vm, err := p.findVMByIP(app.IP)
	if err != nil {
		return rca.AlarmContext{}, err
	}
	host, err := p.findHostByIP(vm.HostIP)
	if err != nil {
		return rca.AlarmContext{}, err
	}
	np, err := p.findNP(host.NetworkPartition)
	if err != nil {
		return rca.AlarmContext{}, err
	}
	idc, err := p.findIDC(np.IDC)
	if err != nil {
		return rca.AlarmContext{}, err
	}

	ctx := rca.AlarmContext{
		App:            p.newAppNode(app),
		VirtualMachine: p.newVMNode(vm),
		HostMachine:    p.newHostNode(host),
		NetPartition:   p.newNPNode(np),
		IDC:            p.newIDCNode(idc),
	}
	return ctx, nil
}

func (p *fixtureProvider) resolveHost(event rca.AlarmEvent) (rca.AlarmContext, error) {
	var host hostFixture
	var err error
	if event.HostIP != "" {
		host, err = p.findHostByIP(event.HostIP)
	} else if cmdb := event.Attrs["cmdb_key"]; cmdb != "" {
		host, err = p.findHostByCMDB(cmdb)
	} else {
		host, err = p.findHostByIP(event.IP)
	}
	if err != nil {
		return rca.AlarmContext{}, err
	}
	np, err := p.findNP(host.NetworkPartition)
	if err != nil {
		return rca.AlarmContext{}, err
	}
	idc, err := p.findIDC(np.IDC)
	if err != nil {
		return rca.AlarmContext{}, err
	}

	ctx := rca.AlarmContext{
		HostMachine:  p.newHostNode(host),
		NetPartition: p.newNPNode(np),
		IDC:          p.newIDCNode(idc),
	}
	return ctx, nil
}

func (p *fixtureProvider) findApp(event rca.AlarmEvent) (appFixture, error) {
	if app, ok := p.data.appByIP[event.IP]; ok {
		return app, nil
	}
	if event.Service != "" {
		if app, ok := p.data.appByName[event.Service]; ok {
			return app, nil
		}
	}
	return appFixture{}, fmt.Errorf("app not found for event %s", event.ID)
}

func (p *fixtureProvider) findVMByIP(ip string) (vmFixture, error) {
	vm, ok := p.data.vmByIP[ip]
	if !ok {
		return vmFixture{}, fmt.Errorf("vm not found for ip %s", ip)
	}
	return vm, nil
}

func (p *fixtureProvider) findHostByIP(ip string) (hostFixture, error) {
	host, ok := p.data.hostByIP[ip]
	if !ok {
		return hostFixture{}, fmt.Errorf("host not found for ip %s", ip)
	}
	return host, nil
}

func (p *fixtureProvider) findHostByCMDB(cmdb string) (hostFixture, error) {
	host, ok := p.data.hostByKey[cmdb]
	if !ok {
		return hostFixture{}, fmt.Errorf("host not found for cmdb %s", cmdb)
	}
	return host, nil
}

func (p *fixtureProvider) findNP(name string) (npFixture, error) {
	np, ok := p.data.npByName[name]
	if !ok {
		return npFixture{}, fmt.Errorf("network partition %s not found", name)
	}
	return np, nil
}

func (p *fixtureProvider) findIDC(name string) (idcFixture, error) {
	idc, ok := p.data.idcByName[name]
	if !ok {
		return idcFixture{}, fmt.Errorf("idc %s not found", name)
	}
	return idc, nil
}

func (p *fixtureProvider) newAppNode(app appFixture) *rca.Node {
	key := fmt.Sprintf("APP_%d", app.ID)
	return &rca.Node{
		NodeRef: rca.NodeRef{
			CMDBKey: key,
			Type:    rca.NodeTypeApp,
			Name:    app.Name,
			Labels:  []string{"App"},
			Props: map[string]any{
				"ip":   app.IP,
				"name": app.Name,
			},
		},
	}
}

func (p *fixtureProvider) newVMNode(vm vmFixture) *rca.Node {
	key := fmt.Sprintf("VM_%d", vm.ID)
	childCount := len(p.data.vmApps[key])
	if childCount == 0 {
		childCount = 1
	}
	return &rca.Node{
		NodeRef: rca.NodeRef{
			CMDBKey: key,
			Type:    rca.NodeTypeVirtualMachine,
			Name:    vm.HostName,
			Labels:  []string{"VirtualMachine", "Compute"},
			Props: map[string]any{
				"ip":       vm.IP,
				"host_ip":  vm.HostIP,
				"hostname": vm.HostName,
			},
		},
		ChildCounts: map[rca.NodeType]int{
			rca.NodeTypeApp: childCount,
		},
	}
}

func (p *fixtureProvider) newHostNode(host hostFixture) *rca.Node {
	key := fmt.Sprintf("HM_%d", host.ID)
	totalVM := len(p.data.hostVMs[key])
	if totalVM == 0 {
		totalVM = 1
	}
	return &rca.Node{
		NodeRef: rca.NodeRef{
			CMDBKey: key,
			Type:    rca.NodeTypeHostMachine,
			Name:    host.HostName,
			Labels:  []string{"HostMachine", "Machine", "Compute"},
			Props: map[string]any{
				"ip":       host.IP,
				"hostname": host.HostName,
			},
		},
		ChildCounts: map[rca.NodeType]int{
			rca.NodeTypeVirtualMachine: totalVM,
		},
	}
}

func (p *fixtureProvider) newNPNode(np npFixture) *rca.Node {
	key := fmt.Sprintf("NP_%d", np.ID)
	hostCount := len(p.data.npHosts[key])
	physicalCount := len(p.data.npPhysicals[key])
	return &rca.Node{
		NodeRef: rca.NodeRef{
			CMDBKey: key,
			Type:    rca.NodeTypeNetPartition,
			Name:    np.Name,
			Labels:  []string{"NetPartition"},
			Props: map[string]any{
				"name": np.Name,
				"cidr": np.CIDR,
			},
		},
		ChildCounts: map[rca.NodeType]int{
			rca.NodeTypeHostMachine:     hostCount,
			rca.NodeTypePhysicalMachine: physicalCount,
		},
	}
}

func (p *fixtureProvider) newIDCNode(idc idcFixture) *rca.Node {
	key := fmt.Sprintf("IDC_%d", idc.ID)
	npCount := len(p.data.idcPartitions[key])
	return &rca.Node{
		NodeRef: rca.NodeRef{
			CMDBKey: key,
			Type:    rca.NodeTypeIDC,
			Name:    idc.Name,
			Labels:  []string{"IDC"},
			Props: map[string]any{
				"name": idc.Name,
			},
		},
		ChildCounts: map[rca.NodeType]int{
			rca.NodeTypeNetPartition: npCount,
		},
	}
}

// ---------- helpers ----------

func findCandidate(list []rca.Candidate, key string) (rca.Candidate, bool) {
	for _, cand := range list {
		if cand.Node.CMDBKey == key {
			return cand, true
		}
	}
	return rca.Candidate{}, false
}

func findPath(paths []rca.AlarmPath, key string) (rca.AlarmPath, bool) {
	for _, path := range paths {
		if path.Candidate.CMDBKey == key {
			return path, true
		}
	}
	return rca.AlarmPath{}, false
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
