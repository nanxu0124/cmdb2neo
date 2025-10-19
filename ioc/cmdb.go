package ioc

import (
	"cmdb2neo/internal/app"
	"cmdb2neo/internal/cmdb"
	"fmt"
	"strings"
	"time"
)

// InitCMDBClient 构建 CMDB 数据源客户端。
func InitCMDBClient(cfg *app.Config) (cmdb.Client, error) {
	return newCmdbClient(cfg)
}

func newCmdbClient(cfg *app.Config) (cmdb.Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	baseURL := strings.TrimSpace(cfg.Sync.Source.BaseURL)
	if baseURL == "" {
		if cfg.Sync.InitialResync {
			return nil, fmt.Errorf("sync.source.base_url is required for initial resync")
		}
		return &cmdb.StaticClient{}, nil
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
		BaseURL:        baseURL,
		TokenSource:    tokenSource,
		SnapshotAPI:    cfg.Sync.Source.SnapshotAPI,
		AuthHeaderName: cfg.Sync.Source.AuthHeader,
	}
	return cmdb.NewHTTPClient(httpCfg)
}
