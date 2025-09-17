package unit

import (
	"testing"

	"cmdb2neo/internal/cmdb"
)

func TestBuildInitRows(t *testing.T) {
	snapshot := cmdb.Snapshot{
		RunID:             "test",
		IDCs:              []cmdb.IDC{{Id: 1, Name: "TestIDC"}},
		NetworkPartitions: []cmdb.NetworkPartition{{Id: 10, Idc: "1", Name: "prod", CIDR: "10.0.0.0/24"}},
		HostMachines:      []cmdb.HostMachine{{Id: 100, Idc: "1", NetworkPartion: "10", ServerType: "kvm", Hostname: "host1", Ip: "10.0.0.10"}},
		PhysicalMachines:  []cmdb.PhysicalMachine{{Id: 200, Idc: "1", NetworkPartion: "10", ServerType: "baremetal", Hostname: "pm1", Ip: "10.0.0.11"}},
		VirtualMachines:   []cmdb.VirtualMachine{{Id: 300, Idc: "1", NetworkPartion: "10", ServerType: "vm", Hostname: "vm1", Ip: "10.0.0.12", HostIp: "10.0.0.10"}},
		Apps:              []cmdb.App{{Id: 400, Name: "app1", Ip: "10.0.0.12"}},
	}

	nodes, rels := cmdb.BuildInitRows(snapshot)
	if len(nodes) != 6 {
		t.Fatalf("expect 6 nodes, got %d", len(nodes))
	}
	if len(rels) == 0 {
		t.Fatalf("expect relationships, got 0")
	}
}
