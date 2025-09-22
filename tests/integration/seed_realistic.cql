// 清空现有数据（谨慎使用）
MATCH (n) DETACH DELETE n;

// IDC 节点
MERGE (idc_m5:IDC {cmdb_key: 'IDC_101'})
SET idc_m5.name = 'M5',
    idc_m5.location = '上海·M5 生产机房';

MERGE (idc_gz:IDC {cmdb_key: 'IDC_102'})
SET idc_gz.name = 'GZ',
    idc_gz.location = '广州·GZ 备机房';

// Network Partition
MERGE (np_m5_prod:NetPartition {cmdb_key: 'NP_201'})
SET np_m5_prod.name = 'Production Zone',
    np_m5_prod.cidr = '172.20.0.0/20',
    np_m5_prod.idc = 'M5';
MERGE (idc_m5)-[:HAS_PARTITION]->(np_m5_prod);

MERGE (np_m5_dmz:NetPartition {cmdb_key: 'NP_202'})
SET np_m5_dmz.name = 'DMZ Zone',
    np_m5_dmz.cidr = '172.21.0.0/22',
    np_m5_dmz.idc = 'M5';
MERGE (idc_m5)-[:HAS_PARTITION]->(np_m5_dmz);

MERGE (np_gz_backup:NetPartition {cmdb_key: 'NP_301'})
SET np_gz_backup.name = 'Backup Zone',
    np_gz_backup.cidr = '172.30.0.0/21',
    np_gz_backup.idc = 'GZ';
MERGE (idc_gz)-[:HAS_PARTITION]->(np_gz_backup);

// HostMachine
MERGE (host_01:HostMachine:Machine:Compute {cmdb_key: 'HM_4001'})
SET host_01.hostname = 'm5-prod-host-01',
    host_01.ip = '172.20.1.10',
    host_01.idc = 'M5',
    host_01.server_type = 1,
    host_01.status = 'healthy';
MERGE (np_m5_prod)-[:HAS_HOST]->(host_01);

MERGE (host_02:HostMachine:Machine:Compute {cmdb_key: 'HM_4002'})
SET host_02.hostname = 'm5-prod-host-02',
    host_02.ip = '172.20.1.20',
    host_02.idc = 'M5',
    host_02.server_type = 1,
    host_02.status = 'down',
    host_02.down_reason = 'NIC failure on TOR-23';
MERGE (np_m5_prod)-[:HAS_HOST]->(host_02);

MERGE (host_dmz:HostMachine:Machine:Compute {cmdb_key: 'HM_4003'})
SET host_dmz.hostname = 'm5-dmz-host-01',
    host_dmz.ip = '172.21.1.10',
    host_dmz.idc = 'M5',
    host_dmz.server_type = 1,
    host_dmz.status = 'healthy';
MERGE (np_m5_dmz)-[:HAS_HOST]->(host_dmz);

// PhysicalMachine
MERGE (phy_01:PhysicalMachine:Machine:Compute {cmdb_key: 'PM_6001'})
SET phy_01.hostname = 'm5-phy-blade-01',
    phy_01.ip = '172.20.100.10',
    phy_01.idc = 'M5';
MERGE (np_m5_prod)-[:HAS_PHYSICAL]->(phy_01);

MERGE (phy_02:PhysicalMachine:Machine:Compute {cmdb_key: 'PM_6002'})
SET phy_02.hostname = 'm5-phy-blade-02',
    phy_02.ip = '172.20.100.20',
    phy_02.idc = 'M5',
    phy_02.status = 'degraded';
MERGE (np_m5_prod)-[:HAS_PHYSICAL]->(phy_02);

MERGE (phy_gz:PhysicalMachine:Machine:Compute {cmdb_key: 'PM_6003'})
SET phy_gz.hostname = 'gz-phy-blade-01',
    phy_gz.ip = '172.30.100.10',
    phy_gz.idc = 'GZ';
MERGE (np_gz_backup)-[:HAS_PHYSICAL]->(phy_gz);

