# 26 申报不全的估价按缺项分流：计价点名缺项、结算逐件带回、客户可补的停等受控补充并记缺口

Category: enhancement
Status: ready-for-agent
Blocked by: 无（依赖的票 23 代码已在 `feat/goods-details-admin`，未进 main；本票接着它做）

依据：[ADR-0381](../../../docs/adr/0381-an-incomplete-estimate-declaration-waits-for-customer-supplement-or-operator-registration-by-who-supplies-the-gap.md) 决定一至四；ADR-0094 决定三、五（先存等待、处理尝试同笔落库）；ADR-0380 决定一、三（等运营登记那一路与尝试两格的既有落点）。

## 要做的

- **计价（估价口点名缺项，ADR-0381 决定二）**：
  - 改在受理前估价与试算共用的那道输入完备性门上（`estimateInput` 一带；受理前走 `FormPreAcceptanceEstimatesHandler` → `FormPlanGivenEstimateEvaluationsHandler`，试算走 `FormEstimateEvaluationsHandler`，三只 handler 共用它）：卡绑的分区目录按始发邮编前缀分而寄件邮编没报 → 缺项 `ORIGIN_POSTAL`；卡有按尺寸判的规则而尺寸没报 → 缺项 `DIMENSIONS`。点名走输入不全（`INPUT_INCOMPLETE`），不再走完评价停在 `ZONE_UNRESOLVED` / `DIMENSIONS_REQUIRED` 的无合计。
  - 判据来源：`ORIGIN_POSTAL` 要读在用分区目录版本的始发维度声明（`CatalogueOrigin` 是目录版本上的格，卡结构只知道绑了分区目录）——此刻没有在用目录版本的不点名、照旧走解不出那一路；`DIMENSIONS` 两处都要问：判定条件（`PricingPlanStructures.ReadsFeature`、`package_weight_features.go`）与重量策略的体积重路（`CalculatePricingWeight`，`ReadsFeature` 看不见它）。
  - 缺项码封闭集加 `ORIGIN_POSTAL`、`DIMENSIONS` 两码（`form_estimate_evaluations.go` 的常量组）。
  - 受理前估价只有一张在用卡（多版同时适用是适用冲突那一格），缺项即该卡点名的缺项；试算照 ADR-0152 逐卡交回、各卡各点各的缺项、不并。「报了但解不出」（邮编报了而目录不盖它、汇率没解出）照旧评成待决或无合计。
- **结算（逐件原样带回，ADR-0381 决定三）**：
  - `ports.PreAcceptanceTrialAnswer` 在 `INCOMPLETE` 时另带逐件缺项（成员按申报包裹指名、码原样），`adapters/parcelpricing/pre_acceptance_trials.go` 不再折掉。
  - `ApplyPreAcceptanceControlResult` 的未形成答复带着它走到小包托运的端口（`psports.PreAcceptanceControlAssessment` 加一格），原因码照旧 `ESTIMATE_DECLARATION_INCOMPLETE`。
- **小包托运（分流与停等受控补充，ADR-0381 决定一、四）**：
  - 结算消费侧适配器（`adapters/settlementaccounting/pre_acceptance_control.go`）分流：缺项含 `ZONE` 或 `ADDRESS_NATURE` → 既有的等运营登记那一路（票 23）；其余封闭集内的缺项 → 等受控补充；缺项缺席 → 照旧内部重试；封闭集外的码 → 报错交人处理（同 `notFormedOutcomeFor` 对原因码「逐取值列出、default 报错」的纪律，不静默归类）。改 `TestNotFormedReasonsLandOnTheCellOfTheirRecoveryAction` 对应行。
  - 缺项码译成本上下文封闭集：`ESTIMATE_MEASUREMENT_MISSING`、`ESTIMATE_DIMENSIONS_MISSING` 指名申报包裹；`ESTIMATE_RECIPIENT_POSTAL_MISSING`、`ESTIMATE_ORIGIN_POSTAL_MISSING` 委托级、不指件、逐件重复的归并成一条。
  - 财务控制那一步新的停法：先存等待受控补充（缺口记在任务上，「接受前财务控制」组，照既有 `supplementGapDocument` 的形状）再交回待判原因，处理尝试同笔落库；待判原因与续办方用等待受控补充既有语言（`CustomerSupplementPending` / `ResumeByCustomerSupplement`）。写法对照 `awaitOperatorRegistration`（ADR-0094 决定五：等待没存下就不返回那个原因）。
  - 处理尝试照票 23 的两格（结算原因码、申报的服务产品）再另记计价缺项码（原样、去重、没有不记）；迁移给 `acceptance_processing_attempt` 加一列（可空、旧行不回填）。两路等待（受控补充、运营登记）都记缺项码。
  - 查阅面：委托详情与补件队列在这种停法下交出缺口（既有 `supplementGaps` 形状）、「上次处理」带尝试上的各格；补件队列应当列出它（`waitingOn` 同一套）。
  - 续办不加新机制：受控补充重报整份内容（收寄邮编、逐件测量与尺寸都在载荷里）、形成新提交版本，新版本信封驱动新一轮判断——既有路，核一条真库用例即可。
- **组合根**：无新信封、无新消费门；装配应当不动，核一遍。

## 完成判据

- 真库端到端：
  - 提交一份缺某件重量、缺收件邮编的委托（卡不读分区与地址性质）→ 停在`等待受控补充`，任务缺口两条（`ESTIMATE_MEASUREMENT_MISSING` 指名那一件、`ESTIMATE_RECIPIENT_POSTAL_MISSING` 委托级），处理尝试一行带 `ESTIMATE_DECLARATION_INCOMPLETE`、申报产品、缺项码；补件队列列出它；受控补充补齐 → 新版本被判、接受。
  - 卡带始发邮编前缀分区目录而寄件邮编没报 → 计价答输入不全缺 `ORIGIN_POSTAL`（不再 `ZONE_UNRESOLVED` 无合计）→ 停等受控补充。
  - 卡要直接给分区（无分区目录）→ 缺项 `ZONE` → 停等运营登记（票 23 那一路），处理尝试带缺项码；原卡按 ADR-0234 收尾（或出新版本）后登一张带分区目录、生效期覆盖提交时点的售价卡 → 重驱、接受（直接另登一张在用卡会落适用冲突，不是本判据要的形）。
  - 客户可补与卡侧缺项同场 → 停等运营登记（等运营登记先）。
- 计价单测：始发前缀目录＋空寄件邮编、尺寸规则＋无尺寸各答输入不全并点名新码；报了邮编而目录不盖它照旧无合计。
- 依赖读不回照旧内部重试、不落等待不落尝试。
- 全量 `go test`（带 PG）、`go vet`、`gofmt -l` 干净。

## 不做的

- 管理台（票 27）。
- 「委托待受控补充」信封（ADR-0371 决定三）：整条路今天都不交，已记在 UC-PS-001 交接表、仓配切片承接，本票不扩。
- 补充届满处置（租户取值，PS CONTEXT 既有那句）。
- 演示种子不加这条路（演示租户种子都带齐资料；要演示补件动线另立）。
- 按原因或计价范围收窄重驱（ADR-0380 越权风险点「多唤醒」）。
