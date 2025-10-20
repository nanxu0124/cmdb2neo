package server

import (
	"context"
	"strings"

	"cmdb2neo/internal/app"
	"cmdb2neo/internal/job"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// HTTPServer 封装 HTTP 服务运行所需的依赖。
type HTTPServer struct {
	Engine  *gin.Engine
	Logger  *zap.Logger
	Config  app.Config
	Service *app.Service
	Job     *job.Scheduler
	Hourly  *job.HourlyLogger
}

// NewHTTPServer 构建 HTTPServer。
func NewHTTPServer(engine *gin.Engine, logger *zap.Logger, cfg app.Config, svc *app.Service, scheduler *job.Scheduler, hourly *job.HourlyLogger) *HTTPServer {
	return &HTTPServer{
		Engine:  engine,
		Logger:  logger,
		Config:  cfg,
		Service: svc,
		Job:     scheduler,
		Hourly:  hourly,
	}
}

// Run 启动 HTTP 服务及相关后台任务。
func (s *HTTPServer) Run(ctx context.Context) error {
	listen := strings.TrimSpace(s.Config.HTTP.Listen)
	if listen == "" {
		listen = ":8080"
	}

	cancelJob := func() {}
	if s.Job != nil {
		cancelJob = s.Job.Start(ctx)
		defer cancelJob()
	}
	cancelHourly := func() {}
	if s.Hourly != nil {
		cancelHourly = s.Hourly.Start(ctx)
		defer cancelHourly()
	}

	if s.Config.Sync.InitialResync && s.Service != nil {
		if err := s.Service.Init(ctx); err != nil {
			if s.Logger != nil {
				s.Logger.Error("initial CMDB sync failed", zap.Error(err))
			}
		} else if s.Logger != nil {
			s.Logger.Info("initial CMDB sync completed")
		}
	} else if s.Logger != nil {
		s.Logger.Info("initial CMDB sync skipped by configuration")
	}

	if s.Logger != nil {
		s.Logger.Info("http server starting", zap.String("listen", listen))
	}
	return s.Engine.Run(listen)
}

// Shutdown 释放资源。
func (s *HTTPServer) Shutdown(ctx context.Context) {
	if s.Service != nil {
		if err := s.Service.Close(ctx); err != nil && s.Logger != nil {
			s.Logger.Warn("close app service failed", zap.Error(err))
		}
	}
	if s.Logger != nil {
		_ = s.Logger.Sync()
	}
}
