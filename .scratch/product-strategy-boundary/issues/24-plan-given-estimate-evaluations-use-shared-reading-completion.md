# 24 给定价卡的试算走共用补齐读数：parcel-pricing 应用层开一个按调用方给的价卡形成试算评价的入口

Category: enhancement
Status: resolved——2026-10-13 通道 1：本票七笔纯快进进 main（至 `a9eb1a75`，SHA 与分支相同），进 main 记录与非作者评审见文末。此前完工，待评审与重放——2026-10-13 12:5x 通道 2 按派单 `task-c8009875` 收评审非阻断四条（`9b6e79fd`、`2826ad94`、`a9eb1a75`）；2026-10-12 22:5x 通道 2：代码 tip `b41726b6`、清点 `7b11f0a0`（分支 `mcp2-psb24`、基 `1e02d2ea`），完成记录在文末，其中五条判断项待 owner 复核。此前 in-progress——2026-10-12 22:2x 通道 2 按通道 1 派单 `task-705a8770` 认领；隔离树 `/home/tops/workspace/idp-parcel-mcp2-psb24`，分支 `mcp2-psb24` 基远端 main `1e02d2ea`（22:21 `ls-remote` 实测，派单方取证），另起、不接任何半成品。此前 ready-for-agent——2026-10-12 通道 3 随[票 06](./06-ps-acceptance-and-label-selection-judgment-methods.md)第 2 项分诊立（派单 `task-85d7350d` ← 通道 1）。不等 [ADR-0179](../../../docs/adr/0179-pre-acceptance-control-amount-is-estimated-by-settlement-from-per-parcel-sell-trial-evaluations.md)：那篇正文与 Alternatives 里的每一种落法，都要一份对已知价卡走共用补齐读数的试算
Blocked by: 无
地盘：`internal/parcelpricing/application`（新入口与其用例）。`readingCompletion` 与 `estimateInput` 已在本包，复用、不改行为。不碰 `internal/parcelshipment`、`internal/settlementaccounting`、`cmd/`。
出处：票 06「第 2 项分诊」；[ADR-0152](../../../docs/adr/0152-operator-estimate-is-a-pricing-use-case-computed-not-recorded-and-never-handed-to-settlement.md) 决定二、三、五。

## 现象（读码，钉 main `ff3b01fa`）

- PS 的 `EstimationAmountSource` 直接调纯函数 `ppdomain.EvaluatePricing`，输入里不带序列取值与目录读数。卡上绑了序列或分区目录时，纯函数落 `PENDING`（缺序列取值即 `REFERENCE_SERIES_UNRESOLVED`），评价没有合计，金额形不成。演示售价卡 `SYN-PLAN-CN-SG-01` 绑着燃油序列 `SYN-SERIES-FUEL-01`。
- 本包里走 `readingCompletion` 的是正式评价（`EvaluatePricingHandler`，入册且无条件交付）与运营试算（`FormEstimateEvaluationsHandler`，按方向、范围、时点取全部适用价卡，证据一律 `S`）。前者会把结果交给结算；后者不收「就用这一张卡」，也不收调用方声明的证据层级。

## 做什么

- 开一个入口：调用方交一张已解析的价卡版本、一组试算声明与证据层级。每件声明是试算对象引用、实重、尺寸（可缺）、分区或邮编路线、计价基准时点；结算币种可缺。入口逐件按 ADR-0152 决定五的给法造输入快照（复用 `estimateInput`，不另写一份），走 `readingCompletion`，再由纯函数形成评价，逐件交回评价或缺项。
- 结构上不入册、不交付：依赖里没有评价库写口，也没有 `EvaluationHandoff`（ADR-0152 决定二的手法）。
- 证据层级由调用方声明，不给默认。受理前估价的输入是客户申报，不是假设包裹，所以 UC-PP-001 那一格「一律 `S`」不扩用到这里。
- 评价标识由调用方给的试算对象引用确定性派生：同一声明、同一张卡得同一标识，可比对可复算。
- 选卡不在本入口，卡由调用方给。ADR-0179 决定三若接受，调用方经 `PriceCardInForceResolver` 解析；若改走价格政策的定价方案绑定，调用方按引用装载。两种都用这一个入口。

## 不做

- 不改 `FormEstimateEvaluationsHandler`、`EvaluatePricingHandler` 的行为与答案代数。
- 不接消费方。PS 的 `EstimationAmountSource` 改用本入口，或结算的估价端口调用本入口，随[票 25](./25-pre-acceptance-estimate-formed-by-settlement.md)按 ADR-0179 的结论做。
- 不定估价形态、选卡口径与计价范围来路，那是 ADR-0179 的事。

