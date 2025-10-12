package ioc

import "cmdb2neo/internal/app"

const defaultConfigPath = "configs/config.yaml"

// InitConfig 读取应用配置。
func InitConfig() (app.Config, error) {
	return app.LoadConfig(defaultConfigPath)
}
