package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cmdb2neo/internal/app"
	"cmdb2neo/internal/cmdb"
	"cmdb2neo/internal/graph"
	"cmdb2neo/internal/logging"
	"cmdb2neo/internal/rca"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := app.LoadConfig("configs/config.yaml")
	if err != nil {
		fmt.Printf("load config failed: %v\n", err)
		return
	}

	logger, err := logging.New()
	if err != nil {
		fmt.Printf("init logger failed: %v\n", err)
		return
	}
	defer func() { _ = logger.Sync() }()

	listen := cfg.HTTP.Listen
	if strings.TrimSpace(listen) == "" {
		listen = ":8080"
	}

	graphClient, err := graph.NewClient(ctx, graph.Config{
		URI:                  cfg.Neo4j.URI,
		Username:             cfg.Neo4j.Username,
		Password:             cfg.Neo4j.Password,
		Database:             cfg.Neo4j.Database,
		MaxConnectionPool:    cfg.Neo4j.MaxConnectionPool,
		ConnectionTimeoutSec: cfg.Neo4j.ConnectTimeoutSecond,
	})
	if err != nil {
		logger.Fatal("create neo4j client failed", zap.Error(err))
	}
	defer func() { _ = graphClient.Close(context.Background()) }()

	provider := rca.NewGraphTopologyProvider(graphClient)
	analyzer, err := rca.NewAnalyzer(provider, nil, rca.DefaultConfig())
	if err != nil {
		logger.Fatal("create analyzer failed", zap.Error(err))
	}

	if cfg.Sync.InitialResync {
		if err := runInitialSync(ctx, cfg); err != nil {
			logger.Error("initial CMDB sync failed", zap.Error(err))
		} else {
			logger.Info("initial CMDB sync completed")
		}
	} else {
		logger.Info("initial CMDB sync skipped by configuration")
	}

	cancelSync := startSyncScheduler(ctx, logger, time.Duration(cfg.Sync.IntervalSeconds)*time.Second)
	defer cancelSync()

	srv := &httpServer{analyzer: analyzer, logger: logger}
	engine := setupRouter(srv)

	logger.Info("http server starting", zap.String("listen", listen))
	if err := engine.Run(listen); err != nil {
		logger.Fatal("http server stopped", zap.Error(err))
	}
}

type httpServer struct {
	analyzer *rca.Analyzer
	logger   *zap.Logger
}

type analyzeRequest struct {
	WindowID string           `json:"window_id"`
	Events   []rca.AlarmEvent `json:"events"`
}

type analyzeResponse struct {
	WindowID string     `json:"window_id"`
	Result   rca.Result `json:"result"`
}

func (s *httpServer) handleAnalyze(c *gin.Context) {
	var req analyzeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request payload"})
		return
	}
	if len(req.Events) == 0 {
		c.JSON(400, gin.H{"error": "events payload is empty"})
		return
	}
	windowID := req.WindowID
	if strings.TrimSpace(windowID) == "" {
		windowID = fmt.Sprintf("auto-%d", time.Now().Unix())
	}
	result, err := s.analyzer.Analyze(c.Request.Context(), windowID, req.Events)
	if err != nil {
		s.logger.Error("analyze failed", zap.Error(err))
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, analyzeResponse{WindowID: windowID, Result: result})
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
				logger.Info("scheduled CMDB sync (mock)")
			case <-ctx.Done():
				logger.Info("sync scheduler stopped")
				return
			}
		}
	}()
	return cancel
}

func setupRouter(handler *httpServer) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery())

	engine.POST("/api/v1/rca/analyze", handler.handleAnalyze)

	return engine
}

func runInitialSync(ctx context.Context, cfg app.Config) error {
	client, err := buildCMDBClient(cfg)
	if err != nil {
		return err
	}
	svc, err := app.NewService(ctx, cfg, client)
	if err != nil {
		return err
	}
	defer svc.Close(ctx)
	return svc.Init(ctx)
}

func buildCMDBClient(cfg app.Config) (cmdb.Client, error) {
	if strings.TrimSpace(cfg.Sync.Source.BaseURL) == "" {
		return nil, fmt.Errorf("sync.source.base_url is required for initial resync")
	}
	var tokenSource cmdb.TokenSource
	if cfg.Sync.Source.AuthEndpoint != "" && cfg.Sync.Source.Username != "" {
		ts, err := cmdb.NewPasswordTokenSource(cmdb.PasswordTokenConfig{
			Endpoint: cfg.Sync.Source.AuthEndpoint,
			Username: cfg.Sync.Source.Username,
			Password: cfg.Sync.Source.Password,
			Timeout:  5 * time.Second,
		})
		if err != nil {
			return nil, err
		}
		tokenSource = ts
	} else if cfg.Sync.Source.StaticToken != "" {
		tokenSource = &cmdb.StaticTokenSource{Value: cfg.Sync.Source.StaticToken}
	}
	httpCfg := cmdb.HTTPConfig{
		BaseURL:        cfg.Sync.Source.BaseURL,
		TokenSource:    tokenSource,
		SnapshotAPI:    cfg.Sync.Source.SnapshotAPI,
		AuthHeaderName: cfg.Sync.Source.AuthHeader,
	}
	return cmdb.NewHTTPClient(httpCfg)
}
