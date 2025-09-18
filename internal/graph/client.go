package graph

import (
	"context"
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// Reader 定义只读查询接口，便于测试替换实现。
type Reader interface {
	RunRead(ctx context.Context, query string, params map[string]any) ([]map[string]any, error)
}

// Config 描述连接 Neo4j 的必要参数。
type Config struct {
	URI                  string
	Username             string
	Password             string
	Database             string
	MaxConnectionPool    int
	ConnectionTimeoutSec int
}

// Client 封装了只读能力的 Neo4j 访问。
type Client struct {
	driver   neo4j.DriverWithContext
	database string
}

// NewClient 创建并校验连接。
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	if cfg.URI == "" {
		return nil, fmt.Errorf("neo4j uri 不能为空")
	}
	auth := neo4j.BasicAuth(cfg.Username, cfg.Password, "")
	driver, err := neo4j.NewDriverWithContext(cfg.URI, auth, func(conf *neo4j.Config) {
		if cfg.MaxConnectionPool > 0 {
			conf.MaxConnectionPoolSize = cfg.MaxConnectionPool
		}
		if cfg.ConnectionTimeoutSec > 0 {
			conf.SocketConnectTimeout = time.Duration(cfg.ConnectionTimeoutSec) * time.Second
		}
	})
	if err != nil {
		return nil, fmt.Errorf("创建 neo4j driver 失败: %w", err)
	}
	if err := driver.VerifyConnectivity(ctx); err != nil {
		_ = driver.Close(ctx)
		return nil, fmt.Errorf("neo4j 无法连通: %w", err)
	}
	return &Client{driver: driver, database: cfg.Database}, nil
}

// Close 关闭底层连接。
func (c *Client) Close(ctx context.Context) error {
	if c == nil || c.driver == nil {
		return nil
	}
	return c.driver.Close(ctx)
}

// RunRead 执行只读查询并返回记录集合。
func (c *Client) RunRead(ctx context.Context, query string, params map[string]any) ([]map[string]any, error) {
	session := c.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: c.database, AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	resultAny, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, query, params)
		if err != nil {
			return nil, err
		}
		records := make([]map[string]any, 0)
		for res.Next(ctx) {
			records = append(records, res.Record().AsMap())
		}
		if err := res.Err(); err != nil {
			return nil, err
		}
		return records, nil
	})
	if err != nil {
		return nil, err
	}
	records, ok := resultAny.([]map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected read result type %T", resultAny)
	}
	return records, nil
}
