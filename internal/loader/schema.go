package loader

import (
	"context"
	"fmt"
	"strings"

	"cmdb2neo/internal/cypher"
)

// SchemaManager 负责初始化约束和索引。
type SchemaManager struct {
	client *Client
}

func NewSchemaManager(client *Client) *SchemaManager {
	return &SchemaManager{client: client}
}

func (m *SchemaManager) Ensure(ctx context.Context) error {
	statements := strings.Split(cypher.MustAsset("init_schema.cql"), ";")
	for _, raw := range statements {
		query := strings.TrimSpace(raw)
		if query == "" {
			continue
		}
		if err := m.client.RunRaw(ctx, query, nil); err != nil {
			return fmt.Errorf("执行 schema 语句失败: %w", err)
		}
	}
	return nil
}
