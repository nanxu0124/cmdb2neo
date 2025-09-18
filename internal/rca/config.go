package rca

import "time"

// ScoreWeights 控制各维度分数权重。
type ScoreWeights struct {
	Coverage float64 `json:"coverage"`
	TimeLead float64 `json:"time_lead"`
	Impact   float64 `json:"impact"`
	Base     float64 `json:"base"`
}

// LayerConfig 针对不同层的阈值与评分配置。
type LayerConfig struct {
	CoverageThreshold float64      `json:"coverage_threshold"`
	MinChildren       int          `json:"min_children"`
	Weights           ScoreWeights `json:"weights"`
}

// Config 根因分析总配置。
type Config struct {
	Hierarchy []NodeType               `json:"hierarchy"`
	Layers    map[NodeType]LayerConfig `json:"layers"`
	Window    time.Duration            `json:"window"`
}

// DefaultConfig 返回一份默认的阈值配置。
func DefaultConfig() Config {
	return Config{
		Hierarchy: []NodeType{
			NodeTypeVirtualMachine,
			NodeTypeHostMachine,
			NodeTypePhysicalMachine,
			NodeTypeNetPartition,
			NodeTypeIDC,
		},
		Layers: map[NodeType]LayerConfig{
			NodeTypeVirtualMachine: {
				CoverageThreshold: 0.5,
				MinChildren:       1,
				Weights:           ScoreWeights{Coverage: 0.6, TimeLead: 0.2, Impact: 0.15, Base: 0.05},
			},
			NodeTypeHostMachine: {
				CoverageThreshold: 0.6,
				MinChildren:       2,
				Weights:           ScoreWeights{Coverage: 0.55, TimeLead: 0.2, Impact: 0.2, Base: 0.05},
			},
			NodeTypePhysicalMachine: {
				CoverageThreshold: 0.6,
				MinChildren:       1,
				Weights:           ScoreWeights{Coverage: 0.55, TimeLead: 0.15, Impact: 0.25, Base: 0.05},
			},
			NodeTypeNetPartition: {
				CoverageThreshold: 0.7,
				MinChildren:       1,
				Weights:           ScoreWeights{Coverage: 0.5, TimeLead: 0.2, Impact: 0.25, Base: 0.05},
			},
			NodeTypeIDC: {
				CoverageThreshold: 0.75,
				MinChildren:       1,
				Weights:           ScoreWeights{Coverage: 0.5, TimeLead: 0.25, Impact: 0.2, Base: 0.05},
			},
		},
		Window: 5 * time.Minute,
	}
}
