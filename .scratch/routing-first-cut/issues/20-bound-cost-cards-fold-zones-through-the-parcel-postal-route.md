# 20 绑了分区目录的成本卡：候选成本经包裹邮编路线与 parcel-pricing 读数补全取区

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-dcgrfc20（2026-10-19 作者子代理，见文末「完成记录」与「评审记录」）。此前 ready-for-agent——2026-10-18 Claude Code 会话立（用户「你作为业务和系统专家自决」授权），ADR-0189 决定四拆出的那一步。
Blocked by: 无（[18](./18-leg-endpoints-folded-into-price-card-zones.md) 已落段计价区域那一路）
出处：[ADR-0189](../../../docs/adr/0189-leg-pricing-zones-are-supplied-per-card-and-unbound-cards-take-a-registered-leg-zone.md) 决定一第一条、决定四；ADR-0152 决定三（估价与正式评价同一份读数补全）。

## 现状（读码，钉 `f8351cef`）

- `RouteCandidateCostAdapter.evaluateLegs` 直接调 `ppdomain.EvaluatePricingAcrossPlans`，不经 parcel-pricing 共用的读数补全：绑了分区目录的卡读不到目录读数，恒答 `ZONE_UNRESOLVED`（待判断）；按序列换算的卡同理（`Series` 未接）。
- NR 在初始路由请求上携带了寄件、收件两端的国家与邮编（ADR-0075，`CreateInitialRouteCommand.Carried.Geo`），但只交给证据视图，成本取数侧拿不到。

## 做什么

1. 把携带的邮编路线交进成本取数侧（`LoadCandidateCosts` 或判断键的携带内容），`LegZone` 带上 `Postal`；不新开读 parcel-shipment 地址的口（ADR-0075）。
2. 逐段评价改走 parcel-pricing 的读数补全（目录读数与序列读数），与正式 BUY 评价、受理前估价同一份实现；不在 NR 侧另写一份补全。
3. 生产装配 `routeCosts` 接上目录与序列的在用版本解析器（同 `preAcceptanceTrials` 的接法）。

## 完成判据

- 真库用例：一张绑了分区目录的 BUY 成本卡，经包裹邮编路线读出区域后出价；没绑目录、也没登段计价区域的卡仍待判断；要按序列换算的卡在序列有在用版本时出价。
- 未绑目录的卡照旧取段计价区域（票 18 的用例不变）。

## 完成记录（2026-10-19，Claude Code 作者子代理，分支 `claude-dcgrfc20`，基 `5f0aa66d`）

授权：用户原话「你作为业务和系统专家自决……解决所有的问题」。权威 ADR-0189 决定一、四正文不改，落地时要定的几格写进它的文末补记「补记：票 routing-first-cut/20 落绑目录那一路时的补充裁定」，这里只摘要。提交 `3df0eeba`、`d10c631b`、`07706f7f`。

### 做了什么

- parcel-pricing：
  - 邮编路线的拼法收成一处 `PostalRouteFromDeclared`（收件邮编没报或不成形即没有路线，寄件邮编不成形只按收件拼）。正式评价的输入解析（`adapters/transportfulfillment`）与受理前估价的申报读口（`adapters/parcelshipment`）原各写一份，改用它；选路比价是第三个用处。
  - 「对给定价卡形成试算」的声明加可缺的比较币种与事实引用两格；两格只在声明了时进评价标识，受理前估价的标识不变。
- network-routing：
  - 候选成本取数口 `LoadCandidateCosts` 加随请求携带内容的参数，编排把交给证据视图的同一份交过去。
  - 成本取数侧逐段走 PP 的试算入口（窄面 `PlanGivenEstimates`）：交包裹事实、这一段登的区（没登空着）、包裹邮编路线、比较币种与事实引用，哪张卡用哪一格由 PP 按目录绑定定。答复逐格译回：已评价交回评价；输入不全即这一段待判断；补齐读数读不回整次报错（编排落 `COST_SOURCE_UNAVAILABLE`）；未受理与认不出的形状上抛。
  - 拆掉 NR 自带序列读数那一口（`ReferenceSeriesSource`、`RouteCandidateCostDeps.Series`，生产从没接）与 `LegZone` 的邮编两格。
