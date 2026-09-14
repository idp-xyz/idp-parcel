# sa-cc/16 与 sa-cc/01 两份非作者评审 Standards 非阻断收口：`ErrNilDependency` 文本去「pre-acceptance control」限定、sa-cc/01 注释「三件引用 / 五种结果」计数换点名、PP 合成评价夹具第三份抽成 PP 侧测试专属导出包

Category: chore
Status: in-progress——2026-09-14 08:5x 通道 4 按通道 1 派单 task-65ce5275 自立自做（SA 两条 + PP 一条合一票，先例 sa-cc/16；PP 那条作者与评审同判归 PP owner，本票代收），分支 `mcp4-tails` 基远端 main `cdf17834`（树 `D:/tops/idp-parcel-mcp4-tails`），与 [lc/38](../../label-channel-service-first-release/issues/38-lc37-review-tails-adr0049-citation-five-values-count-and-fact-reference-rename.md) 同分支——两票都撞 `cmd/parcel-dispatch/assemble.go` / `assemble_test.go`；要裁的为零
Blocked by: 无（[16](16-sa-evaluation-request-and-duty-verification-adoption-standards-tails.md) 与 [01](01-buy-evaluation-to-sa-inbox-consumer.md) 均已进 main）。撞点：通道 3 同期动 `internal/visibilityexception/**`、`cmd/parcel-api/assemble_claims.go`、ADR-0136，与本票零重叠；共享树不碰

## 缺口（取证于 `cdf17834`，开工先重量，与派单不符处如实记）

**一、`ErrNilDependency` 的文本对它今天的用途是一句错话**（16 评审 ← 通道 1 Standards ①、16「进 main 记录」候选后继、16 作者完成记录 ① 自报）。

- `internal/settlementaccounting/application/apply_pre_acceptance_control.go` 的 `ErrNilDependency` 文本是「settlement accounting: pre-acceptance control dependency is nil」。自 sa-cc/16 ① 起它是 `NewRequestBuyEvaluationHandler` 与 `NewApplyPreAcceptanceControlHandler` 两只构造器的共用哨兵（16 裁「沿用同包既有那一枚，不另铸第二枚——两个『唯一答复』不能并存」），于是请求评价编排缺件时报的是「pre-acceptance control dependency is nil: evaluation request registry」——限定词说的是另一条编排。
- **重量**：`git grep -n 'pre-acceptance control dependency' -- internal/ cmd/` 于 `cdf17834` 只命中那一行声明；两处用例（`apply_pre_acceptance_control_credit_basis_test.go`、`request_buy_evaluation_test.go`）都按 `errors.Is` 断言、不钉文本，改文本不动用例。

**二、sa-cc/01 新增注释里的跨文件计数**（01 评审 ← 通道 3 Standards ①、01 推送方处置 ①「归 SA owner，随下一张 SA 注释改口小票」）。

- 「三件引用」数的是 `FormSupplierExpectedCostCommand` 的来源引用字段，「五种结果」数的是 `saports.BuyEvaluationOutcome`，都是别处的东西（AGENTS.md「计数与行号同构」）；每处已逐项点名，去数词零损失。
- **重量**：派单点名四处——`form_on_buy_evaluation_recorded.go` `ErrSourceReferencesUnrecorded` 头注、`assemble.go` `buyEvaluationRecordedUndecidedSentinels` 头注、`form_on_buy_evaluation_recorded_test.go` 与 `adapters/inbox/buy_evaluation_recorded_consumer_test.go` 两份测试头注。`git grep -n -E '三件|五种' -- <那四份 + assemble.go>` 于 `cdf17834`：`form_on_buy_evaluation_recorded.go` 三句（`ErrSourceReferencesUnrecorded` 头注「协议三件引用」、`FormOnBuyEvaluationRecordedAdapter` 头注「评价的五种结果怎么分格」与「按回指取三件引用」）、`form_on_buy_evaluation_recorded_test.go` 两句（文件头「还缺三件来源引用」、`TestABuyEvaluationStopsUndecidedWaitingForTheEvaluationRequestRecord` 头注「评价的五种结果一视同仁」）、`assemble.go` 两句（`buyEvaluationRecordedUndecidedSentinels` 名单里 `ErrSourceReferencesUnrecorded` 那一格「供应商协议三件」、`formSupplierExpectedCostOnBuyEvaluationConsumer` 头注「供应商协议三件引用」）；**`buy_evaluation_recorded_consumer_test.go` 零命中**——派单说的第四份在 `cdf17834` 上没有计数，如实记，不动它。
- **保留**：01 作者判断项 ③ 说「三件引用」是裁决与 08 票面的既有词——票面 / 裁决文不动；sa-cc/08 自己那批 `.go`（`request_buy_evaluation.go`、SA `domain` / `ports` / `adapters/postgres` 里的「三件引用」）不是 01 评审点的、也不是 16 收的，本票不扩到它们，记为判断项交评审。

**三、PP 合成评价夹具的第三份**（01 评审 ← 通道 3 Standards ② + 01 作者判断项 ①，作者与评审同判 rule-of-three 已到、归 PP owner；01 推送方处置 ②）。

