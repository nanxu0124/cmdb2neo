UNWIND $rows AS row
MATCH (start {cmdb_key: row.start_key})
MATCH (end {cmdb_key: row.end_key})
MERGE (start)-[r{{.RelType}}]->(end)
SET r += row.properties,
    r.first_seen_run_id = row.run_id,
    r.last_seen_run_id = row.run_id,
    r.active = true
