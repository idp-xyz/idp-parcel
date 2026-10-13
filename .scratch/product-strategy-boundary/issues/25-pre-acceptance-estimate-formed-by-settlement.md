# 25 受理前估价由结算形成：采用登记、按提交版本的逐件 SELL 试算、估价留痕与续办触发（ADR-0179 的实施）

Category: enhancement
Status: in-progress——2026-10-13 16:5x 通道 2 在可提交点部分收口（派单 `task-dd4926b1` 时限内）：第 1–3 条、第 4 条上半、第 6 条前半（续办门与路由）与顺带四条落在 `mcp2-psb25`，代码 tip `3917951f`、清点 `ea62cb39`；第 4 条下半、第 5 条、第 6 条后半与第 7 条未做，接手要点见文末「完成记录（部分）」下「余下几条」。已落各笔可单独评审与重放：第 3 条的信封自 `3917951f` 起有订阅方，金额格挪格之前续办门对等待队列多驱一轮，无害。此前 in-progress——2026-10-13 15:5x 改派通道 2（派单 `task-dd4926b1` ← 通道 1；原 `task-d96ec0e6` → 通道 5 作废），分支 `mcp2-psb25`，基 `mcp5-psb25@b9486c5d`（其下 `b09b904c`），隔离 worktree `idp-parcel-mcp2-psb25`；第 1 条由通道 5 落于 `b9486c5d`。此前 in-progress——2026-10-13 通道 5 认领（派单 `task-d96ec0e6` ← 通道 1），分支 `mcp5-psb25`，基 `b09b904c`（= origin/main），隔离 worktree `idp-parcel-mcp5-psb25`。同批收 [routing-first-cut/19](../../routing-first-cut/issues/19-financial-control-undecided-leaves-no-trace-on-the-task.md) 增量复评（`task-40247234`）的四条非阻断，出处 rfc/19 票面 Comments：PS `ReleasePreAcceptanceControl` 装配缺口那一格补用例；`cmd/parcel-api` 三处编排注释与 `partycommercial` 一条用例注释改实；`NewPreAcceptanceControlAdapter` 注释只写本适配器对 nil 怎么答；`pre_acceptance_control.go` 常量块与 `ControlScopeNotAssembled` 注释不数「两半」、点名解析口与账户目录。此前 ready-for-agent——2026-10-12 20:4x 通道 1：[ADR-0179](../../../docs/adr/0179-pre-acceptance-control-amount-is-estimated-by-settlement-from-per-parcel-sell-trial-evaluations.md) 已接受（用户授权通道 1 代裁）。接受时改了两处，都不动下面的「地盘」，「做什么」第 4 条与完成判据已照改：决定二多一格停法（任一份合计的币种不是控制作用域的币种也答未形成，结算不换算），决定六的留痕多存每份试算评价的版本清单；PP `CONTEXT` 那两句随接受已改，从本票的文档一格划掉。此前 needs-info——2026-10-12 通道 3 随[票 06](./06-ps-acceptance-and-label-selection-judgment-methods.md)第 2 项分诊立（派单 `task-85d7350d` ← 通道 1），等用户决定是否接受 ADR-0179
Blocked by: [24](./24-plan-given-estimate-evaluations-use-shared-reading-completion.md)（ADR-0179 已于 2026-10-12 接受）
地盘（按 ADR-0179 现稿）：
- `internal/settlementaccounting`：估价端口、采用登记册与迁移、施加编排在要金额时先形成估价、估价留痕；`adapters/parcelpricing` 下新的受理前试算消费侧适配器。
- `internal/parcelpricing`：「按委托提交版本形成受理前试算」入口；读 PS 提交版本申报的消费侧适配器。
- `internal/parcelshipment`：按（租户、委托、提交版本）作答的逐件申报测量与收寄邮编读口；`adapters/settlementaccounting` 的控制请求去掉金额；`adapters/parcelpricing` 的 `estimation_amount.go` 与其用例退场；收结算登记信封的续办门。
- 装配：`cmd/parcel-dispatch`（`acceptanceFinancialControl` 与续办门）、`cmd/parcel-api`（只接释放半边的几处装配注释）、`cmd/parcel-settlement-register`（登记命令）。
- 文档：UC-SA-002 输入表「财务控制请求」一格；CONTEXT-MAP 补 SA → PP 受理前试算、PP → PS 提交版本申报读口两条关系；开发主线 PN-02 行带日期的补记；参数登记册 `PAR-COM-15` 行「〔ADR-0146 拆分〕」一句收短。
出处：票 06「第 2 项分诊」；ADR-0179 Decision 与 Consequences。

