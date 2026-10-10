# 17 初始路由的计价输入：预路由用客户声明，逐段折成价卡区域待 PP owner 定

Category: enhancement
Status: 完工，待评审与重放——2026-10-10 23:2x 通道 5 在分支 `mcp5-rfc17`（基 `5e6cd4b6`）上做完并推 origin：代码 `943a4379`、`26da76a6`，清点 `783b664b`，票面为其后一笔；完成记录见文末。进 main 时取 `943a4379`、`26da76a6`、`783b664b` 与票面笔；认领笔 `57d5478e` 只改本行。此前：in-progress——2026-10-10 22:5x 通道 5 认领（单 task-e4c159c7，改派自 task-58320bc4），分支 `mcp5-rfc17` 基 main `5e6cd4b6`。此前：ready-for-agent——2026-10-10 22:2x 通道 1 分诊（用户授权自决）：本票收为机制半边，「逐段折成价卡区域」拆到 [18](18-leg-endpoints-folded-into-price-card-zones.md)（needs-info），见「分诊裁定」。此前 needs-triage——2026-10-10 通道 1 立（用户授权自决），出自 [11](11-demo-network-adopted-as-reference-configuration.md) 的判据一取证
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

- [x] 逐段区域未配置时，该段答待判断，候选随之缺成本依据；初始路由不再整判断形成 `COST_SOURCE_NOT_CONFIGURED`，也不把缺的成本折成零。
- [x] 包裹事实来自客户声明并带来源标记，有用例钉住。
- [x] 生产装配的真库用例（带 DSN）钉住上面两条；证据层级 `S`。
- [x] 落包前核 `internal/architecture` 的门禁（ADR-0171 越权风险点 1 同题）。
- [x] 改了导出签名的，开工时广播一声。

## 不做

- 不定区域折法，不给演示各段落区（18）。
- NR 不自算计价重量、不自定区域（ADR-0148 决定四第 7 条）。

## 完成记录（2026-10-10，通道 5，task-e4c159c7，分支 `mcp5-rfc17` 基 `5e6cd4b6`）

**落点**

| 笔 | 做了什么 |
|---|---|
| `57d5478e` | 票面认领 |
| `943a4379` | `internal/networkrouting/adapters/parcelpricing`：去掉 `PricingInputSource`；新增 `ParcelFactsSource` / `ParcelFacts` 与 `DeclaredParcelFacts`（读 PS `DeclaredMeasurementView`，事实引用以 `declaration@baseline` / `declaration@<版本>` 标来源）；新增 `LegZoneSource` / `LegZoneQuery` / `LegZone`；可缺的 `ReferenceSeriesSource`；`Clock`。`RouteCandidateCostDeps` 的 `Input` 换成 `Facts`、`Zones`、`Series`、`Clock`；计价输入逐段成形（`evaluateLegs`、`legSnapshot`）。`cmd/parcel-dispatch`：`routeCosts(db, clock)` 接 `routeParcelFacts` 与 `unconfiguredLegZones`，删 `unconfiguredRoutePricingInput`。用例：适配器单测、`declared_parcel_facts_test.go`、真库用例改钉待判断并新增一条 |
| `26da76a6` | 真库用例登记演示成本卡 `SYN-PLAN-CN-SG-COST-01/v1`（`registerDemoCostCard`），各段才有评价目标、走得到折区域那一步 |
| `783b664b` | 在 `26da76a6` 干净树重生成清点：networkrouting 生产 71 → 72、测试 71 → 72；消费缝 networkrouting→parcelpricing 1 → 2 |
| 本笔 | 本完成记录、Status |

**判据逐条**

- 判据一：`TestAnUnconfiguredLegZoneLeavesTheCandidatePendingWithoutStoppingTheJudgment`（区域未配置：不报错、候选 `PENDING`、无出处）；`TestWithoutParcelFactsEveryLegStaysPending`（没有包裹事实同样待判断，且不去问区域）；`TestEachLegIsPricedInItsOwnZone`（两段各按自己的区出价，区域逐段问、带候选与段号）。编排侧 `RankingCostsPending` 落 `CANDIDATE_COSTS_PENDING`（`create_initial_route.go` 既有映射，本票未动）。
- 判据二：`TestTheDeclaredMeasurementBecomesTheParcelFactsTaggedAsADeclaration`（2.5 KG 与外廓照声明，事实引用为申报测量 `parcel-1` `declaration@baseline`）；只报重量、不可用的三格（没有画像、单位 `kg` 不在计价封闭集、不属已接受委托）与读口故障上抛各一条；`TestTheParcelFactsReachEveryLegsEvaluation`（1kg 出价、12kg 落档外待判断：评价读的是这份事实）。
- 判据三（`cmd/parcel-dispatch`，带 DSN）：`TestTheAdoptedDemoNetworkGetsPastRouteEvidenceThroughProductionWiring` 改钉生产 `routeCosts` 对演示候选交回 `PENDING`、不报错；新增 `TestADeclaredParcelReachesTheCostSideWhileLegZonesStayUnconfigured`——受理链接受一件声明 2.5 KG 的包裹，生产 `routeParcelFacts` 读到这份声明且标 `declaration@baseline`，同一判断经生产 `routeCosts`（演示网络 + 演示成本卡）答 `PENDING`、无出处、不折零。证据层级 `S`。
- 判据四：新文件落在 `internal/networkrouting/adapters/parcelpricing` 并导入 PS——`isCrossContextAdapter` 认 `internal/<owner>/adapters/<provider>/` 位置，`go test ./internal/architecture/...` 绿。
- 判据五：2026-10-10 22:5x 开工时 broadcast 导出签名改动（六个通道已投递）。

