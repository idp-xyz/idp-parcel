# 80 集运单元的权威运输交接不推进成员的节点控制：单元控制与成员「随单元受控」的规则没立

Category: enhancement
Status: needs-triage——2026-10-19 票 [61](./61-node-control-transfers-out-on-an-authoritative-handover.md) 的作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [ADR-0214](../../../docs/adr/0214-node-control-transfers-out-and-in-on-an-authoritative-transport-handover.md) 决定二与越权风险点「集运单元的交接暂不推进」；方案「后继」一节「集运单元交接：单元控制与成员「随单元受控」；NR 逐成员复核」那一条（[handover-dispatch-plan](../handover-dispatch-plan.md)）。
Blocked by: 无

## 现象

- 交接对象是本租户的集运单元时，NO 只入账一行 `CONSOLIDATION_UNIT_PENDING_RULE`，不推进任何控制。包裹号与集运单元号同串时也按单元判。
- 整袋交出是网络里的常态：一袋几十件交给干线承运商，袋里每件的节点控制都一直在身。直到每件另有一次逐件交接，或人工处置。
- TF CONTEXT「运输交接与实物控制」写明：封签连续且相符的封闭集运单元被有效交接时，可以形成单元控制，以及成员「随单元受控」的派生判断；封签缺失、破损或不一致时，成员控制形成待确认或冲突。NO 这一侧还没有对应的规则：
  - 单元有没有自己的节点控制；
  - 成员控制怎样随单元转出；
  - 封签不符时成员怎么记。

## 要裁的

- **单元控制是不是 NO 的一种控制**：今天 `PhysicalControl` 逐件成立，类型上没有批次维度。单元控制是另立一种，还是只派生「成员随单元」的转出？
- **成员随单元转出的判据**：要不要读 NO 自己的封装快照与封签记录，判「封签连续且相符」；判不了时成员记什么——入账待确认，还是照转并记信息格。
- **NR 逐成员复核**：交接触发的路由复核（票 [63](./63-network-routing-reassesses-on-a-handover-into-a-node.md)）对集运单元今天也不逐成员。
- 三类：判据与派生是机制；封签核对的严格程度可能是产品策略。

## 规模

L
