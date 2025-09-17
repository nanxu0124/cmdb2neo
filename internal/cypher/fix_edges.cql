MATCH (vm:VirtualMachine)
WHERE vm.host_ip IS NOT NULL
MATCH (host:HostMachine {ip: vm.host_ip})
MERGE (host)-[r:HOSTS_VM]->(vm)
SET r.last_seen_run_id = $run_id,
    r.active = true;

MATCH (app:App)
MATCH (vm:VirtualMachine {ip: app.ip})
MERGE (app)-[r:DEPLOYED_ON]->(vm)
SET r.last_seen_run_id = $run_id,
    r.active = true;
