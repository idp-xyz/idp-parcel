# 17 初始路由的计价输入：预路由用客户声明，逐段折成价卡区域待 PP owner 定

Category: enhancement
Status: resolved——2026-10-11 00:26 通道 1 重放进 main：`e856cc2c`…`cb9d21ef`，推送方代落注释 `2e466286`，清点 `86100a68`；复评与进 main 记录见文末。此前 阻断已修，待 Spec 轴复评与重放——2026-10-10 23:5x 通道 5（task-c99d4d48）：通道 3 非作者评审（task-25096e0d，钉 `f698902d`）的 Spec 阻断一由 `98780032` 补用例修好，生产码不动；Standards 非阻断三条落在 `892325f5`（两处注释、一处哨兵改名，后者是生产码），其余处置见文末 Comments。进 main 时在下面那一列之外加取 `98780032`、`892325f5` 与本笔。此前：完工，待评审与重放——2026-10-10 23:2x 通道 5 在分支 `mcp5-rfc17`（基 `5e6cd4b6`）上做完并推 origin：代码 `943a4379`、`26da76a6`，清点 `783b664b`，票面为其后一笔；完成记录见文末。进 main 时取 `943a4379`、`26da76a6`、`783b664b` 与票面笔；认领笔 `57d5478e` 只改本行。此前：in-progress——2026-10-10 22:5x 通道 5 认领（单 task-e4c159c7，改派自 task-58320bc4），分支 `mcp5-rfc17` 基 main `5e6cd4b6`。此前：ready-for-agent——2026-10-10 22:2x 通道 1 分诊（用户授权自决）：本票收为机制半边，「逐段折成价卡区域」拆到 [18](18-leg-endpoints-folded-into-price-card-zones.md)（needs-info），见「分诊裁定」。此前 needs-triage——2026-10-10 通道 1 立（用户授权自决），出自 [11](11-demo-network-adopted-as-reference-configuration.md) 的判据一取证
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

- 判据一：`TestAnUnconfiguredLegZoneLeavesTheCandidatePendingWithoutStoppingTheJudgment`（区域未配置：不报错、候选 `PENDING`、无出处）；`TestWithoutParcelFactsEveryLegStaysPending`（没有包裹事实同样待判断，且不去问区域）；`TestEachLegIsPricedInItsOwnZone`（两段各按自己的区出价，区域逐段问、带候选与段号）；评审后补 `TestAPricedLegDoesNotStandInForALegWhoseZoneIsUnconfigured`（段 1 有区出价、段 2 区域未配置：候选 `PENDING`、无出处，不以段 1 的价顶替）。编排侧 `RankingCostsPending` 落 `CANDIDATE_COSTS_PENDING`（`create_initial_route.go` 既有映射，本票未动）。
- 判据二：`TestTheDeclaredMeasurementBecomesTheParcelFactsTaggedAsADeclaration`（2.5 KG 与外廓照声明，事实引用为申报测量 `parcel-1` `declaration@baseline`）；只报重量、不可用的三格（没有画像、单位 `kg` 不在计价封闭集、不属已接受委托）与读口故障上抛各一条；`TestTheParcelFactsReachEveryLegsEvaluation`（1kg 出价、12kg 落档外待判断：评价读的是这份事实）。
- 判据三（`cmd/parcel-dispatch`，带 DSN）：`TestTheAdoptedDemoNetworkGetsPastRouteEvidenceThroughProductionWiring` 改钉生产 `routeCosts` 对演示候选交回 `PENDING`、不报错；新增 `TestADeclaredParcelReachesTheCostSideWhileLegZonesStayUnconfigured`——受理链接受一件声明 2.5 KG 的包裹，生产 `routeParcelFacts` 读到这份声明且标 `declaration@baseline`，同一判断经生产 `routeCosts`（演示网络 + 演示成本卡）答 `PENDING`、无出处、不折零。证据层级 `S`。
- 判据四：新文件落在 `internal/networkrouting/adapters/parcelpricing` 并导入 PS——`isCrossContextAdapter` 认 `internal/<owner>/adapters/<provider>/` 位置，`go test ./internal/architecture/...` 绿。
- 判据五：2026-10-10 22:5x 开工时 broadcast 导出签名改动（六个通道已投递）。

**判别力**（变异只在临时分离检出上做，证完拆掉，未提交）

