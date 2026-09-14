# SA 的外部资金事实一事实一行：更正版本在提供方自己就存不下，sa-cc/02 裁决 2「更正再发一封」没有落地路径

Category: bug
Status: draft——2026-09-14 14:0x 通道 1 立票（sa-cc/13 完成记录判断项 ① 的发现；归 SA owner）。只写票面未动代码；取证锚 main `0bd86d42`
Blocked by: 无（要裁的归 SA owner）

## 缺口（取证于 `0bd86d42`，逐符号名）

- `migrations/settlement_accounting/0004_external_funds.sql`：`external_funds_fact` 主键 `(tenant_id, fact_id)`——一事实一行；后续 SA 迁移无一改过这道主键。
- `internal/settlementaccounting/adapters/postgres/external_funds_fact.go` `ExternalFundsFacts.Save`：`ON CONFLICT DO NOTHING`，同引用第二次 `Save` 答 `FundsFactAlreadyAdopted`——更正版本（同 `fact_id`、新 `version`、`corrects` 回指前版）落不进去。
- `internal/settlementaccounting/domain/external_funds.go` `ExternalFundsFact.CorrectAmount`：结构拷贝出回指前版的新版本——**在 SA 无生产调用点**（`git grep 'CorrectAmount(' -- internal/settlementaccounting ':!*_test.go'` 零命中）。
- `adopted_funds_fact_view.go` `AdoptedFundsFactView` 头注自己写「库里将来一事实多行（更正版本各成一行）时，这一句照样只答被问的那一版」——「将来」今天还没到。
- 于是 sa-cc/02 裁决 2「更正 / 撤销复用同一事件类型 `settlement-accounting.external-funds-fact.adopted` 再发一封、ID `<租户>/funds-fact/<事实>/<版本>`、载荷带可缺席的 `corrects`」在 SA 侧没有能产生第二封的写路径；CC 侧（sa-cc/13）已能收——`cmd/parcel-dispatch` 的正例只好另用一条事实、v1 经 CC 写口预铺、SA 只存 v2。

## 语言从哪里来

- SA `CONTEXT.md` 外部资金事实那句（sa-cc/03 裁决 2 补过「含来源提供的付款人」半句）与「它不修改外部资金事实」——更正是新版本不是修改，所以要存得下多版本。
- AT-SA-114（`CorrectAmount` 头注引）：差额与核销的重算随新有效版本另行进行。

## 做法（待裁后写实）

1. SA `external_funds_fact` 加版本维：主键加 `version`、或版本子表（形由作者定，照 CC 0021 的取舍写为何）；`FundsFactKey` 随之带版本或另立读口；新迁移，`0004` 不改。
2. 生产入口：一条「采用更正版本」的用例 / 命令（`CorrectAmount` 的调用方），走 sa-cc/02 的 handoff 再发一封。
3. `AdoptedFundsFactView` 不改（已按版本查）。

## 红线

- 不覆盖前版；更正是新版本回指前版（`Corrects`）。
- 不改 CC；CC 已能按（引用 + 版本）收。

## 完成判据（待裁后写实）

1. SA 应用层：v1 已采用，更正 v2 → 新一行回指 v1、再发一封；同版本重放`已采用`。
2. 真库：新迁移往返；`0004` 一字未动。
3. `cmd/parcel-dispatch` 正例改回同一事实两版都在 SA（sa-cc/13 那一格里「另用一条事实」的绕法退役）。

## 地盘

`internal/settlementaccounting/{domain,ports,application,adapters/postgres}`、`migrations/settlement_accounting/`（新序号）、`cmd/parcel-dispatch/assemble_test.go`（共享文件，动前占号）。CC 侧不动。

## 要裁的

1. 主键加版本还是版本子表——归 SA owner（SA 侧 `duty_payment_verification_adoption` 等表若外键到 `external_funds_fact (tenant_id, fact_id)`，取舍同 CC 0021 头注那段）。
2. 「采用更正版本」的入口长在哪（CLI / HTTP / 只有 inbox 消费者）——归 SA owner。

## 参照

[02](02-sa-external-funds-fact-adoption-hands-off-an-envelope.md) 裁决 2；[03](03-cc-inbox-consumer-receives-external-funds-fact.md)；[13](13-cc-correction-version-inbound-registration-and-rereconciliation.md) 完成记录判断项 ①；`migrations/customs_compliance/0021_external_funds_fact_versions.sql` 头注（CC 侧取版本子表的理由）。

## Comments

- 2026-09-14 14:0x · 通道 1：立票（sa-cc/13 实施中发现）。只写票面，未动代码。
