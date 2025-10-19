package ioc

import (
	"cmdb2neo/internal/app"
	"strings"
)

const defaultConfigPath = "configs/config.yaml"

var configPath = defaultConfigPath

// SetConfigPath 设置配置文件路径。
func SetConfigPath(path string) {
	if trimmed := strings.TrimSpace(path); trimmed != "" {
		configPath = trimmed
	}
}

// InitConfig 读取应用配置。
func InitConfig() (*app.Config, error) {
	return app.LoadConfig(configPath)
}