- 生产装配：`routeCosts` 接 `planGivenEstimateEvaluations`，与 `preAcceptanceTrials` 共用这一只的装法（序列、目录两对在用解析）。
- 文档：UC-NR-001 立 `AT-NR-070`–`072`；ADR-0189 Status 前向指针、Links 里 ADR-0179 那一行补说明、文末补记；AT 盘点追记。

### 代决

- **走受理前估价开的那一口，不另开入口**：分区给法逐卡定（ADR-0152 决定五）只在 PP 写一处，NR 不按卡分支。
- **邮编路线经取数口参数交，不经段计价区域那一口**：路线是整件包裹的，不随段变。
- **序列读数只有 PP 在用序列一处来源**。
- **出处里的评价标识改由 PP 派生**（`EST-…`）：同一判断里引同一张卡、同一份声明的几段标识相同，靠段号区分。
- **选路比价评价记 `S`**，与受理前试算同一条理由。
- **AT 号自取 `AT-NR-070`–`072`**：协调方给的「本票要点」写「按各 UC 现有最大号之后取」，AT-NR 是全上下文一条序列，`069` 已由票 demo-chain-gaps/63 用掉。

### 越权风险点

ADR-0189 文末补记接正文编号续写，这里只列标题：

5. 选路比价借受理前估价开的那一口（PP owner）。ADR-0152 决定八把「给结算、路由或任何下游消费试算结果的口」列为不做的，两者的关系见补记。
6. 出处里的评价标识换了形状，已落库计划的出处不回填（NR owner）。
7. 路线按整件包裹、不按段：几段同引一张按目的邮编分区的卡就几段同区（PP、NR owner）。
8. NR 不能另带一份比价汇率（NR、PP owner）。

另一条只在本票：

9. `AT-NR-070`–`072` 没经协调方预分，与在途的 NR 票（如 claude-dcg64）可能撞号，进 main 时再顺延。

### 用例

- PP：`TestPostalRouteFromDeclaredDropsAMalformedOriginButNotAMalformedDestination`；`TestPlanGivenEstimateCarriesTheDeclaredComparisonCurrencyAndFacts`。
- NR 编排：`TestTheCarriedProjectionReachesTheCostSource`（`AT-NR-070` 编排半边）。
- NR 取数侧（经真的 PP 试算入口，补齐读数那几口换替身）：
  - `TestABoundCardsZoneIsReadFromTheCatalogueThroughThePostalRoute`（`AT-NR-070`）
  - `TestABoundCardWithoutAPostalRouteStaysPending`、`TestAnUnboundCardDoesNotReadItsZoneFromThePostalRoute`、`TestAnUnreadableCatalogueFailsTheWholeCostLoad`（`AT-NR-071`）
  - `TestAComparisonCurrencyReachesThroughTheLegs` 改走在用序列、`TestASeriesBoundCardWithoutAnInForceSeriesStaysPending`（`AT-NR-072`）
  - 票 18 那条 `TestARegisteredLegZoneDoesNotStandInForABoundCardsCatalogue` 并进 `AT-NR-071` 两条，删去。
- 生产装配真库（`cmd/parcel-dispatch/route_cost_reading_completion_wiring_test.go`，卡在用例里造、与演示成本卡同身份同档位）：
  - `TestABoundCostCardIsPricedThroughThePostalRouteInProductionWiring`：绑目录的卡经 200000 → 018000 出价 25500 分、三条出处；不带收件邮编时同一张卡待判断、登的 Z1 不顶上。
  - `TestAnUnboundCostCardWithoutALegZoneStaysPendingInProductionWiring`：没绑目录、没登段计价区域的卡待判断。
  - `TestAnExchangeRateBoundCostCardIsPricedWhenTheSeriesIsInForce`：SGD 卡绑 `SYN-SERIES-FX-CNY-SGD`、比较币种 CNY，序列在用时三段各 454.75 CNY、合 136425 分；序列没登时待判断。
  - `TestAnUnboundCostCardIsPricedThroughItsRegisteredLegZonesInProductionWiring`（评审修订时补，判据二）：没绑目录的卡、种子照登三段各 Z1 的段计价区域，出价 25500 分、三条出处；请求带不带收件邮编都一样，区出自登的段计价区域。与上一条只差段计价区域那一份。
  - 票 18 的 `TestADeclaredParcelOnZonedLegsIsPricedThroughProductionWiring` 随票 43 改走目录那一路（演示成本卡绑了目录），不再证判据二。

### 验证

