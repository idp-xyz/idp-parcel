# ADR-0366：接单与择优都不设毛利门槛，低毛利只在预估口径经营毛利上按租户阈值提示；时效只作择优的资格条件，不进排序、不与成本加权

Status: Accepted（2026-11-05，Cursor 会话代产品 owner、SA、PS 与 NR owner 决（代决）：用户原话与授权同 [ADR-0364](./0364-bracket-bounds-total-bounds-basis-and-shipment-weight-aggregation-are-declared-by-the-price-card-and-a-catalogue-may-register-an-explicit-rest-row.md)；起因是用户当日提供、未入本仓的原型《用户手册与计费计算说明》v1.1 里的「最低毛利率」门槛与「最短运输时效」择优策略。越权风险点见文末，待 SA、PS 与 NR owner 复核。）<!-- 索引注记 -->｜**判据**：毛利不是接受判断、面单渠道择优或路由择优的资格条件，也不是排序维度——同一票的售价不随候选变，成本最低即毛利最高；结算在预估口径经营毛利上按租户登记的阈值标出低毛利，精确十进制比较、不先取整，答低于、不低于、判不出或未配置，不阻断、不形成费用；时效只作资格（客户承诺、截单与衔接的时间可行性），不进排序、不与成本加权，也不作受控改路的改善形态，面单渠道一侧由产品—渠道映射圈定

Date: 2026-11-05

## Context

取证于 main `dde9b0c65`，只记 `S`。

- 原型手册在物流产品上设「最低毛利率」（示例 8%），报价与下单时排除低于门槛的方案；毛利率先保留两位小数再与门槛比，销售为零按 0% 处理。择优策略有「最低采购成本」与「最短运输时效」两种，后者比较方案时效范围的起始天数，相同再比成本。
- 本仓的接受判断由商业唯一解析、接受前可达性与接受前财务控制组成（[产品基线](../product/PRODUCT-BASELINE.md)「委托、包裹与标识」写商业唯一解析；[UC-PS-001](../application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md) 写另外两项，并写明结算「不自行增设接单门槛」），没有毛利一项；接受前只有逐件试算估价（[ADR-0179](./0179-pre-acceptance-control-amount-is-estimated-by-settlement-from-per-parcel-sell-trial-evaluations.md)），BUY 成本还可能待判断。
- 面单渠道择优首发只有成本单维（[PS CONTEXT「渠道择优决定」](../domain/parcel-shipment/CONTEXT.md)、参数登记册 `PAR-NET-16`）。路由择优先排除硬约束、再满足客户承诺、截单和衔接可行性，最后按成本单维择优（[NR CONTEXT](../domain/network-routing/CONTEXT.md)），同一处写着「时效、可靠性等其他维度保持未配置」；`PAR-NET-16` 写的是等 `PAR-NET-01` 的时效分布与历史异常数据之后再按新版本纳入。
- 经营毛利只能派生、按阶段成对采用；预估口径成对采用当前有效客户预估费用与当前有效供应商预期成本，缺失成本不得记为零（[SA CONTEXT「经营指标、人工调整与系统边界」](../domain/settlement-accounting/CONTEXT.md)）。

## Decision

**一、接单不设毛利门槛。**（产品策略）

- 售价由客户合同与价卡定，接受是在履行合同。拿受理前的估计值拒一个合同内的订单，等于把「成本没算出来」变成「拒绝客户」。接受判断的组成不变。

**二、择优不设毛利门槛，毛利也不作排序维度。**（产品策略）

- 同一票的售价按价卡定、不随渠道或路由候选变时（客户锁定了渠道产品时根本不择优），成本最低就是毛利最高。门槛改变不了选中谁，只会在全部候选都低于门槛时多出「无人可选」；那时该由人换产品、调价或认亏，不由系统拒单。
- 成本加成的 `SELL` 卡（PP CONTEXT「`SELL` 只有在规则明确时才能引用一次冻结的 `BUY` 评价」）售价随所选候选的成本变。按比例加成时毛利率不随候选变，门槛照样改变不了选择；毛利额随成本变大，若按毛利额择优会偏向更贵的候选、抬高客户价。择优照旧按成本，不按毛利额。

**三、低毛利在预估口径经营毛利上提示。**（产品策略；机制见三类）

