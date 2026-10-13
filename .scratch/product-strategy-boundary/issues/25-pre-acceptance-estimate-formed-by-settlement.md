# 25 受理前估价由结算形成：采用登记、按提交版本的逐件 SELL 试算、估价留痕与续办触发（ADR-0179 的实施）

Category: enhancement
Status: resolved——2026-10-13 19:2x 通道 1：本票二十二笔分两段纯快进进 main（至 `caab1e00`、至 `fb39860d`，SHA 与分支 `mcp2-psb25` 相同），进 main 记录与两段非作者评审见文末；两段评审的非阻断本票不再收，去向见 Comments 末段。此前完工，待评审与重放——2026-10-13 18:0x 通道 2 完工（派单 `task-f3d84381` ← 通道 1，接 `mcp2-psb25@caab1e00`）：余下四条落在 `mcp2-psb25`，代码 tip `e96345be`（最后一笔改行为的是 `bfc70773`，其后是文档 `236921b5` 与一条补判据二的用例）、清点 `26b3aac5`，见文末「完成记录」。`caab1e00` 之后的新笔待非作者评审，评审与重放归通道 1。此前 in-progress——2026-10-13 16:5x 通道 2 在可提交点部分收口（派单 `task-dd4926b1` 时限内）：第 1–3 条、第 4 条上半、第 6 条前半（续办门与路由）与顺带四条落在 `mcp2-psb25`，代码 tip `3917951f`、清点 `ea62cb39`；第 4 条下半、第 5 条、第 6 条后半与第 7 条未做，接手要点见文末「完成记录（部分）」下「余下几条」。已落各笔可单独评审与重放：第 3 条的信封自 `3917951f` 起有订阅方，金额格挪格之前续办门对等待队列多驱一轮，无害。此前 in-progress——2026-10-13 15:5x 改派通道 2（派单 `task-dd4926b1` ← 通道 1；原 `task-d96ec0e6` → 通道 5 作废），分支 `mcp2-psb25`，基 `mcp5-psb25@b9486c5d`（其下 `b09b904c`），隔离 worktree `idp-parcel-mcp2-psb25`；第 1 条由通道 5 落于 `b9486c5d`。此前 in-progress——2026-10-13 通道 5 认领（派单 `task-d96ec0e6` ← 通道 1），分支 `mcp5-psb25`，基 `b09b904c`（= origin/main），隔离 worktree `idp-parcel-mcp5-psb25`。同批收 [routing-first-cut/19](../../routing-first-cut/issues/19-financial-control-undecided-leaves-no-trace-on-the-task.md) 增量复评（`task-40247234`）的四条非阻断，出处 rfc/19 票面 Comments：PS `ReleasePreAcceptanceControl` 装配缺口那一格补用例；`cmd/parcel-api` 三处编排注释与 `partycommercial` 一条用例注释改实；`NewPreAcceptanceControlAdapter` 注释只写本适配器对 nil 怎么答；`pre_acceptance_control.go` 常量块与 `ControlScopeNotAssembled` 注释不数「两半」、点名解析口与账户目录。此前 ready-for-agent——2026-10-12 20:4x 通道 1：[ADR-0179](../../../docs/adr/0179-pre-acceptance-control-amount-is-estimated-by-settlement-from-per-parcel-sell-trial-evaluations.md) 已接受（用户授权通道 1 代裁）。接受时改了两处，都不动下面的「地盘」，「做什么」第 4 条与完成判据已照改：决定二多一格停法（任一份合计的币种不是控制作用域的币种也答未形成，结算不换算），决定六的留痕多存每份试算评价的版本清单；PP `CONTEXT` 那两句随接受已改，从本票的文档一格划掉。此前 needs-info——2026-10-12 通道 3 随[票 06](./06-ps-acceptance-and-label-selection-judgment-methods.md)第 2 项分诊立（派单 `task-85d7350d` ← 通道 1），等用户决定是否接受 ADR-0179
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

## 完成记录（通道 2 · 2026-10-13 · 两次派单 ← 通道 1：`task-dd4926b1` 做到 `caab1e00`，`task-f3d84381` 自 `caab1e00` 接着做 · 分支 `mcp2-psb25` 基 `mcp5-psb25@b9486c5d`（其下 `b09b904c`），代码 tip `e96345be`，清点 `26b3aac5`）

