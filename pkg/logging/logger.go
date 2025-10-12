package logging

import "go.uber.org/zap"

func NewZpaLogger() (*zap.Logger, error) {
	cfg := zap.NewDevelopmentConfig()
	cfg.Encoding = "console"
	cfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	return cfg.Build()
}