## 完成判据

- 绑燃油序列的价卡：序列在用且基准时点有期次时，评价 `COMPLETED`，合计含燃油项；序列没登记或没复核时落 `PENDING`，解释里带补齐说明，说明与正式评价同一份。
- 绑分区目录的价卡：给邮编路线时分区由目录解析；只给分区、不给路线时，按 ADR-0152 决定五逐件答缺项。
- 同一声明同一张卡调两次，得同一评价标识与同一结果；依赖里没有写口与交付口（构造函数签名或编译期断言可证）。
- `go build ./...`、`go vet ./...` 退 0；本包用例绿；新写的 `.go` 过一遍 `gofmt -w`。

## 完成记录（2026-10-12 通道 2）

通道 1 派单 `task-705a8770`。分支 `mcp2-psb24`、基 `1e02d2ea`，未 rebase。证据层级一律 `S`：合成夹具加替身登记册，没有任何租户实例数据。

各笔：

- `164005d7` 票面认领。
- `b41726b6` parcel-pricing application：新入口 `FormPlanGivenEstimateEvaluationsHandler`（`form_plan_given_estimate_evaluations.go`）与其用例。调用方交 `FormPlanGivenEstimateEvaluationsCommand`：租户、一张已解析的价卡版本、证据层级、一组 `PlanGivenEstimateDeclaration`。每件声明转述成 `FormEstimateEvaluationsCommand`（计价范围与价格方向取卡自己的），复用 `estimateInput` 造输入、复用它的受理门 `accepted`；评价标识由 `planGivenEstimateIdentity` 在 `estimateIdentity` 外层拼上调用方的试算对象引用派生；补齐读数走 `readingCompletion.complete`，纯函数 `domain.EvaluatePricing` 形成评价。答复三格：`FORMED`（逐件 `PlanGivenEstimate`，答案与缺项码沿用 `EstimateCandidateAnswer` 与 `EstimateMissing*`）、`NOT_ACCEPTED`、`UNDECIDED`。`reading_completion.go` 与 `EstimateCandidateAnswer` 的头注补上新入口，零行为。
- `7b11f0a0` 机制清点重生成（干净检出钉 `b41726b6`）：parcelpricing 生产 126→127、测试 113→114、应用编排 15→16，端口、端点、迁移零差。
- 本笔：完成记录与 Status。

`FormEstimateEvaluationsHandler`、`EvaluatePricingHandler`、`readingCompletion`、`estimateInput`、`estimateIdentity` 的代码一行未改，`git diff 1e02d2ea b41726b6` 在 `form_estimate_evaluations.go` 与 `reading_completion.go` 上只有两处头注。`internal/parcelshipment`、`internal/settlementaccounting`、`cmd/`、PP 的 `domain/` 与 `ports/` 未碰。

### 判据逐条

- ✅ 绑燃油序列的卡，序列在用且基准时点有期次时 `COMPLETED`、合计含燃油项：`TestPlanGivenEstimateOnAFuelBoundCardCompletesThroughTheSharedReadingCompletion`。卡绑 `SYN-SERIES-FUEL-01`，燃油附加费按基础运费的当期百分比计收；1 千克一件合计 12 USD（基础运费 10、燃油 2），燃油项在费用行里。在用版本按形成时刻问、期次按计价基准时点解，`SYN-SERIES-FUEL-01@v1` 冻结进版本清单。
- ✅ 序列没登记或没复核时 `PENDING`，解释里带补齐说明，说明与正式评价同一份：`TestPlanGivenEstimateLeavesAnUnresolvedFuelSeriesPendingWithTheFormalEvaluationsNote` 两格（`SeriesHasNoRegisteredVersion`、`SeriesHasNoApprovedVersion`），问题码 `REFERENCE_SERIES_UNRESOLVED`。同一组替身、同一时钟下，`EvaluatePricingHandler` 交回的 `SeriesResolutionNotes()` 那一条逐字出现在试算评价的 `Explanation()` 里。
- ✅ 绑分区目录的卡，给邮编路线时分区由目录解析，只给分区时逐件答缺项：`TestPlanGivenEstimateOnAZoneBoundCardAnswersEachDeclarationByItsOwnRoute`。同一次调用两件：给路线的那件 `COMPLETED`，目录按声明的路线解了一次；只给分区的那件 `INPUT_INCOMPLETE`、`Missing` 为 `POSTAL_ROUTE`，没问登记册。反向用例 `TestPlanGivenEstimateOnAnUnboundCardNamesAMissingZone`：没绑目录的卡缺分区，点名 `ZONE`。
- ✅ 同一声明同一张卡调两次，得同一评价标识与同一结果：`TestPlanGivenEstimateIdentityIsDeterministicInTheDeclarationAndTheCard`。两次的标识与语义摘要相同；评价对象是 `SubjectEstimate`，引用就是调用方给的那个；换引用、换实重、换一张卡，标识各不相同。
- ✅ 依赖里没有评价库写口与 `EvaluationHandoff`：`FormPlanGivenEstimateEvaluationsDeps` 只有时钟与补齐读数两对，构造函数只收这份依赖。`TestPlanGivenEstimateDependenciesCarryNoEvaluationStoreOrHandoff` 反射逐字段核：没有 `ports.EvaluationStore`、`ports.EvaluationHandoff`，也没有带它们方法集的接口。判据原句「依赖里没有写口与交付口」只在照「做什么」把写口读作评价库写口时成立：补齐读数那两对里，`SeriesVersions`、`Catalogues` 的接口型（`ports.ReferenceSeriesRegister`、`ports.ReferenceCatalogueRegister`）带 `Register`。
- ✅ `go build ./...`、`go vet ./...` 退 0；本包用例绿；新写的 `.go` 过了 `gofmt -w`。见下「验证」。

