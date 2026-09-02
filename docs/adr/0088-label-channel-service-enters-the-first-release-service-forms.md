# ADR-0088：面单渠道服务进入首发对客服务形态——`PAR-COM-12` 由范围裁剪改为纳入，领域模型不为范围决定让步

Status: 已接受（2026-09-01 用户裁定，经 MCP-2 广播转达；2026-09-02 MCP-2 受用户委托落文）

## Context

首发范围此前把面单渠道服务排在对客销售形态之外：[首发试点范围](../product/PILOT-SCOPE.md)写「首发服务形态确认为网络服务」「面单渠道服务不作为首发对客户销售的独立服务形态」；[参数登记册](../product/PILOT-PARAMETER-REGISTER.md)的 `PAR-COM-04` 登记为网络服务、`PAR-COM-12` 登记为本期不适用；[验收矩阵](../product/PILOT-ACCEPTANCE-MATRIX.md)的 `CPS-07` 为 `N/A`；[party-commercial CONTEXT](../domain/party-commercial/CONTEXT.md) 据此写下硬句「首发生产由 `PAR-COM-12` 明确为不适用……不得进入首发实现或生产准入关键路径」，`UC-PC-001` 的首发叠加条件与 `AT-PC-015`、`UC-PC-002` 的 `AT-PC-030` 都按不适用写成。

那次选择的性质要说准。领域发现阶段在「网络服务 / 面单渠道服务 / 组合」三选一时选了网络服务，记录的理由是面单渠道服务「上线较快，但无法验证本产品最核心的网络运营能力」（`docs/archive/DOMAIN-DISCOVERY-QA.md`）。这是一次**首发范围裁剪**，目的是让首个试点把最核心的网络能力验证出来；它从来不是产品边界判断。[产品基线](../product/PRODUCT-BASELINE.md)、PC 与 PS 两份 CONTEXT、GLOSSARY 自始把面单渠道服务建为长期产品模型——责任起点、正式承诺、终局规则、取消边界各有独立定义，[ADR-0084](./0084-label-transaction-is-an-independent-aggregate-with-two-level-results.md) 又把面单交易立为独立聚合并明写「写入方缺席是设计」。

2026-08-31 首个候选租户的需求材料到手（证据只以指纹索引登记，见 `.scratch/tenant-implementation-01/instance-register-draft.md`），其两条主力业务流之一是**纯面单渠道转售**：客户下单、取末端渠道面单、轨迹回传、结算，无自营现场作业，带多家末端渠道报价表与大量目的邮编覆盖。这与 `PAR-COM-12` 正面相抵，且不是边缘条目。

