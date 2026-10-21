# 66 NR 复核给候选出价用的是客户声明，不是收寄时已取得的物理事实或当前有效实测

Category: bug
Status: ready-for-agent——建议 R2（上线后尽快）：答案不错在结构上，错在依据。复核的排序与改善判断建立在客户声明上，实测与声明不符的包裹，复核比出的成本、选出的候选与按 CONTEXT 应得的不同；没有响亮失败，审计时从计划出处才看得出。2026-10-21 协调方裁定立（票 [demo-chain-gaps/115](../../demo-chain-gaps/issues/115-route-reassessment-never-loads-candidate-costs.md) 隔离评审 B1；用户「你作为业务和系统专家自决……解决所有的问题」授权），115 作者子代理执笔，分支 claude-dcg115。
Blocked by: [pp-pricing-input-seams/04](../../pp-pricing-input-seams/issues/04-no-actual-measurement-registry-and-valid-measurements-read-port.md)（`node-operations` 实际测量登记册与「仍有效的实际测量」只读口；没有它，到站复核那一半无从取数）
出处：demo-chain-gaps/115 隔离评审 B1。

## 现象

票 115 让复核向候选成本取数侧取成本。取数侧与初始路由是同一种装法，包裹事实一律取客户声明。NR CONTEXT 写的是按复核点分来源：

> 预路由使用客户声明快照；首次收寄复核使用当时已经取得的物理事实，到站复核使用当前有效物理实测。

ADR-0148 决定四第 7 条引同一句；UC-NR-003 目标与边界一节写「本用例必须重新使用已经发生的接货位置、控制依据和节点实测」。复核今天用的是预路由那一格的来源。

115 把它留作过渡做法（见 115 完成记录的评审修订代决），没有改成「没有物理事实即成本待判断」：那样复核一律排不了序，115 修的问题原样回来。

## 证据（读码，钉 `14419d5e3`，证据 `S`）

- `cmd/parcel-dispatch/assemble_reassess_on_handover.go` 的 `reassessRouteHandler` 接 `routeCosts(db, clock)`；`routeCosts`（`assemble.go`）的包裹事实取 `routeParcelFacts`，即 `nrpricing.NewDeclaredParcelFacts`。
- `internal/networkrouting/adapters/parcelpricing/declared_parcel_facts.go`：`DeclaredParcelFacts` 按判断键问 parcel-shipment 的客户声明，头注写明对应 CONTEXT「预路由使用客户声明快照」。
- 取数侧端口 `ParcelFactsSource.ParcelFactsFor(ctx, key)` 只收判断键，不知道这是哪一种判断（预路由、收寄复核、到站复核），也就选不了来源。
- `git grep -n -i -E 'measure|weight|dimension' -- internal/nodeoperations/ports/ internal/nodeoperations/domain/ migrations/node_operations/` 零命中：`node-operations` 没有实际测量登记册，NR 读不到实测（pp-pricing-input-seams/04）。
- 收寄采用记录（PS）与权威运输交接（TF）随复核触发进来，都不带重量或尺寸。

## 三类

机制。「哪个复核点用哪种物理事实」是 CONTEXT 已定的规则，不是租户取值。物理事实本身是作业事实，租户上线时由节点实测产生。

## 规模

M。NR 侧改取数侧的事实来源选择、复核编排把判断种类交给取数侧；NO 侧只加 ports 与只读查询（登记册本身归 pp-pricing-input-seams/04）；PS 侧若收寄采用要带「当时已取得的物理事实」，另议。

## 最小的诚实修法

1. 取数侧的包裹事实口带上判断种类：预路由、收寄复核、到站复核。初始路由照旧取客户声明。
2. 收寄复核取收寄时已取得的物理事实，到站复核取 `node-operations`「仍有效的实际测量」，都经只读口；事实引用标明来源，随出处留痕。
3. 该取的物理事实没有时怎么落，要裁：
   - 退回客户声明，并在出处与复核记录里标明退回（可审计，排序照常）；
   - 或该候选成本待判断（照 CONTEXT 字面，复核排不了序，候选评估记未决）。
   归 NR owner，NO、PS owner 会签。
4. 用例：收寄复核与到站复核各一条，实测与声明不符时，复核按实测出价、选中的候选随之不同。

## 相关

- demo-chain-gaps/115：引入复核取成本、留下本票这一格。
- pp-pricing-input-seams/04：NO 实际测量登记册。
- UC-NR-003「给开发的交接」：「复核取物理事实」一行指本票。
