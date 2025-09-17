MATCH (n{{.LabelPattern}})
WHERE n.last_seen_run_id < $retention_run_id AND coalesce(n.active, false) = false AND n.deactivated_at < $expired_before
DETACH DELETE n
