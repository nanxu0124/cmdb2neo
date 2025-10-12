package ioc

import (
	"cmdb2neo/internal/graph"
	"cmdb2neo/internal/rca"
)

// InitRCAConfig 返回默认根因分析配置。
func InitRCAConfig() rca.Config {
	return rca.DefaultConfig()
}

// InitRCAProvider 构建拓扑数据提供者。
func InitRCAProvider(client graph.Reader) rca.TopologyProvider {
	return rca.NewGraphProvider(client)
}

// InitRCAAnalyzer 构建根因分析器。
func InitRCAAnalyzer(provider rca.TopologyProvider, cfg rca.Config) (*rca.Analyzer, error) {
	return rca.NewAnalyzer(provider, cfg)
}