做什么七条与顺带四条全部落地。第一次派单在时限内收口于 `caab1e00`（第 1–3 条、第 4 条上半、第 6 条前半与顺带四条），通道 3 在评 `b09b904c..caab1e00`；第二次派单落余下四条，`caab1e00` 之后的新笔另评。推送方自审不算非作者评审；评审与重放归通道 1。证据层级一律 `S`。

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
| `caab1e00` | 本完成记录跟上第 6 条前半（第一次派单收口于此） |
| `2f4df44f` | 第 4 条下半之一：估价留痕册——迁移 `0035`、端口 `PreAcceptanceEstimateTrace` / `PreAcceptanceEstimateTraces`、Postgres 册（同键同内容答重放、异内容答冲突、不覆盖，比较在 SQL 里做）；类型可达性基线剪 `PreAcceptanceEstimate`。取舍见判断项 7 |
| `44cb67cf` | 第 4 条下半之二与第 5 条（一笔：命令去掉 `AmountMinor` 之后，PS 那一侧的金额源没有去处，分两笔要么留一个算了不交的中间态，要么有一笔编不过）：SA 施加编排按采用行要试算、合计、先留痕再逐项控制，同一金额用于各控制项；新原因十二格；结果加 `Estimate()`。PS 适配器 `ControlAmountSource` 退场、命令交委托与提交版本，`estimation_amount.go` 与其用例删。`cmd/parcel-dispatch` 接采用册、留痕册与 `preAcceptanceTrials`（PP 受理前试算入口一整条），「Amounts 留空……」那句与两处「控制金额源仍为 nil」改成机制说法；`cmd/parcel-api` 三处注释只剩 Scopes。两份棘轮基线恢复成 `b09b904c` 的内容 |
| `bfc70773` | 第 6 条后半：`SA-CONTROL_AMOUNT_NOT_CONFIGURED` 落`未配置`；新纵向用例 `TestAnAmountWaitIsResumedByTheEstimateAdoptionEnvelope`，作用域那条闭环的末段随之改；闭环夹具加估价采用续办门 |
| `ffbb3890` | 机制清点在 `bfc70773` 上重生成，覆盖 `2f4df44f`、`44cb67cf`、`bfc70773` |
| `236921b5` | 第 7 条：UC-SA-002 输入表、CONTEXT-MAP 两条关系、开发主线 PN-02 补记、`PAR-COM-15` 收短 |
| `e96345be` | 完成判据二的用例：燃油 + 分区卡、两件委托，施加编排、SA 适配器与 PP 受理前试算入口接成一条（只加一份 `_test.go`） |
| `26b3aac5` | 机制清点在 `e96345be` 上重生成（detached 检出） |

### 完成判据

