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
	MaxConnectionPool    int    `yaml:"max_connection_pool_size"`
	ConnectTimeoutSecond int    `yaml:"connect_timeout_second"`
}

type Sync struct {
	BatchSize       int        `yaml:"batch_size"`
	ParallelWorkers int        `yaml:"parallel_workers"`
	Retry           Retry      `yaml:"retry"`
	InitialResync   bool       `yaml:"initial_resync"`
	IntervalSeconds int        `yaml:"interval_seconds"`
	JobCron         string     `yaml:"job_cron"`
	Source          SyncSource `yaml:"source"`
}

type Retry struct {
	Attempts       int `yaml:"attempts"`
	BackoffSeconds int `yaml:"backoff_seconds"`
}

type HTTP struct {
	Listen string `yaml:"listen"`
}

type Config struct {
	Neo4j Neo4j `yaml:"neo4j"`
	Sync  Sync  `yaml:"sync"`
	HTTP  HTTP  `yaml:"http"`
}

type SyncSource struct {
	BaseURL      string `yaml:"base_url"`
	SnapshotAPI  string `yaml:"snapshot_api"`
	AuthHeader   string `yaml:"auth_header"`
	StaticToken  string `yaml:"static_token"`
	AuthEndpoint string `yaml:"auth_endpoint"`
	Username     string `yaml:"username"`
	Password     string `yaml:"password"`
}

// LoadConfig 从文件加载配置。
func LoadConfig(path string) (*Config, error) {
	cfg := new(Config)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置失败: %w", err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}
	return cfg, nil
}