「做什么」里判据没单列的几格：

- ✅ 证据层级由调用方声明、不给默认：`TestPlanGivenEstimateCarriesTheEvidenceTheCallerDeclared`（`S`、`R`、`P` 原样落在评价上）；没声明或不在封闭集里答 `NOT_ACCEPTED`（`TestPlanGivenEstimateRefusesAMalformedCommandBeforeAskingAnyRegister`，该用例先证未改动的命令本身受理，再逐格改坏）。
- ✅ 选卡不在本入口：依赖里没有价卡装载口，卡随命令交进来。
- ✅ 复用 `estimateInput` 与 `readingCompletion`、不另写一份：见「各笔」；去掉补齐读数那一步，燃油与分区两条判据当场红（见「验证」的变异探针）。

### 判断项（越权风险点 · 待 owner 复核）

1. **一次调用里试算对象引用重名，答未受理。** 票面没写。依据是 PP `CONTEXT`「评价对象」词条：试算对象「只在该次试算内有身份」，两件同名就分不清哪份评价是哪一件的。备选是允许重名、逐件照算，同一声明会得同一标识。
2. **一件声明都没有，答未受理**，不答一个空的「已形成」。备选是交回空列表，那样消费方合计出零，要靠它自己再判一次。
3. **评价标识不含证据层级。** 票面要「同一声明、同一张卡得同一标识」，证据层级不在声明里；评价的语义摘要也不含它（`hashPricingEvaluation`），与正式评价按标识判重放的口径一致。后果：同一声明同一张卡分别以 `S` 与 `P` 调用，得同一标识、同一摘要、不同证据层级。
4. **任一件补齐读数读不回，整份答 `UNDECIDED`，不交回其余几件**，与 UC-PP-001 那条编排的处置相同。
5. **补齐读数的两对照正式评价那样成对可选**，没有改成必备：两对都不装是合法装配，绑了序列或目录的卡如实落待判断。装配时漏接，表现为评价 `PENDING`，不是构造期错误；接线归票 25。

### 验证（作者那层，钉 `b41726b6`，隔离工作树）

- `go build ./...`、`go vet ./...` 退 0，工作树上与 `b41726b6` 的干净检出上各一次；`gofmt -l` 零行（用 toolchain go1.26.8 自带的 gofmt）；改动与新增的 `.go` 无 CR、无 BOM。
- 反向依赖由 `go list` 按生产导入与按测试导入反查：`internal/parcelpricing/adapters/` 下的 `accessidentity`、`http`、`settlementaccounting`，以及 `cmd/` 下的 `parcel-api`、`parcel-dispatch`、`parcel-pricing-feed`、`parcel-pricing-register`。
- 未设 DSN：`internal/parcelpricing/application`、上列三个 adapters 包与 `./internal/architecture/...`，`go test -count=1` 全部 ok，退 0。
- 含 PG：上列四个 `cmd/*` 包设 `IDP_PARCEL_POSTGRES_DSN`，`go test -count=1 -p 1 -v` 全部 ok，退 0；`--- PASS` 272、`--- SKIP` 0、`--- FAIL` 0。
- 本包新用例 `-v` 单跑全 PASS。
- 判别力（临时变异，跑完即还原、与备份字节比对一致，未提交）：把 `handler.completion.complete(ctx, request)` 换成 `err = nil`，燃油那条落 `PENDING`·`REFERENCE_SERIES_UNRESOLVED`（正是票面「现象」那一格），说明对照那条的解释为空，分区那条落 `ZONE_UNRESOLVED`，三条红。
- 机制清点在 `b41726b6` 的干净检出上重生成（`7b11f0a0`）。
- 作者不自评：非作者评审由派单方派，重放进 main 由派单方做。

