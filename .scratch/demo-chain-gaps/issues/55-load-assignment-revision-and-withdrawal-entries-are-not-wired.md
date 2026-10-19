# 55 装载分配的变化与撤回没有入口：归还机制已就位，生产上却撤不回、改不了

Category: enhancement
Status: needs-triage——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [21](./21-load-assignment-allocates-against-capacity-reservations.md) 的完成记录。
Blocked by: [21](./21-load-assignment-allocates-against-capacity-reservations.md)
出处：[ADR-0203](../../../docs/adr/0203-capacity-assurance-requires-a-reservation-on-every-pool-of-the-schedule-and-reservations-are-operation-decisions.md) 文末补记（票 21）的越权风险点「变化与撤回没有入口」；票 [tf-unwired-seven/05](../../tf-unwired-seven/issues/05-dispatch-task-and-load-assignment-bodies.md) 文末「变化与撤回的入口没有接」。

## 现象

票 21 让装载分配逐池占预占：分配量随版本存，预占上的分配关系取各分配当前那一版，撤回版不带分配量、改版带自己重新声明的一组，所以撤回与改版一落册，旧的那一组就归还了（真库证过）。但生产上只有形成首版的那一口；领域的 `ReviseMembers`、`Withdraw` 没有任何编排与端点。一份分配一旦形成，它占着的那段预占就只能等预占到期才回来：运营人员既撤不回一个装不上的分配，也改不了成员。释放口对已分配的那一段答守恒撞线，同样放不出来。

## 证据（读码）

- `internal/transportfulfillment/application/form_load_assignment.go` 只形成首版；同一分配换版本号答 `LOAD_ASSIGNMENT_ALREADY_FORMED`。
- `domain.LoadAssignment.ReviseMembers`、`Withdraw` 在 cmd 下零调用。
- 预占口与释放口回显的预占带 `allocated`，释放撞线时看得见挡住它的是分配。

## 三类

入口与编排是机制；谁有权撤回一份已分配、撤回要不要理由是产品角色模型（决定种类）与租户取值（授予）。规模 M。

## 最小的诚实修法

两种运营决定（变化、撤回）各开一口，挂操作者渠道的运营决定面，照 ADR-0151 每口一种决定种类。变化口收新成员与重新声明的分配量，编排走与形成同一道容量保障判定（`FormLoadAssignmentHandler.assureCapacity`，提出来共用，不另立一份口径）；提出来时分配时点要作参数传入——形成取分配时刻、变化取变化时刻（`revisedAt`）。它此刻直接读 `assignment.AssignedAt()`，而分配时刻整条链上不变（0009），照搬就会按首版的分配时刻判有效区间：预占在首版之后才立、或在首版之后已过期时结论反过来。撤回口只收版本与时刻。两口都按（租户、分配）读链头、回指链头，并发由 0031 的「一版至多一个后继」索引兜底。

## 要裁的

- 撤回要不要带理由、理由是不是封闭词。
- 物理装载已经发生之后还能不能撤回（CONTEXT：分配不证明装载，但撤回一份已实际装载的分配读起来像撤销事实）。

## 既有票

[tf-unwired-seven/05](../../tf-unwired-seven/issues/05-dispatch-task-and-load-assignment-bodies.md)（resolved，记着入口未接，不在其判据内）。
