package rcav2

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

// TopoNode 表示在 StageB 中构建的拓扑树节点。
type TopoNode struct {
	Node
	Parent   *TopoNode
	Children map[string]*TopoNode
	Impacts  map[string]*TopoImpact
	Events   map[string]AlarmEventRef
}

// TopoImpact 描述父节点下的某个子节点对告警的影响。
type TopoImpact struct {
	Node   NodeRef
	Events map[string]AlarmEventRef
}

// NewTopoNode 基于 Node 信息创建拓扑节点。
func NewTopoNode(node Node) *TopoNode {
	return &TopoNode{
		Node:     node,
		Children: make(map[string]*TopoNode),
		Impacts:  make(map[string]*TopoImpact),
		Events:   make(map[string]AlarmEventRef),
	}
}

// AddEvent 将事件记录到当前节点。
func (n *TopoNode) AddEvent(id string, ref AlarmEventRef) {
	if n.Events == nil {
		n.Events = make(map[string]AlarmEventRef)
	}
	n.Events[id] = ref
}

// AttachChild 维护父子关系。
func (n *TopoNode) AttachChild(child *TopoNode) {
	if child == nil {
		return
	}
	if n.Children == nil {
		n.Children = make(map[string]*TopoNode)
	}
	n.Children[child.NodeRef.Key] = child
	child.Parent = n
}

// AddImpact 在父节点上记录来自子节点的告警。
func (n *TopoNode) AddImpact(child *TopoNode, ref AlarmEventRef) {
	if child == nil {
		return
	}
	if n.Impacts == nil {
		n.Impacts = make(map[string]*TopoImpact)
	}
	impact, ok := n.Impacts[child.NodeRef.Key]
	if !ok {
		impact = &TopoImpact{Node: child.NodeRef, Events: make(map[string]AlarmEventRef)}
		n.Impacts[child.NodeRef.Key] = impact
	}
	impact.Events[ref.ID] = ref
}

// Coverage 计算节点的告警覆盖率以及被影响的子节点集合。
func (n *TopoNode) Coverage() float64 {
	if len(n.Children) == 0 && len(n.Impacts) == 0 {
		return 1.0
	}

	childType := n.ChildType()
	total := n.ChildCounts[childType]

	coverage := float64(len(n.Impacts)) / float64(total)
	if coverage > 1 {
		coverage = 1
	}
	return coverage
}

// ChildType 返回当前节点活跃子节点的类型。
func (n *TopoNode) ChildType() NodeType {
	for _, impact := range n.Impacts {
		if impact == nil || len(impact.Events) == 0 {
			continue
		}
		return impact.Node.Type
	}
	return NodeType("")
}

// ComputeScore 根据权重计算节点得分。
func (n *TopoNode) ComputeScore(weights ScoreWeights) ScoreDetail {
	coverage := n.Coverage()

	raw := weights.Base + weights.Coverage*coverage
	if raw < 0 {
		raw = 0
	}
	if raw > 1 {
		raw = 1
	}
	return ScoreDetail{
		Coverage:   coverage,
		Base:       weights.Base,
		RawScore:   raw,
		Normalized: raw,
	}
}

type AppOutage struct {
	AppName       string          `json:"app_name"`
	Datacenter    string          `json:"datacenter"`
	TotalNodes    int             `json:"total_nodes"`
	AlarmedNodes  int             `json:"alarmed_nodes"`
	Coverage      float64         `json:"coverage"`
	Threshold     float64         `json:"threshold"`
	AffectedNodes []AppOutageNode `json:"affected_nodes"`
}

type AppOutageNode struct {
	ServerType ServerType `json:"server_type"`
	IP         string     `json:"ip"`
	HostIP     string     `json:"host_ip,omitempty"`
	Partition  string     `json:"partition,omitempty"`
	RuleNames  []string   `json:"rule_names,omitempty"`
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

// AlarmPath 记录某个候选节点下的触发链路。
type AlarmPath struct {
	Candidate NodeRef      `json:"candidate"`
	Impacts   []PathImpact `json:"impacts"`
}

// PathImpact 描述一个子节点及由它继续扩散的告警。
type PathImpact struct {
	Node    NodeRef         `json:"node"`
	Events  []AlarmEventRef `json:"events"`
	Impacts []PathImpact    `json:"impacts,omitempty"`
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
	AppOutages []AppOutage `json:"app_outages"`
	Candidates []Candidate `json:"candidates"`
	Paths      []AlarmPath `json:"paths,omitempty"`
	Prompt     string      `json:"prompt,omitempty"`
}
