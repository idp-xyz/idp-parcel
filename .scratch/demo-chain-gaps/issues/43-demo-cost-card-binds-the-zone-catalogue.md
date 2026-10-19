# 43 演示成本卡绑分区目录：干线发生项往下的 BUY 评价越过 ZONE_UNRESOLVED，形成供应商预期成本

Category: enhancement
Status: ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [27](./27-transport-charge-occurrences-are-registered-and-handed-off.md) 的隔离评审：27 的完成判据「BUY 评价 → ADR-0200 形成供应商预期成本」没有走通，移交本票。Blocked by 解除之前不认领。
Blocked by: [routing-first-cut/20](../../routing-first-cut/issues/20-bound-cost-cards-fold-zones-through-the-parcel-postal-route.md)
地盘：同票 [30](./30-buy-cost-card-declares-amount-rounding.md)——`scripts/demo-seeds/data/pricing/price-card-cn-sg-cost.json`、`scripts/demo-seeds/seedgen` 里成本卡与模板夹具同摘要的那一处、`seed.sh` 一行（若需要）；README「已知边界」里票 27 记的那一条。
出处：票 27 完成记录「偏离与待裁」第 2 条（甲路）；票 30 完成记录「待裁」；[ADR-0189](../../../docs/adr/0189-leg-pricing-zones-are-supplied-per-card-and-unbound-cards-take-a-registered-leg-zone.md) 决定一第一条、越权风险点 3。

## 现象

演示走到干线 `ACTUAL_FULFILLMENT` 发生项登成（201）之后，结算发起一份 BUY 请求、计价形成一份评价，评价答 `PENDING` / `ZONE_UNRESOLVED`（「the price card binds no zone catalogue and the input names no zone」）。结算按 ADR-0200 把待判断入账，供应商预期成本 0 行。

## 证据（票 27 实测，一次性库，证据 `S`）

- 演示成本卡 `price-card-cn-sg-cost.json` 不绑分区目录。
- 计价按发生项造的输入取成员包裹的申报与邮编路线（`200000`→`018000`），不带分区。卡没绑目录、输入也没给分区时，分区解不出，评价不形成。

## 三类

租户取值（演示租户的成本卡绑哪一份目录）。机制已在：ADR-0189 决定一第一条「卡绑目录、经邮编路线解区」。规模 S。

## 最小的诚实修法（票 27 的甲路）

演示成本卡绑分区目录 `SYN-CAT-ZONE-CN-SG`。按发生项造的输入已带邮编路线，经目录解出 `Z1`，与今天三段各 `Z1` 同价。改成本卡与 seedgen 里同摘要的模板夹具；在用解析多于一版即适用冲突，所以照票 30 的先例就地改 `v1`，不另发一版。

**为什么要等 routing-first-cut/20**：在它落地之前，路由的候选成本取数不经读数补全，绑了目录的卡恒答待判断，初始路由会退回 `CANDIDATE_COSTS_PENDING`——修了 BUY 这一头，演示的第一步反而断了。

## 完成判据

- 演示动线：干线参与结束后登 `ACTUAL_FULFILLMENT` 答 201 → BUY 请求 → 计价评价形成（不再是 `ZONE_UNRESOLVED`）→ ADR-0200 形成一行供应商预期成本。证据 `S`。
- 初始路由仍形成，不退回 `CANDIDATE_COSTS_PENDING`。
- 一次性库 `seed.sh` 退 0；`./scripts/demo-seeds/...` 绿。
- README「已知边界」里票 27 记的那一条随之删去或改写。

## 不在本票

乙路（按发生项取段计价区域：实际履约段经计划段连到 NR 的线路段，再读段计价区域）是跨 PP、SA、NR 的产品机制，要新 ADR，留给那几位 owner。真实租户若用按线路计价、不绑目录的干线卡，迟早要它；ADR-0189 越权风险点 3 一直挂着。

## 既有票

[routing-first-cut/20](../../routing-first-cut/issues/20-bound-cost-cards-fold-zones-through-the-parcel-postal-route.md)（前置）；[30](./30-buy-cost-card-declares-amount-rounding.md)（同一张卡的金额取整）。
