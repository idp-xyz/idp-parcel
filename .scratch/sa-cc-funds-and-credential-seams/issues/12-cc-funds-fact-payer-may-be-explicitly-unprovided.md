# CC 入向登记要付款人非空，而 SA 采用的事实可以没有付款人：来源未提供且真实程序不要求时，CC 应「明确记录」而不是拒收

Category: enhancement
Status: resolved——2026-09-14 12:5x 通道 1（推送方自办：通道 4 旧会话 10:5x 后 crash，用户 12:2x 改指「不要考虑 mcp-4 了」）在分支 `mcp1-sacc12`（基远端 main `1ca125a5`；之一 `97ba925a` 由通道 4 旧会话完成，之二 `5ef53c6e` 含其 crash 现场——推送方封存 `7f5db322` 后 `reset --soft` 拆回、随用例重切——之三 `2ed944c4`、本笔）完成判据 1–3 全部落地，验证与判断项见「完成记录」。**非作者评审缺席**：隔离子代理 12:5x 连续三次「Authentication error」不可用、无其它在线通道，先以作者两轴自查替代并如实标注在「完成记录」，进 main 前子代理若恢复再补一跑。进 main 的 SHA 由推送方重放后另记。此前 in-progress——2026-09-14 10:3x 通道 4 按通道 1 派单 task-d6d4f149 认领（`/implement`；分支 `mcp4-sacc12` 基远端 main `1ca125a5`，树 `D:/tops/idp-parcel-mcp4-sacc12`；迁移序号钉 `customs_compliance/0020`）。此前 ready-for-agent——2026-09-14 10:2x 通道 1 按用户 10:1x「授权代裁」（CC owner 口径）裁「要裁的」1：「真实程序要求付款人」是**登记的规则一格**（形照 ADR-0137 决定三规则型目录行），不是 `PAR-CUS-*` 待提供参数；机制半边现在做、实例半边留空拒默认值；做法 3 的停格一并裁定，全文见文末「裁决」。取证锚仍是票面的 `f96169d2`，作者开工先在 main 重量。此前 draft——2026-09-10 22:0x 通道 5 立票（sa-cc/03 实施中按通道 1 裁决「CC 放宽另立 draft」，task-764b20a1）。只写票面未动代码；取证锚 `f96169d2`
Blocked by: 无（03 已落地；本票要裁的一条归 CC owner）

## 缺口（取证于 `f96169d2`）

- `internal/customscompliance/application/reconcile_duty_payment.go` 的 `ReceiveFundsFact` 对空 `Payer` 答 `未受理`；迁移 `customs_compliance/0016` 的 `external_funds_fact.payer_ref text NOT NULL` + 非空 CHECK。
- SA 侧（sa-cc/03 落地）：`domain.ExternalFundsFact.Payer()` 第二值为 false 即「来源未提供」，采用不拒；`settlement_accounting/0018` `payer_ref` 可空。
- 于是一条来源没给付款人的事实，经 `settlement-accounting.external-funds-fact.adopted` 信封到 CC 消费者，`ReceiveOnAdoptedFundsFactAdapter` 如实交空、编排答 `未受理`、消费门入账不重投（`receive_on_adopted_funds_fact.go` 头注）——事实进不了税费付款核对的入向登记册，**这是有意的诚实停点，不是 bug**（03 判断题 ③）。

## 语言从哪里来

- CC `CONTEXT.md`「税费付款核对」词条：「按明确申报范围、法定义务以及**来源提供或真实程序要求的**付款人、金额、币种、业务时间等维度进行的版本化比较判断」——付款人是「来源提供**或**真实程序要求」的维度，两种来处都成立时才必备。
- CC `CONTEXT.md`（sa-cc/03 票面引）：「来源未提供且程序不要求的维度要『明确记录』」。
- mech/07 CC-c 把付款人列为关联核对最少要读的四件之一——与上一句的张力正是本票要裁的。

## 做法（待裁后）

1. `ExternalFundsFactRegistration.Payer` 允许「来源未提供」的显式形（不是空串默认：领域上一格，或 `(string, bool)`），`ReceiveFundsFact` 不再因付款人缺席答 `未受理`。
2. 新迁移（序号重取）放宽 `customs_compliance.external_funds_fact.payer_ref` 为可空 + 拒空白 CHECK；`0016` 不改。
3. `VerifyPayment` 的调用方在关联核对时看得见「付款人未提供」这一格——真实程序要求付款人而来源没给时，核对该停在哪一格（待确认？不适用？）随裁决定。
4. `ReceiveOnAdoptedFundsFactAdapter` 不改：它今天已如实转述缺席。