## 做什么（按 ADR-0179 现稿，分笔提交）

1. **PS 读口。** 按（租户、委托、提交版本）逐件答申报测量与收寄两端邮编，不经接受基线。ADR-0171 的申报测量与地址要素读口只经接受基线的声明成员走，受理前答「无」，不拓宽它们，另开一口。
2. **PP 受理前试算入口。** 按 ADR-0179 决定三解析在用 `SELL`·客户费用价卡（`PriceCardInForceResolver`，时点取控制回显的判断时点），逐件用第 1 条读口的申报造声明，交[票 24](./24-plan-given-estimate-evaluations-use-shared-reading-completion.md)的入口形成试算评价。结算币种由调用方交入。不入册、不交付。未配置、适用冲突、缺件各自成格，不挑卡、不顶值。
3. **SA 采用登记。** 新册：键（租户、结算账户），值（形态、计价范围），不设修订（照 ADR-0158）。迁移、`parcel-settlement-register` 子命令；登记与登记行同一事务发「参数已登记」类信封（ADR-0094 决定四）。首版形态只有「逐件 SELL 试算合计」一种，词表外的形态登记时拒。
4. **SA 形成估价。** 施加编排在策略要求冻结或信用校验时，先按采用行经估价端口要试算，按 ADR-0179 决定二的停法合计；同一金额用于本次各控制项。币种取控制作用域，最小币单位按 ISO 4217（PP 领域 `Currency.MinorUnitScale`）。册上没有这一户时答未形成，原因指名金额未配置；估价随控制结果留痕，含每份试算评价的版本清单（决定六）。
5. **PS 控制请求去掉金额。** `ControlAmountSource`、`EstimationAmountSource`、`EstimationBasisSource` 退场。`acceptanceFinancialControl` 里那句「Amounts 留空：估价缝属实例半边，见 acceptanceChainConsumer 的注释」随之消失（票 06「顺带」点名它指向一段不存在的注释）；`ControlAmountSource` 与 `NewPreAcceptanceControlAdapter` 头注里「接通前属实例半边」一类说法一并改成机制说法。
6. **金额那一格的续办。** 按 [routing-first-cut/19](../../routing-first-cut/issues/19-financial-control-undecided-leaves-no-trace-on-the-task.md)「分诊」拆出的「未配置」端口格与未决原因，把金额未配置挪进`等待运营登记`，与第 3 条的信封、PS 新续办门同一笔落地。19 若还没落，本票先按 19 的分诊拆那一格，19 接着用。
7. **文档。** 地盘里列的那几处一并改，开发主线与登记册只追加补记或收短，不改写旧单元格。

## 不做

- 不登任何租户的行，不预选形态；演示租户的采用行、售价卡绑分区目录、演示委托带邮编都归[票 26](./26-demo-tenant-adopts-the-pre-acceptance-estimate-form.md)。
- 不加第二种估价形态，不碰选卡走价格政策绑定那条路。
- 不改释放路径：释放按原关联认领，不问金额。

## 完成判据

- 空册：控制停在金额未配置；委托进`等待运营登记`，处理尝试落库，信封入账而不是 `ABANDONED`。登记采用行之后，信封重驱同一委托，越过金额格。带 DSN 的真库用例，写明 `-v` 下为 `PASS` 不是 `SKIP`。
- 绑燃油序列的卡、多件委托：冻结金额等于逐件试算合计；任一件缺测量或邮编、任一份评价没有合计、任一份合计的币种不是控制作用域的币种时不冻结，答未形成。
- 估价留痕可复算：同一输入、同一版本清单重算出同一合计。
- 释放路径的既有用例绿。
- 架构门禁绿；机制清点重生成并提交；带 DSN `go test -p 1 -count=1 ./...` 绿。

