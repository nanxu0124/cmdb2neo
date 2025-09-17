package domain

import (
	"fmt"
	"sort"
	"strings"
)

const (
	LabelIDC             = "IDC"
	LabelNetPartition    = "NetPartition"
	LabelPhysicalMachine = "PhysicalMachine"
	LabelHostMachine     = "HostMachine"
	LabelVirtualMachine  = "VirtualMachine"
	LabelApp             = "App"
	LabelMachine         = "Machine"
	LabelCompute         = "Compute"

	RelHasPartition = "HAS_PARTITION"
	RelHasHost      = "HAS_HOST"
	RelHasPhysical  = "HAS_PHYSICAL"
	RelHostsVM      = "HOSTS_VM"
	RelAppDeploy    = "DEPLOYED_ON"
)

const (
	PrefixIDC          = "IDC"
	PrefixNetPartition = "NP"
	PrefixHostMachine  = "HM"
	PrefixPhysical     = "PM"
	PrefixVirtual      = "VM"
	PrefixApp          = "APP"
)

// MakeKey 统一生成 cmdb_key，带上前缀以避免不同实体冲突。
func MakeKey(prefix string, rawID any) string {
	return fmt.Sprintf("%s_%v", prefix, rawID)
}

// LabelPattern 根据标签集合拼成 Cypher 模板所需的字符串，如 ":A:B"。
func LabelPattern(labels []string) string {
	if len(labels) == 0 {
		return ""
	}
	sorted := append([]string(nil), labels...)
	sort.Strings(sorted)
	return ":" + strings.Join(sorted, ":")
}

// JoinLabels 简单拼接标签用于 map key（内部使用）。
func JoinLabels(labels []string) string {
	sorted := append([]string(nil), labels...)
	sort.Strings(sorted)
	return strings.Join(sorted, ":")
}
