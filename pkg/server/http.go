package server

import (
	"context"
	"strings"
	"time"

	"cmdb2neo/internal/app"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// HTTPServer 封装 HTTP 服务运行所需的依赖。
type HTTPServer struct {
	Engine  *gin.Engine
	Logger  *zap.Logger
	Config  app.Config
	Service *app.Service
}

// NewHTTPServer 构建 HTTPServer。
func NewHTTPServer(engine *gin.Engine, logger *zap.Logger, cfg app.Config, svc *app.Service) *HTTPServer {
	return &HTTPServer{
		Engine:  engine,
		Logger:  logger,
		Config:  cfg,
		Service: svc,
	}
}

// Run 启动 HTTP 服务及相关后台任务。
func (s *HTTPServer) Run(ctx context.Context) error {
	listen := strings.TrimSpace(s.Config.HTTP.Listen)
	if listen == "" {
		listen = ":8080"
	}

	var cancelScheduler context.CancelFunc = func() {}
	if interval := time.Duration(s.Config.Sync.IntervalSeconds) * time.Second; interval > 0 {
		cancelScheduler = startSyncScheduler(ctx, s.Logger, interval)
		defer cancelScheduler()
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

func startSyncScheduler(parent context.Context, logger *zap.Logger, interval time.Duration) context.CancelFunc {
	if interval <= 0 {
		return func() {}
	}
	ctx, cancel := context.WithCancel(parent)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if logger != nil {
					logger.Info("scheduled CMDB sync (mock)")
				}
			case <-ctx.Done():
				if logger != nil {
					logger.Info("sync scheduler stopped")
				}
				return
			}
		}
	}()
	return cancel
}
