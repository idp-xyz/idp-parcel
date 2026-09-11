# BUY 评价已发出的信封没有 SA 侧消费者：`parcel-pricing.evaluation.recorded` 落进 Outbox 后无人接，`FormSupplierExpectedCostHandler` 只有测试调得到

Category: enhancement
Status: resolved——2026-09-11 22:0x 通道 5 作者完工（task-35caf74b；分支 `mcp5-sacc01` 基 `02e1dfc4`，代码 + 本完成记录同笔，清点紧随一笔；带 DSN 四包 PASS 123 / SKIP 0 / FAIL 0）：(c) 范围「信封到未决」全部落地——消费者、处理方适配器、dispatch 路由行与未决哨兵、真库一正一反；判据 1 随裁决 (c) 改口顺延至形成路后继票（推送方 21:3x 同意，见「裁决」与「完成记录」）。进 main 的 SHA 由推送方重放后另记；等非作者评审。此前 in-progress——2026-09-11 21:2x 通道 5 认领（task-35caf74b；分支 `mcp5-sacc01` 基 `02e1dfc4`，隔离树 `D:/tops/idp-parcel-mcp5-sacc01`）。派单取证与票面的差别：「`adapters/inbox/`（新目录）」已过时——sa-cc/09 已建该目录（`duty_payment_verification_consumer{,_test}.go`），本票照那只的形新增文件、不动它；08 已进 main，但「形成」那条路按裁决仍不在本票。此前 ready-for-agent——2026-09-10 17:3x 通道 5 按通道 1 派单 task-9a2ff746（用户授权代裁）写入裁决：「要裁的」1 取 (c)，本票范围是「信封到未决」，三件引用等 [08](08-sa-evaluation-request-orchestration-records-source-references.md)（见「要裁的」下「裁决」）。此前 draft——2026-09-10 通道 4 立票（task-9880bbc9），只写票面未动代码；取证锚 `3f485e97`
Blocked by: 无（(c) 范围不等 08；08 落地后「形成」那条路另补）

## 缺口（取证于 `3f485e97`）

- 提供方已发：`internal/parcelpricing/adapters/postgres/evaluation_handoff.go` 的 `evaluationEventType = "parcel-pricing.evaluation.recorded"`，指针载荷 `{tenantId, evaluationId}`，`ports.EvaluationHandoffIntent` 注释写「费用采用在 settlement-accounting」。
- SA 侧编排已落：`internal/settlementaccounting/application/form_supplier_expected_cost.go` 的 `FormSupplierExpectedCostHandler.Handle`（mech/06 SA-c，`1ea2597`），读口 `ports.BuyEvaluationView.LoadBuyEvaluation`，登记面 `ports.ExpectedCostRegistry`。
- 中间那层没有：`git ls-files internal/settlementaccounting/adapters` 无 `inbox`；`cmd/parcel-dispatch/assemble.go` 路由表无 `parcel-pricing.evaluation.recorded`（`git grep -n 'evaluation.recorded' -- cmd/` 零）；`git grep -E 'SupplierExpectedCost' -- cmd/` 只命中 `unwired_orchestration.go` 读面桩。
- 消费侧适配器**已在**：`internal/settlementaccounting/adapters/parcelpricing/buy_evaluation.go` 的 `BuyEvaluationAdapter` 实现 `saports.BuyEvaluationView`（pricing-amount-precision/02 消费侧那一项，ADR-0107 决定五「单位换写不是算术」）。所以从信封到编排之间缺的**只有** inbox 消费者与 dispatch 路由行。

## 语言从哪里来

- SA `CONTEXT.md`：「结算输入已接收：固定关务交接、税费、付款核对、外部资金事实、付款方、合同责任和结算依据的采用版本；接收不表示代垫或回收已经成立。」
- UC-SA-002 步 5 BUY 方向（`form_supplier_expected_cost.go` 文件头：「把一份已完成的 BUY `PricingEvaluation` 与 TF 的运输收费发生项、费用项目和供应商协议一起采用为供应商预期成本的首版」）。
- mech/06「SA-c：缝的形状」原句：「触发用提供方已发的 `parcel-pricing.evaluation.recorded`……内容按引用查」「SA 侧的 inbox 消费者是接线的下一层，不在本票：那封信封只带评价引用，而形成预期成本还要发生项（TF）、费用项目与供应商协议（PC/TF）——它们只在 SA 请求评价那一步（UC-SA-002 步 2）才聚在一处，而那条编排今天同样不存在。」

## 做法

