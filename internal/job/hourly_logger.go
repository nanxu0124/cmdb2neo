package job

import (
	"context"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

// HourlyLogger 每小时输出日志，主要用于健康探针或占位任务。
type HourlyLogger struct {
	logger *zap.Logger
	cron   *cron.Cron
}

func NewHourlyLogger(logger *zap.Logger) *HourlyLogger {
	return &HourlyLogger{logger: logger}
}

// Start 启动按小时执行的日志任务，返回停止函数。
func (h *HourlyLogger) Start(parent context.Context) context.CancelFunc {
	if h == nil {
		return func() {}
	}
	c := cron.New()
	_, err := c.AddFunc("@hourly", func() {
		if h.logger != nil {
			h.logger.Info("hourly job heartbeat", zap.Time("timestamp", time.Now()))
		}
	})
	if err != nil {
		if h.logger != nil {
			h.logger.Error("failed to register hourly job", zap.Error(err))
		}
		return func() {}
	}
	h.cron = c
	c.Start()
	if h.logger != nil {
		h.logger.Info("hourly job started")
	}

	stop := func() {
		ctx := h.cron.Stop()
		<-ctx.Done()
		if h.logger != nil {
			h.logger.Info("hourly job stopped")
		}
	}

	go func() {
		<-parent.Done()
		stop()
	}()

	return stop
}

