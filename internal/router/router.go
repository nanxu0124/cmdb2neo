package router

import "github.com/gin-gonic/gin"

// NewEngine 构建 gin 引擎并注册所有模块路由。
func NewEngine(rcaHandler *RCAHandler) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery())

	api := engine.Group("/api/v1")
	rcaGroup := api.Group("/rca")
	rcaHandler.RegisterRoutes(rcaGroup)

	return engine
}
