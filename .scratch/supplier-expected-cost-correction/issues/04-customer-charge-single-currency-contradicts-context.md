# `CustomerCharge` 只带单一币种与金额，与 CONTEXT 的币种三件组对不上

Category: bug
Status: needs-triage

由 [ADR-0067](../../../docs/adr/0067-cost-correction-restates-the-whole-evaluation-result.md) 的
Consequences 划出：「`CustomerCharge` 只带单一币种与金额，没有原币/结算币这一对，与 CONTEXT 的
『每条费用分别保存原币金额、合同结算币金额及换算依据』对不上。本记录不裁那一处，另记。」本票即那处「另记」。

## 病灶

[SA CONTEXT](../../../docs/domain/settlement-accounting/CONTEXT.md)「赔付、追偿、税费、币种与法人」
一节明文：

> 每条费用分别保存原币金额、合同结算币金额及换算依据。

以及 ADR-0067 补进的同节新句：原币金额、合同结算币金额和换算依据是**从同一个评价采用来的一组**，
不是可以各自停在不同评价上的三件。

`internal/settlementaccounting/domain` 的 `CustomerCharge` 今天只有单一币种与单一金额，三件组
一件都放不下。供应商侧（`SupplierExpectedCost`）三件俱全，客户侧缺位。

## 不在本票内

- 本票只记录模型与 CONTEXT 的失配，不提方案——客户费用是否真的需要三件组、还是 CONTEXT 那句
  对客户侧另有解读，属领域 owner 裁断。
- ADR-0067 已明确**不裁**这一处；不要把该 ADR 当本票的答案引用。

## 影响面

`CustomerCharge` 有无应用层调用方、有无落库数据未取证；triage 时先补这两问。

## Comments

- 2026-08-20 · MCP-1：随 ADR-0067 入库创建，防「另记」落空。