1. `internal/settlementaccounting/adapters/inbox/`（新目录）：`BuyEvaluationRecordedConsumer`，形照 PS 的 `psinbox.*Consumer`（按事件类型取指针载荷、租户在信封上、幂等由 inbox 账本守）。
2. 消费者按 `evaluationId` 经 `BuyEvaluationView` 取评价，方向 / 目的不是 BUY·SUPPLIER_COST 的信封**不处理不报错**（不是本消费者的信封）。
3. 命令里的发生项 / 费用项目 / 供应商协议引用从哪来——见「要裁的」第 1 条；裁前消费者只能对「命令齐不齐」答**未决并指名等谁**（`ExpectedCostUndecided`），不造引用。
4. `cmd/parcel-dispatch/assemble.go` 路由表加一行（共享接线文件，动前占号）。
5. 真库装配用例：入队一封 → 消费一次 → `ExpectedCostRegistry` 一版或未决一格；重投不翻倍。

## 红线

- 金额、币种、换算步骤、规则版本整组出自评价，消费者不复制、不取整、不补默认（ADR-0107：消费方不得在评价之外取整）。
- 不为「命令缺三件引用」发明来源：缺就未决。
- 不改 `form_supplier_expected_cost.go` 的结果代数；不改 PP 的信封形。

## 完成判据

1. ~~`git grep -w NewFormSupplierExpectedCostHandler -- cmd/` 有非测试调用点（dispatch 装配）。~~ **随裁决 (c) 改口（2026-09-11 21:3x 推送方裁、通道 5 写入）**：本票**不装** `NewFormSupplierExpectedCostHandler`——「命令齐」那一支今天不可达（理由见「裁决」末句），把它接进处理方就是在 `cmd/` 装一只没人调的编排，用永远答「不在」的来源口顶上则是把替身放进生产装配；形成路后继票装它，Blocked by 08 的回指（评价 → 评价请求标识，PP 侧归 11）。本票落地的是它前面那一段：消费者 + 处理方适配器 + 路由行 + 未决哨兵。
2. 应用层：BUY 信封 → 未决并指名「等评价请求记录（票 08）」一条路有用例；「形成」那条路等 08 落地后由 08 或其后继票补，本票不做；非 BUY 信封不处理。
3. 真库：`cmd/parcel-dispatch` 装配用例一正一反（含 DSN PASS，无 DSN SKIP）。
4. 基线不加宽；机制清点 tip 重生成（消费缝新增 SA→PP 一组）。

## 地盘

`internal/settlementaccounting/adapters/inbox/`（新）、`cmd/parcel-dispatch/assemble.go` 一行 + `assemble_test.go`。不动 `internal/parcelpricing/**`，不动 `adapters/parcelpricing/buy_evaluation.go`。

## 要裁的

1. **三件引用从哪来**：发生项（TF）、费用项目、供应商协议引用不在信封里。选项 (a) 先立 UC-SA-002 步 2「结算提交主要范围、计算目的和合格来源引用」那条请求评价的编排，评价请求本身记下三件、消费者按评价引用回查；(b) 消费者从 TF 收费发生项登记册按评价的输入反查；(c) 本票只做「信封到未决」，三件等 (a) 另票。归 SA owner。

### 裁决

- **1 → (c) + 立 [08](08-sa-evaluation-request-orchestration-records-source-references.md)**（通道 1 推送方裁、通道 5 写入，2026-09-10 17:0x；task-b941ce87 分类 B、task-9a2ff746 落笔）。本票只做「信封到未决」：消费者收 BUY 信封、按评价引用取评价、对「命令齐不齐」答 `ExpectedCostUndecided` 并指名等评价请求记录；三件引用由 08「UC-SA-002 步 2 请求评价编排（评价请求记下三件引用）」提供，08 是实现 UC-SA-002 步 2 已写明的「结算提交主要范围、计算目的和合格来源引用」。**(b) 否**：SA→TF 新跨上下文读口要动 CONTEXT-MAP，且按评价输入反查登记册是推导不是引用（SA CONTEXT「结算输入已接收……的采用版本」——采用的是引用，不是反查出来的匹配）。本票 Status 转 ready-for-agent（(c) 范围），完成判据 2 随改；08 立票时若冒出 UC 空白列进 08 的「要裁的」。
- **(c) 下「命令齐」那一支为何今天不可达（2026-09-11 21:3x 通道 5 实测于 main `02e1dfc4`、推送方同意后追记）**：08 已进 main，评价请求登记册 `EvaluationRequests` 与 `ports.EvaluationRequestView.FindByID` 在场；但 PP 的 `PricingEvaluation` 今天**不回指** `evaluationRequestId`（回指是 PP 侧的事，归 11，11 仍 draft），SA 读口交回的 `ports.BuyEvaluationAdoption` 也**没有**盛回指的槽——消费者从信封只拿得到评价引用，从评价拿不到请求标识，于是既到不了 08 的册、也拿不到预期成本的版本身份，命令永远凑不齐。判据 1 因此随本条改口（见「完成判据」1）；形成路后继票在 11 落地、读口补回指槽之后再装 `NewFormSupplierExpectedCostHandler`。

