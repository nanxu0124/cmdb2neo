# Realistic RCA Fixture

本目录提供一套较完整的 CMDB 样例数据和一次真实感的报警风暴，用于 Neo4j 拓扑建图与根因分析测试。

## 数据集概览

- **机房**：
  - `IDC_101`（上海 M5 生产机房）
  - `IDC_102`（广州 GZ 备机房）
- **网络分区**：M5-Production、M5-DMZ、GZ-Backup
- **宿主机**：`HM_4001` 健康，`HM_4002`（生产主机，当前置为 down），`HM_4003`（DMZ）
- **物理机**：与生产分区绑定的刀片服务器，`PM_6002` 状态 degraded
- **虚拟机**：订单、支付、库存、报表、边缘代理等，支付相关 VM 全部落在 `HM_4002`
- **应用**：订单 API 正常，支付链路（API + Worker）及库存 API 均异常，边缘代理正常

使用 `tests/integration/seed_realistic.cql` 可在 Neo4j 中创建上诉节点与关系（脚本默认先清空图谱，请谨慎执行）。

## 报警风暴 (`alarm_events.json`)

- 3 条来自支付链路、库存链路的 `App` 级别 P1 告警（HTTP 5xx、Kafka Timeout、实例不可达），IP 均指向 `HM_4002` 所承载的 VM。
- 1 条来自 `HostMachine` 的 P0 告警，显示宿主机 `172.20.1.20` 网卡下线。
- 告警均发生在同一个 5 分钟时间窗口内，Host 告警领先 App 告警约 20~40 秒。

## 根因分析预期

1. **App → VM 聚合**：
   - `payment-gateway-api`、`payment-async-worker`、`inventory-service-api` 对应 VM `VM_5002`、`VM_5003`、`VM_5004`，均位于 `HM_4002`。
   - VM 层覆盖率达到 100%（3/3），因此 `VM_5002/5003/5004` 均被标记为派生异常。
2. **VM → Host**：
   - `HM_4002` 下有 3/4 个 VM 告警（健康 VM `VM_5006` 仍在运行）。覆盖率 0.75，高于默认阈值 0.6。
   - Host 告警（P0）时间领先 VM 告警，因此置信度进一步提高。
3. **Host → NetPartition / IDC**：
   - `Production Zone` 仅有 `HM_4001` 与 `HM_4002` 两台宿主机，覆盖率 0.5；根据阈值可选择是否继续上卷。若阈值设为 0.7，则停在 Host 层。

**最终根因候选**：`HM_4002`（m5-prod-host-02），原因 “NIC failure on TOR-23”。其告警链路包含支付与库存相关 App 所在的 VM 与 NetPartition。

## 使用方式

1. 在 Neo4j 中执行 `tests/integration/seed_realistic.cql` 初始化图谱。
2. 将 `tests/integration/fixtures/realistic/*.json` 作为输入，运行现有 ETL 或单元测试。
3. 使用 `alarm_events.json` 调用 `Analyzer`，预期输出 Host 层为主要根因。