- M1 区域未配置即整判断答 `ErrRouteCostSourceNotConfigured`：单测判据一那条红；`cmd/parcel-dispatch` 在 `943a4379` 上**没红**——各段停在「方案不在册」，走不到折区域。`26da76a6` 登记演示成本卡后，`TestADeclaredParcelReachesTheCostSideWhileLegZonesStayUnconfigured` 报 `err = network routing: route cost source not configured` 红。
- M2 没评价的段跳过（折零）：单测缺依据、区域未配置、没有包裹事实三条红；`cmd` 两条红。
- M2b（通道 3 评审所设：M2 再加「全段都没评价才答待判断」）：原有用例全绿——M2 的红原来都出自空合计报错（`invalid rounding policy`），没有一条断言「缺一段即待判断」。评审后补的混合段用例在 M2b 下红（`fact = PRICED 500 minor, want PENDING`），撤掉变异后绿。
- M3 一次判断只问第 1 段的区：`TestEachLegIsPricedInItsOwnZone` 红。
- M4 来源标记去掉：判据二那条单测与 `cmd` 新增那条红。

**判断项**

1. **包裹事实取数侧落在 `adapters/parcelpricing`，不在 `adapters/parcelshipment`。** 它读 PS、交出计价输入的原料，与 ADR-0171 越权风险点 1 同一处取舍；门禁放行的是位置，不看读了几家。代价：清点按适配器所在目录计消费缝，这条 NR→PS 的读记在 networkrouting→parcelpricing（1 → 2），networkrouting→parcelshipment 照旧是 3，缝表里看不见这条边。**越权风险点 · 待 NR owner 复核。**
2. **`LegZone` 照 PP 计价输入的两条路留两形**（调用方分区；邮编路线交绑了目录的卡解，ADR-0109 决定四），两样都在时一并交评价。18 的两种候选形态各落其中一条，本票不替它选。**越权风险点 · 待 PP owner 复核。**
3. **计价范围取各段那张卡自己的。** PP 评价要求输入与卡同一计价范围（`PRICING_SCOPE_MISMATCH`），一条线路的各段可以引不同范围的卡。代价：输入的范围抄自卡，`PRICING_SCOPE_MISMATCH` 在这条路上恒不触发，这道校验守不到选路比价。**越权风险点 · 待 PP owner 复核。**
4. **评价对象用试算（`ESTIMATE`）**，引用由判断键确定性派生（`route-cost-` 加摘要）：选路比价只为排候选，不形成费用。**越权风险点 · 待 PP owner 复核。**
5. **计价时点取路由判断时钟**，与证据视图同读一只 `systemClock`，不改 `RouteCandidateCostSource` 端口签名。两处各读一次钟；要同一时刻得把判断时点经端口传进来，是另一笔导出签名改动，本票不做。
6. **序列取值（汇率）另立可缺的 `ReferenceSeriesSource`，生产不接。** 它不是包裹事实；原先随整份快照进来，拆口之后要有去处，否则 10 的比较币种用例无处供读数。代价：生产上没有序列读数，要按序列换算的卡照旧待判断。这是本票拆口后新显出的缺口：10 时整份计价输入未配置，把它盖住了，没单独记过。它不是 10 判断项 3 那一格——那条记的是策略版本登记的比较价格政策无人消费。
7. **没有可用的包裹事实一律答没有、各段待判断，不整判断停下。** 本取数侧不再答 `ErrRouteCostSourceNotConfigured`；端口哨兵与编排的 `COST_SOURCE_NOT_CONFIGURED` 一格留着，端口契约不动。
8. **单位不折大小写，照 ADR-0171 决定三。** PS `EstimationAmountSource` 折大写，两处先例不一；本票取同题定为机制的那一份。演示提交报 `KG`，不受影响。**越权风险点 · 待 PP owner 复核哪一份为准。**
9. **读包裹事实排在解逐段依据之后**：依据译不出的响亮错误照旧先报；没有包裹事实时不去问区域。
10. **包裹事实取 PS 查询时刻的当前采用，不钉判断键里的 `AcceptanceBaseline`。** `DeclaredMeasurementView.LoadDeclaredMeasurement` 只收租户与声明包裹，本取数侧没拿判断键的 `AcceptanceBaseline` 去钉版本；来源标记记的是读到的那一版（`declaration@baseline` 或 `declaration@<版本>`）。这合不合 network-routing CONTEXT「预路由使用客户声明快照」，归 NR owner 判。**越权风险点 · 待 NR owner 复核。**（出自通道 3 评审 Spec 非阻断④）