- ✅ 空册停在金额未配置、进`等待运营登记`、处理尝试落库、信封入账而不是 `ABANDONED`；登记采用行之后信封重驱同一委托、越过金额格。`cmd/parcel-dispatch` 的 `TestAnAmountWaitIsResumedByTheEstimateAdoptionEnvelope`（带 DSN `-v` 为 `PASS`）：结算账户先登好、采用册空着——提交门入账，委托落`等待运营登记`，处理尝试带 `FINANCIAL_CONTROL_NOT_CONFIGURED` 与续办引用一行，两本账与留痕册一行不落；经真登记编排与真交接适配器登记采用行，恰好一封「受理前估价采用已登记」，估价采用续办门重驱同一委托，按登记的计价范围问到试算（试算替身答没有在用的售价卡 → `未形成`、整封回滚、账本无痕、处理尝试仍一行）。SA 施加编排、两本账、采用册与留痕册是生产实现、真库，替身只在控制策略与试算两口。作用域那条闭环 `TestAScopeWaitIsResumedByTheSettlementAccountRegistrationEnvelope` 越过作用域之后停在同一格、本封入账、处理尝试两行。变异 M1（金额未配置挪回`未形成`，`go test -overlay`，副本在本机 `/tmp/mcp2-m1/`）下两条转红、基线绿。第一次派单已落的两段照旧绿：一是信封那一端到续办门——SA 真交接适配器铸出的那一封经路由表投给 PS 新续办门、空队列入账定稿（`cmd/parcel-dispatch` 的 `TestARegisteredEstimateAdoptionReachesItsResumeGateOnTheProductionGraph`，带 DSN `-v` 为 `PASS`），续办门对该租户等待队列逐份再驱、账本记在自己名下（PS inbox 的 `TestAnEstimateAdoptionEnvelopeRedrivesTheTenantQueueUnderItsOwnLedgerName`）；二是采用行与「受理前估价采用已登记」信封同一事务，`cmd/parcel-settlement-register` 的 `TestPreAcceptanceEstimateAdoptionHandsOffItsEnvelopeInTheSameTransaction` 带 DSN `-v` 为 `PASS`（账户不在册上行与信封都不落、词表外形态事务前拒、首登一行一封、重放不翻倍、冲突不交）；「冲突不交」由应用层替身数调用证（`TestAnAdoptionRegistrationHandsOffByItsEffectAndAConflictHandsOffNothing`），理由同 rfc/19 评审回修第 3 条。
- ✅ 冻结金额等于逐件试算合计、四种停法。接成一条的是 `adapters/parcelpricing` 的 `TestAFuelBoundCardOnTwoParcelsFreezesTheSumOfTheirTrials`（`e96345be`）：绑燃油序列与分区目录的售价卡、两件 1 千克，计价真入口与真补齐读数算出每件 12.50 + 20% 燃油 = 15.00 CNY，SA 真施加编排冻结 3000，留痕逐件的版本清单含价卡、分区目录与燃油序列；同包 `TestAFuelBoundCardStopsBeforeFreezingWhenAnyParcelCannotBeEstimated` 证同一张卡下一件没报测量（`ESTIMATE_DECLARATION_INCOMPLETE`）、一件超出费率表即那一份没有合计（`ESTIMATE_TRIAL_WITHOUT_TOTAL`）、卡的币种不是作用域的（同样没有合计：卡上没有汇率序列时计价不出别的币种的合计）都不冻结、账本与留痕不写。全在内存，替身只在读数源与结算的几本账上。以下是各段自己的用例。合计与停法在 SA 领域：`TestAPerParcelSellTrialSumAddsEveryParcelAndKeepsItsProvenance`、`TestAPerParcelSellTrialSumStopsInsteadOfFillingAGap`（无试算、无合计、币种不对、合计为零、一份为负、溢出）；缺测量 / 缺邮编 / 卡没绑分区目录在 PP 入口逐件点名（`TestPreAcceptanceEstimatesNameEachMissingCellWithoutFillingIt`），经 SA 适配器译成「缺件」格（`TestPricingCellsStaySeparateAndUndecidedIsUnavailable`）；SA 适配器经真入口与真补齐读数把两件 12.50 CNY 译成 1250、合计 2500（`TestPreAcceptanceTrialsCarryMinorUnitTotalsAndTheirVersionManifest`）。施加编排上冻结的就是那个合计：`TestTheFrozenAmountIsTheEstimateFormedFromTheAdoptedRow`（两件 1250 + 800 → 冻结 2050；试算按采用行的计价范围、作用域币种、控制判断时点与委托版本原样去问），`TestACombinedPolicyOccupiesOneEstimateOnBothLedgers`（冻结与暴露同一金额，试算只问一次、留痕一份），`TestEveryEstimateCellStopsBeforeTouchingEitherLedger`（缺件、任一份无合计、币种不是作用域的、合计非正、一份都没有，连同读不回三格、选卡两格、不可见，各落一个原因，都在碰两本账之前停）；PS 适配器 `TestNotFormedReasonsLandOnTheCellOfTheirRecoveryAction` 逐格译回。没有的一格：真库上「燃油卡 + 多件 + 冻结」走到底的纵向——生产装配的 PP 那一串在 `cmd/parcel-dispatch` 整图构造的用例里接得起来，但真价卡、分区目录与带邮编的委托是演示租户的取值（psb/26），本票不造。
- ✅ 估价留痕可复算。`TestAPreAcceptanceEstimateTraceIsWrittenOnceAndReproducible`（带 DSN `-v` 为 `PASS`）：一行带形态、计价范围、价卡、逐件评价标识与版本清单、合计，按 `trials` 列逐件加回得同一合计；同一请求同一份再写是重放（落库时刻换了、判断时点带纳秒尾数也是），同合计异逐件与换合计都是冲突，不覆盖。编排侧：同一请求第二次带另一份估价停在`请求冲突`、不碰两本账（`TestTheSameRequestWithAnotherEstimateIsAConflictBeforeEitherLedger`）。PP 侧同一委托同一版本得同一组评价标识（第一次派单的 `TestPreAcceptanceEstimatesAreReproducibleForTheSameSubmissionVersion`）。
- ✅ 释放路径的既有用例绿；另补释放那一径四格的新用例（顺带①）。
- ✅ 架构门禁绿、机制清点重生成并提交（`26b3aac5`）、带 DSN `go test -p 1 -count=1 ./...` 绿——全量钉 `236921b5`，`e96345be` 只多一份 `_test.go`，另验，见「验证」。

