# UC-CC-009 的税费付款核对形成后不向 `settlement-accounting` 交接：`VerifyPayment` 落库即止，SA 的实际代垫成立判断拿不到「关务税费及付款核对」这一项输入

Category: enhancement
Status: ready-for-agent——2026-09-10 17:3x 通道 5 按通道 1 派单 task-9a2ff746（用户授权代裁）写入裁决：分区主体取租户 / 申报范围、SA 侧消费者另立 [09](09-sa-consumes-duty-payment-verification-envelope-into-advance-recovery.md)（见「要裁的」下「裁决」），本票再无待裁问题。此前 draft——2026-09-10 通道 4 立票（task-9880bbc9），只写票面未动代码；取证锚 `3f485e97`
Blocked by: 无

## 缺口（取证于 `3f485e97`）

- `internal/customscompliance/application/reconcile_duty_payment.go` 的 `VerifyPayment`（步 7）写 `ports.DutyVerificationStore`，**不调任何 `HandOff*`**（`git grep -n -i handoff -- internal/customscompliance/application/reconcile_duty_payment.go` 零）。
- CC 今天的 handoff 只有三种：`CustomsCaseHandoff` / `DeclarationSubmissionHandoff` / `CaseClosureHandoff`（`git grep -n -i 'HandOff' -- internal/customscompliance/application`）。
- SA 侧 `UC-SA-001` 的实际代垫判断按 CONTEXT 要「关务税费及付款核对」作输入之一；今天没有信封把它送过去。
- mech/07「没做」第 3 条：「UC-CC-009 步 8 核对向 `settlement-accounting` 的交接（outbox 意图）……一张」。

## 语言从哪里来

- SA `CONTEXT.md`：「实际代垫成立判断使用付款方身份、外部资金事实、关务税费及付款核对和合同责任，不由任一单项输入直接推导」；「结算输入已接收：固定关务交接、税费、付款核对……的采用版本；接收不表示代垫或回收已经成立。」
- CC `CONTEXT.md`：「税费付款核对……分别判断覆盖状态、差额和事实有效性，不形成实际付款、客户回收或监管放行」；「本上下文只拥有监管核定、法定义务、税费付款核对和放行门禁判断及其与外部资金事实的可追溯关系」。
- UC-CC-009 标题所指的「结算交接」（范围节第 9 条：「……财务接入、结算交接和恢复测试骨架」）。

## 做法

1. `ports.DutyPaymentVerificationHandoff` + `adapters/postgres/duty_payment_verification_handoff.go`，事件类型 `customs-compliance.duty-payment-verification.formed`，载荷只带引用（租户、申报范围、核对版本、监管核定税费版本、资金事实引用）；形照 CC 既有 `case_closure_handoff` 那一族。
2. `VerifyPayment` 在核对**形成**（新版本）那一格同事务交意图；`已存在` 不重发；未决两格（资金事实未接收 / 协作事项未形成）不发。
3. SA 侧消费者**不在本票**（见「要裁的」第 2 条）；本票只让信封出得去。

## 红线

- 信封不带金额结论（覆盖 / 差额 / 有效性三态由 SA 按引用读 CC 读口——CC 是权威）。
- 不把「核对形成」解释成「代垫成立」：CONTEXT 明写任何单项不能推导。
- 不动三表分层。

## 完成判据

1. 应用层：形成 → 一封；`已存在` → 不发；未决 → 不发；交接失败 → 技术未形成 + 续办引用（仓内既有形）。
2. 真库：入队一封、分区键如裁、`EnqueueOnce` 不翻倍。
3. 清点 tip 重生成（outbox handoff +1）。

## 地盘

`internal/customscompliance/ports/`、`internal/customscompliance/application/reconcile_duty_payment.go`、`internal/customscompliance/adapters/postgres/`（新文件）、`reconcile_duty_payment` 的装配点（先核在哪，`git grep -n DutyPaymentReconciliationHandler -- cmd/` 今天零——装配随 [07](07-cc-credential-and-duty-reconciliation-registration-faces.md) 的登记面一起落，本票的信封在那之前只有测试路径能触发，票面如实记）。

## 要裁的

1. **分区主体**：租户 / 申报范围（同范围的核对版本有序）还是租户 / 监管核定税费版本。归 CC owner，一句。
2. **SA 侧消费者归谁立**：本目录再加一张，还是并进 SA 的 `UC-SA-001` 实际代垫编排票（今天那条编排是否存在要先核 `assess_advance_recovery.go`）。归 SA owner。

### 裁决

（通道 1 推送方裁、通道 5 写入，2026-09-10 17:0x；task-b941ce87 分类两条均 A、task-9a2ff746 落笔。A 类不落 ADR。）

- **1 → 租户 / 申报范围。** 同票 02 的口径：「ID 管幂等、分区键管顺序：同一主体的版本链排一条队」（[ADR-0069](../../../docs/adr/0069-customs-case-chain-ordering-absorbed-by-reread-and-retry.md) 决定二）；核对版本按申报范围逐笔形成（CC CONTEXT「按真实程序逐范围分别形成覆盖状态、差额状态和有效性状态」），有版本链的主体是「这一申报范围的付款核对」，不是税费版本（税费更正只是新核对版本的来源之一）。主体名「租户 / 申报范围」按 [ADR-0074](../../../docs/adr/0074-tf-object-partitions-carry-a-port-segment-apart-from-ve-parcel-partitions.md) 决定五进中心登记表，随本票落地同笔。
- **2 → 本目录加一张 [09](09-sa-consumes-duty-payment-verification-envelope-into-advance-recovery.md)「SA 消费付款核对信封 → 接 `assess_advance_recovery.go`」。** `internal/settlementaccounting/application/assess_advance_recovery.go` 已存在（通道 5 于 66cad4c4 核），消费者接它即可，不重开已 resolved 的 UC-SA-001 票；SA CONTEXT「不由任一单项输入直接推导」仍由那条编排守，消费者只译不判。

## 参照

[mech/07](../../mechanism-executor-triage/issues/07-cc-four-executors-behind-existing-uc-steps.md) CC-c 与「没做」第 3 条；UC-CC-009；UC-SA-001；[remaining-work-dd5ed934.md](../../unresolved-review-20260904/remaining-work-dd5ed934.md) 五-8 ③；`internal/customscompliance/application/close_customs_case.go` 的 `handOffClosure`（本上下文 handoff 先例）。

## Comments

- 2026-09-10 · 通道 4：立票。未动代码。
