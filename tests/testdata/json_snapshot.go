package testdata

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"cmdb2neo/internal/cmdb"
)

// LoadSnapshotFromJSON 读取 tests/unit 目录下的 JSON，组装成 CMDB 快照。
func LoadSnapshotFromJSON(tb testing.TB) cmdb.Snapshot {
	tb.Helper()

	idcs := loadIDCs(tb)
	idcNameToID := make(map[string]string, len(idcs))
	for _, idc := range idcs {
		idcNameToID[idc.Name] = strconv.Itoa(idc.Id)
	}

	networkPartitions, npNameToID := loadNetworkPartitions(tb, idcNameToID)
	hostMachines := loadHostMachines(tb, idcNameToID, npNameToID)
	physicalMachines := loadPhysicalMachines(tb, idcNameToID, npNameToID)
	virtualMachines := loadVirtualMachines(tb, idcNameToID, npNameToID)
	apps := loadApps(tb)

	return cmdb.Snapshot{
		RunID:             "json-fixture",
		IDCs:              idcs,
		NetworkPartitions: networkPartitions,
		HostMachines:      hostMachines,
		PhysicalMachines:  physicalMachines,
		VirtualMachines:   virtualMachines,
		Apps:              apps,
	}
}

func loadIDCs(tb testing.TB) []cmdb.IDC {
	tb.Helper()
	type raw struct {
		Id       int    `json:"id"`
		Name     string `json:"name"`
		Location string `json:"location"`
	}
	var items []raw
	readJSON(tb, "idc.json", &items)
	result := make([]cmdb.IDC, 0, len(items))
	for _, item := range items {
		result = append(result, cmdb.IDC{Id: item.Id, Name: item.Name, Location: item.Location})
	}
	return result
}

func loadNetworkPartitions(tb testing.TB, idcNameToID map[string]string) ([]cmdb.NetworkPartition, map[string]int) {
	tb.Helper()
	type raw struct {
		Id   int    `json:"id"`
		Idc  string `json:"idc"`
		Name string `json:"Name"`
		CIDR string `json:"CIDR"`
	}
	var items []raw
	readJSON(tb, "network_partition.json", &items)
	result := make([]cmdb.NetworkPartition, 0, len(items))
	nameToID := make(map[string]int, len(items))
	for _, item := range items {
		idc := item.Idc
		if mapped, ok := idcNameToID[item.Idc]; ok {
			idc = mapped
		}
		result = append(result, cmdb.NetworkPartition{Id: item.Id, Idc: idc, Name: item.Name, CIDR: item.CIDR})
		nameToID[item.Name] = item.Id
	}
	return result, nameToID
}

func loadHostMachines(tb testing.TB, idcNameToID map[string]string, npNameToID map[string]int) []cmdb.HostMachine {
	tb.Helper()
	type raw struct {
		Id               int    `json:"id"`
		Idc              string `json:"idc"`
		NetworkPartition string `json:"network_partition"`
		ServerType       int    `json:"server_type"`
		Ip               string `json:"ip"`
		HostName         string `json:"host_name"`
	}
	var items []raw
	readJSON(tb, "host_machine.json", &items)
	result := make([]cmdb.HostMachine, 0, len(items))
	for _, item := range items {
		idc := item.Idc
		if mapped, ok := idcNameToID[item.Idc]; ok {
			idc = mapped
		}
		np := item.NetworkPartition
		if id, ok := npNameToID[item.NetworkPartition]; ok {
			np = strconv.Itoa(id)
		}
		result = append(result, cmdb.HostMachine{
			Id:             item.Id,
			Idc:            idc,
			NetworkPartion: np,
			ServerType:     strconv.Itoa(item.ServerType),
			Ip:             item.Ip,
			Hostname:       item.HostName,
		})
	}
	return result
}

func loadPhysicalMachines(tb testing.TB, idcNameToID map[string]string, npNameToID map[string]int) []cmdb.PhysicalMachine {
	tb.Helper()
	type raw struct {
		Id               int    `json:"id"`
		Idc              string `json:"idc"`
		NetworkPartition string `json:"network_partition"`
		ServerType       int    `json:"server_type"`
		Ip               string `json:"ip"`
		HostName         string `json:"host_name"`
	}
	var items []raw
	readJSON(tb, "physical_machine.json", &items)
	result := make([]cmdb.PhysicalMachine, 0, len(items))
	for _, item := range items {
		idc := item.Idc
		if mapped, ok := idcNameToID[item.Idc]; ok {
			idc = mapped
		}
		np := item.NetworkPartition
		if id, ok := npNameToID[item.NetworkPartition]; ok {
			np = strconv.Itoa(id)
		}
		result = append(result, cmdb.PhysicalMachine{
			Id:             item.Id,
			Idc:            idc,
			NetworkPartion: np,
			ServerType:     strconv.Itoa(item.ServerType),
			Ip:             item.Ip,
			Hostname:       item.HostName,
		})
	}
	return result
}

func loadVirtualMachines(tb testing.TB, idcNameToID map[string]string, npNameToID map[string]int) []cmdb.VirtualMachine {
	tb.Helper()
	type raw struct {
		Id               int    `json:"id"`
		Idc              string `json:"idc"`
		NetworkPartition string `json:"network_partition"`
		ServerType       int    `json:"server_type"`
		Ip               string `json:"ip"`
		HostName         string `json:"host_name"`
		HostIp           string `json:"host_ip"`
	}
	var items []raw
	readJSON(tb, "virtual_machine.json", &items)
	result := make([]cmdb.VirtualMachine, 0, len(items))
	for _, item := range items {
		idc := item.Idc
		if mapped, ok := idcNameToID[item.Idc]; ok {
			idc = mapped
		}
		np := item.NetworkPartition
		if id, ok := npNameToID[item.NetworkPartition]; ok {
			np = strconv.Itoa(id)
		}
		result = append(result, cmdb.VirtualMachine{
			Id:             item.Id,
			Idc:            idc,
			NetworkPartion: np,
			ServerType:     strconv.Itoa(item.ServerType),
			Ip:             item.Ip,
			Hostname:       item.HostName,
			HostIp:         item.HostIp,
		})
	}
	return result
}

func loadApps(tb testing.TB) []cmdb.App {
	tb.Helper()
	type raw struct {
		Id   int    `json:"id"`
		Ip   string `json:"ip"`
		Name string `json:"name"`
	}
	var items []raw
	readJSON(tb, "app.json", &items)
	result := make([]cmdb.App, 0, len(items))
	for _, item := range items {
		result = append(result, cmdb.App{Id: item.Id, Ip: item.Ip, Name: item.Name})
	}
	return result
}

func readJSON(tb testing.TB, filename string, v any) {
	tb.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		tb.Fatalf("runtime caller failed for %s", filename)
	}
	baseDir := filepath.Dir(file)     // tests/testdata
	testsDir := filepath.Dir(baseDir) // tests
	path := filepath.Join(testsDir, "unit", filename)
	data, err := os.ReadFile(path)
	if err != nil {
		tb.Fatalf("read %s failed: %v", path, err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		tb.Fatalf("unmarshal %s failed: %v", path, err)
	}
}
