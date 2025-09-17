package app

import (
	"context"
	"fmt"

	"cmdb2neo/internal/cmdb"
	"cmdb2neo/internal/loader"
	"go.uber.org/zap"
)

// InitFlow 负责首跑初始化：建 schema -> 写节点 -> 写关系 -> 补边。
type InitFlow struct {
	CMDB   cmdb.Client
	Schema *loader.SchemaManager
	Nodes  *loader.NodeUpserter
	Rels   *loader.RelUpserter
	Fixer  *loader.EdgeFixer
	Logger *zap.Logger
}

// Run 执行初始化流程。
func (f *InitFlow) Run(ctx context.Context) error {
	if f.CMDB == nil || f.Nodes == nil || f.Rels == nil {
		return fmt.Errorf("初始化依赖未注入完整")
	}
	if f.Logger == nil {
		f.Logger = zap.NewNop()
	}

	snapshot, err := f.CMDB.FetchSnapshot(ctx)
	if err != nil {
		return fmt.Errorf("拉取 CMDB 快照失败: %w", err)
	}
	f.Logger.Info("加载 CMDB 快照", zap.Int("idc", len(snapshot.IDCs)), zap.Int("np", len(snapshot.NetworkPartitions)), zap.Int("host", len(snapshot.HostMachines)), zap.Int("physical", len(snapshot.PhysicalMachines)), zap.Int("vm", len(snapshot.VirtualMachines)), zap.Int("app", len(snapshot.Apps)))

	nodes, rels := cmdb.BuildInitRows(snapshot)

	if f.Schema != nil {
		if err := f.Schema.Ensure(ctx); err != nil {
			return err
		}
	}

	if err := f.Nodes.InitNodes(ctx, nodes); err != nil {
		return err
	}
	if err := f.Rels.InitRels(ctx, rels); err != nil {
		return err
	}
	if f.Fixer != nil {
		if err := f.Fixer.Run(ctx, snapshot.RunID); err != nil {
			return err
		}
	}
	f.Logger.Info("初始化同步完成")
	return nil
}
