package app

import (
	"context"
	"fmt"

	"cmdb2neo/internal/cmdb"
	"cmdb2neo/internal/loader"
	"go.uber.org/zap"
)

// SyncFlow 负责增量同步。
type SyncFlow struct {
	CMDB    cmdb.Client
	Nodes   *loader.NodeUpserter
	Rels    *loader.RelUpserter
	Fixer   *loader.EdgeFixer
	Cleaner *loader.Cleaner
	Logger  *zap.Logger
}

func (f *SyncFlow) Run(ctx context.Context) error {
	if f == nil {
		return fmt.Errorf("sync flow 未初始化")
	}
	if f.CMDB == nil || f.Nodes == nil || f.Rels == nil || f.Cleaner == nil {
		return fmt.Errorf("sync flow 依赖未注入完整")
	}

	snapshot, err := f.CMDB.FetchSnapshot(ctx)
	if err != nil {
		return fmt.Errorf("拉取 CMDB 快照失败: %w", err)
	}
	if f.Logger != nil {
		f.Logger.Info("加载 CMDB 快照",
			zap.String("run_id", snapshot.RunID),
			zap.Int("idc", len(snapshot.IDCs)),
			zap.Int("np", len(snapshot.NetworkPartitions)),
			zap.Int("host", len(snapshot.HostMachines)),
			zap.Int("physical", len(snapshot.PhysicalMachines)),
			zap.Int("vm", len(snapshot.VirtualMachines)),
			zap.Int("app", len(snapshot.Apps)))
	}

	nodes, rels := cmdb.BuildInitRows(snapshot)

	if err := f.Nodes.UpsertNodes(ctx, nodes); err != nil {
		return fmt.Errorf("增量写入节点失败: %w", err)
	}
	if err := f.Rels.UpsertRels(ctx, rels); err != nil {
		return fmt.Errorf("增量写入关系失败: %w", err)
	}
	if f.Fixer != nil {
		if err := f.Fixer.Run(ctx, snapshot.RunID); err != nil {
			return fmt.Errorf("补边失败: %w", err)
		}
	}

	if err := f.Cleaner.HardDeleteRelationships(ctx, snapshot.RunID); err != nil {
		return fmt.Errorf("删除过期关系失败: %w", err)
	}
	if err := f.Cleaner.HardDeleteNodes(ctx, snapshot.RunID); err != nil {
		return fmt.Errorf("删除过期节点失败: %w", err)
	}

	if f.Logger != nil {
		f.Logger.Info("增量同步完成", zap.String("run_id", snapshot.RunID))
	}
	return nil
}
