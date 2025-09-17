package cmdb

// IDC 表示机房。
type IDC struct {
	Id       int    `json:"id"`
	Name     string `json:"name"`
	Location string `json:"location"`
}

// NetworkPartition 表示网络分区。
type NetworkPartition struct {
	Id   int    `json:"id"`
	Idc  string `json:"idc"`
	Name string `json:"name"`
	CIDR string `json:"cidr"`
}

// PhysicalMachine 表示物理机。
type PhysicalMachine struct {
	Id             int    `json:"id"`
	Idc            string `json:"idc"`
	NetworkPartion string `json:"network_partion"`
	ServerType     string `json:"server_type"`
	Ip             string `json:"ip"`
	Hostname       string `json:"hostname"`
}

// HostMachine 表示宿主机。
type HostMachine struct {
	Id             int    `json:"id"`
	Idc            string `json:"idc"`
	NetworkPartion string `json:"network_partion"`
	ServerType     string `json:"server_type"`
	Ip             string `json:"ip"`
	Hostname       string `json:"hostname"`
}

// VirtualMachine 表示虚拟机。
type VirtualMachine struct {
	Id             int      `json:"id"`
	Idc            string   `json:"idc"`
	NetworkPartion string   `json:"network_partion"`
	ServerType     string   `json:"server_type"`
	Ip             string   `json:"ip"`
	Hostname       string   `json:"hostname"`
	HostIp         string   `json:"host_ip"`
}

// App 表示应用。
type App struct {
	Id   int    `json:"id"`
	Ip   string `json:"ip"`
	Name string `json:"name"`
}

// Snapshot 汇总快照数据。
type Snapshot struct {
	RunID             string
	IDCs              []IDC
	NetworkPartitions []NetworkPartition
	PhysicalMachines  []PhysicalMachine
	HostMachines      []HostMachine
	VirtualMachines   []VirtualMachine
	Apps              []App
}
