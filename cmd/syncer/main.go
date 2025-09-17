package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"cmdb2neo/internal/app"
	"cmdb2neo/internal/cmdb"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "configs/config.yaml", "配置文件路径")
	flag.Parse()

	if flag.NArg() == 0 {
		usage()
		os.Exit(1)
	}

	cmd := flag.Arg(0)

	cfg, err := app.LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	client := &cmdb.StaticClient{Snapshot: mockSnapshot()}

	svc, err := app.NewService(ctx, cfg, client)
	if err != nil {
		fmt.Fprintf(os.Stderr, "构建服务失败: %v\n", err)
		os.Exit(1)
	}
	defer svc.Close(ctx)

	switch cmd {
	case "init":
		err = svc.Init(ctx)
	case "sync":
		err = svc.Sync(ctx)
	case "reconcile":
		err = svc.Reconcile(ctx)
	case "validate":
		err = svc.Validate(ctx)
	default:
		usage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "%s 执行失败: %v\n", cmd, err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Println("用法: syncer [-config configs/config.yaml] {init|sync|reconcile|validate}")
}

func mockSnapshot() cmdb.Snapshot {
	return cmdb.Snapshot{
		RunID:             time.Now().UTC().Format("20060102T150405Z"),
		IDCs:              []cmdb.IDC{{Id: 1, Name: "默认机房", Location: "上海"}},
		NetworkPartitions: []cmdb.NetworkPartition{{Id: 10, Idc: "1", Name: "prod-net", CIDR: "10.0.0.0/24"}},
		HostMachines:      []cmdb.HostMachine{{Id: 100, Idc: "1", NetworkPartion: "10", ServerType: "kvm", Hostname: "host-1", Ip: "10.0.0.10"}},
		PhysicalMachines:  []cmdb.PhysicalMachine{{Id: 200, Idc: "1", NetworkPartion: "10", ServerType: "baremetal", Hostname: "pm-1", Ip: "10.0.0.11"}},
		VirtualMachines:   []cmdb.VirtualMachine{{Id: 300, Idc: "1", NetworkPartion: "10", ServerType: "vm", Hostname: "vm-1", Ip: "10.0.0.12", HostIp: "10.0.0.10"}},
		Apps:              []cmdb.App{{Id: 400, Name: "order-service", Ip: "10.0.0.12"}},
	}
}
