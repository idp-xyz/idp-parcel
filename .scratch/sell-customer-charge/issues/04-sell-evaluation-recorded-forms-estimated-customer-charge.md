# 04 结算：SELL 评价已记录经费用项目映射形成预估客户费用

Category: enhancement
Status: ready-for-agent——2026-10-19 Claude Code 会话立。
Blocked by: [01](./01-sell-request-identity-and-registers.md)；端到端验收要 [03](./03-pricing-consumes-sell-evaluation-requests.md)
地盘：`internal/settlementaccounting` 的 application（形成编排外层）、adapters/parcelpricing（SELL 消费门与评价读口）、adapters/inbox；`cmd/parcel-dispatch` 路由与装配。
出处：[ADR-0190](../../../docs/adr/0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md) 决定五、六；ADR-0169 决定一、二。

## 做什么

1. SELL 消费门不再一律答未登记：按评价回指读回 SELL 请求，按映射册把全部费用行代码映到同一个费用项目；映不全或多于一个答待判断（`AT-SA-174`）。
2. 费用标识由请求标识确定性派生，调用既有的 `FormSellCustomerChargeHandler`。
3. 评价未成形、不可计价、冲突：在请求册上记状态，入账不重投。
4. 派发：`parcel-pricing.evaluation.recorded` 改成扇出，BUY 门与 SELL 门各一路、各自的 inbox 与未决哨兵。

## 完成判据

- 应用层用例：映到一个项目形成；映不全、映到多个、未登记映射各一条；重放不多形成。
- 真库装配：一封 SELL 评价已记录信封形成一笔预估客户费用，金额整组出自评价。
