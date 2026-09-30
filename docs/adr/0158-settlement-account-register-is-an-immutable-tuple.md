# ADR-0158：结算账户登记册是一行固定属性，接受前控制只查应收

Status: Accepted（2026-09-30。本项是机制，接受依据是 ADR-0146 决定一「机制与产品策略同属产品交付轨道，开发方现在就做」。）
Date: 2026-09-30

## Context

受理前财务控制的作用域源已经接上 `PolicyBackedControlScopeSource`。它向 `SettlementAccountDirectory` 要一个结算账户，生产装配把目录留空，因为 settlement-accounting 只有 `SettlementAccountID` 值对象，没有登记册。租户因此无处登记 `PAR-SET-01`。

CONTEXT 术语「结算账户」写明它固定责任法人、结算相对方、收付方向、结算币种和结算政策；实际付款责任方与结算相对方不同时另行保存。同一节还要求每个账户写明对账周期、业务时区、截单时刻和付款条件。`PAR-SET-01` 写明不同责任法人、相对方、方向、币种或政策不得合并。

作用域缝交给目录的是解析回显：政策、方式、责任法人、相对方、币种。回显里没有收付方向。

## Decision

**一、结算账户是登记册上的一行，不是带修订或有效区间的版本。** 一行固定责任法人、结算相对方、收付方向、结算币种、结算政策。同一租户里这五格再出现一次即拒绝。同一账户标识再登不同内容也拒绝，已落的行不改。改五格中的任何一格是另一个账户。

**二、对账周期、业务时区、截单时刻、付款条件、合同或责任依据是这一行的必填原文。** 本上下文不解析它们，也不填默认。实际付款责任方只在与结算相对方不同时另存；写成同一方则拒。

**三、接受前控制的目录只查收付方向为应收的那一行。** 键是回显里的责任法人、相对方、币种、结算政策，加上应收。方式不进键：预付或账期是政策适用范围的属性。应付行在同一册上，不是这一问的答案。册上没有相符的应收行，目录答未找到，控制停在 `CONTROL_SCOPE_NOT_CONFIGURED`。

**四、登记命令是 `parcel-settlement-register settlement-account`。** 载荷译装在 `registrationjson`，与该入口既有命令同一条路。不登任何租户的行，不造默认账户。

## 候选与反方

- **给账户加有效区间，按判断时点取当时有效的一行。** 本缝要的是「这一组固定属性对应哪个账户」。区间是另一项租户取值，本记录不发明它的形状。否决。
- **目录不限收付方向，五格相符就返回。** 回显没有方向。同一组的应收与应付是两个账户，不限方向就会把应付拿去冻货主的钱，或在两行之间无法取舍。否决。
- **对账周期等四格先不进册，等格式定了再加列。** CONTEXT 要求账户写明它们。缺了这四格，租户仍然无处登记 `PAR-SET-01` 点名的那些条件。存原文、不解析，格式以后仍可收紧。否决。

## Consequences

- 生产装配的 `SettlementAccountDirectory` 接这本登记册。空册与「没有相符应收行」都停在 `CONTROL_SCOPE_NOT_CONFIGURED`。
- 控制金额源仍不在本记录里接。
- 迁移是 `settlement_accounting/0022_settlement_account.sql`，按目录嵌入。

## 越权风险点

1. 接受前控制只查应收。归 parcel-shipment 与 settlement-accounting 的 owner 复核。
2. 对账四格存原文、不校验格式。归 settlement-accounting 的 owner 复核。
3. 账户没有有效区间。归 settlement-accounting 的 owner 复核。

## Links

- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)
- [settlement-accounting CONTEXT](../domain/settlement-accounting/CONTEXT.md) 术语「结算账户」
- 参数登记册 `PAR-SET-01`
- 票 `.scratch/product-strategy-boundary/issues/16-mechanism-gaps-without-a-ticket.md` 第 2 项