### 顺带四条（rfc/19 增量复评 `task-40247234` 的非阻断，`87058cf9`）

1. **Spec ②**：`TestAReleaseWithoutAFormedScopeNamesItsGapAndClaimsNothing` 钉释放那一径四格——没接作用域源、解析口或账户目录没接各报 `CONTROL_SCOPE_NOT_ASSEMBLED`，账户没登记报 `CONTROL_SCOPE_NOT_CONFIGURED`，解析没采用结算政策报 `CONTROL_SCOPE_SETTLEMENT_POLICY_NOT_ADOPTED`，都是可重试错误、原冻结原样占着。M8（释放改回 `CONTROL_SCOPE_NOT_CONFIGURED`，`go test -overlay`，副本在本机 `/tmp/mcp2-m8/`）下装配缺口两格转红、基线绿。Standards ③（释放那份 switch 没有用例守）同一条用例收。
2. **Spec ③**：`cmd/parcel-api` 的 `buildRejectionOrchestration`、`buildDispositionOrchestration`、`buildWithdrawalOrchestration` 注释改为「适配器报装配缺口 `CONTROL_SCOPE_NOT_ASSEMBLED`、编排留补偿续办」；`partycommercial` 的 `TestARegisteredSettlementBasisFormsAThreeDimensionSelector` 注释改为停在 `CONTROL_SCOPE_SETTLEMENT_POLICY_NOT_ADOPTED`、与账户没登记分码。只动注释；「实例半边」的定性与给释放半边接作用域源归 owner 另票，未碰。
3. **Standards ①**：`NewPreAcceptanceControlAdapter` 注释只写本适配器对两个 nil 怎么答，不复述 `cmd/parcel-api` 的装配。第 5 条落地时 `Amounts` 那半句随 `ControlAmountSource` 一起走（`44cb67cf`，现在只剩 Scopes 那一个 nil）。
4. **Standards ②**：常量块与 `ControlScopeNotAssembled` 注释不数「两半」，点名解析口、账户目录；同包用例 `TestUnconfiguredSourcesStopAtNotFormedWithoutAskingTheProvider` 头注同一处一并改。

### 判断项与越权风险点

