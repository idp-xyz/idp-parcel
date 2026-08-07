# ADR-0012: 在 idp-parcel 内建立小包计价上下文

Status: Accepted  
Date: 2026-08-07  
Supersedes: [ADR-0011](./0011-parcel-pricing-context-within-idp-parcel.md)

## Context

本仓曾收到一套外部提供的国际小包计费参考设计，内容覆盖价卡与费率表、重量与附加费规则、`BUY`/`SELL`/`INTERNAL` 价格方向、确定性计算、回放和价卡治理。它是外部输入，不是本仓规格，也不是本仓权威来源。

这些能力不能原样复制为一套新系统：`party-commercial` 已拥有合同、商业适用性和价格方案绑定，`settlement-accounting` 已拥有费用、应收应付、对账和核销；`node-operations`、`parcel-shipment`、`network-routing` 与 `transport-fulfillment` 也分别拥有测量、包裹、路由和履约事实。因此需要明确“可执行的价卡与纯计价”这一缺失的业务边界，同时避免出现第二套合同、费用、账本或主数据。

该参考设计随附的治理案例集依赖一份源价卡，其源文件身份只能由仓库所有者断言、不能由哈希复核，案例声明的源内容差异也尚未取得业务裁决。因此本仓只吸收其业务语义和治理约束，不把其金额样例当作生产证据。源完整性判断的权威在[小包计价上下文](../domain/parcel-pricing/CONTEXT.md)，不在本记录。

[ADR-0011](./0011-parcel-pricing-context-within-idp-parcel.md) 记录了与本记录相同的边界决策，但它的 Links 把上述外部参考设计目录列为可点击的依据，读者必须打开仓库外的参考树才能读懂决策来由。这与“单一权威”红线冲突：一项决策只应有一处定义，且该处必须自洽。本记录取代 ADR-0011，把同一决策改写为不依赖任何外部目录的形式；ADR-0011 的正文保留为历史，不作修改。

本记录不改变 ADR-0011 已经确立的边界，也不新增计价能力范围。

## Decision

在 `idp-parcel` 内建立 `parcel-pricing` 限界上下文。它是领域和代码模块边界，不是独立产品、独立数据库、独立部署单元或微服务；实现继续遵循 [ADR-0009](./0009-go-modular-monolith-and-versioned-bento-contracts.md) 的模块化单体约束。

职责分层如下：

- `party-commercial` 拥有客户合同、供应商协议、商业适用范围、价格方向授权、定价方案绑定及结算政策。
- `parcel-pricing` 拥有小包 `PricingPlanVersion`、`RateTableVersion`、计价政策、价格规则工件、受控发布版本清单和纯 `PricingEvaluation`；它只消费其他上下文提供的事实快照，不拥有测量、地址、包裹、履约或财务结果。
- `settlement-accounting` 消费已解析的商业依据和 `PricingEvaluation`，形成计费重量、计算展开、费用、应收/预期成本、调整、对账和核销；它不能发布或修改价卡。

`BUY`、`SELL` 和 `INTERNAL` 是相互隔离的价格方向。销售价可以显式引用一次已冻结的采购评价，但不能读取可变的“当前成本”或隐式继承采购价。所有发布后的价卡、规则工件、版本清单和评价依据不可覆盖；更正通过新版本、重放或追加金额调整表达。

外部参考设计对本仓不具备权威性。本仓已吸收的内容以各 `CONTEXT.md` 与 ADR 为准；参考设计与本仓权威文档冲突时，以本仓权威文档为准。任何规则都必须能在本仓内读懂，不得以“见参考设计某节”作为依据。

## Consequences

- 小包价卡可以独立演进、校验、解释和回放，且不污染合同和账务边界。
- `UC-PC-001/002` 需要管理商业绑定并返回已解析定价依据；`UC-SA-002` 需要消费评价并保存采用解释；`UC-SA-004` 需要引用采购评价和承运商账单认定。
- 首期需要维护价表来源、适用期、分区、重量区间、币种、附加费、取整和可复算样例；未提供真实参数时保持 `待参数化`，不制造生产默认价。
- 纯计价结果不是费用、应收、应付、付款或毛利；正式 Quote 生命周期、完整多币种指数体系和任意脚本扩展延后到有业务证据时再决策。
- 外部参考设计的吸收记录降级为历史索引：它说明本仓当初从何处取得语义，不构成规则依据。删除该参考树不应使任何现行规则失去出处；发现某条规则只能靠参考树读懂时，属于缺陷，应把内容补进本仓权威文档而不是恢复外部引用。

## Alternatives considered

- **把所有计价内容留在 `party-commercial`**：合同与价卡绑定可以保留，但可执行规则、回放和发布治理会与商业适用性混在一起，无法清楚消费测量和履约事实。
- **把所有计价内容放进 `settlement-accounting`**：会让费用结果的所有者同时成为价格规则发布者，难以保证 BUY/SELL 隔离和纯计算可回放。
- **复制外部参考设计为独立计费平台或微服务**：会重复合同、产品、渠道账号、费用、对账和承运商主张所有权，并超出当前产品设计阶段的范围。
- **只删除 ADR-0011 的外部链接**：能让链接消失，但会改写已接受记录的正文历史，且删链接不等于内容自洽，读者反而失去决策来由；不采用。

## Links

- [领域上下文地图](../domain/CONTEXT-MAP.md)
- [小包计价上下文](../domain/parcel-pricing/CONTEXT.md)
- [ADR-0009：采用 Go 模块化单体并复用版本化 Bento 技术合同](./0009-go-modular-monolith-and-versioned-bento-contracts.md)
- [`PP-S03-W01` Golden Case 源证据闭合工作单](../design/pp-s03-w01-golden-case-source-evidence-request.md)
- [ADR-0011：在 idp-parcel 内建立小包计价上下文](./0011-parcel-pricing-context-within-idp-parcel.md)（已被本记录取代，仅作历史）
