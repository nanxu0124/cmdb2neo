MATCH (n{{.LabelPattern}})
WHERE n.last_seen_run_id < $retention_run_id
DETACH DELETE n