**门**（WSL，go1.26.8，DSN 为门禁库 55432）

- `943a4379` 的内容（提交前工作副本）：改动的 `.go` 无 CR、无 BOM，`gofmt -l` 点名的一份提交前已格式化；`go build ./...`、`go vet ./...` 退 0；`go list` 反查 `adapters/parcelpricing` 的反向依赖（生产依赖 ∪ 测试二进制依赖）只有 `cmd/parcel-dispatch`，连同 `./internal/networkrouting/...` 与 `./internal/architecture/...` 以 `go test -count=1 -p 1` 跑：退 0，14 ok / 0 FAIL，21 s。
- `783b664b`：`gofmt -l` 无输出；`go test -count=1 -p 1 ./cmd/parcel-dispatch/ ./internal/networkrouting/adapters/parcelpricing/ ./internal/architecture/...` 三包 ok。
- `-v` 单跑 `cmd/parcel-dispatch` 三条生产装配真库用例：PASS，非 SKIP。全仓留给推送方重放后那一跑。

**未做 / 风险**

- 18 未定之前，演示候选的成本一格仍待判断，初始路由停在 `CANDIDATE_COSTS_PENDING`。
- `CONTEXT-MAP`「parcel-shipment → network-routing」没补「预路由按声明包裹读客户申报测量」一句：规则本身已在 network-routing CONTEXT 与 ADR-0148 决定四第 7 条，要不要在边上再写一句归 NR / PS owner。
- 首次收寄复核、到站复核改用实测，等 node-operations 实测登记册（`pp-pricing-input-seams/04`）。

## Comments

**评审 ← 通道 3 · 钉 `f698902d` · 23:4x**（非作者评审，`task-25096e0d`，只读。原文照录。）

rfc/17 非作者评审（通道 3，diff 5e6cd4b6..f698902d，只读）结论：Spec 轴 1 条阻断（只缺用例，生产码正确），Standards 轴无阻断。

【Standards】
阻断：无。
非阻断：① internal/networkrouting/ports/ports.go 的 ErrRouteCostSourceNotConfigured、RouteCandidateCostSource 头注仍写「计价输入没接时以它作答」「消费方实例半边」——943a4379 后生产已无产出方，17 分诊裁定 2 也判该归类不准；哨兵可留，注释该改。② cmd/parcel-dispatch/reference_network_wiring_test.go registerDemoCostCard 头注「演示网络三段共引」数的是种子里的段数，犯 AGENTS「改文档」计数条，写「各段共引」即可。③ declared_parcel_facts.go ParcelFactsFor 把 PS 租户/包裹键译不出报成 errUntranslatableEvaluation（untranslatable pricing evaluation），名不副实（判断项）。
无发现：红线（未给任何租户灌区域值；种子、迁移、PS/PP 生产码零改动）；位置合 ADR-0025，门禁 isCrossContextAdapter 放行，PS 类型未进 nrports 签名；导出签名的外部调用方只有 cmd/parcel-dispatch（assemble.go 与接线测试）；注释都是中文、无行号；⑧ 单位表与 ADR-0171 决定三同一张封闭表（NewWeightUnit/NewLengthUnit），大小写敏感。

【Spec】
阻断：① 判据一「不折零」对混合段没有判别力。变异 M2b（cost_source.go compose 里没评价的段 continue，全段都没评价才答待判断）下，adapter 包、整个 internal/networkrouting/...、cmd/parcel-dispatch（带 DSN）全绿。完成记录里 M2 的红全部来自空合计报错（minorUnitsOnce: invalid rounding policy），没有一条断言「缺一段即 PENDING」。补一条「段 1 有区、段 2 未配置 → PENDING、无出处」的用例即解，生产码不动；18 一落地正好会走到这一格。
非阻断：② 判断项的代价没写全——①：清点把这条 NR→PS 的读记在 networkrouting→parcelpricing（1→2），→parcelshipment 仍是 3，缝表里看不见这条边；③：计价范围抄卡自己的，PRICING_SCOPE_MISMATCH 在这条路上恒不触发。③ ⑥ 不是 10 判断项 3 那一格（那条记的是策略版本的比较价格政策无人消费）；「生产上没有序列读数」在 10 时被整份输入未配置盖住了、没单独记过，宜改写成本票新显出的缺口。④ DeclaredParcelFacts 取的是 PS 「查询时刻的当前采用」，不钉判断键里的 AcceptanceBaseline；是否合「预路由使用客户声明快照」归 NR owner 判。⑤ 「编排落 CANDIDATE_COSTS_PENDING」没有任何用例经处理器钉住（域层只钉 RankingCostsPending，真库用例只测到适配器）；映射是既有代码，不算本票欠的。
无发现：判据②–⑤成立；判断项② LegZone 留两形，没替 18 选形态；26da76a6 的卡读自既有种子文件（seed.sh 本就登它），只登进测试库；没做票面外的活（Series、Clock、试算对象、逐段成形输入都是拆口必需）。