1. **越权风险点 · 待 owner 复核（SA）**：采用册对账户册设外键，账户没登记就不收采用行，登记口答用法格「先登记账户」（`ErrCatalogueTargetMissing`）。ADR-0179 决定五没写这一格；照 SA 读口登记册（迁移 `0023`）的先例办——不收会让采用行指向一户不存在的账户，而施加编排按作用域账户查采用行，那一行永远查不到。
2. **越权风险点 · 待 owner 复核（SA、PS）**：「受理前估价采用已登记」另开事件类型 `settlement-accounting.pre-acceptance-estimate-adoption.registered`，不复用「结算账户已登记」那一条；载荷同一份两键契约（`tenantId`、`registrationKind` 取 `PRE_ACCEPTANCE_ESTIMATE_ADOPTION`），ID 为 `outboxintent.FingerprintEventID("pre-acceptance-estimate-adoption", 租户, 账户)`，分区装租户、与另两口同队。分区登记表那一行照 rfc/19 的形写成待裁。复用旧类型能省一扇 PS 门，但事件名会说错登记的是什么。
3. 判断项：PS 申报的单位不在计价单位表内时，PP 适配器当这件没报测量，与没报同一个缺项码 `MEASUREMENT`——照旧执行器 `estimation_amount.go` 的处置，两者都停在未形成；恢复动作一个是客户补件、一个是产品认单位，今天分不开。
4. 判断项：评价合计的小数位多于币种的 ISO 4217 位数时，SA 适配器不截断、当这一份没有合计，估价答未形成。旧执行器在同一格报 `ErrUntranslatableAnswer`（编程错误）；这里降为业务停，因为位数由价卡的取整策略定，不是适配器译坏了。
5. 判断项：PP 入口的试算对象引用由委托、提交版本与件带长度前缀派生（取自 `ba61f682`），不直接用声明包裹标识——后者在委托之间未必唯一，同一标识的两件会撞出同一评价标识。
6. 判断项：选卡未配置、适用冲突、解析口读不回三格都不读客户申报（`TestPreAcceptanceEstimatesStopAtEachCardCellWithoutReadingTheDeclaration`）。
7. **越权风险点 · 待 owner 复核（SA）**：留痕落库另开一张表 `settlement_accounting.pre_acceptance_estimate_trace`（迁移 `0035`），不给冻结、信用暴露两本账加列。ADR-0179 决定六写的是「跟冻结与信用暴露的结果一起保存」，本票读作同一事务、另一张表，键与两本账同形（租户 + 结算作用域 + 控制请求标识）。理由：`业务限制`从不入两本账（迁移 `0001` 原话「`业务限制`从不入账本」，`FreezeLedger.Freeze` 与 `CreditExposureLedger.Expose` 对超额只交回结果不登记），加列的话受限那一次的估价无处可存；组合策略两项共用一个金额，加列要存两份；两本账的领域对象与重建规格都不用动。迁移只增不删，选错了要靠下一份迁移收，所以标在这里。
8. 判断项：留痕先于两本账写、与它们同一事务，同一请求身份带另一份估价答冲突 → `请求冲突`，与两本账「同一请求身份携带不同内容」同一条判据。先写的理由：一次落了库的冻结或暴露必有说得出金额来历的那一行——账期那一项停在`未配置`时事务照样提交，前一项的冻结已经落了。代价是同一请求在同一判断时点上估出了另一份（比如有人补登了一份在那个时点之前生效的价卡）时停在冲突，而不是悄悄换一个金额；那正是两本账对异金额的答复。
9. 判断项：估价那一口每格各一个未形成原因（`CONTROL_AMOUNT_NOT_CONFIGURED`、三个 `*_UNAVAILABLE`、选卡两格、不可见、缺件、四种合计停法），PS 侧只有金额未配置落`未配置`，其余落`未形成`：补价卡、补件或改卡都推得动它们，但没有一样会发「参数已登记」类信封来重驱这份委托。分开是为了续办引用与处理尝试里留下停在哪一格。
10. 判断项：估价在控制策略之后、两本账之前形成，`无控制`不形成估价——合同不要求控制时连客户的申报都不该读，与「控制策略先于余额」同一条顺序纪律。命令上的委托与提交版本是 PS 字面原样的字符串，与 `PreAcceptanceTrialQuery` 同形，SA 不为它们立值对象。
11. 判断项：`cmd/parcel-dispatch` 把受理前试算评价的证据层级装配成 `S`（`preAcceptanceTrialEvidence`），理由与 `evaluationRequestFormationEvidence` 同一条：本仓尚无租户，价卡登记册里只有合成实例。
12. 范围一句：CONTEXT-MAP 只补了地盘列的两条（SA → PP 受理前试算、PP → PS 提交版本申报）。「受理前估价采用已登记」那条 SA → PS 的交接与「结算账户已登记的交接」同形，没有单列。PS 应用层「三停续办引用两两不同」那条用例只改了用例里的原因串与格（`bfc70773`），应用层代码一行没动。

### 余下几条

无。第一次派单收口时写的接手要点原样留在下面，第二次派单照它办：第 4 条下半落在 `2f4df44f` 与 `44cb67cf`，第 5 条随 `44cb67cf`，第 6 条后半在 `bfc70773`，第 7 条在 `236921b5`；与要点不同的只有两处——留痕取了另开一张表（判断项 7），第 4 条下半与第 5 条并成一笔（理由见「各笔」那一行）。

