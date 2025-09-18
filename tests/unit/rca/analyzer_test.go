package rca_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"cmdb2neo/internal/rca"
)

type mockProvider struct {
	contexts map[string]rca.AlarmContext
}

type mockStore struct {
	saved  bool
	window string
	result rca.Result
}

func (m *mockProvider) ResolveContext(_ context.Context, event rca.AlarmEvent) (rca.AlarmContext, error) {
	ctx, ok := m.contexts[event.ID]
	if !ok {
		return rca.AlarmContext{}, fmt.Errorf("unknown event %s", event.ID)
	}
	return ctx, nil
}

func (m *mockStore) Save(_ context.Context, windowID string, result rca.Result) error {
	m.saved = true
	m.window = windowID
	m.result = result
	return nil
}

func TestAnalyzerBasic(t *testing.T) {
	events := loadAlarmEvents(t)

	vm1 := newNode("VM_100", rca.NodeTypeVirtualMachine, "vm-100", map[rca.NodeType]int{rca.NodeTypeApp: 2})
	host1 := newNode("HM_10", rca.NodeTypeHostMachine, "host-10", map[rca.NodeType]int{rca.NodeTypeVirtualMachine: 2})
	np1 := newNode("NP_1", rca.NodeTypeNetPartition, "np-1", map[rca.NodeType]int{rca.NodeTypeHostMachine: 1})
	idc := newNode("IDC_1", rca.NodeTypeIDC, "idc-1", map[rca.NodeType]int{rca.NodeTypeNetPartition: 1})

	contexts := map[string]rca.AlarmContext{
		"evt-app-1": {
			App:            newNode("APP_1", rca.NodeTypeApp, "order-service", nil),
			VirtualMachine: vm1,
			HostMachine:    host1,
			NetPartition:   np1,
			IDC:            idc,
		},
		"evt-app-2": {
			App:            newNode("APP_2", rca.NodeTypeApp, "payment-service", nil),
			VirtualMachine: vm1,
			HostMachine:    host1,
			NetPartition:   np1,
			IDC:            idc,
		},
	}

	provider := &mockProvider{contexts: contexts}
	store := &mockStore{}

	cfg := rca.DefaultConfig()
	cfg.Hierarchy = []rca.NodeType{rca.NodeTypeVirtualMachine, rca.NodeTypeHostMachine}
	vmConfig := cfg.Layers[rca.NodeTypeVirtualMachine]
	vmConfig.CoverageThreshold = 0.5
	vmConfig.MinChildren = 1
	cfg.Layers[rca.NodeTypeVirtualMachine] = vmConfig

	hostConfig := cfg.Layers[rca.NodeTypeHostMachine]
	hostConfig.CoverageThreshold = 0.5
	hostConfig.MinChildren = 1
	cfg.Layers[rca.NodeTypeHostMachine] = hostConfig

	analyzer, err := rca.NewAnalyzer(provider, store, cfg)
	if err != nil {
		t.Fatalf("new analyzer: %v", err)
	}

	result, err := analyzer.Analyze(context.Background(), "window-001", events)
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}

	if !store.saved || store.window != "window-001" {
		t.Fatalf("expected store to persist result, got saved=%v window=%s", store.saved, store.window)
	}

	if len(result.Candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(result.Candidates))
	}

	vmCandidate := findCandidate(t, result.Candidates, rca.NodeTypeVirtualMachine)
	if vmCandidate.Coverage < 0.99 {
		t.Fatalf("vm coverage expect ~1, got %.2f", vmCandidate.Coverage)
	}

	hostCandidate := findCandidate(t, result.Candidates, rca.NodeTypeHostMachine)
	if hostCandidate.Coverage < 0.49 || hostCandidate.Coverage > 0.51 {
		t.Fatalf("host coverage expect 0.5, got %.3f", hostCandidate.Coverage)
	}

	if len(result.Paths) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(result.Paths))
	}

	if len(result.UnexplainedEvents) != 0 {
		t.Fatalf("expected no unexplained events, got %d", len(result.UnexplainedEvents))
	}
}

func newNode(key string, typ rca.NodeType, name string, childCounts map[rca.NodeType]int) *rca.Node {
	return &rca.Node{
		NodeRef: rca.NodeRef{
			CMDBKey: key,
			Type:    typ,
			Name:    name,
		},
		ChildCounts: childCounts,
	}
}

func loadAlarmEvents(t *testing.T) []rca.AlarmEvent {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime caller failed for %s", "alerm_events.json")
	}
	baseDir := filepath.Dir(file)
	testsDir := filepath.Dir(baseDir)
	path := filepath.Join(testsDir, "alerm_events.json")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read events json: %v", err)
	}
	var raw []struct {
		ID         string `json:"id"`
		Source     string `json:"source"`
		Priority   string `json:"priority"`
		NodeType   string `json:"node_type"`
		IP         string `json:"ip"`
		HostIP     string `json:"host_ip"`
		Service    string `json:"service"`
		OccurredAt string `json:"occurred_at"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal events json: %v", err)
	}
	events := make([]rca.AlarmEvent, 0, len(raw))
	for _, item := range raw {
		ts, err := time.Parse(time.RFC3339, item.OccurredAt)
		if err != nil {
			t.Fatalf("parse time %s: %v", item.OccurredAt, err)
		}
		events = append(events, rca.AlarmEvent{
			ID:       item.ID,
			Source:   item.Source,
			Priority: item.Priority,
			NodeType: rca.NodeType(item.NodeType),
			IP:       item.IP,
			HostIP:   item.HostIP,
			Service:  item.Service,
			Occurred: ts,
		})
	}
	return events
}

func findCandidate(t *testing.T, list []rca.Candidate, targetType rca.NodeType) rca.Candidate {
	t.Helper()
	for _, c := range list {
		if c.Node.Type == targetType {
			return c
		}
	}
	t.Fatalf("candidate with type %s not found", targetType)
	return rca.Candidate{}
}
