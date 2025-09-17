package cmdb

import (
	"strconv"
	"time"

	"cmdb2neo/internal/domain"
)

// BuildInitRows 根据 CMDB 快照生成建图所需的节点和关系。
func BuildInitRows(snapshot Snapshot) ([]domain.NodeRow, []domain.RelRow) {
	runID := snapshot.RunID
	if runID == "" {
		runID = time.Now().UTC().Format("20060102T150405Z")
	}
	now := time.Now().UTC()

	nodes := make([]domain.NodeRow, 0, len(snapshot.IDCs)+len(snapshot.NetworkPartitions)+len(snapshot.PhysicalMachines)+len(snapshot.HostMachines)+len(snapshot.VirtualMachines)+len(snapshot.Apps))
	rels := make([]domain.RelRow, 0, len(snapshot.NetworkPartitions)+len(snapshot.PhysicalMachines)+len(snapshot.HostMachines)+len(snapshot.VirtualMachines)+len(snapshot.Apps))

	idcKeyMap := make(map[string]string, len(snapshot.IDCs))
	for _, idc := range snapshot.IDCs {
		idStr := strconv.Itoa(idc.Id)
		key := domain.MakeKey(domain.PrefixIDC, idc.Id)
		idcKeyMap[idStr] = key
		nodes = append(nodes, domain.NodeRow{
			CMDBKey:   key,
			Labels:    []string{domain.LabelIDC},
			Properties: map[string]any{
				"cmdb_id":  idc.Id,
				"name":     idc.Name,
				"location": idc.Location,
			},
			RunID:     runID,
			UpdatedAt: now,
		})
	}

	npKeyMap := make(map[string]string, len(snapshot.NetworkPartitions))
	for _, np := range snapshot.NetworkPartitions {
		npStr := strconv.Itoa(np.Id)
		key := domain.MakeKey(domain.PrefixNetPartition, np.Id)
		npKeyMap[npStr] = key
		props := map[string]any{
			"cmdb_id": np.Id,
			"name":    np.Name,
			"cidr":    np.CIDR,
			"idc":     np.Idc,
		}
		if idcKey, ok := idcKeyMap[np.Idc]; ok {
			props["idc_key"] = idcKey
			rels = append(rels, domain.RelRow{
				StartKey:   idcKey,
				EndKey:     key,
				Type:       domain.RelHasPartition,
				Properties: map[string]any{"source": "cmdb"},
				RunID:      runID,
			})
		}
		nodes = append(nodes, domain.NodeRow{
			CMDBKey:   key,
			Labels:    []string{domain.LabelNetPartition},
			Properties: props,
			RunID:     runID,
			UpdatedAt: now,
		})
	}

	hostByIP := make(map[string]string, len(snapshot.HostMachines))
	for _, host := range snapshot.HostMachines {
		key := domain.MakeKey(domain.PrefixHostMachine, host.Id)
		if host.Ip != "" {
			hostByIP[host.Ip] = key
		}
		props := map[string]any{
			"cmdb_id":        host.Id,
			"hostname":       host.Hostname,
			"ip":             host.Ip,
			"idc":            host.Idc,
			"network_partion": host.NetworkPartion,
			"server_type":    host.ServerType,
		}
		if npKey, ok := npKeyMap[host.NetworkPartion]; ok {
			props["network_partion_key"] = npKey
			rels = append(rels, domain.RelRow{
				StartKey:   npKey,
				EndKey:     key,
				Type:       domain.RelHasHost,
				Properties: map[string]any{"source": "cmdb"},
				RunID:      runID,
			})
		}
		nodes = append(nodes, domain.NodeRow{
			CMDBKey: key,
			Labels: []string{
				domain.LabelHostMachine,
				domain.LabelMachine,
				domain.LabelCompute,
			},
			Properties: props,
			RunID:     runID,
			UpdatedAt: now,
		})
	}

	for _, pm := range snapshot.PhysicalMachines {
		key := domain.MakeKey(domain.PrefixPhysical, pm.Id)
		props := map[string]any{
			"cmdb_id":        pm.Id,
			"hostname":       pm.Hostname,
			"ip":             pm.Ip,
			"idc":            pm.Idc,
			"network_partion": pm.NetworkPartion,
			"server_type":    pm.ServerType,
		}
		if npKey, ok := npKeyMap[pm.NetworkPartion]; ok {
			props["network_partion_key"] = npKey
			rels = append(rels, domain.RelRow{
				StartKey:   npKey,
				EndKey:     key,
				Type:       domain.RelHasPhysical,
				Properties: map[string]any{"source": "cmdb"},
				RunID:      runID,
			})
		}
		nodes = append(nodes, domain.NodeRow{
			CMDBKey: key,
			Labels: []string{
				domain.LabelPhysicalMachine,
				domain.LabelMachine,
				domain.LabelCompute,
			},
			Properties: props,
			RunID:     runID,
			UpdatedAt: now,
		})
	}

	vmKeyByIP := make(map[string]string, len(snapshot.VirtualMachines))
	for _, vm := range snapshot.VirtualMachines {
		key := domain.MakeKey(domain.PrefixVirtual, vm.Id)
		if vm.Ip != "" {
			vmKeyByIP[vm.Ip] = key
		}
		props := map[string]any{
			"cmdb_id":        vm.Id,
			"hostname":       vm.Hostname,
			"ip":             vm.Ip,
			"host_ip":        vm.HostIp,
			"idc":            vm.Idc,
			"network_partion": vm.NetworkPartion,
			"server_type":    vm.ServerType,
		}
		if hostKey, ok := hostByIP[vm.HostIp]; ok && vm.HostIp != "" {
			rels = append(rels, domain.RelRow{
				StartKey:   hostKey,
				EndKey:     key,
				Type:       domain.RelHostsVM,
				Properties: map[string]any{"via": "host_ip"},
				RunID:      runID,
			})
		}
		nodes = append(nodes, domain.NodeRow{
			CMDBKey: key,
			Labels: []string{
				domain.LabelVirtualMachine,
				domain.LabelCompute,
			},
			Properties: props,
			RunID:     runID,
			UpdatedAt: now,
		})
	}

	for _, app := range snapshot.Apps {
		key := domain.MakeKey(domain.PrefixApp, app.Id)
		props := map[string]any{
			"cmdb_id": app.Id,
			"name":    app.Name,
			"ip":      app.Ip,
		}
		if vmKey, ok := vmKeyByIP[app.Ip]; ok && app.Ip != "" {
			rels = append(rels, domain.RelRow{
				StartKey:   key,
				EndKey:     vmKey,
				Type:       domain.RelAppDeploy,
				Properties: map[string]any{"via": "vm_ip"},
				RunID:      runID,
			})
		}
		nodes = append(nodes, domain.NodeRow{
			CMDBKey:   key,
			Labels:    []string{domain.LabelApp},
			Properties: props,
			RunID:     runID,
			UpdatedAt: now,
		})
	}

	return nodes, rels
}