## 完成记录

（通道 5 · task-35caf74b · 2026-09-11 21:2x–22:0x · 隔离树 `D:/tops/idp-parcel-mcp5-sacc01`，分支 `mcp5-sacc01` 基远端 main `02e1dfc4`，全程未 rebase。）

**逐笔**：认领 `0f8757ee`（Status → in-progress）；代码 + 本完成记录 + spec 01 行**同一笔**（按推送方今晚新纪律「完成记录随最后一笔代码同笔」，SHA 见完工报）；机制清点在该笔干净检出重生成、紧随一笔。

**动过的文件**：新增 `internal/settlementaccounting/adapters/inbox/buy_evaluation_recorded_consumer{,_test}.go`、`internal/settlementaccounting/adapters/parcelpricing/form_on_buy_evaluation_recorded{,_test}.go`；改 `cmd/parcel-dispatch/assemble.go`（import 两行、`buyEvaluationRecordedUndecidedSentinels`、`wireDispatcher` 头注一句 + 体内一块 + 路由表一行、装配函数 `formSupplierExpectedCostOnBuyEvaluationConsumer`）与 `assemble_test.go`（`saTestValue` 之后一条真库用例 + PP 评价夹具 `syntheticPricingEvaluation` + `ppTestValue`）；本票面与 spec 01 行。**地盘自扩一条**：`internal/settlementaccounting/adapters/parcelpricing/` 新文件（不动 `buy_evaluation.go`），形照 sa-cc/09 把处理方放在提供方同名适配器包的先例，推送方 21:3x 认可。`internal/parcelpricing/**`、`buy_evaluation.go`、`form_supplier_expected_cost.go` 零改（`git diff --name-only 02e1dfc4` 核过）。

**逐条对完成判据**：

1. **改口后**：✓ 不装 `NewFormSupplierExpectedCostHandler`；`git grep -w NewFormSupplierExpectedCostHandler -- cmd/` 仍零，且这是本票的**预期结果**而非缺口（理由见「裁决」末句）。组合根 `formSupplierExpectedCostOnBuyEvaluationConsumer` 头注写明为何不装、后继票在哪里补。
2. ✓ 「未决并指名」一条路：`TestABuyEvaluationStopsUndecidedWaitingForTheEvaluationRequestRecord`（评价五种结果逐格，都停在 `ErrSourceReferencesUnrecorded`，回查恰一次）；非 BUY 不处理：`TestASellEvaluationIsNotThisConsumersEnvelopeAndIsSilentlyDone`。**层的注**：票面写「应用层」，本票用例落在处理方适配器 `FormOnBuyEvaluationRecordedAdapter`——(c) 下命令凑不齐，`FormSupplierExpectedCostHandler` 到不了，「应用层的 `ExpectedCostUndecided`」在这条线上今天没有可达的落点；哨兵 `ErrSourceReferencesUnrecorded` 是它在消费层的对应格，头注写明。形成路后继票接上编排后，那一层的用例随之落。
3. ✓ 真库一正一反 `TestARecordedBuyEvaluationStopsUndecidedAtTheExpectedCostSeamThroughTheRouteTable`：正——PP 真 `Evaluations.Save` + 真 `OutboxEvaluationHandoff` 入队一份 BUY·SUPPLIER_COST 评价的信封（事件类型由提供方写、路由按消费方常量认，两串相等在此钉住），一拍后 `dispatch.consumer_undecided`、SA inbox 无账、`supplier_expected_cost` 零行；反——同形信封指 SELL·CUSTOMER_CHARGE 评价 → 入账定稿（inbox 一行、零写入），另一封指 PP 还没有的评价 → `consumer_undecided`。带 DSN PASS，无 DSN 包级 `ok`（`pgtest` 跳过）。
4. ✓ 基线不加宽：`internal/architecture` 五道门禁零改、全 ok；机制清点在分支 tip 干净检出重生成、紧随一笔（消费缝 SA←PP 那一组新增，各项数字以清点笔的提交说明为准——本记录先于清点成笔，不预报数字）。

