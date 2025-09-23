package rcav1

import "time"

// ServerType 表示告警所在的承载层。
type ServerType string

const (
	ServerTypeHost     ServerType = "1"
	ServerTypeVM       ServerType = "2"
	ServerTypePhysical ServerType = "3"
)

// NodeType 用于表示拓扑层级。
type NodeType string

const (
	NodeTypeApp             NodeType = "App"
	NodeTypeVirtualMachine  NodeType = "VirtualMachine"
	NodeTypeHostMachine     NodeType = "HostMachine"
	NodeTypePhysicalMachine NodeType = "PhysicalMachine"
	NodeTypeNetPartition    NodeType = "NetPartition"
	NodeTypeIDC             NodeType = "IDC"
)

// AlarmEvent 描述一次告警事件输入。
type AlarmEvent struct {
	AppName          string     `json:"app_name"`
	Datacenter       string     `json:"datacenter"`
	HostIP           string     `json:"host_ip"`
	IP               string     `json:"ip"`
	NetworkPartition string     `json:"network_partition"`
	ServerType       ServerType `json:"server_type"`
	RuleName         string     `json:"rule_name"`
	OccurredAt       time.Time  `json:"occurred_at"`
}

// NodeRef 是拓扑节点的引用信息。
type NodeRef struct {
	Key       string         `json:"key"`
	Type      NodeType       `json:"type"`
	Name      string         `json:"name"`
	IDC       string         `json:"idc"`
	Partition string         `json:"partition,omitempty"`
	Labels    []string       `json:"labels,omitempty"`
	Props     map[string]any `json:"props,omitempty"`
}

// Node 在 NodeRef 的基础上补充子节点基线。
type Node struct {
	NodeRef
	ChildCounts map[NodeType]int `json:"child_counts,omitempty"`
}

// Chain 表示一条完整的拓扑链路。
type Chain struct {
	App             *Node
	VirtualMachine  *Node
	HostMachine     *Node
	PhysicalMachine *Node
	NetPartition    *Node
	IDC             *Node
}

// Instance 代表某个 APP 实例的部署节点信息。
type Instance struct {
	AppName    string
	ServerType ServerType
	Datacenter string
	IP         string
	HostIP     string
	Partition  string
	NodeKey    string
}

// Candidate 根因候选输出。
type Candidate struct {
	Node       NodeRef     `json:"node"`
	Confidence float64     `json:"confidence"`
	Coverage   float64     `json:"coverage"`
	Reason     string      `json:"reason"`
	Metrics    ScoreDetail `json:"metrics"`
	Explained  []string    `json:"explained_event_ids"`
}

// ScoreDetail 拆解得分来源。
type ScoreDetail struct {
	Coverage   float64 `json:"coverage"`
	Impact     float64 `json:"impact"`
	Base       float64 `json:"base"`
	RawScore   float64 `json:"raw_score"`
	Normalized float64 `json:"normalized"`
}

// AlarmPath 记录候选节点下的告警链路。
type AlarmPath struct {
	Candidate NodeRef      `json:"candidate"`
	Impacts   []PathImpact `json:"impacts"`
}

// PathImpact 描述候选子节点和关联告警。
type PathImpact struct {
	Node   NodeRef         `json:"node"`
	Events []AlarmEventRef `json:"events"`
}

// AlarmEventRef 是压缩后的事件引用。
type AlarmEventRef struct {
	ID       string    `json:"id"`
	RuleName string    `json:"rule_name"`
	NodeType NodeType  `json:"node_type"`
	Occurred time.Time `json:"occurred_at"`
}

// Result 为一次 RCA 分析输出。
type Result struct {
	Candidates        []Candidate  `json:"candidates"`
	Paths             []AlarmPath  `json:"paths"`
	UnexplainedEvents []AlarmEvent `json:"unexplained_events"`
}