// VirtualMachine
MERGE (vm_order:VirtualMachine:Compute {cmdb_key: 'VM_5001'})
SET vm_order.hostname = 'vm-order-api',
    vm_order.ip = '172.20.2.11',
    vm_order.host_ip = '172.20.1.10',
    vm_order.status = 'healthy';
MERGE (host_01)-[:HOSTS_VM]->(vm_order);

MERGE (vm_pay_api:VirtualMachine:Compute {cmdb_key: 'VM_5002'})
SET vm_pay_api.hostname = 'vm-payment-api',
    vm_pay_api.ip = '172.20.2.21',
    vm_pay_api.host_ip = '172.20.1.20',
    vm_pay_api.status = 'down';
MERGE (host_02)-[:HOSTS_VM]->(vm_pay_api);

MERGE (vm_pay_worker:VirtualMachine:Compute {cmdb_key: 'VM_5003'})
SET vm_pay_worker.hostname = 'vm-payment-worker',
    vm_pay_worker.ip = '172.20.2.22',
    vm_pay_worker.host_ip = '172.20.1.20',
    vm_pay_worker.status = 'down';
MERGE (host_02)-[:HOSTS_VM]->(vm_pay_worker);

MERGE (vm_inventory:VirtualMachine:Compute {cmdb_key: 'VM_5004'})
SET vm_inventory.hostname = 'vm-inventory-api',
    vm_inventory.ip = '172.20.2.31',
    vm_inventory.host_ip = '172.20.1.20',
    vm_inventory.status = 'degraded';
MERGE (host_02)-[:HOSTS_VM]->(vm_inventory);

MERGE (vm_edge:VirtualMachine:Compute {cmdb_key: 'VM_5005'})
SET vm_edge.hostname = 'vm-edge-proxy',
    vm_edge.ip = '172.21.2.11',
    vm_edge.host_ip = '172.21.1.10',
    vm_edge.status = 'healthy';
MERGE (host_dmz)-[:HOSTS_VM]->(vm_edge);

MERGE (vm_reporting:VirtualMachine:Compute {cmdb_key: 'VM_5006'})
SET vm_reporting.hostname = 'vm-reporting',
    vm_reporting.ip = '172.20.2.41',
    vm_reporting.host_ip = '172.20.1.20',
    vm_reporting.status = 'healthy';
MERGE (host_02)-[:HOSTS_VM]->(vm_reporting);

// App 节点
MERGE (app_order:App {cmdb_key: 'APP_7001'})
SET app_order.name = 'order-service-api',
    app_order.ip = '172.20.2.11',
    app_order.status = 'ok';
MERGE (app_order)-[:DEPLOYED_ON]->(vm_order);

MERGE (app_payment_api:App {cmdb_key: 'APP_7002'})
SET app_payment_api.name = 'payment-gateway-api',
    app_payment_api.ip = '172.20.2.21',
    app_payment_api.status = 'down';
MERGE (app_payment_api)-[:DEPLOYED_ON]->(vm_pay_api);

MERGE (app_payment_worker:App {cmdb_key: 'APP_7003'})
SET app_payment_worker.name = 'payment-async-worker',
    app_payment_worker.ip = '172.20.2.22',
    app_payment_worker.status = 'down';
MERGE (app_payment_worker)-[:DEPLOYED_ON]->(vm_pay_worker);

MERGE (app_inventory:App {cmdb_key: 'APP_7004'})
SET app_inventory.name = 'inventory-service-api',
    app_inventory.ip = '172.20.2.31',
    app_inventory.status = 'down';
MERGE (app_inventory)-[:DEPLOYED_ON]->(vm_inventory);

MERGE (app_edge:App {cmdb_key: 'APP_7005'})
SET app_edge.name = 'edge-proxy',
    app_edge.ip = '172.21.2.11',
    app_edge.status = 'ok';
MERGE (app_edge)-[:DEPLOYED_ON]->(vm_edge);

MERGE (app_reporting:App {cmdb_key: 'APP_7006'})
SET app_reporting.name = 'reporting-service',
    app_reporting.ip = '172.20.2.41',
    app_reporting.status = 'ok';
MERGE (app_reporting)-[:DEPLOYED_ON]->(vm_reporting);