## 红线

- 不拿 SA 的来源身份或别的维顶替付款人；不写任何真实银行 / 支付字段。
- 「程序要不要求付款人」若属实例半边（`PAR-CUS-*`），一行都不预填。

## 完成判据

1. 应用层：来源未提供付款人的事实 → `已接收`，登记里付款人显式「未提供」；同引用重放 → `已存在`；程序要求而未提供时核对的停格如裁决。
2. 真库：放宽后的往返；`0016` 一字未动。
3. sa-cc/03 的越权风险点 (b) 由 CC owner 在本票一并复核。

## 地盘

`internal/customscompliance/{ports,application,adapters/postgres}`、`migrations/customs_compliance/`（新序号）。SA 侧不动。

## 要裁的

1. **（已裁，见「裁决」）**「真实程序要求付款人」是实例半边还是登记的规则：是每个真实程序登记进门禁目录 / 核对规则的一格（形照 ADR-0137 决定三的规则型目录行），还是 `PAR-CUS-*` 待提供参数——归 CC owner，一句。裁前 `ReceiveFundsFact` 保持必填。

## 裁决（2026-09-14 10:2x，通道 1 推送方按用户「授权代裁」以 CC owner 口径裁）

1. **是登记的规则一格，不是参数。** 参数登记册登的是取值（税率、口岸代码那一类），「某真实程序核对时要不要付款人」是核对规则本体的一维：按（租户、真实程序 / 申报路径）登记「付款人必备否」，形照 ADR-0137 决定三的规则型目录行——登记面 + 读口 + 「未登记 → 未配置」诚实格，全是机制半边，现在就做；哪个程序要、哪个不要是实例半边，一行不预填、不给默认值（AGENTS.md 红线）。已有的门禁 / 核对规则目录若装得下这一格就加一格，装不下另立一册——作者按 ADR-0137 落地的表形定，票面完成记录写为何。
2. **做法 3 的停格**：程序**要求**付款人而来源未提供 → 核对停在**未决（原因：要求而未提供）**并点名缺付款人（10:5x 改口：原写「待确认」，作者重量后指出 CC CONTEXT 原词是「规则要求但缺失时保持未决」，且既有 `DutyReconciliationUndecided` + reason 的形让 CLI / HTTP / inbox 零改——以 CONTEXT 原词为准；不是「不适用」——不适用是「这条维度与本程序无关」，与「该有而没有」是两格）；程序**不要求** → 付款人显式记「未提供」，核对照常进行；程序**未登记要不要** → **未决（原因：规则未配置）**，核对不进行，点名缺的是规则不是事实（同 10:5x 改口：两格都落既有 `DutyReconciliationUndecided`，靠 reason 分恢复动作，不加新 outcome）。三格的恢复动作各不相同（补事实 / 无 / 补规则），按 ADR-0029 分格。
3. **做法 1 的形**：「来源未提供」取领域上一格（显式值），不取 `(string, bool)`——它要进登记册、读回、进核对判断三处，一格比一对更难写错；SA 侧 `Payer()` 的 `(value, bool)` 是 SA 的形，CC 消费侧适配器负责译，不要求两边同形。
4. **不改的**：`0016` 不改（新迁移放宽）；SA 侧不动；`ReceiveOnAdoptedFundsFactAdapter` 不改；不拿任何别的维顶替付款人（票面红线）。**完成判据 3**（复核 sa-cc/03 越权风险点 (b)）照旧归本票。

## 参照

[03](03-cc-inbox-consumer-receives-external-funds-fact.md)（裁决与越权风险点 (b)）；[02](02-sa-external-funds-fact-adoption-hands-off-an-envelope.md)；ADR-0137 决定四。

## 完成记录

分支 `mcp1-sacc12`，基 `1ca125a5`（`mcp4-sacc12` 指针留在封存笔 `7f5db322`，只防丢、不集成）：