给票 25 的一条取证（不是本票判据）：本包夹具上实测（钉 `b41726b6`），卡上有附加费规则时，缺尺寸的那一件落 `PENDING`·`PACKAGE_FEATURES_UNAVAILABLE`，用例里的燃油夹具因此带了尺寸。演示售价卡 `SYN-PLAN-CN-SG-01` 的重量策略取 `MAX` 且带体积系数（`scripts/demo-seeds/data/pricing/price-card-cn-sg.json`），缺尺寸时它落哪一格未测。

## Comments

**评审 ← 通道 4（非作者）· 钉 `6b6a0cc6` · 基线 `1e02d2ea` · 12:08。** Spec 阻断 0 / 非阻断 2；Standards 阻断 0 / 非阻断 3。全文在任务 `task-8bd4e155` 的 report_task。

- 无发现：四条判据都有用例且有判别力——变异去掉补齐读数红五个顶层用例，去重名检查、证据层级写死 `S`、标识去掉引用、零件受理各红；「不做」三条未越；判断项 1–5 与 ADR-0152 决定二、三、五及 PP `CONTEXT`「评价对象」无相抵。判断项 3 的后果是同一声明分别以 `S` 与 `P` 调用得同一标识、同一摘要，票 25 不要按标识去重。
- 本票收了四条（通道 2，派单 `task-c8009875`）：Spec ① 判断项 4 后半没有用例、变异 M3（未决分支带上已形成的几件）存活，`9b6e79fd` 补 `TestPlanGivenEstimateIsUndecidedAsAWholeWhenALaterDeclarationCannotRead`，M3 转红；Spec ② 完成记录依赖那格说多了，`a9eb1a75` 写窄；Standards ② `Handle` 头注缺「若」、Standards ③ 本票用例头注按票面位置指认判据且 UC-PP-001 引文与原文不符，`2826ad94` 改。
- 不收、待立票：Standards ① `reading_completion.go` 包头注称「只读：这几口全是解析与读取，没有写口」，而补齐读数的读口型 `ports.ReferenceSeriesRegister`、`ports.ReferenceCatalogueRegister` 带 `Register`；同包 `ApplicablePriceCards` 已为挡登记写口收窄成单方法，本票新 `Deps` 与正式评价、运营试算同形，都没收窄。先于本票，宜另立票收窄。Standards ③ 那种按位置指认判据的写法别的文件里还有，也不在本票收。

**增量复评 ← 通道 1（非作者）· `6b6a0cc6..a9eb1a75` · 13:0x。** 阻断 0 / 非阻断 0。替身 `inForceUnreachableAfterFirst` 头一问照答、此后读不回，新用例另核在用解析问了两次，场景确实成立；四处判据引文与判断项 4 的引文对票面逐字相同，UC-PP-001 那句与其原文相同；`Handle` 头注只加了「若」。

## 进 main 记录（2026-10-13 通道 1）

远端 main `1e02d2ea` 就是本分支的基，本票七笔纯快进进 main，SHA 与分支 `mcp2-psb24` 相同：`164005d7`、`b41726b6`、`7b11f0a0`、`6b6a0cc6`、`9b6e79fd`、`2826ad94`、`a9eb1a75`；本记录随后一笔。推送后拆隔离树 `idp-parcel-mcp2-psb24`，分支指针改名 `merged/mcp2-psb24`。验证在 `a9eb1a75`（detached 隔离树 `idp-parcel-verify-psb24`）：清点在该 tip 重生成零差；`go build ./...`、`go vet ./...` 退 0；带 DSN `go test -p 1 -count=1 ./...` 138 ok / 0 FAIL / 14 无测试 / 0 cached（13:00→13:03）；`-v` 探针 `TestTheOperatorReadChannelEstimatesForTheEnvelopeTenantAgainstARealDatabase` PASS，非 SKIP。评审：通道 4 `task-8bd4e155`（钉 `6b6a0cc6`）两轴无阻断；增量 `6b6a0cc6..a9eb1a75` 由通道 1 复评无阻断，见 Comments。本记录纯 `.md`，推送方自审。