判据在[开发主线的能力范围判据](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md#能力范围判据)：「必须能指出一类真实存在的承运或渠道商业模式需要它，且该模式在上述目标客户群里是常规形态而非边缘情形。」目标客户群是经营跨境出口小包网络的物流企业；这类企业的尾程本就大量走末端渠道面单，把这段能力单独对客销售是其常见产品线，不是某一家的特例。判据说它该进。

用户于 2026-09-01 在三个选项中裁定 B（选项与代价见 `.scratch/tenant-implementation-01/decision-brief-scope-and-frontline.md` 裁决项 1）。本 ADR 是那次裁决的权威落文与适用论证；裁决内容归用户，本文不新增第二个决定。

## Decision

1. **首发服务形态改为网络服务与面单渠道服务的明确组合。** 面单渠道服务成为首发对客户销售的独立服务形态。`PAR-COM-04` 与 `PAR-COM-12` 随之修订；`CPS-07` 由 `N/A` 改为条件阻断下的 `P` 级验证；PILOT-SCOPE 中以「如果包含面单渠道服务」为前提的验证条目全部适用。
2. **领域语言一字不改。** 面单渠道服务按 PC/PS CONTEXT 既有定义进入首发：责任来源于面单交易绑定的账号与合同，不虚构运营企业收寄；运输观察从实际承运商收寄事实开始；终局按面单服务终局规则判断，不默认等待实际交付。首发范围的变化只发生在产品文档，不在领域文档新造或改写任何词条——这正是否决选项 C 的理由，见下。
3. **「一条线路」约束的作用面。** PILOT-SCOPE「首发只选择一条真实国际小包线路」及单一起运区域、单一出口/进口路径、单一目的国的约束继续作用于网络服务主链路。面单渠道服务不组织实物网络履约，其首发范围以登记的渠道产品与产品—渠道映射为界（`PAR-INT-02`），不以线路表达。本条不放宽网络服务的线路数。
4. **两种形态并存时的责任判断。** 一份委托属哪种服务形态，按其采用的服务产品版本判断，不按是否调用了渠道推断。网络服务为完成外包履约而调用渠道下单、取面单或查询结果，仍属网络服务履约协作，不因此改按面单渠道服务确定客户责任——PILOT-SCOPE 这一句原样保留。
5. **红线不放松。** 渠道报价表、渠道账号与授权、面单服务终局规则版本等一切取值属实例半边：登记状态最高到「待核验」，机制里不内置任何渠道字段名、体积系数或金额。PILOT-SCOPE 既有两条约束原样适用于面单渠道服务：不建设采购过程（不向渠道询价、竞价），不对客户形成价格承诺（渠道候选择优是运营企业内部成本决策，首发只按成本单维）。

## Consequences

- **随本 ADR 修订的权威文本**：PILOT-SCOPE（已确认目标、试点需要验证的产品原则、明确不包含、范围约束表）；参数登记册 `PAR-COM-04`、`PAR-COM-12`、`PAR-INT-02`；验收矩阵 `CPS-06`、`CPS-07`；PC CONTEXT 那句首发范围硬句；`UC-PC-001` 首发叠加条件与 `AT-PC-015`；`UC-PC-002` `AT-PC-030`；[渠道适配缝设计备忘](../design/channel-adapter-seams-design-note.md)的范围口径。ADR-0084 Context 里「产品基线明写独立面单渠道服务不进入首发生产」是它成文时的事实约束，按「不改写已接受 ADR 历史」保留原文；其裁决不受影响，且它立下的面单交易聚合正是本决定落地时要接上写入方的那一个。
- **机制半边要补的能力形状**，盘点归 `.scratch/label-channel-service-first-release/`，本 ADR 不预判各项在代码中的现状：面单渠道服务委托从提交、「面单服务正式承诺」到终局的受理链路是否逐环有执行器；末端渠道取面单的适配缝（渠道适配缝备忘指向 `parcel-shipment` 的面单交易）从墙后之物变为首发主链路；渠道候选择优归哪个上下文执行——候选来自产品—渠道映射与渠道约束，比较规则同 `PAR-NET-16`（各候选按自己的体积系数与进位算出计费重之后再比总价）；面单文件生成与修改的文档适配器；末端渠道轨迹回传的事实收编（按渠道适配缝备忘：先由事实所有者收编，不直插投影）。
- **实例半边新增或改变的登记**：`PAR-INT-02` 从「待决策」转「待提供」；每家渠道产品的报价表按 `PAR-SET-03` 的 `BUY` 价卡纪律逐份登记，价表族、体积系数、进位分段缺一不得参与比价；对客价卡按 `PAR-COM-10`；面单服务终局规则版本按 `PAR-COM-05` 与 `PAR-COM-07`。
- 网络服务主链路的试点约束（一条线路、一个锚点客户、三阶段执行与 `PN-08` 治理）不变；面单渠道服务的历史回放、影子与限量生产按同一 `PN-08` 治理走，不另设阶段模型。
- 首发范围扩大带来的实施面扩大是本决定明知的代价：末端渠道适配器从「专线的一个环节」升格为「独立产品的主链路」，多家渠道的适配与择优取证全部前置。

## Alternatives

- **A. 维持 `PAR-COM-12`，首期只做专线流。** 范围零改动、实施面最小；代价是候选租户主力业务之一不进系统、实施价值折半，而被排除的正是判据认定的常规形态。否决。
- **C. 尾程流按「专线产品的退化形态」建模试运行。** 不动范围文字而实质承接业务；代价是让「网络服务」出现无干线段、无收寄的实例，「网络服务以有效网络收寄结果为责任起点」这条硬句被迫开例外，报表与毛利口径也要另向客户解释——拿领域模型迁就范围决定，污染统一语言且不可逆。明确否决。
- **只改登记册，不改 PILOT-SCOPE。** 登记册只判实例是否就绪，可选范围归 PILOT-SCOPE 约束（两文自己写明了这个分工）；单改一处会让两份权威文本互相打架。否决。

## Links

- 修订对象：[PILOT-SCOPE](../product/PILOT-SCOPE.md)、[PILOT-PARAMETER-REGISTER](../product/PILOT-PARAMETER-REGISTER.md)、[PILOT-ACCEPTANCE-MATRIX](../product/PILOT-ACCEPTANCE-MATRIX.md)
- 判据来源：[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)「目标客户群」「能力范围判据」
- 领域定义：[party-commercial CONTEXT](../domain/party-commercial/CONTEXT.md)（面单渠道服务、面单服务终局规则、产品—渠道映射、渠道约束）、[parcel-shipment CONTEXT](../domain/parcel-shipment/CONTEXT.md)（面单服务正式承诺、面单服务终局边界）
- 相关：[ADR-0016](./0016-product-delivery-and-tenant-pilot-as-parallel-tracks.md)（产品交付与租户试点双轨）、[ADR-0084](./0084-label-transaction-is-an-independent-aggregate-with-two-level-results.md)（面单交易独立聚合）、[ADR-0072](./0072-access-channel-capability-is-shared-and-registry-shape-awaits-channel-evidence.md)（接入渠道能力归属）
- 来源：`.scratch/tenant-implementation-01/decision-brief-scope-and-frontline.md` 裁决项 1、`.scratch/tenant-implementation-01/requirement-mapping.md` 尾程流表
