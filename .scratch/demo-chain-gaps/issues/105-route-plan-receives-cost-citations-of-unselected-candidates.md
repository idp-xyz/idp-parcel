# 105 两条以上候选都出价时，未选中候选的成本出处也交给计划，段号重复，初始路由计划形成报错

Category: bug
Status: needs-triage——2026-10-19 Claude Code 作者子代理立（出自票 [routing-first-cut/20](../../routing-first-cut/issues/20-bound-cost-cards-fold-zones-through-the-parcel-postal-route.md) 的隔离评审；用户「你作为业务和系统专家自决……解决所有的问题」授权）。
Blocked by: 无
地盘：NR `adapters/parcelpricing/cost_source.go`（`LoadCandidateCosts` 的合成）、`ports` 的 `RouteCandidateCosts`、`application/create_initial_route.go`（`planRecord`）。
出处：票 [routing-first-cut/10](../../routing-first-cut/issues/10-candidate-cost-from-leg-buy-evaluations.md) 完成记录「出处留痕：选中候选逐段……随计划 jsonb 一并落」；[ADR-0189](../../../docs/adr/0189-leg-pricing-zones-are-supplied-per-card-and-unbound-cards-take-a-registered-leg-zone.md) 文末补记越权风险点 6。

## 现象

候选成本取数侧把**每一条**已出价候选的逐段出处都交回，编排原样交给计划；计划只容得下选中那一条的出处。两条以上候选都出价、排序选中其一时，初始路由计划形成报错，整次处理报错——既不是未决，也不是无路由。

演示网络只有一条候选（`SYN-LINE-CN-SG-01@1`），所以演示动线没有暴露。真实租户的多线路、多供应商比价正是这一格。

## 证据（读码，钉 `5f0aa66d`，证据 `S`）

- `RouteCandidateCostAdapter.LoadCandidateCosts` 逐候选合成，已出价候选的出处一律追加进 `RouteCandidateCosts.Citations`；待判断、不可计价的候选不留出处。
- 出处 `PlannedLegCostCitation` 只带段号，不带候选。
- `CreateInitialRouteHandler.planRecord` 把整份 `costs.Citations` 原样交 `FormInitialRoutePlan`。后者要求出处段号互不重复、不越过选中候选的段数，不然答 `ErrInvalidRoutePlan`，编排上抛 `form initial route plan`。
- 于是两条候选各自第 1 段的出处段号都是 1，计划形成必报错。选中候选段数比另一条已出价候选少时，越界那一条也触发同一错。
- 取数侧单测 `TestTwoCandidatesPriceIndependentlyWithTheirOwnCards` 已显示两条候选各交回一条段号 1 的出处；编排层没有「两条候选都出价」的用例，计划形成那一层也就没人撞上。
- 票 routing-first-cut/20 把出处的评价标识从 NR 按「候选 + 段号」铸的形状换成 parcel-pricing 派生的 `EST-…`，从标识上更认不出一条出处属于哪条候选。

## 三类

机制。规模 S。

## 最小的诚实修法

取数侧不知道哪条会被选中（排序在编排里），所以出处要带候选维：`RouteCandidateCosts` 的出处按候选分开交回（出处本身加候选，或按候选键分组），编排在排序选中之后只取选中候选那一份交给计划。计划形状与 `FormInitialRoutePlan` 的段号校验不变。

完成判据：

- 编排层用例：两条候选都出价、选中便宜那条，计划形成，出处只有选中候选的各段。
- 取数侧用例：两条候选的出处各自认得出属于哪条候选。

## 要裁的

- 未选中候选的出处要不要也随计划留痕（审计「为什么没选它」）。按票 routing-first-cut/10 的口径（「选中候选逐段」）只留选中候选那一份，不改计划形状。要留就得让计划的出处带候选维、段号校验按候选分，那是另一种计划形状，归 NR owner 裁。

## 既有票

routing-first-cut/10（逐段成本出处随计划留痕）、routing-first-cut/20（出处的评价标识改由 PP 派生）。
