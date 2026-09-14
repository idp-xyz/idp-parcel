# 资金事实新版本到达后没有编排接着做：登记册看得见 v2 回指 v1，UC-CC-009「形成新核对版本并保留原覆盖判断」在 CC 侧仍无入口

Category: enhancement
Status: draft——2026-09-14 14:0x 通道 1 立票（按 sa-cc/13 裁决 2「触发重核对不在本票……推送方据此立票」，形取 13 完成记录「后继票的形」）。只写票面未动代码；取证锚 main `0bd86d42`（sa-cc/13 重放 tip）
Blocked by: 无（[13](13-cc-correction-version-inbound-registration-and-rereconciliation.md) 已进 main：`ExternalFundsFactRegister.ListFundsFactVersions` 列得出全部版本与回指）；**要裁的 1 归 CC owner，裁前不动代码**

## 缺口（取证于 `0bd86d42`，逐符号名）

- 13 落地后：更正版本 v2 经 `settlement-accounting.external-funds-fact.adopted` 信封到 CC，`ReceiveFundsFact` 答`已接收`、`external_funds_fact_version` 落第二行回指 v1。到此为止——`external_funds_fact_consumer.go` 头注写的是「触发重核对归后继票，今天登记册上看得见、还没有编排接着做」。
- `VerifyPayment` 的三轴（覆盖 / 差额 / 有效性）与关联依据由调用方交（`VerifyDutyPaymentCommand`），调用方今天不在 `parcel-dispatch` 进程里（sa-cc/03 票面红线原句）；新版本到达时没有调用方在场，编排不算三轴。
- `DutyVerificationStore` 今天只有按幂等键的 `FindVerification`，没有「按（租户、资金事实）列全部核对版本」的读口——新版本到达时连「这条事实有没有过核对」都问不出来。
- `LoadFundsFact` 交回「最近接收的那一版」是 13 端口头注写明的临时口径：核对命令上没有版本，`duty_payment_verification` 也没有 `funds_version` 列——核对引用的是事实身份，不是事实的哪一版。

## 语言从哪里来

- CC `CONTEXT.md`「税费付款核对」：「部分付款、超额付款、错误范围、错误币种、重复付款、资金退回和付款撤销都必须保留原事实并形成新的核对判断」；集成规则「资金退回、付款撤销或外部资金事实更正只作为重新核对的来源事实，不能成为关务核对状态」。
- UC-CC-009 一致性节：「外部资金事实迟到、更正、资金退回或付款撤销时，形成新核对版本并保留原覆盖判断；不删除原付款、不按最后到达覆盖」。

## 做法（待裁后写实）

1. **触发落点**：CC 内部，`ReceiveFundsFact` 答`已接收`且该事实已有既往核对版本时——触发在消费侧适配器 `ReceiveOnAdoptedFundsFactAdapter` 之后一格另起一只编排（消费侧适配器仍只译不判，sa-cc/03 做法 3），不塞进 `ReceiveFundsFact`。
2. **要读哪几口**：`ExternalFundsFactRegister.ListFundsFactVersions`（新版内容与回指）、`DutyVerificationStore` 新增「按（租户、资金事实）列全部核对版本」读口、`DutyCollaborationStore`（协作事项仍在）、`PayerRequirementRuleView`（付款人维照 sa-cc/12 三停格）。
3. **核对按版本读**：`VerifyDutyPaymentCommand` 加 `FundsVersion`，`duty_payment_verification` 加 `funds_version` 列（新迁移，`0016` 不改），`LoadFundsFact`「最近接收」口径退役。
4. **交接不变**：新核对版本形成后走 [05](05-cc-duty-reconciliation-hands-off-to-settlement-accounting.md) 的 `DutyPaymentVerificationHandoff`（每版一封）交 SA；SA 侧采用（sa-cc/09 那族）按版本收。

## 红线

- 不按到达顺序覆盖：原核对版本一行不动（UC-CC-009 原句）。
- 三轴与关联依据不由编排猜——真实关联规则属实例半边，一行不预填。
- 消费者与消费侧适配器只译不判。

## 完成判据（待裁后写实）

1. 应用层：v1 已核对，v2 到达 → 形成新核对版本（形随裁决 1），原核对版本仍在、内容不变；无既往核对的事实新版本到达 → 不形成、不报错。
2. 真库：新迁移往返；`0016` / `0021` 一字未动；`cmd/parcel-dispatch` 真库装配用例在 13 那一格之后再扩一格（v2 到达 → 新核对版本 / 待重核对事项落行）。
3. `LoadFundsFact`「最近接收」的临时口径退役，13 端口头注那句随之改口。

## 地盘

`internal/customscompliance/{ports,application,adapters/postgres,adapters/settlementaccounting}`、`migrations/customs_compliance/`（新序号）、`cmd/parcel-dispatch/assemble_test.go`（共享文件，动前占号）。SA 侧不动。

## 要裁的

1. **新核对版本的三轴与关联依据从哪来**——归 CC owner。(a) 复用前版三轴与依据、有效性轴标 `PENDING`，形成一版「待人判」的核对；(b) 不形成核对，只登一条「待重核对」事项（新表或核对表一格）交人 / 交规则。两条路都不让编排猜三轴；差别在「形成了一版核对」还是「登了一条待办」。
2. 触发要不要区分 `corrects` 在不在（首版到达从不触发；有回指才可能有既往核对）——可由做法 1「已有既往核对版本」一条覆盖，裁时确认。

## 参照

[13](13-cc-correction-version-inbound-registration-and-rereconciliation.md) 完成记录「后继票的形」；[05](05-cc-duty-reconciliation-hands-off-to-settlement-accounting.md)；[12](12-cc-funds-fact-payer-may-be-explicitly-unprovided.md) 三停格；`internal/customscompliance/application/reconcile_duty_payment.go`（`VerifyPayment` / `ReceiveFundsFact`）；`internal/customscompliance/ports/ports.go`（`ExternalFundsFactRegister` 头注「两个读口分工」、`DutyVerificationStore`）；CC `CONTEXT.md` 税费付款核对与集成规则两句；UC-CC-009 一致性节。

## Comments

- 2026-09-14 14:0x · 通道 1：立票（推送方按 13 裁决 2 立）。只写票面，未动代码。
- 2026-09-14 16:0x · 通道 1（接管会话）：sa-cc/13 补评审 ← 通道 4 Spec 非阻断 2 给本票的一句——`VerifyPayment` 在生产有调用方（`cmd/parcel-api/assemble_customs_registration.go` `transactionalDutyPaymentVerificationRegistration.VerifyPayment`、`cmd/parcel-customs-register/translate.go`）；13 之后同一事实 ≥ 2 版时付款人维读的是最近接收那一版，而 `duty_payment_verification` 键（租户、税费、资金事实、范围、指纹）里没有资金版本——v2 到达后调用方以同三轴同依据重核落`已存在`，形成不出「新核对版本」。**本票立字时把「核对身份缺资金版本」写成完成判据**（做法 3 已接，判据里点名）。同一张键的另一维（程序）在 [22](22-cc-duty-verification-procedure-is-caller-asserted-and-not-recorded.md)，两票若同期在途改键合一笔。
