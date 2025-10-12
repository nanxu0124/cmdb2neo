package loader

import (
	"context"
	"fmt"

	"cmdb2neo/internal/cypher"
	"cmdb2neo/internal/domain"
	"cmdb2neo/pkg/util"
)

// RelUpserter 负责关系批量写入。
type RelUpserter struct {
	client    *Client
	batchSize int
}

func NewRelUpserter(client *Client, batchSize int) *RelUpserter {
	if batchSize <= 0 {
		batchSize = 100
	}
	return &RelUpserter{client: client, batchSize: batchSize}
}

func (u *RelUpserter) InitRels(ctx context.Context, rows []domain.RelRow) error {
	return u.write(ctx, rows, true)
}

func (u *RelUpserter) UpsertRels(ctx context.Context, rows []domain.RelRow) error {
	return u.write(ctx, rows, false)
}

func (u *RelUpserter) write(ctx context.Context, rows []domain.RelRow, init bool) error {
	if len(rows) == 0 {
		return nil
	}
	grouped := make(map[string][]domain.RelRow)
	for _, row := range rows {
		grouped[row.Type] = append(grouped[row.Type], row)
	}

	tplName := "upsert_rels.cql"
	if init {
		tplName = "init_edges.cql"
	}

	for relType, rows := range grouped {
		if len(rows) == 0 {
			continue
		}
		relPattern := fmt.Sprintf(":%s", relType)
		query := cypher.MustTemplate(tplName, map[string]string{"RelType": relPattern})
		for _, chunk := range util.Batch(rows, u.batchSize) {
			params := map[string]any{"rows": toRelParameters(chunk)}
			if err := u.client.RunWrite(ctx, query, params); err != nil {
				return fmt.Errorf("写入关系失败 type=%s: %w", relType, err)
			}
		}
	}
	return nil
}

func toRelParameters(rows []domain.RelRow) []map[string]any {
	res := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		res = append(res, map[string]any{
			"start_key":  row.StartKey,
			"end_key":    row.EndKey,
			"properties": map[string]any(row.Properties),
			"run_id":     row.RunID,
		})
	}
	return res
}
