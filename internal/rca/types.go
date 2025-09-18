package rca

import "time"

// NodeType 表示拓扑节点类型。
type NodeType string

const (
	NodeTypeApp             NodeType = "App"
	NodeTypeVirtualMachine  NodeType = "VirtualMachine"
	NodeTypeHostMachine     NodeType = "HostMachine"
	NodeTypePhysicalMachine NodeType = "PhysicalMachine"
	NodeTypeNetPartition    NodeType = "NetPartition"
	NodeTypeIDC             NodeType = "IDC"
)

// NodeRef 为图中的节点引用，包含根因分析需要的基本信息。
type NodeRef struct {
	CMDBKey string         `json:"cmdb_key"`
	Type    NodeType       `json:"type"`
	Name    string         `json:"name"`
	Labels  []string       `json:"labels,omitempty"`
	Props   map[string]any `json:"props,omitempty"`
}

// Node 表示带有子节点基线信息的拓扑节点。
type Node struct {
	NodeRef
	ChildCounts map[NodeType]int `json:"child_counts,omitempty"`
}

// AlarmEvent 表示同一时间窗口内的一条原始告警。
type AlarmEvent struct {
	ID       string            `json:"id"`
	Source   string            `json:"source"`
	Priority string            `json:"priority"`
	NodeType NodeType          `json:"node_type"`
	IP       string            `json:"ip"`
	HostIP   string            `json:"host_ip"`
	Service  string            `json:"service"`
	Occurred time.Time         `json:"occurred_at"`
	Attrs    map[string]string `json:"attributes,omitempty"`
}

// AlarmContext 代表一次告警在图中的完整路径。
type AlarmContext struct {
	App             *Node
	VirtualMachine  *Node
	HostMachine     *Node
	PhysicalMachine *Node
	NetPartition    *Node
	IDC             *Node
}

// Candidate 根因候选节点及其得分详情。
type Candidate struct {
	Node       NodeRef     `json:"node"`
	Confidence float64     `json:"confidence"`
	Coverage   float64     `json:"coverage"`
	Metrics    ScoreDetail `json:"metrics"`
	Explained  []string    `json:"explained_event_ids"`
}

// ScoreDetail 拆解根因分数来源。
type ScoreDetail struct {
	Coverage     float64 `json:"coverage"`
	TimeLead     float64 `json:"time_lead"`
	Impact       float64 `json:"impact"`
	Base         float64 `json:"base"`
	RawScore     float64 `json:"raw_score"`
	Normalized   float64 `json:"normalized"`
	WindowLength float64 `json:"window_length_seconds"`
}

// AlarmPath 描述候选节点下的告警链路。
type AlarmPath struct {
	Candidate NodeRef      `json:"candidate"`
	Impacts   []PathImpact `json:"impacts"`
}

// PathImpact 表示候选节点某个子节点及相关告警。
type PathImpact struct {
	Node   NodeRef         `json:"node"`
	Events []AlarmEventRef `json:"events"`
}

// AlarmEventRef 为压缩后的事件信息。
type AlarmEventRef struct {
	ID       string    `json:"id"`
	NodeType NodeType  `json:"node_type"`
	Occurred time.Time `json:"occurred_at"`
}

// Result 为一次根因分析的输出。
type Result struct {
	Candidates        []Candidate  `json:"candidates"`
	Paths             []AlarmPath  `json:"paths"`
	UnexplainedEvents []AlarmEvent `json:"unexplained_events"`
}