| SHA | 内容 |
|---|---|
| `97ba925a` | feat（通道 4 旧会话，之一）：领域一格 `domain.FundsPayer`（`ProvidedFundsPayer` / `FundsPayerNotProvided`，零值两格都不是）与封闭二值 `domain.PayerRequirement` + `Admit`；ports `ExternalFundsFactRegistration.Payer` / `AdoptedFundsFact.Payer` 换成一格、新增 `PayerRequirementRuleRegistry` / `PayerRequirementRuleView`；迁移 `0020_funds_fact_payer_may_be_unprovided_and_payer_rule.sql`（`payer_ref DROP NOT NULL` + `payer_provided_or_null` CHECK；新表 `duty_payment_payer_rule`，无默认行）；postgres `payerColumn` / `payerFromColumn` + 新文件 `payer_requirement_rule.go`；SA→CC 消费侧 `SettlementAdoptedFundsFactSource` 把 `(value, bool)` 译成一格；`ReceiveFundsFact` 不再因缺付款人`未受理`、只拒零值；真库例 `TestAFundsFactWithoutAPayerRoundTripsAsExplicitlyNotProvided` / `TestPayerRequirementRulesRoundTripPerProcedure` |
| `5ef53c6e` | feat（之二）：`VerifyDutyPaymentCommand.Procedure`；`DutyPaymentReconciliationDeps.PayerRules` 进构造门；`VerifyPayment` 两道前置之后、形成之前读规则——`PayerRequirementNotConfigured` / `PayerRequiredNotProvided` / `PayerRequirementViewUnavailable` 三个 reason 落既有 `DutyReconciliationUndecided`；`registrationjson` `procedureRef` 必填；HTTP `businessUndecided` 把付款人两格业务未决归 200 带 reason；三处装配各一行；用例 `TestThePayerDimensionIsJudgedByTheProcedureRule` / `TestThePayerRuleIsReadAfterBothPrerequisitesAndNamesItsOwnFailure`、HTTP 转写四格、CLI `TestExecuteDutyPaymentVerificationPayerGridsKeepTheirExitCodes` / `TestCommandForRequiresTheProcedureOnAVerification`、SA 消费侧替身 `unreachedDutyStores.LoadPayerRequirement` 守「入向登记不读规则」 |
| `2ed944c4` | feat（之三）：登记面 `RegisterCaseConfigurationHandler.RegisterPayerRequirement`（`RegisterPayerRequirementCommand`；同键同值`已存在`、换值`内容冲突`不顶替、形状缺格`未受理`、依赖故障未决）+ `RegisterCaseConfigurationDeps.PayerRules` / `PayerRuleView` 在 `cmd/parcel-api` 与 `cmd/parcel-customs-register` 组合根接上；用例 `register_payer_requirement_test.go` 四例 |
| （本笔） | 三处注释里的计数换点名（`dutyReconciliationAnswer` 头注「三种存储不可用」已因本票变假）；本票 Status → resolved + 本完成记录 |

**逐条对完成判据**：**1** 应用层——`TestAFundsFactWithoutAPayerIsReceivedWithThePayerRecordedAsNotProvided`：来源未提供 → `已接收`、登记里 `Payer.Provided()` 为假且 `Valid()` 为真（显式「未提供」）、同引用重放`已存在`、同引用从「未提供」换「提供了」`内容冲突`、零值`未受理`不落；三停格——`TestThePayerDimensionIsJudgedByTheProcedureRule`：要求而未提供 → 未决 `PAYER_REQUIRED_NOT_PROVIDED` 不落不交；不要求 → `DUTY_VERIFICATION_FORMED`、落一行交一封、事实的付款人仍「未提供」；没登 → 未决 `PAYER_REQUIREMENT_NOT_CONFIGURED` 不落不交。**2** 真库——`TestAFundsFactWithoutAPayerRoundTripsAsExplicitlyNotProvided`（直读 `payer_ref` 为 NULL、读回仍是那一格）、`TestPayerRequirementRulesRoundTripPerProcedure`（两值往返、重登不顶替、未登记 found=false、零值拒、无事务拒）、`TestTheReconciliationTablesRejectWhatTheDomainRejects`（CHECK 拒空白付款人 / 词形集外）；`git diff 1ca125a5 --stat -- migrations/` 只有 `0020`，**`0016` 一字未动**。**3** sa-cc/03 越权风险点 (b)「付款人在 SA 可缺席而 CC 必填，两侧不对称是有意的」——复核结论：**不对称撤销，CC 取「明确记录」**。理由是 CC CONTEXT 原句：付款人是「来源提供**或**真实程序要求的」维度，「未提供或不适用必须明确记录，规则要求但缺失时保持未决」——入向登记那一步没有「真实程序」可对，判不了要不要，拒收等于替所有程序预设「要」；所以登记照单记「未提供」，要不要在核对那一步对着（租户、监管程序）登记的规则问，缺规则停未决而不取默认。SA 侧与 03 裁决 A 保持一致，不动。

