package ioc

import (
	"cmdb2neo/internal/rca"
	"cmdb2neo/internal/router"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// InitRCAHandler 构建根因分析 HTTP 处理器。
func InitRCAHandler(analyzer *rca.Analyzer, logger *zap.Logger) *router.RCAHandler {
	return router.NewRCAHandler(analyzer, logger)
}

// InitGinEngine 构建 gin 引擎。
func InitGinEngine(rcaHandler *router.RCAHandler) *gin.Engine {
	return router.NewEngine(rcaHandler)
}