- **第 4 条下半**：`ApplyPreAcceptanceControlHandler` 加两件依赖——`ports.PreAcceptanceEstimateAdoptionView`（`328478ef`）与 `ports.PreAcceptanceTrialSource`（`7458bdb3`）；命令去掉 `AmountMinor`、加委托与提交版本两个原样引用。策略要求控制时按（租户、作用域账户）查采用行：没有行答新的未形成原因（金额未配置）；有行按（采用行计价范围、作用域币种、`AsOf.At()`）要试算，用 `FormPerParcelSellTrialSum` 合计，同一金额用于各控制项。`continuationFor` 摘要里的金额那一格换成委托与版本。估价留痕随冻结 / 暴露落库（决定六）：给两本账加列，还是另开一张以控制请求身份为键、与冻结同事务的留痕表，要定（倾向后者，重放按请求身份吞）。既有 SA 应用层、PS 适配器、cmd 用例里交 `AmountMinor` 的都要跟。新原因先在 PS `notFormedOutcomeFor` 落`未形成`，第 6 条再挪。两份棘轮基线里本票加的三行随之剪掉。
- **第 5 条**：PS `ControlAmountSource`、`EstimationAmountSource`、`EstimationBasisSource` 退场（`estimation_amount.go` 与其用例删；它的造单夹具已搬进 PP 适配器用例），适配器把委托与提交版本交给 SA 命令。`cmd/parcel-dispatch` 的 `acceptanceFinancialControl` 装 SA 采用册读口与 `PreAcceptanceTrials`：后者要 PP `FormPreAcceptanceEstimatesHandler`（在用价卡解析口、PP `adapters/parcelshipment` 包 PS 申报读口、plan-given 入口的四只补齐口、装配声明的证据层级）。那句「Amounts 留空……」与头注里「接通前属实例半边」随之消失。
- **第 6 条后半**：续办门与路由已落（`3917951f`）。余下的是挪格——PS `notFormedOutcomeFor` 把第 4 条下半新加的「金额未配置」原因落`未配置`（ADR-0094 决定四：触发已在，可以同笔挪）——和纵向真库用例，照 `TestAScopeWaitIsResumedByTheSettlementAccountRegistrationEnvelope`：空册 → `等待运营登记`、处理尝试落库、提交信封入账；登记采用行 → 信封重驱同一委托、越过金额格。
- **第 7 条**：地盘里列的文档，随第 4–6 条落。

### 验证（第二次派单，作者那层，go1.26.8，证据层级 `S`）

钉 `236921b5`（隔离 worktree `idp-parcel-mcp2-psb25`，`git status --porcelain --untracked-files=all` 为空），17:49–17:52：

- 分支自 `b09b904c` 起新增或改过的 52 个 `.go`，按入库字节（`git cat-file blob HEAD:<path>`）过 toolchain `gofmt -l` 为空、无 CR、无 BOM；迁移 `0034`、`0035` 无 CR。
- `go build ./...`、`go vet ./...` 全仓退 0。
- 带 DSN 跑 `go test -count=1 -p 1 ./...`：139 ok / 0 FAIL / 14 无测试文件 / 0 cached，退 0（17:49:34→17:51:49）。动了迁移，反向依赖就是全部真库包，所以跑全量，不另列包组。各笔提交前另跑过的局部：`2f4df44f` 前是留痕与采用两条真库用例、`./internal/architecture/...`、`./migrations/...`、`./internal/platform/migrate/...`；`44cb67cf` 前是 SA 全部包、PS 两个适配器与应用层、`cmd/parcel-api`、`cmd/parcel-dispatch`、`cmd/parcel-settlement-register` 与架构门禁（带 DSN `-p 1`）；`bfc70773` 前是两条纵向用例与两条译格用例的 `-v` 单跑。都 ok，数不另记。
- `-v` 单跑：`TestAPreAcceptanceEstimateTraceIsWrittenOnceAndReproducible`、`TestAnAmountWaitIsResumedByTheEstimateAdoptionEnvelope`、`TestAScopeWaitIsResumedByTheSettlementAccountRegistrationEnvelope`、`TestARegisteredEstimateAdoptionReachesItsResumeGateOnTheProductionGraph` 全为 `PASS`，无 `SKIP`。
- 机制清点在 `236921b5` 上重跑生成器，`git status --porcelain -- docs/product/MECHANISM-INVENTORY.md` 为空（`ffbb3890` 生成于 `bfc70773`，其后只有文档一笔）。
- 变异：M1 见完成判据第一条。
- `e96345be` 另验（钉 `26b3aac5`）：自 `236921b5` 起动过的 `.go` / `.sql` 只有 `internal/settlementaccounting/adapters/parcelpricing/pre_acceptance_control_estimate_test.go` 一份，按入库字节 gofmt 空、无 CR；全仓 build、vet 退 0；该包与 `./internal/architecture/...` `-count=1` 全 ok，新两条 `-v` 为 `PASS`。纯用例、不碰真库，没有在它上面重跑全量。清点 `26b3aac5` 在 `e96345be` 的 detached 检出上生成。

### 验证（第一次派单，作者那层，go1.26.8，证据层级 `S`）

除另注的一条外钉在 `40dcb520`（隔离 worktree `idp-parcel-mcp2-psb25`，工作树只多本票面这一份未提交改动）：

