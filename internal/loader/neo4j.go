package loader

import (
	"context"
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// Config 控制 Neo4j 连接参数。
type Config struct {
	URI                  string
	Username             string
	Password             string
	Database             string
	MaxConnectionPool    int
	ConnectionTimeoutSec int
}

// Client 封装 Neo4j Driver，提供最小写接口。
type Client struct {
	driver   neo4j.DriverWithContext
	database string
}

// NewClient 创建一个新的 Neo4j 客户端。
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	if cfg.URI == "" {
		return nil, fmt.Errorf("neo4j uri 不能为空")
	}
	auth := neo4j.BasicAuth(cfg.Username, cfg.Password, "")
	driver, err := neo4j.NewDriverWithContext(cfg.URI, auth, func(config *neo4j.Config) {
		if cfg.MaxConnectionPool > 0 {
			config.MaxConnectionPoolSize = cfg.MaxConnectionPool
		}
		if cfg.ConnectionTimeoutSec > 0 {
			config.SocketConnectTimeout = time.Duration(cfg.ConnectionTimeoutSec) * time.Second
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

// Close 关闭连接。
func (c *Client) Close(ctx context.Context) error {
	if c == nil || c.driver == nil {
		return nil
	}
	return c.driver.Close(ctx)
}

// RunWrite 执行写事务。
func (c *Client) RunWrite(ctx context.Context, query string, params map[string]any) error {
	sess := c.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: c.database, AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)
	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, runErr := tx.Run(ctx, query, params)
		return nil, runErr
	})
	if err != nil {
		return fmt.Errorf("执行写入失败: %w", err)
	}
	return nil
}

// RunRaw 在已有事务外执行原始语句（无事务）。
func (c *Client) RunRaw(ctx context.Context, query string, params map[string]any) error {
	sess := c.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: c.database, AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)
	res, err := sess.Run(ctx, query, params)
	if err != nil {
		return fmt.Errorf("执行语句失败: %w", err)
	}
	return consume(ctx, res)
}

func consume(ctx context.Context, result neo4j.ResultWithContext) error {
	for result.Next(ctx) {
		// 消费结果即可
	}
	return result.Err()
}
