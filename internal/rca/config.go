package rca

// ScoreWeights 控制各指标权重。
type ScoreWeights struct {
	Coverage float64 `json:"coverage"`
	Impact   float64 `json:"impact"`
	Base     float64 `json:"base"`
}

// LayerConfig 每层的阈值配置。
type LayerConfig struct {
	CoverageThreshold float64      `json:"coverage_threshold"`
	MinChildren       int          `json:"min_children"`
	Weights           ScoreWeights `json:"weights"`
}

// Config 根因分析配置。
type Config struct {
	Hierarchy          []NodeType               `json:"hierarchy"`
	Layers             map[NodeType]LayerConfig `json:"layers"`
	Datacenters        []string                 `json:"datacenters"`
	AppOutageThreshold float64                  `json:"app_outage_threshold"`
	RequireFullMatch   bool                     `json:"require_full_match"`
}

// DefaultConfig 提供默认配置。
func DefaultConfig() Config {
	return Config{
		Hierarchy: []NodeType{
			NodeTypeApp,
			NodeTypeVirtualMachine,
			NodeTypeHostMachine,
			NodeTypePhysicalMachine,
			NodeTypeNetPartition,
			NodeTypeIDC,
		},
		Layers: map[NodeType]LayerConfig{
			NodeTypeApp: {
				CoverageThreshold: 0.6,
				MinChildren:       1,
				Weights:           ScoreWeights{Coverage: 0.7, Impact: 0.3, Base: 0},
			},
			NodeTypeVirtualMachine: {
				CoverageThreshold: 0.6,
				MinChildren:       1,
				Weights:           ScoreWeights{Coverage: 0.7, Impact: 0.3, Base: 0},
			},
			NodeTypeHostMachine: {
				CoverageThreshold: 0.6,
				MinChildren:       1,
				Weights:           ScoreWeights{Coverage: 0.7, Impact: 0.3, Base: 0},
			},
			NodeTypePhysicalMachine: {
				CoverageThreshold: 0.6,
				MinChildren:       1,
				Weights:           ScoreWeights{Coverage: 0.7, Impact: 0.3, Base: 0},
			},
			NodeTypeNetPartition: {
				CoverageThreshold: 0.7,
				MinChildren:       1,
				Weights:           ScoreWeights{Coverage: 0.7, Impact: 0.3, Base: 0},
			},
			NodeTypeIDC: {
				CoverageThreshold: 0.8,
				MinChildren:       1,
				Weights:           ScoreWeights{Coverage: 0.7, Impact: 0.3, Base: 0},
			},
		},
		Datacenters:        []string{"M5", "星光", "三星大厦"},
		AppOutageThreshold: 0.6,
		RequireFullMatch:   true,
	}
}