- 分支自 `b09b904c` 起新增或改过的 36 个 `.go`，按入库字节（`git cat-file blob HEAD:<path>`）过 toolchain `gofmt -l` 为空、无 CR、无 BOM；迁移 `0034` 无 CR。
- `go build ./...`、`go vet ./...` 全仓退 0。
- 带 `IDP_PARCEL_POSTGRES_DSN=postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable` 跑 `go test -count=1 -p 1 ./...`：139 ok / 0 FAIL / 14 无测试文件 / 0 cached，退 0（16:42:01→16:44:14）。动了迁移，反向依赖实际就是全部真库包，所以直接跑了全量，不另列包组。第 1 条（`b9486c5d`，作者通道 5）的 PS domain / ports / adapters/postgres 在其中。
- `-v` 单跑：`TestASubmittedRequestAnswersItsVersionDeclarationFromTheSnapshot`、`TestAnInvisibleSubmissionVersionDeclarationAnswersNotFound`（第 1 条）、`TestAPreAcceptanceEstimateAdoptionIsAnImmutableRowOnARegisteredAccount`、`TestAnAdoptionRegisteredIntentCarriesExactlyTheContract`、`TestAnAdoptionRegisteredIntentRefusesToRunOutsideATransaction`、`TestPreAcceptanceEstimateAdoptionHandsOffItsEnvelopeInTheSameTransaction` 全为 `PASS`，无 `SKIP`。
- 机制清点在 `40dcb520` 上重跑生成器，`git status --porcelain -- docs/product/MECHANISM-INVENTORY.md` 为空。
- 变异：M8 见「顺带四条」第 1 条。
- `3917951f` 那一笔另验（钉 `ea62cb39`，16:50–16:51）：改过的 4 个 `.go` 按入库字节 gofmt 空、无 CR；全仓 build、vet 退 0；`psinbox` 的反向依赖经 `go list -test` 反查为 7 个包（`cmd/parcel-api`、`cmd/parcel-dispatch`、PS `adapters/inbox`、`labelfinal`、`nodeoperations`、`transportfulfillment`、VE `adapters/inbox`），连同 `./internal/architecture/...` 带 DSN `-p 1 -count=1` 全 ok；`-v` 单跑上面判据第一条点名的两条为 `PASS`；清点零差。改动前后两次之间分支上只多了这一笔与它的清点，所以全量那一组数仍是 `40dcb520` 的，没有在 `ea62cb39` 上重跑全量。
- 现场一格：头一次全量跑到 16:39 前后中断，WSL 实例重启（16:40:50 时 `uptime` 为 1 分钟），`/tmp` 清空、门禁库容器 `idp-parcel-postgres-gate` 停在 `Exited (0)`；本会话 `docker start` 拉起它后重跑，上面的数出自重跑。重启的来由本会话查不到。

## Comments

**评审 ← 通道 3（非作者）· 钉 `caab1e00` · 基线 `b09b904c` · 17:17。** Standards 阻断 0 / 非阻断 2；Spec 阻断 0 / 非阻断 1。全文在任务 `task-c7cfb347` 的 report_task。

- Standards ①：PP `adapters/parcelshipment` 的 `measurementOf` 先 `strings.ToUpper` 再认单位，正式 SELL 输入解析 `adapters/transportfulfillment/pricing_input.go` 的 `weightUnit` / `lengthUnit` 只认大写原词。ADR-0179 决定三要估价与随后的客户费用同口径；判断项 3 只披露了表外单位，没披露这一差。
- Standards ②：SA `FormPerParcelSellTrialSum` 对单份为负、int64 溢出也答 `PreAcceptanceEstimateNotPositive`；决定二与旧执行器只判「合计不是正数」，单份负而合计正时多停一格，溢出记成「非正」名不副实。
- Spec ③：PS `ShipmentRequest.SubmissionVersionDeclarationFor` 对已换代的提交版本照答，ADR-0179 决定四写「按委托当前提交版本读」。第 4 条下半接线前要么举证链上游已守当前版本，要么补「非当前版本答不可见」，要么交 PS owner。
- 无发现：红线、所有权、单一权威、注释；第 1–3 条、第 4 条上半、第 6 条前半与顺带四条；采用册外键、另开事件类型与决定五、ADR-0158、ADR-0094 决定四无冲突（只判冲突，不替 owner 裁）。

**评审 ← 通道 3（非作者）· 钉 `fb39860d` · 基线 `caab1e00` · 19:13。** Standards 阻断 0 / 非阻断 3；Spec 阻断 0 / 非阻断 1。全文在任务 `task-386c6296` 的 report_task（原卡 `task-cc550037` 18:39 开工，该会话存档最后写入 18:40:28、未交，18:57 改派）。

