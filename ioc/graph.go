package ioc

import (
	"context"

	"cmdb2neo/internal/app"
	"cmdb2neo/internal/graph"
)

// InitGraphClient 构建只读图数据库客户端。
func InitGraphClient(ctx context.Context, cfg app.Config) (*graph.Client, error) {
	return graph.NewClient(ctx, graph.Config{
		URI:                  cfg.Neo4j.URI,
		Username:             cfg.Neo4j.Username,
		Password:             cfg.Neo4j.Password,
		Database:             cfg.Neo4j.Database,
		MaxConnectionPool:    cfg.Neo4j.MaxConnectionPool,
		ConnectionTimeoutSec: cfg.Neo4j.ConnectTimeoutSecond,
	})
}
