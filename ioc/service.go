package ioc

import (
	"context"

	"cmdb2neo/internal/app"
	"cmdb2neo/internal/cmdb"
)

// InitAppService 构建 CMDB 同步服务。
func InitAppService(ctx context.Context, cfg app.Config, client cmdb.Client) (*app.Service, error) {
	return app.NewService(ctx, cfg, client)
}
