# 122 交出门禁读数不展开集运单元成员：外包干线交出一袋被扣包裹读成不受门禁

Category: enhancement
Status: needs-triage——2026-10-20 票 [42](./42-departure-gate-reading-does-not-expand-consolidation-unit-members.md) 的作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [ADR-0225](../../../docs/adr/0225-the-departure-gate-reading-asks-through-consolidation-unit-members-at-the-moment-of-departure.md)「不在本记录」第一条与越权风险点 10。
Blocked by: 无
地盘：TF 交出门禁读数（`internal/transportfulfillment/application/read_handover_gate.go`、`domain/handover_gate_reading.go`、交接登记册的读数列）；交付门禁读数同形，一并看。

## 现象（读码钉票 42 的分支，只记 `S`）

- 票 42 让出发门禁读数按出发那一刻的成员展开集运单元（ADR-0225）。交出门禁读数（ADR-0213 决定四）是另一套读法：一版`已交接`只问交接对象那一件、只装一格（`HandoverGateReading` 的 `Finding`），对象是一袋时照旧按包裹引用问关务。
- 关务按包裹认对象，不在册的对象跨关务区域移动答未决、其余动作答不受门禁（ADR-0206 决定四）。于是外包干线在口岸把一袋交给承运商、去向同区时，交出读数读成不受门禁；去向跨区时读成未决、说不出袋里哪一件被挡。
- 领域已经拒收单元那一格（`formHandoverGateReading` 不收 `ConsolidationUnit()` 为真的那一格），交付门禁读数同（`formDeliveryGateReading`）。交付的对象几乎总是包裹，交出不是。

## 三类

机制（交出读数装多格：单元那一格与成员各自一格，展开复用出发读数那一套）。规模 M。

## 最小的诚实修法

交出读数改成与出发读数同形：逐对象那几格加上本侧停点；`HandoverGateReader.read` 复用出发那一侧的展开（问节点作业成员关系与收寄身份，时点取交接的业务时间），单元那一格由领域取成员里最重的一格。持久化同出发读数加 `members` 键，存量照读。

## 要裁的

- 成员时点取交接的业务时间（与出发同理），还是交接判断时刻。
- 交接答复的线格式是否像出发那样带成员。
- 展开写成两种读数共用的一处（建议），还是交出那一侧另写。

## 既有票

- [42](./42-departure-gate-reading-does-not-expand-consolidation-unit-members.md)、[80](./80-a-consolidation-unit-handover-does-not-move-member-control.md)（单元交接不推进成员控制）