**分格（ADR-0029，写进 `assemble.go` 名单头注）**：未决两格——`ErrEvaluationNotVisible`（读不回 / 读口答不出，可见性滞后）、`ErrSourceReferencesUnrecorded`（命令缺三件引用，等 08 回指 / 11）；入账一格——`ErrNotABuyEvaluation` 由处理方吞成 nil（做法 2）；名单外三格——`ErrUntranslatableAnswer`（提供方形状变了）、`ErrAmountPrecisionUndeclared`（卡没声明取整策略，重投不会变好，要改卡再评一份）、`ErrUntranslatableReference`（引用坏了）。

**事件类型串**：PP 的 `evaluationEventType` 未导出（`internal/parcelpricing/adapters/postgres/evaluation_handoff.go`），本票按派单「未导出就钉字面」在 `sainbox.BuyEvaluationRecordedEventType` 自写 `parcel-pricing.evaluation.recorded`，相等性由判据 3 那条真库用例用提供方真适配器入队钉住；没有为此改 PP。

**验证**（隔离树，21:5x，Windows 本机）：`gofmt -l` 空；`go build ./...` / `go vet` 退 0；无 DSN：`internal/architecture` + `internal/settlementaccounting/...` + `cmd/parcel-dispatch` 全 ok；带 DSN `go test -p 1 -count=1 -v` 四包（SA `adapters/inbox`、SA `adapters/parcelpricing`、`cmd/parcel-dispatch`、`internal/architecture`）**PASS 123 / SKIP 0 / FAIL 0**，本票新增 12 例逐条 PASS。55432 占 / 释均已广播。`-race` 本机无 cgo 未跑。

**评审**（`/implement` Step 3，基线 `0f8757ee`；子代理鉴权错，两轴由作者串行自跑，不以此代替非作者评审）：
- Standards：阻断 0。判断项 ① `assemble_test.go` 的 `syntheticPricingEvaluation` 是 PP 评价夹具的第三份同形（前两份在 SA `adapters/parcelpricing/buy_evaluation_test.go` 与 PP `adapters/postgres/evaluation_test.go`，测试包互不可导入）——rule-of-three 已到，抽成 PP 导出的合成夹具包归 PP owner，本票不动 `internal/parcelpricing/**`。② 消费者名 `settlement-accounting/form-supplier-expected-cost` 与装配函数名按缝的目的取，而本票的停点是未决：inbox 名是账本身份、改名等于换消费者，所以按目的命名而不按今天的停点。③ 注释里「三件引用」数的是另一文件的命令字段——它是裁决与 08 票面的既有词，且每处都点名那三件，保留。
- Spec：阻断 0。判据 1 改口（上）；判据 2 层的注（上）；做法 5「重投不翻倍」在 dispatch 层没有直接用例（`systemClock` 推不动 `RetryAfter`），由 inbox 缝的恰一次用例 + 「未决零写入」共同覆盖——没有写入就没有可翻倍的行。越界核：零命中 `internal/parcelpricing/` / `buy_evaluation.go` / `form_supplier_expected_cost.go`。

**判断题**（自己拿不准的取舍，请评审与推送方裁）：
1. `ErrAmountPrecisionUndeclared` 留在未决名单外、让失败码落 `publish_failed`：这份评价按现卡永远采用不了，重投不会变好，我判它「要人动手」；反方是它属实例半边（卡的配置），或许该像 `CONTRACT_UNCONFIGURED` 那样算未决。
2. 「评价读不回」译成未决而不是提交矛盾：`FormSupplierExpectedCostHandler` 对 `!found` 答 `SOURCE_NOT_ACCEPTED`，我在消费层按 09 先例改判可见性滞后——信封与评价在提供方同事务落库，消费者眼里读不回只剩滞后一种成因；反方是两层对同一现象给两种格。
3. 处理方在 (c) 下对评价的五种结果不分格、一律答 `ErrSourceReferencesUnrecorded`：结果分格是编排的事，命令凑不齐时到不了它；反方是 `UNRATABLE` 这种终局格今天也被当未决反复重投，直到后继票接上。

## 参照

[mech/06](../../mechanism-executor-triage/issues/06-sa-four-executors-behind-existing-uc-steps.md)「SA-c：缝的形状」「不在本票」；[remaining-work-dd5ed934.md](../../unresolved-review-20260904/remaining-work-dd5ed934.md) 五-7；ADR-0107；ADR-0029（按恢复动作分格）；PS `adapters/inbox/operator_registration_completed_consumer.go`（消费者形状先例）。

## Comments

- 2026-09-10 · 通道 4：立票。未动代码。
- 2026-09-11 · 通道 5（task-35caf74b）：(c) 范围作者完工。判据 1 随裁决改口（推送方 21:3x 同意），理由与实测写进「裁决」末句；地盘自扩 `adapters/parcelpricing/` 新文件一条，推送方同一轮认可。三道判断题见「完成记录」。分支已推 origin；等推送方派非作者评审后重放。
