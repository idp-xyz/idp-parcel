# 15 发生项更正不驱动供应商预期成本的计价纠错

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-dcg57。此前 in-progress——2026-10-21 认领（57、15 同批）；此前 needs-triage——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [08](./08-buy-evaluation-recorded-consumer-never-forms-supplier-expected-cost.md) 的隔离评审。
Blocked by: 无——[27] 已进 main。此前 [27](./27-transport-charge-occurrences-are-registered-and-handed-off.md)（发生项先要有生产路径；原票 07 已拆分）
出处：[ADR-0200](../../../docs/adr/0200-buy-evaluation-forms-the-supplier-expected-cost-through-its-request.md) 越权风险点 4。

## 现象

另一份请求点名同一发生项的新版本、同一费用项目，评价又命中同一规则版本时，BUY 那一路只答「已有首版」并入账，不追加计价纠错，信封入账后不留痕。settlement-accounting `CONTEXT`「发生项更正 → 追加纠错版本」那条不变式在这一路被跳过。

## 证据（读码）

`internal/settlementaccounting/application/form_requested_supplier_expected_cost.go` 的同三维首版那一格；`domain` 上 `AppendCorrection` 今天没有应用层调用方。

## 三类

机制。规模 M。

## 最小的诚实修法

同三维已有首版、而新评价来自发生项的新版本时，按 ADR-0067「纠错重述整组评价结果」追加一个纠错版本（版本标识由纠错那条路给，不用 `BUYSC-` 派生），前后版本可审计。

## 要裁的

纠错由「发生项新版本」触发，还是由「评价结果不同」触发；同一规则版本、同一金额时是否仍追加（无变化纠错）。归 SA owner。

## 既有票

none

## 文档自洽审计复核（2026-10-21）

审计条目：[audit/sa-cr.md](../../docs-consistency/audit/sa-cr.md) M8（归 C，审计已点名本票，不另立）：SA CONTEXT「供应商成本与共享分摊」与 UC-SA-002 `AT-SA-164` 写发生项有效性更正「追加新的预期成本版本表达计价纠错」，同表 `AT-SA-186` 与代码是「修订版不发起、不记」（结果 `OCCURRENCE_REVISION_NOT_TRIGGERED`，ADR-0208 决定五把怎样驱动纠错归本票）。按 main `67438a0f` 复核，SA 代码未改，现象仍在。

分级：R1，与上线就绪评估「代决复核追加」一致（本票随票 57 的发生项失效入口一并升）。文档侧（`AT-SA-164` 注「未实现，见票 15」并与 `AT-SA-186` 互指）由修订票 docs-consistency/09 做；本票落地时去掉那条注。

## 分诊（2026-10-21，协调方代决：随 57 升为上线前必修，见代决复核追加）

[triage-2026-10-19](../triage-2026-10-19.md) 分诊表把本票归 C，并注「TF `ReviseValidity` 无生产调用方……那个入口一落地即升 A」；代决复核 [owner-review-20261020](../../owner-review-20261020/report.md) 的 M9 把发生项失效的生产入口纳入票 57 那一批，「代决复核追加的上线前必修（2026-10-20）」一节随之把本票升为上线前必修。本票与 57 同一分支、同一份新 ADR（ADR-0232）做；「要裁的」两问（纠错由发生项新版本还是由评价结果不同触发、无变化时是否仍追加）在 ADR-0232 里代决。

## 完成记录（2026-10-21，Claude Code 作者子代理，分支 claude-dcg57，变基在 main `7b598e47` 之上）

与票 [57](./57-segment-commission-adoption-has-no-correction-and-start-does-not-rederive.md) 同批，共用 [ADR-0232](../../../docs/adr/0232-a-segment-commission-adoption-is-withdrawn-by-decision-only-before-occurrences-form-and-occurrence-revision-drives-the-supplier-cost-correction.md)（决定七、八；越权风险点 10 至 13 归本票）。发生项的修订入口由票 57 落地（失效与失效之后的替代版本），本票做结算那一侧。

