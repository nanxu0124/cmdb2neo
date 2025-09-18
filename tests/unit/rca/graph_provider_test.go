package rca_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"cmdb2neo/internal/rca"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type mockGraphReader struct{}

func (m *mockGraphReader) RunRead(_ context.Context, query string, params map[string]any) ([]map[string]any, error) {
	switch {
	case strings.Contains(query, "MATCH (app:App)"):
		service, _ := params["service"].(string)
		return []map[string]any{buildAppRecord(service)}, nil
	default:
		return nil, nil
	}
}

func TestGraphTopologyProviderDropPhysical(t *testing.T) {
	provider := rca.NewGraphTopologyProvider(&mockGraphReader{})
	evt := rca.AlarmEvent{
		ID:       "evt-app-1",
		NodeType: rca.NodeTypeApp,
		Service:  "order-service",
		Occurred: time.Now(),
	}

	ctx, err := provider.ResolveContext(context.Background(), evt)
	if err != nil {
		t.Fatalf("resolve context: %v", err)
	}

	if ctx.HostMachine == nil {
		t.Fatalf("expected host node present")
	}
	if ctx.PhysicalMachine != nil {
		t.Fatalf("expected physical node dropped when host exists")
	}
}

func TestAnalyzerWithGraphProvider(t *testing.T) {
	events := []rca.AlarmEvent{
		{
			ID:       "evt-app-1",
			NodeType: rca.NodeTypeApp,
			Service:  "order-service",
			Occurred: time.Date(2024, 3, 1, 10, 0, 0, 0, time.UTC),
		},
		{
			ID:       "evt-app-2",
			NodeType: rca.NodeTypeApp,
			Service:  "payment-service",
			Occurred: time.Date(2024, 3, 1, 10, 0, 30, 0, time.UTC),
		},
	}

	provider := rca.NewGraphTopologyProvider(&mockGraphReader{})
	cfg := rca.DefaultConfig()
	cfg.Hierarchy = []rca.NodeType{rca.NodeTypeVirtualMachine, rca.NodeTypeHostMachine}
	cfg.Layers[rca.NodeTypeVirtualMachine] = rca.LayerConfig{
		CoverageThreshold: 0.5,
		MinChildren:       1,
		Weights:           rca.ScoreWeights{Coverage: 0.7, TimeLead: 0.2, Impact: 0.1},
	}
	cfg.Layers[rca.NodeTypeHostMachine] = rca.LayerConfig{
		CoverageThreshold: 0.5,
		MinChildren:       1,
		Weights:           rca.ScoreWeights{Coverage: 0.7, TimeLead: 0.2, Impact: 0.1},
	}

	analyzer, err := rca.NewAnalyzer(provider, nil, cfg)
	if err != nil {
		t.Fatalf("new analyzer: %v", err)
	}

	result, err := analyzer.Analyze(context.Background(), "window-graph", events)
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}

	if len(result.Candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(result.Candidates))
	}
	for _, cand := range result.Candidates {
		if cand.Node.Type == rca.NodeTypePhysicalMachine {
			t.Fatalf("physical machine should not appear as candidate in this scenario")
		}
	}
}

func buildAppRecord(service string) map[string]any {
	appKey := "APP_1"
	if service == "payment-service" {
		appKey = "APP_2"
	}

	return map[string]any{
		"app":               neo4j.Node{Id: 1, Labels: []string{"App"}, Props: map[string]any{"cmdb_key": appKey, "name": service}},
		"vm":                neo4j.Node{Id: 2, Labels: []string{"VirtualMachine", "Compute"}, Props: map[string]any{"cmdb_key": "VM_100", "name": "vm-100"}},
		"host":              neo4j.Node{Id: 3, Labels: []string{"HostMachine", "Compute"}, Props: map[string]any{"cmdb_key": "HM_10", "hostname": "host-10"}},
		"physical":          neo4j.Node{Id: 4, Labels: []string{"PhysicalMachine", "Compute"}, Props: map[string]any{"cmdb_key": "PM_1", "hostname": "pm-1"}},
		"np":                neo4j.Node{Id: 5, Labels: []string{"NetPartition"}, Props: map[string]any{"cmdb_key": "NP_1", "name": "net-1"}},
		"idc":               neo4j.Node{Id: 6, Labels: []string{"IDC"}, Props: map[string]any{"cmdb_key": "IDC_1", "name": "idc-1"}},
		"vm_app_count":      int64(2),
		"host_vm_count":     int64(3),
		"np_host_count":     int64(5),
		"np_physical_count": int64(2),
		"idc_np_count":      int64(1),
	}
}
