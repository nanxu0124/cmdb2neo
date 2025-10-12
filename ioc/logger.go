package ioc

import (
	"cmdb2neo/pkg/logging"
	"go.uber.org/zap"
)

// InitLogger 构建全局 logger。
func InitLogger() (*zap.Logger, error) {
	return logging.NewZpaLogger()
}