**做法逐条**：1 一格显式值，不是 `(string, bool)`（裁决 3）✓；2 新迁移放宽，`0016` 不改 ✓；3 `VerifyPayment` 三停格按裁决 2 的 10:5x 改口落——两格都是既有 `DutyReconciliationUndecided` + reason，不加新 outcome，CLI 未改一行（`dutyReconciliationAnswer` 对未决原样带 reason）、HTTP 只扩 `businessUndecided` 点名表、inbox 未改 ✓；4 `ReceiveOnAdoptedFundsFactAdapter` 零 diff ✓。**裁决逐条**：1 规则另立一册 `duty_payment_payer_rule`，键（租户、监管程序）——不挂门禁目录那族，理由在 `PayerRequirementRuleRegistry` 头注（那族按范围 / 动作 / 边界每份申报一行，而「某程序要不要付款人」是程序的属性）；登记面 + 读口 + 未配置格三样齐；表上无默认行、代码无预填 ✓；2 三停格 ✓（上）；3 译在消费侧 `SettlementAdoptedFundsFactSource` ✓；4 不改的四样 ✓。**红线**：夹具值全 `SYN-` 前缀 / 合成，无真实银行或支付字段；无任何程序的「要不要」预填；注释中文、跨文件引用符号名。

**判断项（归 CC owner）**：① **监管程序由核对的调用方交进来，编排不从范围推、也不核它与案件实际程序一致**——范围与程序不是一对一（门禁键里范围与边界并列），与范围那一维同病；越权风险点：调用方报错程序会读到别的程序的规则。② `procedureRef` 成为核对 JSON 的必填键——是裁决 1 的直接后果（规则按程序读），不是新需求；`apps/admin-web/src/pages/customs/presentation.ts` 里列核对键名的那句帮助文本因此**过时**（派单红线「零 `apps/` 改动」，未动），归 admin-web 后继一行。③ 规则登记面今天与 0019 那一格一样没有 HTTP / CLI 子命令，组合根已接；要不要给它们各开一口归后继票。④ 「不要求」那一格核对照常形成，核对记录本身不带付款人维——付款人留在资金事实登记里，核对读它只为判停格；若 owner 要核对记录也留「付款人未提供」一格，是另一张票。

**验证**（树 `D:/tops/idp-parcel-mcp4-sacc12` 切 `mcp1-sacc12`，12:3x–12:4x）：每笔 `gofmt -l` 空、`go build ./...` / `go vet` 0；不带 DSN `go test -count=1` CC 七包 + `cmd/parcel-dispatch` + `cmd/parcel-customs-register` + `cmd/parcel-api` + `internal/architecture` 全 ok（`2ed944c4`）；**带 DSN** `-p 1 -count=1` 同组 + `internal/platform/migrate` **12 ok / 0 FAIL**（`2ed944c4`，占号 / 释号已广播），探针 `TestAFundsFactWithoutAPayerRoundTripsAsExplicitlyNotProvided` / `TestPayerRequirementRulesRoundTripPerProcedure` `-v` 均 **PASS** 非 SKIP。全量与清点由推送方在重放 tip 上兑。

**作者两轴自查（子代理不可用的替代，非「非作者评审」）**：Standards——注释全中文；跨文件引用无行号；`dutyReconciliationAnswer` 头注与 `TestDutyReconciliationAnswerNamesTheUndecidedReason` 头注的依赖故障计数因本票加一格而变假，本笔换点名；`FundsPayer` / `PayerRequirement` 零值语义在类型头注写明；无新 outcome 词。Spec——做法 / 裁决 / 判据逐条如上；`0016` 不在 diff；`0020` 无 INSERT；SA 目录零 diff。

## Comments

- 2026-09-10 · 通道 5：立票（按通道 1 于 sa-cc/03 的裁决「CC 放宽（B）另立 sa-cc 新票 draft」）。未动代码。
- 2026-09-14 12:0x · 通道 1：通道 4 旧会话 crash，树内五份未提交（mtime 10:52–10:53）由推送方代封存 `chore(salvage)` `7f5db322` 推 origin；接管单预派通道 4 后用户改指推送方自办，`mcp1-sacc12` 基 `7f5db322` `reset --soft` 拆回重切。
- 2026-09-14 12:5x · 通道 1：之二 / 之三 + 本笔完成记录；Status resolved。等重放。
