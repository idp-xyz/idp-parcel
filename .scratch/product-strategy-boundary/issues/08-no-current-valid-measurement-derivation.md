# 08 node-operations：当前有效实测的派生方法

Category: enhancement
Status: needs-triage——2026-09-24 通道 4 随票 02 立（登记册逐行拆分划出的产品策略，NO 一张）；以实际测量登记册为前提
Blocked by: `pp-pricing-input-seams/04`（NO 实际测量登记册与「仍有效的实际测量」只读口，draft）
地盘：node-operations 领域与应用层「当前有效实测」的派生；消费方 parcel-pricing 的「实测多于一条」处置随之改。
出处：[票 02](./02-split-parameter-register-and-retriage-deferrals.md)——[参数登记册](../../../docs/product/PILOT-PARAMETER-REGISTER.md) `PAR-NET-06`「当前有效实测规则」；NO `CONTEXT.md`「当前有效实测按明确业务规则从仍有效的实际测量派生」；[ADR-0146](../../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定二。重新定性的暂缓：[`pp-pricing-input-seams/04`](../../pp-pricing-input-seams/issues/04-no-actual-measurement-registry-and-valid-measurements-read-port.md)「做什么」里「不派生当前有效实测」一条，把「明确业务规则（按来源 / 位置 / 设备定证明力）」判为租户的、属实例半边。

## 做什么

1. 当前有效实测的派生方法做成内置策略族：按来源、位置、设备与校准有效性定证明力，多条仍有效实测怎样取一（形态举例：设备优先序、同设备取最近、人工复核覆盖）。租户只选形态、填优先序与容差；没选时照旧由消费方停「输入不可得：实测多于一条」。
2. 消费方拿到恰一条照旧直接用。

## 不做

- 不种「最近一次」之类默认；不替租户定设备优先序或容差。
- 实际测量登记册与只读口本身归 `pp-pricing-input-seams/04`。

## 完成判据

- 选了形态的租户上，多条仍有效实测派生出唯一的当前有效实测（带测试）；没选的租户行为不变；登记册 `PAR-NET-06` 那句同步收短。