## 完成记录（部分）（通道 2 · 2026-10-13 · 派单 `task-dd4926b1` ← 通道 1 · 分支 `mcp2-psb25` 基 `mcp5-psb25@b9486c5d`（其下 `b09b904c`），代码 tip `3917951f`，清点 `ea62cb39`）

时限内在可提交点收口：第 1–3 条、第 4 条上半、第 6 条前半与顺带四条落地，第 4 条下半、第 5 条、第 6 条后半与第 7 条未做。推送方自审不算非作者评审；评审与重放归通道 1。证据层级一律 `S`。

### 各笔

| SHA | 内容 |
|---|---|
| `b7c9da5e` | 通道 5 认领（作者通道 5，此前） |
| `b9486c5d` | 第 1 条：PS 按（租户、委托、提交版本）逐件答申报测量与寄收两段邮编的读口，不经接受基线、不加列不加迁移。**作者通道 5**；本会话的作者验证覆盖它（见「验证」） |
| `8d720c41` | 改派接手：Status 行 |
| `5230046a` | 第 2 条：PP「按委托提交版本形成受理前试算」入口 `FormPreAcceptanceEstimatesHandler`、端口 `SubmissionDeclarationView`、PP 对 PS 的消费侧适配器 `adapters/parcelshipment`；缺项封闭码加 `MEASUREMENT`。采用 `ba61f682`（通道 5 未提交现场）的几处写在提交信里，用例先于读那份现场写成 |
| `b9f75c7b` | 机制清点在 `5230046a` 上重生成，一并覆盖 `b9486c5d` 欠的那份 |
| `87058cf9` | 顺带四条（逐条见下） |
| `328478ef` | 第 3 条：SA 受理前估价采用册（领域、端口、迁移 `0034`、Postgres 册与 Outbox 交接口）、登记编排、`parcel-settlement-register` 新命令 `pre-acceptance-estimate-adoption`；分区主体登记表加本口一行 |
| `0e7d22fe` | 机制清点在 `328478ef` 上重生成 |
| `7458bdb3` | 第 4 条上半：SA 估价形成 `FormPerParcelSellTrialSum`（决定二的停法、合计与决定六的留痕值）、试算端口 `PreAcceptanceTrialSource`、SA 对 PP 的消费侧适配器 `PreAcceptanceTrials`；两份棘轮基线各加理由行。不改任何既有行为 |
| `40dcb520` | 机制清点在 `7458bdb3` 上重生成 |
| `c5ab5fb9` | 本完成记录的首版 |
| `3917951f` | 第 6 条前半：PS 收「受理前估价采用已登记」的登记续办门（与另两份同一份代码、账本名各占一个），`cmd/parcel-dispatch` 构造、套同一份未决哨兵、路由表加一行（单起一段，`assemble.go` 只有新增） |
| `ea62cb39` | 机制清点在 `3917951f` 上重生成 |

### 完成判据

