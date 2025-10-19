package ioc

import (
	"cmdb2neo/internal/app"
	"cmdb2neo/internal/job"
	"go.uber.org/zap"
)

// InitScheduler 构建定时任务调度器。
func InitScheduler(cfg *app.Config, logger *zap.Logger) *job.Scheduler {
	return job.NewScheduler(cfg, logger)
}