- 毛利率 = 预估口径经营毛利 ÷ 当前有效客户预估费用；与租户登记的阈值比，精确十进制、不先取整，等于阈值不算低于。
- 提示格答四种之一：低于阈值；不低于阈值；判不出（成对组成未齐，或客户预估费用为零——售价为零时毛利率没有定义，不按 0% 读）；未配置（租户没登阈值）。
- 提示是读面上的派生判断：不形成费用、调整或异常案件，不阻断接单、择优、交接或结算。阈值是租户取值，产品不内置；登记口与适用范围的形状（全租户，或按服务产品、客户账户收窄）随实现定。

**四、时效只作择优的资格条件，不进排序、不与成本加权。**（产品策略）

- 路由侧维持现行判法：先排除硬约束，再满足客户承诺、截单和衔接可行性，最后按排序形态择优；时效不另作排序维度。受控改路的改善形态同理：首版照旧是成本改善，时效改善不作改善形态（NR CONTEXT「首版形态是成本改善」）。
- 面单渠道侧，候选是否满足产品时效由产品—渠道映射圈定，那是租户取值，本记录不加机制。
- 不做成本与时效加权：加权要给「快一天值多少钱」定一个没有依据的数，结果也难向人解释。产品要卖「最快」，做成另一个服务产品，映射只纳入快的渠道。
- 稳定性、赔付条件等维度不在本记录内，照 `PAR-NET-16`。

三类：

- **机制**：提示格的派生与读面、阈值登记册。
- **产品策略**：决定一至四。
- **租户取值**：阈值及其适用范围；产品—渠道映射纳入哪些渠道。

## 对已接受记录的作用

无条款停用。下列现行文档里时效那一项改按决定四，随本记录同笔改写；稳定性、可靠性与赔付条件照旧：

- [首发试点范围](../product/PILOT-SCOPE.md)「**首发只按成本单维择优**」一句。
- 参数登记册 `PAR-NET-16` 的约束栏与需提供栏。
- NR CONTEXT「时效、可靠性等其他维度保持未配置」一句，与「首版形态是成本改善」一句。

## Consequences

- 接受判断、面单渠道择优与路由择优的代码不动。
- 结算多一个读面判断与一本阈值册，落在 [UC-SA-006](../application/settlement-accounting/UC-SA-006-ALLOCATE-COSTS-AND-DERIVE-OPERATING-RESULTS.md)；实现之前，经营毛利照旧只给数，不给提示。
- 原型手册的「最低毛利率」与「最短运输时效」两项不进本产品；它们对应的经营关切由提示与服务产品分档承接。

## Alternatives considered

- **接单前按估价挡低毛利（原型手册的做法）。** 否决：见决定一。
- **择优排除低于门槛的候选。** 否决：见决定二，不改变选择，只多一种无人可选。
- **时效作第一排序键，成本次之。** 否决：在客户承诺之内让利换没人付钱的时效；要卖快就另立服务产品。
- **成本与时效按权重合成一个分数。** 否决：权重没有依据，分数也解释不了为什么选了这一条。

## 越权风险点

1. **毛利不进任何门槛。**（决定一、二）租户若有合同明示「低于某毛利不接」的条款，另立。归 PS、SA owner。
2. **提示只挂预估口径。**（决定三）已确认与已结算口径要不要也提示，另立。归 SA owner。
3. **售价为零答判不出。**（决定三）归 SA owner。
4. **成本并列不用时效收尾。**（决定四）最低成本并列照旧交授权角色裁决。归 NR、PS owner。
5. **面单渠道的时效资格只靠映射。**（决定四）渠道时效变差时要人改映射，没有自动降级。归 PS、PC owner。
6. **成本加成卡下按成本择优，毛利额不是最大。**（决定二）这是有意的：客户付的价随成本走，按成本择优对客户有利；租户要按毛利额择优，另立。归 PS、NR、PP owner。

## Links

- [ADR-0179](./0179-pre-acceptance-control-amount-is-estimated-by-settlement-from-per-parcel-sell-trial-evaluations.md)：受理前只有逐件试算估价
- [ADR-0148](./0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md)：决定四，路由候选按成本单维择优
- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)：三类；决定七首个排序策略沿成本单维
- [参数登记册](../product/PILOT-PARAMETER-REGISTER.md) `PAR-NET-16`：比价候选与择优规则
- [ADR-0364](./0364-bracket-bounds-total-bounds-basis-and-shipment-weight-aggregation-are-declared-by-the-price-card-and-a-catalogue-may-register-an-explicit-rest-row.md)、[ADR-0365](./0365-an-origin-dimension-zone-catalogue-takes-the-origin-the-price-card-declares-sender-postal-code-or-the-billing-origin-of-the-legs-start-node.md)：同批代决
- [冻结清单「记录」](../../.scratch/launch-line-freeze-2026-10-27.md)：实现项与建议分级
