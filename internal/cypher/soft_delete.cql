MATCH (n{{.LabelPattern}})
WHERE n.last_seen_run_id < $retention_run_id AND coalesce(n.active, true)
SET n.active = false,
    n.deactivated_at = timestamp()
