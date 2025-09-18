package rca

import "context"

// TopologyProvider 将告警事件映射到拓扑路径。
type TopologyProvider interface {
	ResolveContext(ctx context.Context, event AlarmEvent) (AlarmContext, error)
}

// ResultStore 用于持久化根因分析结果至外部存储。
type ResultStore interface {
	Save(ctx context.Context, windowID string, result Result) error
}
