package app

import (
	"context"

	"go.uber.org/zap"
)

// SyncFlow 预留增量同步编排，当前实现为占位。
type SyncFlow struct {
	Logger *zap.Logger
}

func (f *SyncFlow) Run(ctx context.Context) error {
	if f.Logger != nil {
		f.Logger.Info("增量同步暂未实现，直接返回")
	}
	return nil
}
