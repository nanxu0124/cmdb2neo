package domain

import "time"

// NodeRow 是批量 upsert 的统一 DTO。
type NodeRow struct {
	CMDBKey    string                 `json:"cmdb_key"`
	Labels     []string               `json:"labels"`
	Properties map[string]any         `json:"properties"`
	RunID      string                 `json:"run_id"`
	UpdatedAt  time.Time              `json:"updated_at"`
}

// RelRow 代表一条关系需要的信息。
type RelRow struct {
	StartKey   string         `json:"start_key"`
	EndKey     string         `json:"end_key"`
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties"`
	RunID      string         `json:"run_id"`
}

// GroupedRows 用于同一批标签/类型下的批处理。
type GroupedRows struct {
	Key  string
	Rows []NodeRow
}

// GroupedRels 用于同一关系类型下的批处理。
type GroupedRels struct {
	Type string
	Rows []RelRow
}
