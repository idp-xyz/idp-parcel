# ADR-0158：结算账户登记册是一行固定属性，接受前控制只查应收

Status: Accepted（2026-09-30。本项是机制，接受依据是 ADR-0146 决定一「机制与产品策略同属产品交付轨道，开发方现在就做」。）
Date: 2026-09-30

## Context

受理前财务控制的作用域源已经接上 `PolicyBackedControlScopeSource`。它向 `SettlementAccountDirectory` 要一个结算账户，生产装配把目录留空，因为 settlement-accounting 只有 `SettlementAccountID` 值对象，没有登记册。租户因此无处登记 `PAR-SET-01`。

CONTEXT 术语「结算账户」写明它固定责任法人、结算相对方、收付方向、结算币种和结算政策；实际付款责任方与结算相对方不同时另行保存。登记册上这五格再加「同一租户」唯一，且不设修订。`PAR-SET-01` 写明不同责任法人、相对方、方向、币种或政策不得合并。对账周期、业务时区、截单时刻属于对账安排，付款条件属于商业结算政策，不在这一行上再存一份。

作用域缝交给目录的回显把结算政策分成对象与版本。回显里没有收付方向。生产侧把回显拼成「对象/版本」是为了快照原样写回；账户这一格要的是对象。

## Decision

**一、结算账户是登记册上的一行，不是带修订或有效区间的版本。** 一行固定责任法人、结算相对方、收付方向、结算币种、结算政策对象。同一租户里这五格再出现一次即拒绝。同一账户标识再登不同内容也拒绝，已落的行不改。改五格中的任何一格是另一个账户。账户绑政策对象，不绑某一版：采用 v1 与采用 v2 查的是同一行。新版若改了法人、相对方、币种或方向，五格已经是另一行。

**二、这一行另存的原文只有合同或责任依据。** 本上下文不解析它，也不填默认。对账周期、业务时区、截单时刻、付款条件不进这一行：前三格是对账安排，付款条件是商业结算政策，再存一份就有了第二权威。实际付款责任方只在与结算相对方不同时另存；写成同一方则拒。

**三、接受前控制的目录只查收付方向为应收的那一行。** 键是回显里的责任法人、相对方、币种、结算政策对象，加上应收。目录读回显的对象标识，不把「对象/版本」整串当键，也不在适配器里按斜杠切开。方式不进键：预付或账期是政策适用范围的属性。应付行在同一册上，不是这一问的答案。册上没有相符的应收行，目录答未找到，控制停在 `CONTROL_SCOPE_NOT_CONFIGURED`。

**四、登记命令是 `parcel-settlement-register settlement-account`。** 载荷译装在 `registrationjson`，与该入口既有命令同一条路。不登任何租户的行，不造默认账户。

## 候选与反方

- **给账户加有效区间，按判断时点取当时有效的一行。** 本缝要的是「这一组固定属性对应哪个账户」。区间是另一项租户取值，本记录不发明它的形状。否决。
- **目录不限收付方向，五格相符就返回。** 回显没有方向。同一组的应收与应付是两个账户，不限方向就会把应付拿去冻货主的钱，或在两行之间无法取舍。否决。
- **把对账周期、业务时区、截单时刻、付款条件一并写进账户行。** 这四格不是 `PAR-SET-01` 点名的账户键。周期、时区、截单属于对账安排，付款条件属于商业结算政策。账户行再存一份，就有了第二权威。否决。
- **目录按「对象/版本」整串比对，或在适配器里按斜杠切开回显。** 整串比对会让按对象标识登记的账户永远对不上，政策换版也会丢掉账户。斜杠不是对象身份的边界。对象与版本在回显类型上分开，已落库的快照仍按整串重建。否决。

## Consequences

- 生产装配的 `SettlementAccountDirectory` 接这本登记册。空册与「没有相符应收行」都停在 `CONTROL_SCOPE_NOT_CONFIGURED`。
- 本记录部分停用 [ADR-0081](./0081-acceptance-judgment-is-envelope-driven.md) 决定六里「结算账户目录留 nil」这一格。该决定其余各格不由本记录停用。
- 控制金额源仍不在本记录里接。
- 迁移是 `settlement_accounting/0022_settlement_account.sql`，按目录嵌入。

## 越权风险点

1. 接受前控制只查应收。归 parcel-shipment 与 settlement-accounting 的 owner 复核。
2. 账户绑政策对象、不绑某一版。归 parcel-shipment 与 settlement-accounting 的 owner 复核。
3. 账户没有有效区间。归 settlement-accounting 的 owner 复核。

## Links

- [ADR-0081](./0081-acceptance-judgment-is-envelope-driven.md)：本记录部分停用其决定六的结算账户目录留 nil
- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)
- [settlement-accounting CONTEXT](../domain/settlement-accounting/CONTEXT.md) 术语「结算账户」
- 参数登记册 `PAR-SET-01`
- 票 `.scratch/product-strategy-boundary/issues/16-mechanism-gaps-without-a-ticket.md` 第 2 项