- ❌ 空册停在金额未配置、进`等待运营登记`、登记后信封重驱越过金额格。未做：要第 4 条下半（施加编排问采用册）、第 5 条（PS 不再交金额）与第 6 条后半（挪格）。已落的两段：一是信封那一端到续办门——SA 真交接适配器铸出的那一封经路由表投给 PS 新续办门、空队列入账定稿（`cmd/parcel-dispatch` 的 `TestARegisteredEstimateAdoptionReachesItsResumeGateOnTheProductionGraph`，带 DSN `-v` 为 `PASS`），续办门对该租户等待队列逐份再驱、账本记在自己名下（PS inbox 的 `TestAnEstimateAdoptionEnvelopeRedrivesTheTenantQueueUnderItsOwnLedgerName`）；二是采用行与「受理前估价采用已登记」信封同一事务，`cmd/parcel-settlement-register` 的 `TestPreAcceptanceEstimateAdoptionHandsOffItsEnvelopeInTheSameTransaction` 带 DSN `-v` 为 `PASS`（账户不在册上行与信封都不落、词表外形态事务前拒、首登一行一封、重放不翻倍、冲突不交）；「冲突不交」由应用层替身数调用证（`TestAnAdoptionRegistrationHandsOffByItsEffectAndAConflictHandsOffNothing`），理由同 rfc/19 评审回修第 3 条。
- ◑ 冻结金额等于逐件试算合计、四种停法。合计与停法在 SA 领域：`TestAPerParcelSellTrialSumAddsEveryParcelAndKeepsItsProvenance`、`TestAPerParcelSellTrialSumStopsInsteadOfFillingAGap`（无试算、无合计、币种不对、合计为零、一份为负、溢出）；缺测量 / 缺邮编 / 卡没绑分区目录在 PP 入口逐件点名（`TestPreAcceptanceEstimatesNameEachMissingCellWithoutFillingIt`），经 SA 适配器译成「缺件」格（`TestPricingCellsStaySeparateAndUndecidedIsUnavailable`）；SA 适配器经真入口与真补齐读数把两件 12.50 CNY 译成 1250、合计 2500（`TestPreAcceptanceTrialsCarryMinorUnitTotalsAndTheirVersionManifest`）。**还没冻结过任何一笔**：施加编排未接。绑燃油序列那一格由 psb/24 的入口证（`TestPlanGivenEstimateOnAFuelBoundCardCompletesThroughTheSharedReadingCompletion`），本票入口交的是同一个 handler，没有另写「燃油 + 多件」的纵向用例。
- ◑ 估价留痕可复算。留痕值 `PreAcceptanceEstimate` 带形态、计价范围、价卡版本引用、逐件评价标识与版本清单、合计，按留痕复算得同一合计（上面第一条领域用例）；PP 侧同一委托同一版本得同一组评价标识、换版本即不同（`TestPreAcceptanceEstimatesAreReproducibleForTheSameSubmissionVersion`）；SA 适配器带回的版本清单含价卡与分区目录。**留痕落库未做**（第 4 条下半）。
- ✅ 释放路径的既有用例绿；另补释放那一径四格的新用例（顺带①）。
- ✅ 架构门禁绿、机制清点重生成并提交、带 DSN `go test -p 1 -count=1 ./...` 绿——钉在本会话的 tip，不是本票完工的 tip，见「验证」。

### 顺带四条（rfc/19 增量复评 `task-40247234` 的非阻断，`87058cf9`）

1. **Spec ②**：`TestAReleaseWithoutAFormedScopeNamesItsGapAndClaimsNothing` 钉释放那一径四格——没接作用域源、解析口或账户目录没接各报 `CONTROL_SCOPE_NOT_ASSEMBLED`，账户没登记报 `CONTROL_SCOPE_NOT_CONFIGURED`，解析没采用结算政策报 `CONTROL_SCOPE_SETTLEMENT_POLICY_NOT_ADOPTED`，都是可重试错误、原冻结原样占着。M8（释放改回 `CONTROL_SCOPE_NOT_CONFIGURED`，`go test -overlay`，副本在本机 `/tmp/mcp2-m8/`）下装配缺口两格转红、基线绿。Standards ③（释放那份 switch 没有用例守）同一条用例收。
2. **Spec ③**：`cmd/parcel-api` 的 `buildRejectionOrchestration`、`buildDispositionOrchestration`、`buildWithdrawalOrchestration` 注释改为「适配器报装配缺口 `CONTROL_SCOPE_NOT_ASSEMBLED`、编排留补偿续办」；`partycommercial` 的 `TestARegisteredSettlementBasisFormsAThreeDimensionSelector` 注释改为停在 `CONTROL_SCOPE_SETTLEMENT_POLICY_NOT_ADOPTED`、与账户没登记分码。只动注释；「实例半边」的定性与给释放半边接作用域源归 owner 另票，未碰。
3. **Standards ①**：`NewPreAcceptanceControlAdapter` 注释只写本适配器对两个 nil 怎么答，不复述 `cmd/parcel-api` 的装配。第 5 条落地时 `Amounts` 那半句随 `ControlAmountSource` 一起走。
4. **Standards ②**：常量块与 `ControlScopeNotAssembled` 注释不数「两半」，点名解析口、账户目录；同包用例 `TestUnconfiguredSourcesStopAtNotFormedWithoutAskingTheProvider` 头注同一处一并改。

