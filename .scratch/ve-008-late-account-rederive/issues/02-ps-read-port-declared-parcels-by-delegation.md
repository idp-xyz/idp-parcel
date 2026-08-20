# PS 只读口：按（租户 + 委托）交回声明包裹清单

Category: enhancement
Status: resolved
Blocks: 03

## 为什么需要它

`parcel-shipment.acceptance-decision.formed` 的信封**不带包裹清单**。
`nrinbox.AcceptedDecision` 的字段是租户、货主客户账户、来源身份、委托号、提交版本、决定标识、
状态字——包裹清单刻意不传，跨上下文只传引用（该处注释已表明是有意为之）。

03 号票的补派生要「一份委托 → N 件包裹 → 各自重派生」，所以必须能按委托取回声明包裹清单。

## 代价低：列已经在了

ADR-0060 决策一已把 `declared_parcel_ids`（text[] 查询投影列）落在 `shipment_request` 同行，
并按决策四建了谓词 `state = 2` 的部分 GIN。现有 `ShipmentRequests.FindCurrentAcceptedByParcel`
正在用这一列做**按包裹反查委托**。

本票补的是**同一列的反方向单行读**：按（租户 + 委托）取该行的 `declared_parcel_ids`。
**不要迁移、不要新表、不要新列、不要动索引。**

## 要做什么

在 PS 侧加一个只读端口与其 PG 适配器，输入（租户 + 委托标识），交回该委托的声明包裹清单。
形状对齐同文件既有反查口的写法（租户贯通、命名与错误哨兵风格一致）。

零行是正当结果，如实交回空/未找到，**不发明包裹、不造默认**。

## 硬约束

- 本票**只动 `internal/parcelshipment/`**。不碰 VE、不碰 `cmd/parcel-dispatch/assemble.go`。
- 领域包不依赖 HTTP / `pgx`。中文注释。
- 走 `/implement` 内嵌 `/tdd`，先红后绿。
- 隔离 worktree 从 `origin/main`（`0f05304`）长出，不在共享树写。
- 只 `git add` 本票文件，**永不 `git add -A`**。
- 跑测试**按包路径**，不要 `-run "关键字"` 跑子集。

## Comments

- 2026-08-20 MCP-1：用户拍板走路线 a 后析出。与 01 号票无文件重叠，可并行。
- 2026-08-20 MCP-1（集成口）：MCP-5 交付 `11057dc`（已变基到 `004e338` 上）。集成门禁全过：
  gofmt / `go build` / `go vet` / `git diff --check` 零信号，全量 `go test -p 1 -count=1 ./...`
  含 PG 全绿（真库执行非跳过）。已推 `origin/main`，现为 tip。落地口：
  `ports.CurrentDeclaredParcelsView.FindCurrentDeclaredParcels`。
