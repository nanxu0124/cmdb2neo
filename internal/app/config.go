package app

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config 对应配置文件结构。
type Config struct {
	Neo4j struct {
		URI                  string `yaml:"uri"
		Username             string `yaml:"username"
		Password             string `yaml:"password"
		Database             string `yaml:"database"
		MaxConnectionPool    int    `yaml:"max_connection_pool_size"
		ConnectTimeoutSecond int    `yaml:"connect_timeout_second"
	} `yaml:"neo4j"
	Sync struct {
		BatchSize       int `yaml:"batch_size"
		ParallelWorkers int `yaml:"parallel_workers"
		Retry struct {
			Attempts      int `yaml:"attempts"
			BackoffSecond int `yaml:"backoff_seconds"
		} `yaml:"retry"
	} `yaml:"sync"
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