### 判断项与越权风险点

1. **越权风险点 · 待 owner 复核（SA）**：采用册对账户册设外键，账户没登记就不收采用行，登记口答用法格「先登记账户」（`ErrCatalogueTargetMissing`）。ADR-0179 决定五没写这一格；照 SA 读口登记册（迁移 `0023`）的先例办——不收会让采用行指向一户不存在的账户，而施加编排按作用域账户查采用行，那一行永远查不到。
2. **越权风险点 · 待 owner 复核（SA、PS）**：「受理前估价采用已登记」另开事件类型 `settlement-accounting.pre-acceptance-estimate-adoption.registered`，不复用「结算账户已登记」那一条；载荷同一份两键契约（`tenantId`、`registrationKind` 取 `PRE_ACCEPTANCE_ESTIMATE_ADOPTION`），ID 为 `outboxintent.FingerprintEventID("pre-acceptance-estimate-adoption", 租户, 账户)`，分区装租户、与另两口同队。分区登记表那一行照 rfc/19 的形写成待裁。复用旧类型能省一扇 PS 门，但事件名会说错登记的是什么。
3. 判断项：PS 申报的单位不在计价单位表内时，PP 适配器当这件没报测量，与没报同一个缺项码 `MEASUREMENT`——照旧执行器 `estimation_amount.go` 的处置，两者都停在未形成；恢复动作一个是客户补件、一个是产品认单位，今天分不开。
4. 判断项：评价合计的小数位多于币种的 ISO 4217 位数时，SA 适配器不截断、当这一份没有合计，估价答未形成。旧执行器在同一格报 `ErrUntranslatableAnswer`（编程错误）；这里降为业务停，因为位数由价卡的取整策略定，不是适配器译坏了。
5. 判断项：PP 入口的试算对象引用由委托、提交版本与件带长度前缀派生（取自 `ba61f682`），不直接用声明包裹标识——后者在委托之间未必唯一，同一标识的两件会撞出同一评价标识。
6. 判断项：选卡未配置、适用冲突、解析口读不回三格都不读客户申报（`TestPreAcceptanceEstimatesStopAtEachCardCellWithoutReadingTheDeclaration`）。

### 余下几条（接手要点；与之相关的既有代码本会话一行没改）

- **第 4 条下半**：`ApplyPreAcceptanceControlHandler` 加两件依赖——`ports.PreAcceptanceEstimateAdoptionView`（`328478ef`）与 `ports.PreAcceptanceTrialSource`（`7458bdb3`）；命令去掉 `AmountMinor`、加委托与提交版本两个原样引用。策略要求控制时按（租户、作用域账户）查采用行：没有行答新的未形成原因（金额未配置）；有行按（采用行计价范围、作用域币种、`AsOf.At()`）要试算，用 `FormPerParcelSellTrialSum` 合计，同一金额用于各控制项。`continuationFor` 摘要里的金额那一格换成委托与版本。估价留痕随冻结 / 暴露落库（决定六）：给两本账加列，还是另开一张以控制请求身份为键、与冻结同事务的留痕表，要定（倾向后者，重放按请求身份吞）。既有 SA 应用层、PS 适配器、cmd 用例里交 `AmountMinor` 的都要跟。新原因先在 PS `notFormedOutcomeFor` 落`未形成`，第 6 条再挪。两份棘轮基线里本票加的三行随之剪掉。
- **第 5 条**：PS `ControlAmountSource`、`EstimationAmountSource`、`EstimationBasisSource` 退场（`estimation_amount.go` 与其用例删；它的造单夹具已搬进 PP 适配器用例），适配器把委托与提交版本交给 SA 命令。`cmd/parcel-dispatch` 的 `acceptanceFinancialControl` 装 SA 采用册读口与 `PreAcceptanceTrials`：后者要 PP `FormPreAcceptanceEstimatesHandler`（在用价卡解析口、PP `adapters/parcelshipment` 包 PS 申报读口、plan-given 入口的四只补齐口、装配声明的证据层级）。那句「Amounts 留空……」与头注里「接通前属实例半边」随之消失。
- **第 6 条后半**：续办门与路由已落（`3917951f`）。余下的是挪格——PS `notFormedOutcomeFor` 把第 4 条下半新加的「金额未配置」原因落`未配置`（ADR-0094 决定四：触发已在，可以同笔挪）——和纵向真库用例，照 `TestAScopeWaitIsResumedByTheSettlementAccountRegistrationEnvelope`：空册 → `等待运营登记`、处理尝试落库、提交信封入账；登记采用行 → 信封重驱同一委托、越过金额格。
- **第 7 条**：地盘里列的文档，随第 4–6 条落。

