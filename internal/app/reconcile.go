package app

import (
	"context"

	"go.uber.org/zap"
)

// ReconcileFlow 用于软删、硬删、资源回收，占位实现。
type ReconcileFlow struct {
	Logger *zap.Logger
}

func (f *ReconcileFlow) Run(ctx context.Context) error {
	if f.Logger != nil {
		f.Logger.Info("对账流程暂未实现")
	}
	return nil
}
