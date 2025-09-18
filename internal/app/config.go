package app

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
)

type Neo4j struct {
	URI                  string `yaml:"uri"`
	Username             string `yaml:"username"`
	Password             string `yaml:"password"`
	Database             string `yaml:"database"`
	MaxConnectionPool    int    `yaml:"max_connections"`
	ConnectTimeoutSecond int    `yaml:"connect_timeout_second"`
}

type Sync struct {
	BatchSize       int   `yaml:"batch_size"`
	ParallelWorkers int   `yaml:"parallel_workers"`
	Retry           Retry `yaml:"retry"`
}

type Retry struct {
	Attempts       int `yaml:"attempts"`
	BackoffSeconds int `yaml:"backoff_seconds"`
}

type Config struct {
	Neo4j Neo4j `yaml:"neo4j"`
	Sync  Sync  `yaml:"sync"`
}

// LoadConfig 从文件加载配置。
func LoadConfig(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("读取配置失败: %w", err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("解析配置失败: %w", err)
	}
	return cfg, nil
}
