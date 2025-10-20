package ioc

import (
	"context"

	"cmdb2neo/internal/app"
	"cmdb2neo/internal/job"
	"go.uber.org/zap"
)

// InitScheduler 构建定时任务调度器。
<<<<<<< HEAD
func InitScheduler(cfg *app.Config, logger *zap.Logger) *job.Scheduler {
	return job.NewScheduler(cfg, logger)
=======
func InitScheduler(cfg app.Config, svc *app.Service, logger *zap.Logger) *job.Scheduler {
	var syncFn func(context.Context) error
	if svc != nil {
		syncFn = svc.Sync
	}
	return job.NewScheduler(cfg, syncFn, logger)
}

// InitHourlyLogger 构建每小时日志任务。
func InitHourlyLogger(logger *zap.Logger) *job.HourlyLogger {
	return job.NewHourlyLogger(logger)
>>>>>>> recovery-branch
}