- Standards 1（上段③）：未处置，票面未记。PS `adapters/settlementaccounting` 的 `applyCommandFor` 原样交请求里的提交版本，`SubmissionVersionDeclarationFor` 仍各版都认，与 ADR-0179 决定二、四「委托当前提交版本」不合。续办经 `ListWaitingOnOperatorRegistration` 取 `current_submission_version_id`，守住了；提交信封重投按信封里的版本，而 `FormNewSubmissionVersion` 在`已提交`、未形成决定时允许换代，这一路没守。今天采用册空、走不到读申报，故不阻断；psb/26 灌采用行之前须补「非当前版本答不可见」或交 PS owner。
- Standards 2（上段②放大一格）：`estimateGapReason` 把溢出与单份为负同译 `ESTIMATE_NOT_POSITIVE`，经 `notFormedOutcomeFor` 落进 PS 处理尝试与续办引用，与判断项 9「分开是为了留下停在哪一格」相抵。
- Standards 3（上段①）：代码未碰；`cmd/parcel-dispatch` 的 `preAcceptanceTrials` 把 PP 入口接上生产图之后，`measurementOf` 的口径差变为生产可达。
- Spec 1：留痕按内容比且含 `trial_at`（Postgres `PreAcceptanceEstimateTraces.RecordEstimate` 的比较），两本账不比时刻。判断项 8 自述组合策略下冻结已落、账期那一项停`未配置`时事务照样提交，留痕随之已落；登记后重驱时若 `FormJudgmentAsOf` 给出新时点，同一请求身份撞留痕冲突、落`请求冲突`，推不动。判断项 8 以「同一判断时点」为前提，票面未举证该时点跨重驱不变——补一条用例，或在判断项 8 写明前提。
- 无发现：判断项 7 与决定六不冲突（决定六未定载体，同一事务即「一起保存」），采用册外键、另开事件类型与决定五不冲突——只判冲突，不替 SA owner 裁；第 4 条下半、第 5 条、第 6 条后半、第 7 条与票面一致；判断项 10 与代码顺序一致；释放路径未改；判据二未做的一格已归 psb/26。

两段的非阻断都不挡合入，本票不再收。去向待立票或交 owner：第二段 Standards 1（即第一段 Spec ③）须在 [票 26](./26-demo-tenant-adopts-the-pre-acceptance-estimate-form.md) 灌采用行之前处置；第二段 Standards 2、3 与 Spec 1 不挡 26。

## 进 main 记录（2026-10-13 通道 1）

远端 main `b09b904c` 就是本分支的根基，本票二十二笔分两段纯快进进 main，SHA 与分支 `mcp2-psb25` 相同。

- 第一段 `b09b904c..caab1e00` 十四笔（`b7c9da5e`、`b9486c5d` 作者通道 5，其余通道 2），17:28 推出。验证在 `caab1e00` 的 detached 隔离树上：带 DSN `go test -p 1 -count=1 ./...` 139 ok / 0 FAIL / 14 无测试 / 0 cached（日志写于 17:27）。评审：通道 3 `task-c7cfb347`（钉 `caab1e00`）两轴无阻断。
- 第二段 `caab1e00..fb39860d` 八笔（作者通道 2）；本记录随后一笔。验证在 `fb39860d`（detached 隔离树 `idp-parcel-mcp1-land-psb25b`，18:39–18:41）：按入库字节 gofmt 空、迁移无 CR；`go build ./...`、`go vet ./...` 退 0；清点在该 tip 重生成零差；分片核对通过；带 DSN `go test -p 1 -count=1 ./...` 139 ok / 0 FAIL / 14 无测试 / 0 cached；`-v` 探针 `TestAPreAcceptanceEstimateAdoptionIsAnImmutableRowOnARegisteredAccount`、`TestAPreAcceptanceEstimateTraceIsWrittenOnceAndReproducible` 为 PASS，非 SKIP。评审：通道 3 `task-386c6296`（钉 `fb39860d`）两轴无阻断。

推送后拆隔离树 `idp-parcel-mcp2-psb25`、`idp-parcel-mcp5-psb25`（后者三份未跟踪文件与封存笔 `ba61f682` 的 blob 逐一相同，19:16 实测；指针 `salvage/mcp5-psb25-wip` 留着）与验证树 `idp-parcel-mcp1-land-psb25b`，分支指针改名 `merged/mcp2-psb25`、`merged/mcp5-psb25`。本记录纯 `.md`，推送方自审。