**「要裁的」两问（代决，ADR-0232 决定八）**：纠错由**发生项新版本**触发，不由评价结果不同触发；同一规则版本、同一金额也追加——重述的发生项版本与业务时点本身是计价结果的一部分（ADR-0067 决定二），不追加，当前有效的那一版就停在一份已失效的发生项上。版本标识仍由这一份请求派生（ADR-0200 决定二「由纠错那条路给」：这条路就是这份请求），票面「不用 `BUYSC-` 派生」那一句的顾虑由回指与原因两格区分首版与纠错。

**做了什么**

- 失效：派发给发生项已形成信封多扇出一路（名次在发起 BUY 评价之后），按键回读，是失效版本就在发生项锁下记下「被失效的那一版已失效」，给依它形成的每条成本链头长出一版失效纠错版本（`BUYSCW-<被纠正版本>`，回指它，原因 `OCCURRENCE_INVALIDATED`，发生项引用换成失效版本，计价结果原样、`withdrawn` 标不再成立——零是一个价格，失效不是）。原版与计算展开不动，不形成账单贷项（`AT-SA-164` 由此钉住）。重投入账一次；编排层面原样再交答重放、不多一版。
- 替代：失效之后长出的替代版本照常发起 BUY 评价（部分停用 ADR-0208 决定五「修订版不发起」，限替代版本）；评价形成时这个费用项目上的链头依的是另一版发生项，就按 `AppendCorrection` 重述全额、接在链头后面（原因 `OCCURRENCE_SUPERSEDED`）。此前「同三维已有首版即止」只剩同一版发生项的重放（部分停用 ADR-0200 决定三一句，回答其越权风险点 4）。
- 评价晚于失效到：依已失效那一版的评价不形成任何成本，答 `OCCURRENCE_INVALIDATED`、入账；这一格不记 ADR-0229 的停因（没有哪一种参数登记补得回它）。
- 一版至多被纠正一次（唯一索引）；形成、纠错与撤下都在同一把发生项锁下读成本状态（`ExpectedCostRegistry.LoadOccurrenceCostState`）。替代版本的评价换了合同结算币时响亮失败（越权风险点 11）。
- 预期成本列表与查阅口多 `withdrawn` 一格；SA `CONTEXT` 供应商成本两条；UC-SA-002 补 `AT-SA-206`–`210`，`AT-SA-186`、`191` 改口为失效版本不发起、不记。

**迁移**：settlement_accounting `0044`（`withdrawn` 一列、一版至多被纠正一次的唯一索引、发生项失效记录一册）。

**提交**（变基之后）：`cc5f237f` 结算纠错（含派发路由 `routes_supplier_cost_on_occurrence_invalidated.go` 与真库用例）、`761e81bb` 用例 Covers 改准；ADR、文档与盘点同票 57 那几笔。

**与票 14 的对齐**：ADR-0229 已先进 main，变基时合并了 `form_requested_supplier_expected_cost.go`、`trigger_buy_evaluation_on_occurrence_formed.go` 两处冲突，只加不改（新加的一格不进停因；`Revised` 的注释改口为只指失效版本）。本票在 `ExpectedCostRegistry` 加了一口，ADR-0229 那一侧日后新写的替身要补上（ADR-0232 越权风险点 13）。

**验证**：同票 57 的完成记录（逐笔 build+vet、相关包与反向依赖真库测试 48 包 ok）。

## 评审记录（一轮，变基前；2026-10-21，Claude Code 作者子代理，分支 claude-dcg57，未变基）

隔离评审中归本票的各条逐条去向；B1、3 与 TF 一侧的注释见 [57](./57-segment-commission-adoption-has-no-correction-and-start-does-not-rederive.md) 的同名一节。

