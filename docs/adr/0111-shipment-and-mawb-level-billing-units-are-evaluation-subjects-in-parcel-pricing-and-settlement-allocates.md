# ADR-0111: 按票、按 MAWB 计费的项目在 `parcel-pricing` 有自己的评价主体与聚合方式，`settlement-accounting` 用既有分摊把票级 / 主单级金额归因到包裹；主单主体引用 `transport-fulfillment` 的承运总单身份，那本登记册今天不存在

Status: Accepted
Date: 2026-09-04

## Context

清关报价表的计费单位有四种：按 MAWB、按票、按 KG、按件。后两种今天装得下——按 KG 走 `RateTableFamilyUnitPrice`，按件走 `FixedChargeRule`（`AggregationPerPackage`）。前两种装不下（票 [price-card-shape-gaps/03](../../.scratch/price-card-shape-gaps/issues/03-shipment-and-mawb-level-billing-units-have-no-evaluation-subject.md)，核于 `a17bfac`，本记录复核于 `512b419` 各条仍成立）：`AggregationMode` 唯一取值 `AggregationPerPackage` 且在 `PricingPlanVersion` 构造时硬写；`EvaluationSubjectKind` 只有已受理包裹与试算对象；`PricingInputSnapshot` 一份对应一个包裹的重量与尺寸。一笔「每 MAWB 若干」的清关费按包裹评价，要么每个包裹各收一次，要么由调用方平摊后写成定额——摊法没人声明过。

两条硬句框住了落点：[settlement-accounting CONTEXT](../domain/settlement-accounting/CONTEXT.md) 首段「消费 `parcel-pricing` 的纯计价评价，但不拥有可执行价卡或第二套算价规则」；[CONTEXT-MAP](../domain/CONTEXT-MAP.md) 把「承运总单、运输舱单及外部承运凭证的身份和版本」判给 `transport-fulfillment`。前者决定算价只能在 PP；后者决定主单主体的身份从哪来。

## Decision

**一、票级与主单级计费单位落 `parcel-pricing`：加聚合方式与评价主体（票面选项 1）。** `AggregationMode` 加「逐委托」与「逐主单」两格；`EvaluationSubjectKind` 加「委托」与「承运总单」两种主体；费用行上标聚合单位，读面据此知道一行金额是「每包裹」还是「每票」「每主单」。算价规则仍只在 PP 一处——SA CONTEXT 那句硬句一字不动。

**二、票级 / 主单级评价的输入快照带成员清单。** 一份快照对应一个委托或一个主单，携带成员包裹引用与合计量（件数、总实重、总体积重——按重量策略派生的合计计价重量是评价内的中间结果，与逐包裹时同形）；按 KG 的行在这类主体上按合计计价重量查表，按件的行按件数乘定额，按票 / 按主单的行取定额一次。**成员清单是评价输入的一部分**，进语义摘要：同一主单成员变了就是另一次评价。

**三、归因到包裹归 `settlement-accounting` 的既有分摊（票面选项 3 = 1 + SA 既有分摊）。** PP 交出的是票级 / 主单级金额与成员清单；SA 按自己 CONTEXT 已有的「成本分摊结果 / 未分摊余额」机制、按版本化分摊规则把它归因到包裹作经营归因。分摊规则是 SA 的实例参数，PP 不带任何摊法。

**四、主体身份各引其所有者，本上下文不铸。** 委托主体引用 `parcel-shipment` 拥有的委托身份（已受理的委托与其提交版本）；承运总单主体引用 `transport-fulfillment` 拥有的承运总单身份与版本（CONTEXT-MAP 判给它）。**TF 今天没有承运总单登记册**——`query_transport_fulfillment_records.go` 与票 admin-skeleton-closure-batch/05 都记明「承运总单与运输舱单一区在存储上还没有登记册」。因此本记录的主单级那一半**形状定、身份来源缺**：PP 侧的主体种类、聚合方式与快照形状照常落地，但生产上形成一次主单级评价要等 TF 立册；在那之前 E2 转换工具对按 MAWB 的行如实列「未转换：等 TF 承运总单登记册」，**不平摊、不折进按件定额**。

