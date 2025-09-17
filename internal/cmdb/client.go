package cmdb

import "context"

// Client 抽象 CMDB 数据源。
type Client interface {
	FetchSnapshot(ctx context.Context) (Snapshot, error)
}

// StaticClient 用于测试或最小实现，直接返回内存中的快照。
type StaticClient struct {
	Snapshot Snapshot
}

// FetchSnapshot 返回预设快照。
func (c *StaticClient) FetchSnapshot(context.Context) (Snapshot, error) {
	return c.Snapshot, nil
}