- 三份「同形」：SA `internal/settlementaccounting/adapters/parcelpricing/buy_evaluation_test.go`（`buyPlan` / `packageInput` / `evaluate`）、PP `internal/parcelpricing/adapters/postgres/evaluation_test.go`（`syntheticPlan` / `syntheticInput` / `evaluatedFixture`）、`cmd/parcel-dispatch/assemble_test.go`（`syntheticPricingEvaluation`）——各自在测试包里，互不可导入，所以才三份。
- **重量（与派单的前提不符，是本票最大的一处）**：派单写「夹具值一字不变（合成串、金额、币种、规则版本全照旧）」，默认三份是同一组值。`cdf17834` 上三份的**形**同（一张单分区重量段 USD 价表 + 实重策略 + 一份 5 kg / Z1 已受理包裹输入，真算 `EvaluatePricing`、证据级 `EvidenceSynthetic`），**值不同**：价表行金额 cmd / SA 12.5、PP 10；重量取整 cmd / SA `RoundingNone` 步 1 kg、PP `RoundingCeiling` 步 0.5 kg；金额取整 cmd 固定 HALF_UP 0.01 合计、SA 由 `rounded` 开关决定（`TestUndeclaredPrecisionIsRefusedNotRounded` 靠关掉它证拒）、PP 不声明；版本引用 cmd / SA 只带身份（`NewVersionReferenceIdentity`）、PP 带摘要（`NewVersionReference` + `sha256:syn-*`）；卡 / 表 / 策略 / 范围 / 包裹 / 租户的合成串三份各不相同，SA 用例还钉着 `SYN-BUY-CARD@v2`。**一枚零参数的共用夹具做不到「三处调它且值一字不变」**。做法因此是：共用的是「按顺序构造、构造失败即 `t.Fatal`」那一段与三份共有的常量（USD、Z1 0–10 kg、5 kg、`BASE_FREIGHT`、2026 有效期、2026-08-07 基准时刻、实重策略、`EvidenceSynthetic`），三份各自不同的值由调用方以规格显式交进去。三处调用点各自的评价与今天逐字段相同，真库用例不换题。

## 语言从哪里来

- 16 评审 ← 通道 1 Standards ① 原话与推送方处置；01 评审 ← 通道 3 Standards ① ② 原话、01 作者判断项 ① ③、推送方处置 ① ②。
- [16](16-sa-evaluation-request-and-duty-verification-adoption-standards-tails.md) 的形：评审尾巴一张小票包几处、Status 直起 in-progress、要裁的为零。
- 测试专属导出包的先例：`internal/platform/pgtest`——跨包共用的测试助手只能写成普通包（`internal/architecture/boundaries_test.go` `isFirstPartyTestScaffolding` 头注原话「它必须是普通包而非 `_test.go`，因为跨包共用的测试助手只能这么写」），只被 `_test.go` import。lc/08 的替身落的是**只有测试文件的包**，那种形跨包用不上。
- AGENTS.md「写代码注释」：只写代码讲不出的东西；不写行号不写计数。

## 做法

1. **`ErrNilDependency` 文本**：`errors.New("settlement accounting: pre-acceptance control dependency is nil")` → `errors.New("settlement accounting: dependency is nil")`；变量名不改、`errors.Is` 语义不变、两处调用点 `fmt.Errorf("%w: %s", ErrNilDependency, dependency.name)` 不动（口名仍跟在后面）；头注补一句它自 16 起是两只构造器共用。一笔。
2. **七句计数换点名**：「发生项 / 费用项目 / 供应商协议三件引用」→ 去「三件」只留点名；「评价的五种结果」→「评价的每一种结果」或直接点 `saports.BuyEvaluationOutcome`；只改 `.go` 注释。一笔。
3. **PP 侧测试专属包 `internal/parcelpricing/pptest`**（新包，普通 `.go`，只被 `_test.go` import）：导出 `PlanSpec` / `InputSpec` 两份规格与 `Plan(t, spec)` / `Input(t, spec)` / `Evaluate(t, id, plan, input)` 三只构造，另导出 `Value[T]`（`ppTestValue` / `evaluationValue` / `must` 三处同形的统一失败出口）与 `IdentityReference` / `DigestReference`。规格字段全部显式、无默认值——值由调用方给，夹具不替任何一处换题。一笔。
4. **三处改调**：`assemble_test.go` 的 `syntheticPricingEvaluation`、SA 的 `buyPlan` / `packageInput` / `evaluate`、PP 的 `syntheticPlan` / `syntheticInput` / `evaluatedFixture` 本地函数名保留（用例正文零改），函数体换成规格字面量 + 调 `pptest`；`ppTestValue` / `evaluationValue` / `must` 三只同形助手随之退位或改为转调。三处各自的合成串、金额、币种、取整策略、版本引用逐字段照旧。一笔（与 3 可合可分；分成两笔时第 3 笔单独不改任何调用点也编得过）。

## 红线

