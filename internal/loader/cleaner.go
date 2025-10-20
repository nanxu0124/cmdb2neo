package loader

import "context"

// Cleaner 负责删除过期节点和关系。
type Cleaner struct {
	client *Client
}

func NewCleaner(client *Client) *Cleaner {
	return &Cleaner{client: client}
}

// HardDeleteNodes 删除 last_seen_run_id 小于 retentionRunID 的节点。
func (c *Cleaner) HardDeleteNodes(ctx context.Context, retentionRunID string) error {
	query := `MATCH (n) WHERE n.last_seen_run_id < $retention_run_id AND exists(n.cmdb_key) DETACH DELETE n`
	return c.client.RunWrite(ctx, query, map[string]any{"retention_run_id": retentionRunID})
}

// HardDeleteRelationships 删除 last_seen_run_id 小于 retentionRunID 的关系。
func (c *Cleaner) HardDeleteRelationships(ctx context.Context, retentionRunID string) error {
	query := `MATCH ()-[r]-() WHERE r.last_seen_run_id < $retention_run_id DELETE r`
	return c.client.RunWrite(ctx, query, map[string]any{"retention_run_id": retentionRunID})
}
