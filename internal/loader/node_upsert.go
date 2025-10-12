package loader

import (
	"context"
	"fmt"

	"cmdb2neo/internal/cypher"
	"cmdb2neo/internal/domain"
	"cmdb2neo/pkg/util"
)

// NodeUpserter 负责批量写入节点。
type NodeUpserter struct {
	client    *Client
	batchSize int
}

// NewNodeUpserter 创建节点 upsert 器。
func NewNodeUpserter(client *Client, batchSize int) *NodeUpserter {
	if batchSize <= 0 {
		batchSize = 100
	}
	return &NodeUpserter{client: client, batchSize: batchSize}
}

// InitNodes 负责初始化节点（首跑使用）。
func (u *NodeUpserter) InitNodes(ctx context.Context, rows []domain.NodeRow) error {
	return u.write(ctx, rows, true)
}

// UpsertNodes 负责增量 upsert。
func (u *NodeUpserter) UpsertNodes(ctx context.Context, rows []domain.NodeRow) error {
	return u.write(ctx, rows, false)
}

func (u *NodeUpserter) write(ctx context.Context, rows []domain.NodeRow, init bool) error {
	if len(rows) == 0 {
		return nil
	}
	grouped := make(map[string][]domain.NodeRow)
	labelCache := make(map[string]string)
	for _, row := range rows {
		key := domain.JoinLabels(row.Labels)
		grouped[key] = append(grouped[key], row)
		if _, ok := labelCache[key]; !ok {
			labelCache[key] = domain.LabelPattern(row.Labels)
		}
	}

	tplName := "upsert_nodes.cql"
	if init {
		tplName = "init_nodes.cql"
	}

	for key, rows := range grouped {
		if len(rows) == 0 {
			continue
		}
		labelPattern := labelCache[key]
		query := cypher.MustTemplate(tplName, map[string]string{"LabelPattern": labelPattern})
		for _, chunk := range util.Batch(rows, u.batchSize) {
			params := map[string]any{"rows": toNodeParameters(chunk)}
			if err := u.client.RunWrite(ctx, query, params); err != nil {
				return fmt.Errorf("写入节点失败 labels=%s: %w", key, err)
			}
		}
	}
	return nil
}

func toNodeParameters(rows []domain.NodeRow) []map[string]any {
	res := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		res = append(res, map[string]any{
			"cmdb_key":   row.CMDBKey,
			"properties": map[string]any(row.Properties),
			"run_id":     row.RunID,
			"updated_at": row.UpdatedAt,
		})
	}
	return res
}
