# ADR-0011: 在 idp-parcel 内建立小包计价上下文

Status: Superseded by [ADR-0012](./0012-parcel-pricing-context-within-idp-parcel.md)  
Date: 2026-08-06  
Superseded: 2026-08-07

## Context

`foo` 包含一套面向国际小包网络的价卡、费率表、重量与附加费规则、BUY/SELL/INTERNAL 价格方向、确定性计算、回放和价卡治理设计。这些能力不能原样复制为一套新系统：`party-commercial` 已拥有合同、商业适用性和价格方案绑定，`settlement-accounting` 已拥有费用、应收应付、对账和核销；`node-operations`、`parcel-shipment`、`network-routing` 与 `transport-fulfillment` 也分别拥有测量、包裹、路由和履约事实。

因此需要明确“可执行的价卡与纯计价”这一缺失的业务边界，同时避免出现第二套合同、费用、账本或主数据。`foo` 的 Golden Cases 和源价卡证据尚未完全修复，当前只能吸收其业务语义和治理约束，不能把其金额样例直接当作生产证据。

## Decision

在 `idp-parcel` 内建立 `parcel-pricing` 限界上下文。它是领域和代码模块边界，不是独立产品、独立数据库、独立部署单元或微服务；实现继续遵循 [ADR-0009](./0009-go-modular-monolith-and-versioned-bento-contracts.md) 的模块化单体约束。

职责分层如下：

- `party-commercial` 拥有客户合同、供应商协议、商业适用范围、价格方向授权、定价方案绑定及结算政策。
- `parcel-pricing` 拥有小包 `PricingPlanVersion`、`RateTableVersion`、计价政策、价格规则工件、受控发布版本清单和纯 `PricingEvaluation`；它只消费其他上下文提供的事实快照，不拥有测量、地址、包裹、履约或财务结果。
- `settlement-accounting` 消费已解析的商业依据和 `PricingEvaluation`，形成计费重量、计算展开、费用、应收/预期成本、调整、对账和核销；它不能发布或修改价卡。

`BUY`、`SELL` 和 `INTERNAL` 是相互隔离的价格方向。销售价可以显式引用一次已冻结的采购评价，但不能读取可变的“当前成本”或隐式继承采购价。所有发布后的价卡、规则工件、版本清单和评价依据不可覆盖；更正通过新版本、重放或追加金额调整表达。

## Consequences

- 小包价卡可以独立演进、校验、解释和回放，且不污染合同和账务边界。
- `UC-PC-001/002` 需要管理商业绑定并返回已解析定价依据；`UC-SA-002` 需要消费评价并保存采用解释；`UC-SA-004` 需要引用采购评价和承运商账单认定。
- 首期需要维护价表来源、适用期、分区、重量区间、币种、附加费、取整和可复算样例；未提供真实参数时保持 `待参数化`，不制造生产默认价。
- 纯计价结果不是费用、应收、应付、付款或毛利；正式 Quote 生命周期、完整多币种指数体系和任意脚本扩展延后到有业务证据时再决策。

## Alternatives considered

- **把所有计价内容留在 `party-commercial`**：合同与价卡绑定可以保留，但可执行规则、回放和发布治理会与商业适用性混在一起，无法清楚消费测量和履约事实。
- **把所有计价内容放进 `settlement-accounting`**：会让费用结果的所有者同时成为价格规则发布者，难以保证 BUY/SELL 隔离和纯计算可回放。
- **复制 `foo` 作为独立计费平台或微服务**：会重复合同、产品、渠道账号、费用、对账和承运商主张所有权，并超出当前产品设计阶段的范围。

## Links

- [领域上下文地图](../domain/CONTEXT-MAP.md)
- [小包计价上下文](../domain/parcel-pricing/CONTEXT.md)
- [foo 领域模型 V1.0.1](../../foo/国际小包计费领域模型_V1.0.1_终审版.md)
- [foo Rating Runtime 计算语义 V1.0.1](../../foo/Rating_Runtime计算语义规范_V1.0.1_终审版.md)
- [foo Golden Cases JSON](../../foo/international-parcel-rating-golden-cases-v1.0.1.json)