**五、CONTEXT「评价对象」硬句改口，规范化换号一次。** 「有且只有两种」改为四种，新增两种的身份归属如上；`AggregationMode` 与 `EvaluationSubjectKind` 两个封闭集扩格，快照与规范化文档跟，`canonicalization` 按 [ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md) 换号，与 ADR-0107 / 0108 / 0109 / 0110 同期实施合并为一次。设计文档 [`pp-pricing-rule-model-final-design.md`](../design/pp-pricing-rule-model-final-design.md)「聚合方式仍只有逐包裹」那句由本记录取代——该文档属设计交接，按 AGENTS 冲突时以 CONTEXT / ADR 为准，不在本记录里改它。

## Consequences

- **「每 MAWB 若干」第一次有了能复算的评价**：金额在 PP 按卡算，成员在快照里冻着，归因在 SA 按声明的规则摊——三件各在其位。
- **票级评价与「试算对象」是两件事**：试算对象是包裹尚未存在时的临时身份，委托主体是已受理委托的永久身份；前者不进结算的红线不变，后者进结算要经 SA 分摊。
- **主单级那一半会在实现后停在「身份来源缺」上**，这是如实的停，不是缺陷：TF 立承运总单登记册是一张应立而未立的票，归 TF owner；立了之后 PP 不改，只是快照里的主单引用有了真实来源。
- 代价：`PricingInputSnapshot` 多一种形状（成员清单 vs 单包裹），构造门按主体种类拒错配；评价读面、成本分值桥、SA-c 缝要能读到聚合单位与成员清单。
- 代价：两个封闭集扩格，三步法或单独 worktree。
- **本记录不填任何取值**：没有任何真实清关费率进仓；SYN 夹具只记 `S`。

## Alternatives considered

- **票面选项 2：SA 以费用发生项 + 分摊承接，PP 不动。** 否决：费率本身还是得有地方算——「每 MAWB 若干」这个数从哪张卡查出来仍然没有答案；SA 不拥有算价规则，等于把问题挪到 SA 而不是解决，且正面撞 SA CONTEXT 首段硬句。
- **由调用方平摊后写成按件定额。** 否决：摊法没人声明过，是在没人声明过的地方发明规则；且把主单级事实伪装成包裹级事实，争议时无从复算。
- **等 TF 承运总单登记册立好再一并裁。** 否决：票级那一半（委托身份在 PS 已有）今天就能做，且 PP 侧的形状不依赖 TF 册的内部形状，只依赖「有一个身份可引」；等的终点不可预期，先把机制半边做实、身份来源那一格如实留空——与 ADR-0090 / 0092 对「等第一家真渠道」的处置同形。
- **只加聚合方式、不加评价主体（仍按包裹评价、行上标「每票」）。** 否决：一次评价对应一个包裹时「每票」的金额会在每个包裹上各出现一次，读面无从判断该收一次还是 N 次——主体不对，标签救不回来。

## Links

- [票 price-card-shape-gaps/03](../../.scratch/price-card-shape-gaps/issues/03-shipment-and-mawb-level-billing-units-have-no-evaluation-subject.md)：三条候选与取证
- [E1 形状核对报告](../../.scratch/price-card-shape-gaps/report.md)：第 17 项
- [settlement-accounting CONTEXT](../domain/settlement-accounting/CONTEXT.md)：「不拥有可执行价卡或第二套算价规则」——算价只能在 PP 的依据
- [CONTEXT-MAP](../domain/CONTEXT-MAP.md)：承运总单身份归 `transport-fulfillment`
- [parcel-pricing CONTEXT](../domain/parcel-pricing/CONTEXT.md)：「评价对象」词条改口
- [ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md)：规范化版本换号
- [票 admin-skeleton-closure-batch/05](../../.scratch/admin-skeleton-closure-batch/issues/05-node-operations-and-transport-fulfillment-read-faces.md)：承运总单一区「无处可登」的记录