【先列的清单（钉 5e6cd4b6，读 diff 前写）】
A 包裹事实：取 PS DeclaredMeasurementView（键里带 DeclaredParcelID）；来源标 ArtifactDeclaredMeasurement + declaration[@baseline|@版本]（与 ADR-0171 同形）；PS 五格都有落点——无测量、待复核、未申报、非声明包裹都答没有事实，读面坏了就上抛；单位只认 KG/G/LB/OZ 与 CM/IN；不补尺寸，不读实测。
B 区域未配置：该段不评价、算待判断 → 候选 PENDING → RankRouteCandidates → CANDIDATE_COSTS_PENDING；不整判断答哨兵；各段区域不同就要逐段成形输入。
C 不能变：NR 不算计价重量、不定区域、不折零；不给任何租户灌区域值；PS 代码不动；PS 类型不进 nrports；缺依据不落「无当前有效路由」。
对照结果：A/B/C 全部兑现，只有混合段折零没有用例（Spec 阻断①）。

【门】go1.26.8；build/vet 退 0；networkrouting/... + architecture 共 13 包 ok；cmd/parcel-dispatch（DSN 55432，-p 1）ok；-v 单跑 TestTheAdoptedDemoNetworkGetsPastRouteEvidenceThroughProductionWiring / TestADeclaredParcelReachesTheCostSideWhileLegZonesStayUnconfigured / TestTheCostSideResolvesTheLineOfACandidateMintedByCatalogFolding 三条 PASS，非 SKIP。

【变异（go test -overlay，副本与 JSON 在 /tmp/rfc17-mut）】
M1（evaluateLegs 里区域未配置改成 return ErrRouteCostSourceNotConfigured）：TestAnUnconfiguredLegZoneLeavesTheCandidatePendingWithoutStoppingTheJudgment 与 cmd 的 TestADeclaredParcelReaches… 红；cmd 的 TestTheAdoptedDemoNetwork… 不红（没登卡、走不到折区域，与作者所记一致）。
M2（compose 里没评价的段 continue）：TestAMissingBasis…、TestAnUnconfiguredLegZone…、TestWithoutParcelFacts… 与 cmd 两条红，全是空合计报 invalid rounding policy。
M2b（M2 加上「全段未评价才答待判断」）：全绿 → Spec 阻断①。
检出的 git status 始终干净；git worktree remove /tmp/idp-review-rfc17-mcp3 已做（未加 --force）。未改任何分支、未提交、未推。

### 处置（通道 5 · 2026-10-10 23:5x · `task-c99d4d48` · 钉 `892325f5`）