- `go build ./...`、`go vet`（NR、PP、SA、`cmd/parcel-dispatch`、`cmd/parcel-api`、`scripts/demo-seeds`）、`gofmt -l` 干净。
- 带 55432 DSN、`-p 1 -count=1`：`./internal/architecture/`、`./internal/networkrouting/...`、`./internal/parcelpricing/...`、`./internal/settlementaccounting/...`、`./cmd/parcel-dispatch/`、`./cmd/parcel-api/`、`./cmd/parcel-pricing-register/`、`./cmd/parcel-network-register/`、`./scripts/demo-seeds/...` 全绿。中途 `internal/architecture` 的事务闭包门禁拦下新用例里事务回调中的 `t.Fatalf`，已把断言搬到闭包外。
- 一次性库实走见票 [demo-chain-gaps/43](../../demo-chain-gaps/issues/43-demo-cost-card-binds-the-zone-catalogue.md) 完成记录：演示成本卡绑目录之后，初始路由 `ROUTE_FORMED`，三条出处各 85 CNY、评价标识 `EST-…`。

### 偏离与待裁

- 偏离：票面「`LegZone` 带上 `Postal`」没照做。路线是整件包裹的，经取数口参数交；`LegZone` 只剩登的区。票面给的两条路（`LoadCandidateCosts` 或判断键的携带内容）取了前一条。
- 待裁（PP owner）：选路比价借受理前估价开的那一口，与 ADR-0152 决定八的关系，见 ADR-0189 文末补记越权风险点 5。

## 评审记录（2026-10-19，隔离评审；作者子代理修订，分支 `claude-dcgrfc20`，基仍 `5f0aa66d`）

本票与 [demo-chain-gaps/43](../../demo-chain-gaps/issues/43-demo-cost-card-binds-the-zone-catalogue.md) 同一分支、同一次评审，评审记录以本票为准，票 43 只记它自己那一项。阻断 0 项。非阻断 11 项：标「进 main 前建议修」的 4 项全修；其余 7 项里修 4 项（5–8），立票 1 项（10），不采纳 2 项（9、11）。修订提交 `1ff37f4a`、`07be746d`。

### 非阻断