**判别力**（变异只在临时分离检出上做，证完拆掉，未提交）

- M1 区域未配置即整判断答 `ErrRouteCostSourceNotConfigured`：单测判据一那条红；`cmd/parcel-dispatch` 在 `943a4379` 上**没红**——各段停在「方案不在册」，走不到折区域。`26da76a6` 登记演示成本卡后，`TestADeclaredParcelReachesTheCostSideWhileLegZonesStayUnconfigured` 报 `err = network routing: route cost source not configured` 红。
- M2 没评价的段跳过（折零）：单测缺依据、区域未配置、没有包裹事实三条红；`cmd` 两条红。
- M3 一次判断只问第 1 段的区：`TestEachLegIsPricedInItsOwnZone` 红。
- M4 来源标记去掉：判据二那条单测与 `cmd` 新增那条红。

**判断项**

1. **包裹事实取数侧落在 `adapters/parcelpricing`，不在 `adapters/parcelshipment`。** 它读 PS、交出计价输入的原料，与 ADR-0171 越权风险点 1 同一处取舍；门禁放行的是位置，不看读了几家。**越权风险点 · 待 NR owner 复核。**
2. **`LegZone` 照 PP 计价输入的两条路留两形**（调用方分区；邮编路线交绑了目录的卡解，ADR-0109 决定四），两样都在时一并交评价。18 的两种候选形态各落其中一条，本票不替它选。**越权风险点 · 待 PP owner 复核。**
3. **计价范围取各段那张卡自己的。** PP 评价要求输入与卡同一计价范围（`PRICING_SCOPE_MISMATCH`），一条线路的各段可以引不同范围的卡。**越权风险点 · 待 PP owner 复核。**
4. **评价对象用试算（`ESTIMATE`）**，引用由判断键确定性派生（`route-cost-` 加摘要）：选路比价只为排候选，不形成费用。**越权风险点 · 待 PP owner 复核。**
5. **计价时点取路由判断时钟**，与证据视图同读一只 `systemClock`，不改 `RouteCandidateCostSource` 端口签名。两处各读一次钟；要同一时刻得把判断时点经端口传进来，是另一笔导出签名改动，本票不做。
6. **序列取值（汇率）另立可缺的 `ReferenceSeriesSource`，生产不接。** 它不是包裹事实；原先随整份快照进来，拆口之后要有去处，否则 10 的比较币种用例无处供读数。生产上要按序列换算的卡照旧待判断，与 10 判断项 3 记的已知缺口同一格。
7. **没有可用的包裹事实一律答没有、各段待判断，不整判断停下。** 本取数侧不再答 `ErrRouteCostSourceNotConfigured`；端口哨兵与编排的 `COST_SOURCE_NOT_CONFIGURED` 一格留着，端口契约不动。
8. **单位不折大小写，照 ADR-0171 决定三。** PS `EstimationAmountSource` 折大写，两处先例不一；本票取同题定为机制的那一份。演示提交报 `KG`，不受影响。**越权风险点 · 待 PP owner 复核哪一份为准。**
9. **读包裹事实排在解逐段依据之后**：依据译不出的响亮错误照旧先报；没有包裹事实时不去问区域。

**门**（WSL，go1.26.8，DSN 为门禁库 55432）

- `943a4379` 的内容（提交前工作副本）：改动的 `.go` 无 CR、无 BOM，`gofmt -l` 点名的一份提交前已格式化；`go build ./...`、`go vet ./...` 退 0；`go list` 反查 `adapters/parcelpricing` 的反向依赖（生产依赖 ∪ 测试二进制依赖）只有 `cmd/parcel-dispatch`，连同 `./internal/networkrouting/...` 与 `./internal/architecture/...` 以 `go test -count=1 -p 1` 跑：退 0，14 ok / 0 FAIL，21 s。
- `783b664b`：`gofmt -l` 无输出；`go test -count=1 -p 1 ./cmd/parcel-dispatch/ ./internal/networkrouting/adapters/parcelpricing/ ./internal/architecture/...` 三包 ok。
- `-v` 单跑 `cmd/parcel-dispatch` 三条生产装配真库用例：PASS，非 SKIP。全仓留给推送方重放后那一跑。

**未做 / 风险**

- 18 未定之前，演示候选的成本一格仍待判断，初始路由停在 `CANDIDATE_COSTS_PENDING`。
- `CONTEXT-MAP`「parcel-shipment → network-routing」没补「预路由按声明包裹读客户申报测量」一句：规则本身已在 network-routing CONTEXT 与 ADR-0148 决定四第 7 条，要不要在边上再写一句归 NR / PS owner。
- 首次收寄复核、到站复核改用实测，等 node-operations 实测登记册（`pp-pricing-input-seams/04`）。