- **Spec 阻断①**：已修，`98780032`，只动 `cost_source_test.go`，生产码不动。新用例 `TestAPricedLegDoesNotStandInForALegWhoseZoneIsUnconfigured`：两段同引一张卡，段 1 区域答 Z1、段 2 答未配置；断言两段各问一次区域、候选 `PENDING`、无出处。替身加一格 `unzonedOrdinals`，按段号答未配置。红绿取证：通道 3 的 M2b 副本 `/tmp/rfc17-mut/m2b/cost_source.go` 与本树 `cost_source.go` 只差 M2b 那两处，按本树路径另写 overlay（`/tmp/rfc17-mcp5-m2b.json`）。套上它，新用例红：`fact = PRICED 500 minor, want PENDING（段 2 缺成本依据，不以段 1 的价顶替）`，整包也只红这一条。撤掉 overlay，新用例与整包都绿。
- **Standards 非阻断①**：已改，`892325f5`。`ErrRouteCostSourceNotConfigured` 与 `RouteCandidateCostSource` 头注改成机制说法：哨兵只答整条取数路径没接，编排据它形成 `COST_SOURCE_NOT_CONFIGURED`；缺某一段的输入只让那一段待判断。不再写「消费方实例半边」。哨兵与端口契约不动（判断项 7）。
- **Standards 非阻断②**：已改，`892325f5`：`registerDemoCostCard` 头注「三段共引」改「各段共引」。同文件另有两处「三段」，在 `TestTheAdoptedDemoNetworkGetsPastRouteEvidenceThroughProductionWiring` 的场景头注与断言消息里。同一用例的段数断言（`len(evidence.Paths[0].Legs) != 3`）守着它们，段数一变用例就红，不会无声失真，所以没改。
- **Standards 非阻断③**：选改名，`892325f5`。`ParcelFactsFor` 与 `declarationFact` 的三处译不出改报新立的 `errUntranslatableParcelFacts`（`network routing: untranslatable parcel facts`）。它未导出，没有调用方按类别分派，用例也不读这串消息。这是本轮唯一一处生产码行为改动，只改错误消息前缀；复评请顺看 Standards。
- **Spec 非阻断②**：判断项 1、3 原文已补代价：清点里看不见这条 NR→PS 的读；输入的范围抄自卡，`PRICING_SCOPE_MISMATCH` 在这条路上恒不触发。
- **Spec 非阻断③**：判断项 6 已改写，记为本票拆口后新显出的缺口，不再挂到 10 判断项 3。
- **Spec 非阻断④**：已作为判断项 10 补入，标「越权风险点 · 待 NR owner 复核」。
- **Spec 非阻断⑤**：只记，不补。至今没有用例经处理器钉住「编排落 `CANDIDATE_COSTS_PENDING`」：域层只钉 `RankingCostsPending`，真库用例只测到适配器。映射是 `create_initial_route.go` 的既有代码，不是本票欠的。
- **验证**（WSL，go1.26.8，DSN 指 55432）：`892325f5` 的内容在提交前跑——改动文件 `gofmt -l` 无输出，无 CR、无 BOM；`go build ./...`、`go vet ./...` 退 0；`go test -count=1 -p 1 ./internal/networkrouting/... ./internal/architecture/...` 退 0，13 包 ok；带 DSN 跑 `go test -count=1 -p 1 ./cmd/parcel-dispatch/...` 退 0。提交后在干净的 `892325f5` 上 `-v` 单跑 `TestTheAdoptedDemoNetworkGetsPastRouteEvidenceThroughProductionWiring` 与 `TestADeclaredParcelReachesTheCostSideWhileLegZonesStayUnconfigured`，都是 PASS，非 SKIP。只在既有测试文件里加了用例，没增删测试文件，不重生成机制清点。没跑全仓。

### 复评 ← 通道 3 · 钉 `bb2806f1` · 2026-10-11 00:2x（`task-a41762a3` ← 通道 1；推送方照录）

rfc/17 复评（通道 3，diff f698902d..bb2806f1，只读）结论：两轴都无阻断，阻断①已真解，可以重放。

**Spec**

- 阻断：无。阻断①已解——bb2806f1 上 cost_source.go 与 f698902d 逐字节相同（cmp 核过），故原 M2b 副本仍适用；作者的 /tmp/rfc17-mcp5-m2b.json 值就是这份副本、只是键换成作者树路径，确只差 M2b 两处。我按本检出路径重写 JSON 套上：新用例 TestAPricedLegDoesNotStandInForALegWhoseZoneIsUnconfigured 红（fact = PRICED 500 minor, want PENDING），整个 internal/networkrouting/... 只红这一条；同一 overlay 下 -skip 这条 → 包绿（它就是唯一的判别用例）；撤掉 overlay → 12 包全绿。
- 非阻断：无。
- 无发现：处置②–⑤照原意——判断项 1、3 补的代价就是评审所写，仍标待 NR / PP owner 复核；判断项 6 改写为拆口后新显的缺口、不再挂 10 判断项 3；新增判断项 10 只陈述事实、标待 NR owner 复核，没替 owner 拍板；⑤ 只记不补，与原意（既有映射、不算本票欠的）一致。评审原文照录无误。

**Standards**

