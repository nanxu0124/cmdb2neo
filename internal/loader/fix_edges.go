package loader

import (
	"context"
	"fmt"
	"strings"

	"cmdb2neo/internal/cypher"
)

// EdgeFixer 根据属性补边，确保拓扑完整。
type EdgeFixer struct {
	client *Client
}

func NewEdgeFixer(client *Client) *EdgeFixer {
	return &EdgeFixer{client: client}
}

func (f *EdgeFixer) Run(ctx context.Context, runID string) error {
	statements := strings.Split(cypher.MustAsset("fix_edges.cql"), ";")
	for _, stmt := range statements {
		query := strings.TrimSpace(stmt)
		if query == "" {
			continue
		}
		params := map[string]any{"run_id": runID}
		if err := f.client.RunWrite(ctx, query, params); err != nil {
			return fmt.Errorf("补边失败: %w", err)
		}
	}
	return nil
}