- 除做法 1（错误文本）外零行为：生产 `.go` 差只许注释行与那一串文本；`pptest` 是新增的测试脚手架，不进任何生产 import（`go list -deps ./cmd/... ./internal/...` 里不得出现它，与 `./internal/architecture/...` 一并验）。
- 三处夹具的值一字不变——以 SA `TestCompletedEvaluationIsAdoptedAtTheDeclaredIncrementScale` 仍断言 1250 分 / `SYN-BUY-CARD@v2`、PP 真库读回快照仍逐字节同答、cmd 真库用例 BUY 未决 / SELL 入账仍成立为证。
- 不动 `internal/settlementaccounting/adapters/inbox/**`（无计数可改）、迁移、`ports`、`domain`、任何 CC / VE 文件、`apps/`。
- 注释中文；不写行号、不数别处的东西。

## 完成判据

1. `git grep -n 'pre-acceptance control dependency' -- internal/ cmd/` 零；`git grep -n 'ErrNilDependency = errors.New' -- internal/settlementaccounting/` 一处且文本不含编排名；SA `application` 两处拒 nil 用例仍 PASS。
2. `git grep -n -E '三件引用|三件来源|五种结果|供应商协议三件' -- internal/settlementaccounting/adapters/parcelpricing/ cmd/parcel-dispatch/assemble.go` 零。
3. `git grep -n 'syntheticPricingEvaluation\|func buyPlan\|func syntheticPlan' -- cmd/ internal/` 三处本地函数仍在，函数体调 `pptest`；`git grep -l 'parcelpricing/pptest' -- cmd/ internal/` 只命中 `_test.go`。
4. 带 DSN `go test -p 1 -count=1 -v` `cmd/parcel-dispatch` + PP `adapters/postgres` + SA `adapters/parcelpricing`（占 55432 前后广播）：三包 ok、`--- SKIP` 零、真库用例 `--- PASS`；SA `-run 'Adopted|Undeclared|Boundaries|ShipmentLevel'` 断言值未动而 PASS。
5. `gofmt -l` 空、`go build ./...` / `go vet ./...` 退 0；`go test -count=1 ./internal/settlementaccounting/... ./internal/parcelpricing/... ./cmd/parcel-dispatch/... ./internal/architecture/...` ok。
6. 清点预报：`internal/parcelpricing/pptest/*.go` 是普通文件，`tools/mechanism-inventory` 按后缀分类会记 **PP 生产文件 +1**（派单预报的「测试 +1」按工具口径不成立——脚手架包要能被别的包 import 就不能是 `_test.go`，与 `pgtest` 同理）；其余零差（不加端口、不加缝、不加路由）。
7. 完成记录随本票最后一笔代码同提交（Status → resolved、逐笔 SHA、判据逐项、判断项）。

## 地盘

`internal/settlementaccounting/application/apply_pre_acceptance_control.go`（一串文本 + 头注一句）、`internal/settlementaccounting/adapters/parcelpricing/form_on_buy_evaluation_recorded{,_test}.go`（注释）、`internal/settlementaccounting/adapters/parcelpricing/buy_evaluation_test.go`（夹具改调）、`cmd/parcel-dispatch/assemble.go`（注释）、`cmd/parcel-dispatch/assemble_test.go`（夹具改调）、`internal/parcelpricing/adapters/postgres/evaluation_test.go`（夹具改调）、新包 `internal/parcelpricing/pptest/`、本票面、sa-cc spec 一行。

## 要裁的

零。

## 参照

[16](16-sa-evaluation-request-and-duty-verification-adoption-standards-tails.md) Comments「评审 ← 通道 1」Standards ①、「进 main 记录」候选后继；[01](01-buy-evaluation-to-sa-inbox-consumer.md) Comments「评审 ← 通道 3」Standards ① ②、作者判断项 ① ③、「推送方处置」① ②；`internal/platform/pgtest`（测试脚手架包的形）与 `internal/architecture/boundaries_test.go`（`TestProductionPackagesDoNotImportFirstPartyTestScaffolding` 头注讲为什么脚手架必须是普通包）；`internal/architecture/production_wiring_ratchet_test.go` / `production_type_reachability_ratchet_test.go`（扫的是全部非 `_test.go`，`pptest` 会被当生产源看——它只调 PP 已有生产调用点的构造器，基线零差，实跑为证）；`tools/mechanism-inventory/filecensus.go`（按 `_test.go` 后缀分生产 / 测试）。

## Comments

- 2026-09-14 08:5x · 通道 4（task-65ce5275）：立票，Status 直接 in-progress，作者自立自做。**只写票面，未动代码。** 三条在 `cdf17834` 上重量过：条一只一行声明、用例不钉文本；条二实得七句、派单点名的 `buy_evaluation_recorded_consumer_test.go` 零计数；条三三份夹具值不同，「零参数共用夹具 + 值一字不变」两者不可兼得，取「规格显式、值由调用方给」，写进「缺口」三与「做法」3 / 4。能力边界：读过三份夹具全文、`pgtest` 的包形、`boundaries_test.go` 脚手架那两条、两只棘轮门禁的源加载与名单判据、`filecensus.go` 的分类；没读 `EvaluatePricing` 本体与 PP 真库用例正文以外的 PP 代码。
