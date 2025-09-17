# neo4j-cmdb-sync

最小化示例，用于演示如何将 CMDB 快照初始化同步到 Neo4j。当前实现重点在流程和模板，未对接真实 CMDB 与 Neo4j。

## 目录

- `cmd/syncer` CLI 入口，支持 `init/sync/reconcile/validate`
- `internal/app` 编排各流程
- `internal/cmdb` CMDB 模型与静态客户端
- `internal/loader` 封装 Neo4j 写入、模板、补边
- `internal/cypher` Cypher 模板集合

## 快速开始

```bash
# 使用 mock 数据执行初始化
GOCACHE=.gocache go run ./cmd/syncer init
```

若需要连接真实 Neo4j，需要将 `configs/config.yaml` 修改为实际连接信息，并将 `cmdb.StaticClient` 替换为自己的实现。

## 测试

```bash
GOCACHE=.gocache go test ./...
```
