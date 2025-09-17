UNWIND $rows AS row
MERGE (n{{.LabelPattern}} {cmdb_key: row.cmdb_key})
SET n += row.properties,
    n.last_seen_run_id = row.run_id,
    n.updated_at = row.updated_at,
    n.active = true
