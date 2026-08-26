# 管理台骨架页接线：批 A 与批 B

Status: in-progress

盘点见同目录 [report.md](./report.md)（锚 `7ce41e4`）。本 spec 只承载它划出的**阻断物为零的两批**——批 C 压在三堵墙上、批 D 上下文尚未存在，都不在本 spec 范围内。

## 为什么是这两批

二十三张骨架页里，绝大多数缺的不是读面而是**数据本身**：委托侧三堵墙不降，接了也只能演空态。批 A 与批 B 是例外——它们的表、写入方、数据（批 B 是「写入方在、灌一次就有数据」）都已经在了，缺的只是查阅面那一段。这是当下唯一能把页面从骨架转 live 而不违反「不填假值」的路径。

## 子票

- 01 PC 客户合同与供应商协议查阅面（批 A）—— `Status: resolved`，MCP-2（`d69bbca` / `ed2bdab` / `3f5fad9`；交付与取证见票面）
- 02 VE 六类目录查阅面与导航条目（批 B 的干净那半）—— `Status: ready-for-agent`，派 MCP-1
- 03 治理查阅面：先裁租户维键形，再谈 http 包（批 B 的另一半）—— `Status: draft`，键形未裁前不派
- 04 已登记未读行的收口裁决（关务四类 + 商业阶段内容声明一族）—— `Status: draft`，导航裁决先行

票 02 与票 03 原本是同一批，盘点时按「都有 CLI、都零行、都缺查阅面」归在一起；实读库结构后拆开，分界与理由见 report.md 批 B 一节（治理三张表无租户列，`ADR-0077` 的形状装不上）。

## 地盘与占号

按 [parallel-sessions.md](../../docs/agents/parallel-sessions.md)「先分地盘」与「共享接线文件：占号、同笔、逐块核」。

| 谁 | 独占目录 |
|---|---|
| MCP-2（票 01） | `internal/partycommercial/**`、`apps/admin-web/src/pages/party/**`、`scripts/demo-seeds/seeds/commercial/**` |
| MCP-1（票 02） | `internal/visibilityexception/**`、`apps/admin-web/src/pages/visibility/**`、`scripts/demo-seeds/seeds/visibility/**`（新建） |

**共享接线文件（要占号）**：`cmd/parcel-api/endpoints.go`、`cmd/parcel-api/isolated_read.go`、`cmd/parcel-api/main.go`、`apps/admin-web/src/page-registry.tsx`、`apps/admin-web/src/navigation.ts`、`scripts/demo-seeds/seed.sh`。

这六个文件两票都必然要碰，且 `assembleBusinessEndpoints` 的**参数列表与返回切片是同一个 hunk 区**——两边同时改必撞。次序按票号：**票 01 先占、做完释号，票 02 再占**。票 02 的包内工作（ports 读端口、postgres 读适配器、`adapters/http` 的查询处理器与它们的真库测试）不碰任何共享文件，可以与票 01 完全并行；只有最后一段装配要等释号。

释号在频道里说一声，不靠猜。

**2026-08-26：票 01 已释号**（`3f5fad9` 落地，六个共享文件里它实际动了四个：`endpoints.go`、`main.go`、`unwired_orchestration.go`、`page-registry.tsx`，各只加自己的行，未动邻行）。票 02 可以进装配段。`navigation.ts` 与 `seed.sh` 票 01 没碰——两页早在导航里，种子走的是既有 `publish-batch.json`。
