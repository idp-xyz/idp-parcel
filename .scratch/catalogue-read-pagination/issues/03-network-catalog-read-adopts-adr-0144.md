# 03 网络目录读口（`/network-catalog`）迁到 ADR-0144

Category: enhancement
Status: in-progress——2026-09-24 通道 2 认领（派单 `task-374025f0` ← 通道 3），隔离 worktree 分支 `mcp2-crp03`：票 01 尚未重放进 main，分支基 `mcp2-crp01` tip `db96d561` 并拣入本笔，重放时只取本票的笔
Blocked by: 01
地盘：`internal/networkrouting/adapters/http/query_network_catalog.go` 与其测试、它消费的读端口与 postgres 读面、`migrations/` 下 network-routing 模块的
一条新迁移（索引）。
出处：[ADR-0144](../../../docs/adr/0144-catalogue-reads-share-one-cursor-pagination-sort-and-filter-contract.md) 决定三至七；首批之一（规模先到）。

## 做什么

同票 02 的四步，另有两处本册特有：

1. `?family=` 是册子选择器，保持原义，不兼作筛选维（ADR-0144 决定四）；各 `family` 各自声明可排维与筛选维。
2. **缺省序要本票裁一次**：网络版本册常按对象看版本，`-registeredAt` 未必顺手（ADR-0144 越权风险点 1）。若某个 `family` 另点缺省序，在读面头注写明
   理由，并在本票完成记录里列出，供 NR owner 复核。

## 不做

- 不改登记写面；不改作用域与授权；不动网络解析层。

## 完成判据

- 同票 02 的真库与端点用例，按每个 `family` 各跑一遍翻页不重不漏。
- 完成记录列出各 `family` 的缺省序与理由。
