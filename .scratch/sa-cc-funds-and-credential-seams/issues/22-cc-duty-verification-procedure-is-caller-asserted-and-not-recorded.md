# 税费付款核对的监管程序由调用方断言、编排不核它与案件一致、核对记录不带程序：错报 `procedureRef` 可绕过「规则要求而缺失保持未决」，事后也看不出付款人维按哪个程序判

Category: enhancement
Status: draft——2026-09-14 16:0x 通道 1（接管会话）立票（sa-cc/12 补评审 ← 通道 3 Standards 非阻断 ② + Spec 非阻断 ①，评审建议「合一张」；与 12 完成记录判断项 ① ④ 同根；归 CC owner）。只写票面未动代码；取证锚 main `01974923`
Blocked by: 无（12 已进 main；要裁的两条归 CC owner）

## 缺口（取证于 `01974923`，逐符号名）

- `internal/customscompliance/application/reconcile_duty_payment.go` `VerifyDutyPaymentCommand.Procedure`（sa-cc/12 之二加入）：付款人维的规则按它读 `PayerRequirementRuleView`；头注自认「范围与程序不是一对一……编排不从范围推、也不核它与案件实际程序一致」。
- `internal/customscompliance/domain/duty_release.go` `VerifyDutyPayment(...)` 与 `DutyVerificationKey` 都不带 Procedure；迁移 `0016` `duty_payment_verification` 主键 `(tenant_id, duty_ref, funds_ref, scope_ref, version_digest)` 里没有程序——形成的核对**事后看不出付款人维是按哪个程序的规则判的**（ADR-0137 决定一「记录带依据引用」同族缺一维）。
- 后果（评审量的）：调用方错报 `procedureRef` 且该程序登了 `NOT_REQUIRED` → 对一条无付款人的事实形成核对并同事务交结算意图，而案件实际程序要求付款人——「要求而缺失保持未决」被绕；反向错报只多停一次未决，无害。
- 为什么 12 没堵：`VerifyDutyPaymentCommand` 的 Coverage / Delta / Validity / Scope / Basis 本就全由登记方断言、编排不从任一维推另一维（`DutyPaymentVerificationFromJSON` 头注原话），登记方今天已能形成任意核对，Procedure 没给它新能力，信任模型未变。堵它需要一条「范围 / 案件 → 当前有效程序」读口，CC 今天没有：`0019` 门禁规则表主键 `(tenant_id, scope_ref, action, boundary_ref)`，程序不在键里、由申报各自交进来；`domain.CustomsCase` 有 `Procedure` 一格，但核对编排今天不读案件。

## 语言从哪里来

- CC `CONTEXT.md` 税费付款核对词条：付款人是「来源提供或**真实程序**要求的」维度——「真实程序」是案件的属性，不是登记方每次核对时口头报的一个字符串。
- ADR-0137 决定一：门禁判断是登记的事实，记录带依据引用——核对按哪个程序的规则判，是依据的一部分。
- ADR-0029：结果按恢复动作分格——「报的程序与案件程序不一致」的恢复动作是改申报或改案件，与「规则未配置」（补规则）不同格。

## 做法（待裁后写实）

1. **程序来源**（见「要裁的」1）：a) 编排按（租户、范围 / 案件）经新读口取当前有效程序，`procedureRef` 从命令里退役或降为可选核对项；b) 保留调用方交、加一致性核——不一致停在新的未决 reason（如 `PayerProcedureMismatch`，落既有 `DutyReconciliationUndecided`），点名两侧各是什么。
2. **核对记录带程序**（见「要裁的」2）：`VerifyDutyPayment` 与核对记录加 `Procedure`；是否进 `DutyVerificationKey` / 主键随裁决——进键则新迁移改键（`0016` 不改），不进键则只加列 + 读口带出。
3. `registrationjson` / HTTP / CLI 随 1 的取向改口径：a) 路 `procedureRef` 不再必填；b) 路照旧必填。
4. `apps/admin-web` 核对键名帮助文本（12 判断项 ② 记为过时）随本票一并改——若 a) 路则那句直接退役。

## 红线

- 不为任何程序预填「要不要付款人」；不拿范围顶替程序（范围与程序不是一对一，12 头注原话）。
- 「不一致」停未决交人，不自动以任一侧为准。
- `0016` / `0020` 不改；新迁移序号重取。
- 不改 SA。

## 完成判据（待裁后写实）

1. 应用层：报的程序与案件 / 读口程序不一致 → 未决并点名两侧（或 a) 路：命令无程序、编排自取）；一致 → 三停格照 12。
2. 核对记录读回带程序；真库往返；`0016` / `0020` 零 diff。
3. `registrationjson` / HTTP / CLI 口径与 1 一致；`apps/admin-web` 帮助文本同笔改正（12 判断项 ② 收口）。

## 地盘

`internal/customscompliance/{domain,ports,application,adapters/postgres,adapters/registrationjson,adapters/http}`、`migrations/customs_compliance/`（新序号）、`cmd/parcel-api` / `cmd/parcel-customs-register` / `cmd/parcel-dispatch` 装配（共享，动前占号）、`apps/admin-web/src/pages/customs/presentation.ts` 一句。SA 侧不动。

## 要裁的

1. **程序从哪来**——归 CC owner：a) 编排经读口从案件 / 范围取当前有效程序（CC 拥有案件，`CustomsCase.Procedure` 在；要新立「范围 → 案件 → 程序」读口，且一范围多案件时怎么办要一并裁）；b) 调用方交 + 编排核一致（读口同样要有，只是命令仍带程序）。裁前 12 的形不动。
2. **核对身份要不要带程序**——归 CC owner：进 `DutyVerificationKey`（同三轴同依据但程序不同算两份核对，主键改）还是只作记录列（同键不同程序 → `已存在`）。与 [19](19-cc-new-funds-fact-version-forms-a-new-verification-version.md)「核对身份缺资金版本」是同一张键的两维，若两票同期在途，改键合一笔、迁移序号各自重取。

## 参照

[12](12-cc-funds-fact-payer-may-be-explicitly-unprovided.md)（裁决 1 / 2、完成记录判断项 ① ② ④、15:47 补评审 Standards ② + Spec ①）；[19](19-cc-new-funds-fact-version-forms-a-new-verification-version.md)（同一张键的另一维）；ADR-0137 决定一；ADR-0029；`internal/customscompliance/application/reconcile_duty_payment.go` `VerifyDutyPaymentCommand` 头注；`internal/customscompliance/domain/duty_release.go` `VerifyDutyPayment` / `DutyVerificationKey`；`internal/customscompliance/domain/customs_case.go` `CustomsCase.Procedure`；`migrations/customs_compliance/0016_duty_payment_reconciliation.sql`、`0019_duty_payment_gate_rule_and_reading.sql`。

## Comments

- 2026-09-14 16:0x · 通道 1（接管会话）：立票（sa-cc/12 补评审 ← 通道 3：Standards ②「核对事后看不出付款人维是按哪个程序的规则判的」+ Spec ①「判断项 ① 评为非阻断……立票时与 Standards ② 合一张」）。只写票面，未动代码。能力边界：核过 `VerifyDutyPaymentCommand.Procedure`、`VerifyDutyPayment` 签名、`0016` / `0019` 主键、`CustomsCase.Procedure` 存在；**没读**案件与范围今天怎么关联（一范围一案件还是多案件）——「要裁的」1 那半靠 owner 与作者开工时量。