1. **完成判据二在分支 tip 上没有真库证据**（建议进 main 前修）——修了。票 43 让演示成本卡绑了目录之后，`TestADeclaredParcelOnZonedLegsIsPricedThroughProductionWiring` 走的是目录那一路，种子登的 Z1 没人读。`cmd/parcel-dispatch/route_cost_reading_completion_wiring_test.go` 补 `TestAnUnboundCostCardIsPricedThroughItsRegisteredLegZonesInProductionWiring`：种子照登（保留段计价区域）、卡没绑目录，出价 25500 分、三条出处、评价标识 `EST-…`。与 `TestAnUnboundCostCardWithoutALegZoneStaysPendingInProductionWiring` 成对，只差段计价区域那一份。另加一格：请求不带收件邮编也照样出价，证明区出自登的段计价区域、不出自邮编路线。完成记录「用例」末条改为点名新用例，并写明票 18 那条已改走目录。
2. **ADR-0189 Links 里 ADR-0179 出现两次**（建议进 main 前修）——修了。删掉新加的那一行，说明并进原有那一行：「决定四；『对给定价卡形成试算』的入口由它开，文末补记让选路比价也走这一口」。完成记录「做了什么」里「Links 补 ADR-0179」改为「Links 里 ADR-0179 那一行补说明」。
3. **`testSource` 头注数 PP 补齐口的个数**（建议进 main 前修）——修了，改为「补齐读数那几口换成下面两本册」。完成记录「用例」里同样的「四口」一并改掉。
4. **`GeoResolutionProjection` 头注「只用它解析服务区域」被本分支弄得不成立**（建议进 main 前修）——修了，取「不列用途」：「本上下文只在判断里读它，不保存地址本体」。要守的不变量是 ADR-0075 的请求期携带、不落地；列用途还会随下一个读者再变旧。
5. **`PostalRouteFromDeclared` 头注与同包测试头注逐个列调用方**——修了。两处都只写不变量与理由：委托报的两端邮编拼成路线只在这里写，各处各写一份就可能解出两个分区。PS 申报读口 `postalRouteOf` 的头注点名正式评价那一处，是本分支之前就有的成对引用，没动。
6. **ADR-0189 文末补记三处文字**——修了：（1）「Consequences 前置的」改为「决定四末句」；（2）越权风险点 5 改为 ADR-0152 决定八的原句引文「给结算、路由或任何下游消费试算结果的口」（列在「不做的」下）；完成记录越权风险点 5 同改；（3）`TestAnUnboundCardDoesNotReadItsZoneFromThePostalRoute` 改引决定一第二、四条。
7. **票 43 代决「段计价区域的种子三行保留」理由过时**——修了，在票 43，见那边的评审记录。
8. **`planGivenEstimateIdentity` 注释写变更前后对比，且没有用例钉住「不声明时与旧形状逐字相同」**——修了。注释改写理由：受理前估价的逐件试算评价标识随结算的控制结果留痕保存（ADR-0179 决定六），同一声明、同一张卡要一直派生出同一标识，留痕才对得上复算。新用例 `TestAnUndeclaredComparisonAndFactsKeepThePlanGivenIdentity` 钉字面值 `EST-b529fb89ab9a6cc2`。这个值先在本分支跑出来，再把同一条用例放进 `5f0aa66d` 的 PP 包（`git archive` 到临时目录）跑，也是这个值，PASS（`-v` 确认跑到）。跑完临时目录已删。
9. **`docs/adr/README.md` 的 ADR-0189 那一行没提文末补记**——不采纳。README 是跨票热点，协调方口径「只追加行，不重排既有行」，ADR-0189 那一行是既有行，就地改它不在本批允许之内。评审也认了先例不统一、不算违规。从索引进 ADR-0189 的读者，Status 行末的前向指针已经指到补记。进 main 时协调方要统一，可一并补。
10. **多条候选都出价时出处段号重复，计划形成报错**（本分支之前就有）——按评审「不在本票修，另立票」，立票 [demo-chain-gaps/105](../../demo-chain-gaps/issues/105-route-plan-receives-cost-citations-of-unselected-candidates.md)（`needs-triage`，机制，S）。读码复核过：取数侧逐候选追加出处、出处不带候选维；`planRecord` 原样交 `FormInitialRoutePlan`，后者拒重复段号。取数侧单测 `TestTwoCandidatesPriceIndependentlyWithTheirOwnCards` 已显示两条段号 1 的出处。ADR-0189 补记越权风险点 6 补一句指向票 105，因为评价标识换形状之后，从标识上认不出出处属于哪条候选。票号 105 是自取的，见「评审修订后的验证」。
11. **选路比价的证据层级常量在适配器里；构造函数不校验依赖**（本分支之前就有）——不采纳，两点都留原样：
    - 证据层级挪进依赖、由 `routeCosts` 交入，要改 `cmd/parcel-dispatch/assemble.go` 里 `routeCosts` 的既有行。那是跨票热点，本批只追加。适配器常量的注释已点名与 `preAcceptanceTrialEvidence` 同改，将来按租户实例从登记册取证据层级时两处一起挪。
    - 构造函数拒缺依赖：`TestTheCostSideResolvesTheLineOfACandidateMintedByCatalogFolding` 有意只装 `Bases`，走到读逐段依据就停；在构造处校验，这类只证前半程的用例就得补一套替身。生产唯一的装配点 `routeCosts` 有真库用例覆盖，漏装 `Estimates` 时在那里第一次调用就会 panic，跑不过测试。

### 评审修订后的验证（实测，只记 `S`）

- `go build ./...` 通过；`go vet ./internal/networkrouting/... ./internal/parcelpricing/... ./cmd/parcel-dispatch/` 与 `gofmt -l cmd internal scripts` 都干净。
- 带 55432 DSN、`-p 1 -count=1`：`./internal/architecture/`、`./internal/networkrouting/...`、`./internal/parcelpricing/...`、`./cmd/parcel-dispatch/` 全 ok。新用例 `TestAnUnboundCostCardIsPricedThroughItsRegisteredLegZonesInProductionWiring` 用 `-v` 确认 PASS。
- 演示种子没改，没在一次性库上重灌；没跑全量，没重生成机制清点。
- 号：立票自取 `105`。协调方这批没给本票预分票号，评审要求「号接现有最大号」。本树最大是 104；查过在途分支 `claude-dcg95`–`dcg98`、`claude-psxm14` 的新票号（最高到 108，其中 106–108 已占），105 都没有占用。没有新取 AT 号、迁移号或 ADR 号。
- 没有被权限检查拦下的改动。
