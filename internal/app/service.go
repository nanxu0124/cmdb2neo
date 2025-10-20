package app

import (
	"context"
	"fmt"

	"cmdb2neo/internal/cmdb"
	"cmdb2neo/internal/loader"
	"cmdb2neo/pkg/logging"
	"go.uber.org/zap"
)

// Service 负责装配各个 Flow 并提供统一入口。
type Service struct {
	cfg           Config
	cmdbClient    cmdb.Client
	neoClient     *loader.Client
	InitFlow      *InitFlow
	SyncFlow      *SyncFlow
	ReconcileFlow *ReconcileFlow
	logger        *zap.Logger
}

// NewService 根据配置构建 Service。
func NewService(ctx context.Context, cfg Config, cmdbClient cmdb.Client) (*Service, error) {
	if cmdbClient == nil {
		return nil, fmt.Errorf("必须提供 cmdb client")
	}
	logger, err := logging.NewZpaLogger()
	if err != nil {
		return nil, err
	}
	neoClient, err := loader.NewClient(ctx, loader.Config{
		URI:                  cfg.Neo4j.URI,
		Username:             cfg.Neo4j.Username,
		Password:             cfg.Neo4j.Password,
		Database:             cfg.Neo4j.Database,
		MaxConnectionPool:    cfg.Neo4j.MaxConnectionPool,
		ConnectionTimeoutSec: cfg.Neo4j.ConnectTimeoutSecond,
	})
	if err != nil {
		return nil, err
	}
	batchSize := cfg.Sync.BatchSize

	nodeUpserter := loader.NewNodeUpserter(neoClient, batchSize)
	relUpserter := loader.NewRelUpserter(neoClient, batchSize)
	edgeFixer := loader.NewEdgeFixer(neoClient)
	schema := loader.NewSchemaManager(neoClient)

	initFlow := &InitFlow{
		CMDB:   cmdbClient,
		Schema: schema,
		Nodes:  nodeUpserter,
		Rels:   relUpserter,
		Fixer:  edgeFixer,
		Logger: logger,
	}

	syncFlow := &SyncFlow{
		CMDB:    cmdbClient,
		Nodes:   nodeUpserter,
		Rels:    relUpserter,
		Fixer:   edgeFixer,
		Cleaner: loader.NewCleaner(neoClient),
		Logger:  logger,
	}

	svc := &Service{
		cfg:           cfg,
		cmdbClient:    cmdbClient,
		neoClient:     neoClient,
		InitFlow:      initFlow,
		SyncFlow:      syncFlow,
		ReconcileFlow: &ReconcileFlow{Logger: logger},
		logger:        logger,
	}
	return svc, nil
}

// Close 释放资源。
func (s *Service) Close(ctx context.Context) error {
	if s.logger != nil {
		_ = s.logger.Sync()
	}
	if s.neoClient != nil {
		return s.neoClient.Close(ctx)
	}
	return nil
}

func (s *Service) Init(ctx context.Context) error {
	if s.InitFlow == nil {
		return fmt.Errorf("未初始化 init flow")
	}
	return s.InitFlow.Run(ctx)
}

func (s *Service) Sync(ctx context.Context) error {
	if s.SyncFlow == nil {
		return fmt.Errorf("未初始化 sync flow")
	}
	return s.SyncFlow.Run(ctx)
}

func (s *Service) Reconcile(ctx context.Context) error {
	if s.ReconcileFlow == nil {
		return fmt.Errorf("未初始化 reconcile flow")
	}
	return s.ReconcileFlow.Run(ctx)
}

func (s *Service) Validate(ctx context.Context) error {
	// 目前仅做占位
	if s.logger != nil {
		s.logger.Info("validate 尚未实现")
	}
	return nil
}
