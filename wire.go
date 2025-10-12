//go:build wireinject

package main

import (
	"context"

	"cmdb2neo/ioc"
	"cmdb2neo/pkg/server"
	"github.com/google/wire"
)

func InitApp(ctx context.Context) (*server.HTTPServer, func(), error) {
	panic(wire.Build(
		ioc.InitConfig,
		ioc.InitLogger,
		ioc.InitCMDBClient,
		ioc.InitAppService,
		ioc.InitGraphClient,
		ioc.InitRCAConfig,
		ioc.InitRCAProvider,
	ioc.InitRCAAnalyzer,
	ioc.InitRCAHandler,
	ioc.InitGinEngine,
	ioc.InitScheduler,
	server.NewHTTPServer,
	))
}