- **4 供应商账单匹配不看 `withdrawn`——已修，`1335884b`，不另立票 174。** `ReceiveSupplierBillHandler` 读回一行点名的版本之后，它是失效纠错版本、或已有后继（新读口 `ExpectedCostSuccessionView.LoadExpectedCostSuccessor`，账单编排多一口必备依赖 `Successions`），答独立一格 `EXPECTED_COST_NOT_CURRENT`（带出行、版本、撤下与否与后继），不落账单、不交意图；点名当前那一版照常匹配。用例 `TestABillLineNamingANonCurrentExpectedCostIsNotMatched`，真库读口 `TestTheSuccessorOfAnExpectedCostVersionIsReadBack`。ADR-0232 决定七补一条、越权风险点 10 改写；UC-SA-004 补 `AT-SA-211`。
- **5 `RequestedSupplierExpectedCostFirstVersionExists` 注释与请求层用例——已修，`87bb3023`。** 注释改成「同一版发生项、另一份请求已先形成首版」的直接陈述；补 `TestAnotherRequestOnTheSameOccurrenceVersionFindsTheFirstVersion`。
- **6 `supplier_cost_occurrence_state.go`「三件事都先读它」——已修，`87bb3023`。** 改点名 `FormSupplierExpectedCostHandler` 与 `WithdrawSupplierExpectedCostHandler`。
- **7 `ErrCorrectionChangesSettlementCurrency` 单元用例——已补，`87bb3023`。** `TestASupersedingEvaluationChangingTheSettlementCurrencyIsRaised`：断言这个错误，成本册上只有首版与失效纠错版本。
- **8 UC-SA-002 与 ADR-0229 的口径——已修，`35b7da92`、`c7cdd183`。** `AT-SA-186` 结果列改为「失效版本不发起、不记；替代版本没登触发行时与首版一样记停办、补登后续办」；`AT-SA-191` 同引 ADR-0232；ADR-0232「对已接受记录的作用」加一条写明 ADR-0229 决定四「不记的」里「修订版不发起」一格随之收窄，ADR-0229 的 Status 行与 Links 加前向指针（正文不改）。
- **9 结算侧乱序窄窗——已修，`87bb3023`，修法与评审建议的字面不同。** 评审建议「这份评价的发生项版本早于链头的就不接」；结算手里的发生项版本是运输履约交来的不透明引用（拼法 `V` 加序号是 TF 的派生规则，不是交接的公开语言），读出先后就多一处要跟着 TF 改的地方，所以不比版本号。改为按领域不变量收口：替代版本只从失效版本长出（`ReestablishChargeOccurrence`），纠错因此**只接在失效纠错版本后面**；链头依的那一版还没被撤下，就是撤下那一路还没把失效记到结算——答未决 `OCCURRENCE_INVALIDATION_NOT_RECORDED`（重投，零写入）。两种乱序于是都等撤下那一路：替代版本的评价先于失效到的，失效记下之后接在失效纠错版本后面；早一版的评价晚于替代版本的评价到的，失效记下之后答 `OCCURRENCE_INVALIDATED`、入账，链头不退。用例 `TestASupersedingEvaluationWaitsForTheInvalidationToReachSettlement`、`TestALateEvaluationOfAnEarlierVersionIsNotAppendedAfterTheSupersedingOne`；`AT-SA-207` 改写并补这两格（原 `TestARequestNamingANewerOccurrenceVersionAppendsACorrection` 改为先失效、再由 v3 纠错）。代价记作 ADR-0232 越权风险点 15：撤下那一路卡住时替代评价随它未决、烧自己的重投预算。中途试过「按链上走过的版本认早晚」一版，覆盖不到早一版从没形成过成本的那一种乱序，未提交。

**未采纳**：9 的字面修法（比版本先后），理由同上；效果（早一版的评价不接在替代版本后面）已做到。

**验证**：同票 57 那一节。
