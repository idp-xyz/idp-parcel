# 19 NO「集运包含无循环」未落代码，连单元装进自己也不拦

Category: bug
Status: ready-for-agent——R2（上线后尽快）：数据完整性。集运移入已在生产路径上（`cmd/parcel-frontline-import` 的集运导入），袋套袋是常态；成员关系可以成环。下游 TF 出发门禁展开成员时有环检测（`application/read_departure_gate.go` 记 `ConsolidationMembershipCycle`），不会放行或死循环，只会让那一袋停住——看得见的卡住；绕法是移出成环的成员。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
Blocked by: 无
出处：[audit/nr-no.md](../audit/nr-no.md) M6（审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，现象仍在；下游环检测是复核时新读到的，降低了后果）。

## 现象与证据

- 文档：NO CONTEXT「集运包含关系必须无循环；每个作业实物同一时点最多一个直接物理父级」；GLOSSARY「集运单元」：「全部包含层级不得形成循环」；UC-NO-003 `AT-NO-039`：「集运单元嵌套形成循环 | 拒绝成员关系，原关系不被覆盖」。
- 代码：
  - `internal/nodeoperations/domain/consolidation_unit.go` 的 `ConsolidationUnit.AddMember` 只拒已关闭、已封装与重复成员。
  - `application/consolidate_parcels.go` 的移入只问成员自己的 `Containment.CurrentParent`，不查目标单元的祖先链，也不拒「成员就是单元本身」。
  - 嵌套成员确实存在（`ports.ConsolidationMemberAt.NestedUnit`）；`migrations/node_operations` 没有对应约束；`AT-NO-039` 没有测试。

## 三类

机制。无产品策略、无租户取值。

## 规模

S–M：移入时沿目标单元的父级链上溯（含自身），撞到成员即拒；与「同一时点最多一个直接父级」同一读口；补 `AT-NO-039`。

## 最小的诚实修法

移入编排在同一事务里读目标单元的当前祖先链；成员是目标自身或其祖先之一即答「包含循环」并拒绝，原关系不变。并发下两袋互装的竞态靠同一事务的行锁或按单元排序加锁挡住。

## 要裁的

无。
