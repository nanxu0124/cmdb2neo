package job

import (
	"context"
	"strings"
	"sync"
	"time"

	"cmdb2neo/internal/app"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

const defaultCronSpec = "0 7 * * *"

// Scheduler 负责基于 cron 表达式执行后台任务。
type Scheduler struct {
	cronExpr string
	logger   *zap.Logger
	cron     *cron.Cron
}

// NewScheduler 根据配置构建调度器。
func NewScheduler(cfg *app.Config, logger *zap.Logger) *Scheduler {
	spec := ""
	if cfg != nil {
		spec = strings.TrimSpace(cfg.Sync.JobCron)
	}
	if spec == "" {
		spec = defaultCronSpec
	}
	return &Scheduler{cronExpr: spec, logger: logger}
}

// Start 启动调度器，返回用于停止任务的函数。
func (s *Scheduler) Start(parent context.Context) context.CancelFunc {
	if s == nil {
		return func() {}
	}
	c := cron.New()
	id, err := c.AddFunc(s.cronExpr, s.runOnce)
	if err != nil {
		if s.logger != nil {
			s.logger.Error("failed to register cron job", zap.String("cron", s.cronExpr), zap.Error(err))
		}
		return func() {}
	}
	s.cron = c
	c.Start()
	if s.logger != nil {
		entry := c.Entry(id)
		s.logger.Info("job scheduler started", zap.String("cron", s.cronExpr), zap.Time("next", entry.Next))
	}

	var once sync.Once
	stop := func() {
		once.Do(func() {
			ctx := s.cron.Stop()
			<-ctx.Done()
			if s.logger != nil {
				s.logger.Info("job scheduler stopped")
			}
		})
	}

	go func() {
		<-parent.Done()
		stop()
	}()

	return stop
}

func (s *Scheduler) runOnce() {
	if s.logger != nil {
		s.logger.Info("job tick", zap.Time("timestamp", time.Now()))
	}
}
