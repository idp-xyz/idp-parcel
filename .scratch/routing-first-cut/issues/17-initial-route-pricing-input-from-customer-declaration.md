# 17 初始路由的计价输入：预路由用客户声明，逐段折成价卡区域待 PP owner 定

Category: enhancement
Status: in-progress——2026-10-10 22:5x 通道 5 认领（单 task-e4c159c7，改派自 task-58320bc4），分支 `mcp5-rfc17` 基 main `5e6cd4b6`。此前：ready-for-agent——2026-10-10 22:2x 通道 1 分诊（用户授权自决）：本票收为机制半边，「逐段折成价卡区域」拆到 [18](18-leg-endpoints-folded-into-price-card-zones.md)（needs-info），见「分诊裁定」。此前 needs-triage——2026-10-10 通道 1 立（用户授权自决），出自 [11](11-demo-network-adopted-as-reference-configuration.md) 的判据一取证
Blocked by: [15](15-candidate-identifier-minted-with-at-but-cost-adapter-splits-on-slash.md)（已解：15 于 2026-10-10 21:2x 进 main）——计价输入一接上就会撞那条缝；[13](13-customs-applicability-review-tails.md)（已解：13 于 2026-10-10 22:3x 进 main）——不是逻辑依赖，是地盘：13 在 `cmd/parcel-dispatch/assemble.go` 改注释、待评审与重放，等它进 main 再动，免得一个文件两个写入方（[16](16-demo-tenant-pre-acceptance-financial-control-cells.md) 也改同一文件的注释，派单时核）
归档：不属 [psb/04](../../product-strategy-boundary/issues/04-routing-product-strategy-first-cut.md) 的子票集；放在本目录是因为它挡在 11 判据一的路上。
地盘：`internal/networkrouting` 的计价消费方适配器（`adapters/parcelpricing`）与计价输入端口；`cmd/parcel-dispatch` 的 `routeCosts` 装配。区域折法不在本票（18）。
出处：[ADR-0148](../../../docs/adr/0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md) 决定四第 7 条与越权风险点 5；[10](10-candidate-cost-from-leg-buy-evaluations.md) 的完成记录（记为消费方实例半边）；[psb/05](../../product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md) 10-09 重走「成本一格」。

## 现象（通道 2 实测，钉 rfc/11 的 `eb57f999` 与 `bdee59cd`）

- 生产装配的真库用例里，采用演示网络之后初始路由证据已配置，唯一合格候选是 `SYN-LINE-CN-SG-01@1`；成本取数侧答 `ErrRouteCostSourceNotConfigured`，编排形成未决 `COST_SOURCE_NOT_CONFIGURED`。
- 成本单维排序只有一个合格候选也要已计价（`RankRouteCandidates`）；dispatch 的 `routeCosts` 计价输入是 `unconfiguredRoutePricingInput{}`。

## 待分诊

- 先把机制半边与越权风险点 5 分开：NR 侧计价消费方适配器、预路由用客户声明（ADR-0148 决定四第 7 条）是机制，现在就能做；「逐段折成价卡区域」是越权风险点 5，归 PP owner。
- 越权风险点 5：用户已授权通道 1 自决，分诊时可代裁并标「越权风险点 · 待 PP owner 复核」，或拆出一张等裁的票。先核 [psb/14](../../product-strategy-boundary/issues/14-pp-postal-prefix-granularity-and-public-unit-reference-configuration.md)（邮编前缀粒度与公开单元参考配置）是不是那一格的依据。
- 10 把这一格记为「消费方实例半边」：分诊先判这个归类准不准——适配器是机制，区域词汇与演示租户的价卡区域才是租户取值或产品策略。

## 分诊裁定（通道 1 · 2026-10-10 22:2x · 钉 main `1253c768`）

取证：通道 5 只读取证（`task-2a6b94e0`，读码与 `git grep`，未跑用例）。

- ADR-0148 决定四第 7 条原文：「逐段的计价输入由 NR 侧的 parcel-pricing 消费方适配器组装：包裹事实按 CONTEXT『预路由使用客户声明快照…』取；每段怎样折成价卡的区域词汇随 PP 的计价输入缝定。NR 不自算计价重量，也不自定区域。」越权风险点 5 原文：「段起讫怎样折成价卡区域，依赖 PP 计价输入缝的进度；那边未定时取数侧对该段答待判断，候选随之缺成本依据。」
- [psb/14](../../product-strategy-boundary/issues/14-pp-postal-prefix-granularity-and-public-unit-reference-configuration.md) 不是风险点 5 的依据：0148 只在决定二（邮编规范化）引它，它管目录前缀的匹配形态与公开格式。
- 码：`PricingInputFor(ctx, key)` 一次判断只交一份快照，`EvaluatePricingAcrossPlans` 各段共用，逐段区域在端口上无处表达；键含 `DeclaredParcelID`，PS 的 `DeclaredMeasurementView` 已在；[ADR-0171](../../../docs/adr/0171-pricing-input-resolver-uses-declaration-until-a-measurement-port-exists.md) 同题（PP 侧用申报并标来源）定为机制。

裁定：

1. 按 [ADR-0146](../../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 三类分：取客户声明快照作包裹事实是**机制**（NR CONTEXT 已定来源）；段起讫折成价卡区域的方法是 PP 的**产品策略**；演示各段落哪个区是演示租户的**租户取值**。
2. [10](10-candidate-cost-from-leg-buy-evaluations.md) 完成记录把这一格记为「消费方实例半边」，两半都不准：适配器与端口形状是机制，折法是产品策略，只有演示各段的区域是租户取值。10 已 resolved，不改它的票面，以此处为准。
3. 本票收为机制半边，转 ready-for-agent。区域折法拆到 18，needs-info：两种候选形态对 NR 的要求不同（一种要网络节点带邮编），通道 1 不代裁，留给 PP owner 或用户。

## 做什么（机制半边）

1. NR 侧计价消费方适配器的包裹事实取 PS 的客户声明（`DeclaredMeasurementView`），并标明来源是客户声明（ADR-0148 决定四第 7 条；标法与 ADR-0171 同形）。
2. 计价输入端口拆为「包裹事实」与「逐段区域」两口；18 定下之前，逐段区域在生产装配上答`未配置`。
3. `cmd/parcel-dispatch` 的 `routeCosts` 换掉 `unconfiguredRoutePricingInput{}`，接上 1、2。

## 完成判据（草稿：作者可细化，不可放宽）

- [ ] 逐段区域未配置时，该段答待判断，候选随之缺成本依据；初始路由不再整判断形成 `COST_SOURCE_NOT_CONFIGURED`，也不把缺的成本折成零。
- [ ] 包裹事实来自客户声明并带来源标记，有用例钉住。
- [ ] 生产装配的真库用例（带 DSN）钉住上面两条；证据层级 `S`。
- [ ] 落包前核 `internal/architecture` 的门禁（ADR-0171 越权风险点 1 同题）。
- [ ] 改了导出签名的，开工时广播一声。

## 不做

- 不定区域折法，不给演示各段落区（18）。
- NR 不自算计价重量、不自定区域（ADR-0148 决定四第 7 条）。
