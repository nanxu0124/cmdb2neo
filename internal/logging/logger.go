package logging

import "go.uber.org/zap"

// New 返回开发环境的 zap logger。
func New() (*zap.Logger, error) {
	cfg := zap.NewDevelopmentConfig()
	cfg.Encoding = "console"
	cfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	return cfg.Build()
}