- 阻断：无。
- 非阻断（可不改、不挡重放）：internal/networkrouting/ports/ports.go 的 ErrRouteCostSourceNotConfigured 新头注写「只缺某一段的输入（区域、包裹事实）……那一段待判断」——包裹事实是整件判断的输入，缺了是各段全待判断，不是「某一段」；措辞不准，语义没错。
- 无发现：892325f5 的两处头注只写机制，「消费方实例半边」已去掉，没再写实例现状。errUntranslatableParcelFacts 未导出，git grep 只见定义与 declared_parcel_facts.go 里三处 %w 包装，全仓无 errors.Is 按类别分派；编排对非哨兵错误一律落 COST_SOURCE_UNAVAILABLE，行为只变消息前缀。「各段共引」已改；剩下两处「三段」（TestTheAdoptedDemoNetworkGetsPastRouteEvidenceThroughProductionWiring 的场景头注与 Fatalf 消息）和同一用例的 len(evidence.Paths[0].Legs) != 3 断言同处，段数一变用例即红，不会无声失真，理由成立。

门：在 bb2806f1 检出上 go build ./... 与 go vet ./... 退 0；go test -count=1 ./internal/networkrouting/... 12 包 ok；未跑 cmd 与全仓（按卡面）。M2b 红绿：套 M2b 新用例红、他人全绿；M2b + -skip 新用例绿；不套绿。检出 git status 始终干净，git worktree remove /tmp/idp-rereview-rfc17-mcp3 已做（未加 --force），/tmp/rfc17-mut 已删。未改任何分支、未提交、未推。

### 复评处置（通道 1 · 2026-10-11 00:2x）

- Standards 非阻断：推送方代落 `2e466286`，只改 `ErrRouteCostSourceNotConfigured` 头注那一行，把缺某一段的区域（那一段待判断）与缺包裹事实（各段都待判断）分开说。作者通道 5 余量已留尽，一行注释不值再派一轮。

## 进 main 记录（通道 1 · 2026-10-11 00:26）

- 重放：隔离检出 `/tmp/idp-land-rfc17` 从 `53c71086` 起，cherry-pick 分支各笔，零冲突；分支上的清点笔 `783b664b` 不搬，清点在重放 tip 上重生成。`cmd/parcel-dispatch/assemble.go` 也被 [16](16-demo-tenant-pre-acceptance-financial-control-cells.md) 改过注释，自动合并后两边的改动都在：本票改过的每份非清点文件与作者 tip `bb2806f1` 逐文件比，只差 16 改的那两段注释。

  | 分支 `mcp5-rfc17` | main |
  |---|---|
  | `57d5478e` 认领 | `e856cc2c` |
  | `943a4379` 机制半边 | `b33ea7f6` |
  | `26da76a6` 真库用例 | `aaa870f1` |
  | `783b664b` 清点 | 不搬，由 `86100a68` 重生成 |
  | `f698902d` 完成记录 | `c3168f44` |
  | `98780032` 阻断修复（用例） | `1d7be3bc` |
  | `892325f5` 非阻断（注释与哨兵） | `a8bd8cd5` |
  | `bb2806f1` 评审照录与处置 | `cb9d21ef` |
  | 推送方代落（复评非阻断） | `2e466286` |

- 清点 `86100a68`（在 `2e466286` 的检出上重生成）：networkrouting 生产 71→72、测试 71→72；跨上下文消费缝生产文件 94→95，networkrouting→parcelpricing 1→2。
- 门（同一检出 @ `86100a68`）：gofmt 两改动目录无输出；`go build ./...`、`go vet ./...` 退 0；真库探针 `TestFreezeScopesAreInvisibleToEachOther` 为 PASS 非 SKIP；带 DSN `go test -count=1 -p 1 ./...` 00:23:16→00:26:15，138 ok / 0 FAIL / 14 无测试 / 0 cached；`-v` 单跑 `TestTheAdoptedDemoNetworkGetsPastRouteEvidenceThroughProductionWiring`、`TestADeclaredParcelReachesTheCostSideWhileLegZonesStayUnconfigured`、`TestTheCostSideResolvesTheLineOfACandidateMintedByCatalogFolding` 均为 PASS。
- 推：00:26:32 `ls-remote` 核 `53c71086` 未动 → 00:26:36 `push 86100a68:main` 成，远端 main = `86100a68`（本票各笔、代落一笔与清点，其下无他人提交）；共享树 ff 同 SHA。本笔簿记在其上，纯 .md，推送方自审。
- 评审：非作者 ← 通道 3（Spec 阻断一，已修）；复评 ← 通道 3，两轴零阻断（见上）。
