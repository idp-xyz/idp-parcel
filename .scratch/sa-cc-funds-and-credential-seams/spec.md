# `settlement-accounting` ↔ `customs-compliance` 资金与凭证那条缝：七张机制半边的票

Category: chore
Status: in-progress——2026-09-10 15:1x 通道 4 立目录与七张子票（全部 draft，task-9880bbc9）；子票全 resolved 本 spec 才 resolved。只写票面未动代码；取证锚 `3f485e97`

## 从哪里分出来

[unresolved-review-20260904/remaining-work-dd5ed934.md](../unresolved-review-20260904/remaining-work-dd5ed934.md) 表里两行判「仍余且机制半边且无票」：

- **五-7** SA 两件——BUY 评价 → SA 的 inbox 消费者；SA 外部资金事实采用**发信封** + CC 侧 inbox 消费者。
- **五-8** CC 四件机制——UC-CC-003 步 7「记录凭证门禁」的持久化；UC-CC-009 核对向 `settlement-accounting` 的交接（outbox 意图）；放行门禁核对读付款核对；凭证登记 / 关税协作 / 资金事实 / 付款核对的在线登记面与 admin 写面。

两处的源头都是 `mechanism-executor-triage` 三张实现票收口时写下的「不在本票 / 留给后继票的」（[06](../mechanism-executor-triage/issues/06-sa-four-executors-behind-existing-uc-steps.md)「不在本票」节、[07](../mechanism-executor-triage/issues/07-cc-four-executors-behind-existing-uc-steps.md)「没做、留给后继票的」五条）。那两张票 2026-09-04 resolved 之后这些后继没有任何票面在盯，a3a4814 与 dd5ed934 两次重核都量到它们仍在。

**为什么并成一个目录**：七件全在同一条缝上（五-7 的第二件拆成「发信封」与「消费者」两张）——PP 评价进 SA 形成预期成本、SA 采用的外部资金事实进 CC 做税费付款核对、CC 的核对结果回 SA 判实际代垫、凭证与核对各自的门禁持久化、以及让租户能登记这些事实的入口。拆散到 SA / CC 各自目录，谁先谁后的边就没人写。

## 取证（`3f485e97`，逐条可重跑）

- `git ls-files internal/settlementaccounting/adapters` → 只有 `http` / `parcelpricing` / `partycommercial` / `postgres`，**无 `inbox`**；`git ls-files internal/customscompliance/adapters/inbox` → 空。
- `git grep -n 'eventing.EventType' -- internal/settlementaccounting/adapters/postgres` → 十七个事件类型常量，无一是「外部资金事实已采用」。
- `git grep -E 'SupplierExpectedCost|BuyEvaluationAdoption' -- cmd/` → 只命中 `cmd/parcel-api/unwired_orchestration.go` 的读面桩。
- `git grep -E 'RegisterCredential|DutyPaymentReconciliation|ReceiveFundsFact|JudgeCredentialApplicability' -- cmd/` → 零。
- `cmd/parcel-api/endpoints.go` 关务行：`/customs/external-results`、四类目录登记（解释规则 / 门禁目录 / 候选口岸 / 申报路径）、`/customs-case-requirement-registrations`——没有凭证、协作、资金事实、付款核对的登记端点。
- `internal/customscompliance/application/judge_credential_applicability.go` 头注：「本用例只判、不记：步 7 写的『记录凭证门禁』那半留给就绪判断的编排」。
- `internal/customscompliance/application/verify_release_gate.go` 不引用 `DutyVerificationStore`（`git grep -n DutyVerificationStore -- internal/customscompliance/application/verify_release_gate.go` 零）。
- `internal/customscompliance/application/reconcile_duty_payment.go` 三方法 `FormCollaboration` / `ReceiveFundsFact` / `VerifyPayment` 在；无任何 `HandOff*` 调用（`git grep -n -i handoff -- internal/customscompliance/application/reconcile_duty_payment.go` 零）。

## 子票

| 票 | 一件机制 | 上下文 | Blocked by |
|---|---|---|---|
| [01](issues/01-buy-evaluation-to-sa-inbox-consumer.md) | `parcel-pricing.evaluation.recorded` → SA inbox 消费者 → `FormSupplierExpectedCostHandler` | SA | 无（另有一格要裁，见票） |
| [02](issues/02-sa-external-funds-fact-adoption-hands-off-an-envelope.md) | SA 外部资金事实采用同事务经 Outbox 发信封 | SA | 无 |
| [03](issues/03-cc-inbox-consumer-receives-external-funds-fact.md) | CC inbox 消费者收那封信封 → `ReceiveFundsFact` | CC | 02 |
| [04](issues/04-cc-credential-gate-persists-in-readiness-assessment.md) | UC-CC-003 步 7「记录凭证门禁」的持久化 | CC | 无 |
| [05](issues/05-cc-duty-reconciliation-hands-off-to-settlement-accounting.md) | UC-CC-009 核对向 SA 的交接（outbox 意图） | CC | 无 |
| [06](issues/06-cc-release-gate-reads-duty-payment-verification.md) | 放行门禁核对读付款核对 | CC | 无 |
| [07](issues/07-cc-credential-and-duty-reconciliation-registration-faces.md) | 凭证 / 协作 / 资金事实 / 付款核对的在线登记面（CLI 与端点） | CC | 无（要裁 CLI 还是端点，见票） |

**留空位、不立票的一件**：放行层的代码映射 `PAR-CUS-01/02`（`receive_external_result.go` 注释「真实代码映射属实例半边，没有它接入侧拆不出种类」）——登记册待提供，到位后接入侧译装出 `ReleaseContent`，编排不改。它在这里只占一行，不成票。

## 边界（七票共用）

- 七件全是**机制半边**：让事实能进、能出、能记。真实资金事实、真实凭证、真实付款条件与关联规则、真实程序的门禁清单属实例半边（`PAR-CUS-*` / `PAR-SET-*`），一律不写默认值。
- 不拆两侧已落的形状：SA 的 `map_external_funds.go` 四格负向代数、CC 的 `reconcile_duty_payment.go` 三方法与三表分层（UC-CC-009「七层对象必须分离」）都照用；本目录只接线、只补记、只开入口。
- 信封形状照仓内既有 handoff（`ports.*HandoffIntent` + `adapters/postgres/*_handoff.go` + `EnqueueOnce`），分区主体在各票「要裁的」里单列，不在 spec 代裁。
- 不改任何已 resolved 票的 Status；mech/06、mech/07 的「不在本票」节原样留着，本目录是它们的去处。

## Comments

- 2026-09-10 · 通道 4：立目录（task-9880bbc9）。七张子票的「要裁的」条数汇总见完工报；为零的由推送方直接转 ready-for-agent。