### 验证（作者那层，go1.26.8，证据层级 `S`）

除另注的一条外钉在 `40dcb520`（隔离 worktree `idp-parcel-mcp2-psb25`，工作树只多本票面这一份未提交改动）：

- 分支自 `b09b904c` 起新增或改过的 36 个 `.go`，按入库字节（`git cat-file blob HEAD:<path>`）过 toolchain `gofmt -l` 为空、无 CR、无 BOM；迁移 `0034` 无 CR。
- `go build ./...`、`go vet ./...` 全仓退 0。
- 带 `IDP_PARCEL_POSTGRES_DSN=postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable` 跑 `go test -count=1 -p 1 ./...`：139 ok / 0 FAIL / 14 无测试文件 / 0 cached，退 0（16:42:01→16:44:14）。动了迁移，反向依赖实际就是全部真库包，所以直接跑了全量，不另列包组。第 1 条（`b9486c5d`，作者通道 5）的 PS domain / ports / adapters/postgres 在其中。
- `-v` 单跑：`TestASubmittedRequestAnswersItsVersionDeclarationFromTheSnapshot`、`TestAnInvisibleSubmissionVersionDeclarationAnswersNotFound`（第 1 条）、`TestAPreAcceptanceEstimateAdoptionIsAnImmutableRowOnARegisteredAccount`、`TestAnAdoptionRegisteredIntentCarriesExactlyTheContract`、`TestAnAdoptionRegisteredIntentRefusesToRunOutsideATransaction`、`TestPreAcceptanceEstimateAdoptionHandsOffItsEnvelopeInTheSameTransaction` 全为 `PASS`，无 `SKIP`。
- 机制清点在 `40dcb520` 上重跑生成器，`git status --porcelain -- docs/product/MECHANISM-INVENTORY.md` 为空。
- 变异：M8 见「顺带四条」第 1 条。
- `3917951f` 那一笔另验（钉 `ea62cb39`，16:50–16:51）：改过的 4 个 `.go` 按入库字节 gofmt 空、无 CR；全仓 build、vet 退 0；`psinbox` 的反向依赖经 `go list -test` 反查为 7 个包（`cmd/parcel-api`、`cmd/parcel-dispatch`、PS `adapters/inbox`、`labelfinal`、`nodeoperations`、`transportfulfillment`、VE `adapters/inbox`），连同 `./internal/architecture/...` 带 DSN `-p 1 -count=1` 全 ok；`-v` 单跑上面判据第一条点名的两条为 `PASS`；清点零差。改动前后两次之间分支上只多了这一笔与它的清点，所以全量那一组数仍是 `40dcb520` 的，没有在 `ea62cb39` 上重跑全量。
- 现场一格：头一次全量跑到 16:39 前后中断，WSL 实例重启（16:40:50 时 `uptime` 为 1 分钟），`/tmp` 清空、门禁库容器 `idp-parcel-postgres-gate` 停在 `Exited (0)`；本会话 `docker start` 拉起它后重跑，上面的数出自重跑。重启的来由本会话查不到。
