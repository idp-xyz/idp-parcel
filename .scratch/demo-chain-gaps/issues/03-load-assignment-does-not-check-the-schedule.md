# 03 装载分配不核班次在册、不接容量预占

Category: enhancement
Status: needs-triage——2026-10-18 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自同日对演示动线余下各段的只读勘察（隔离子代理，证据 `S`）；下列现象与证据取自勘察结果，本票立时没有逐条复核，接票时先核。
Blocked by: 无
出处：[product-strategy-boundary/05](../../product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md) 2026-10-18 那次重走。

## 现象

【软停】装载分配（运营决定）能过渠道，但不核班次在册、不接容量预占，演示会对一个不存在的班次答已形成

## 证据（读码）

`/transport-fulfillment-load-assignment-registrations` 挂在 operatorDecisions.transport 上，种子已给配置员装载分配授予。`FormLoadAssignmentDeps` 只有 Assignments 和 Clock；`loadAssignmentSpecFrom` 只过 `domain.NewScheduleReference` 这道构造门，不查 `ports.TransportScheduleStore`。`PrepareTransportOpportunityHandler.ConsumeCapacity` 收调用方给的 Assignment 引用，发 `transport-fulfillment.capacity-consumption.recorded`，这个类型没有消费者。分配和容量两边互不相连。

## 三类

机制。规模 M。

## 最小的诚实修法

FormLoadAssignment 读班次册，班次不在册时答一个新的「未受理」格（如 SCHEDULE_NOT_FOUND）；需要容量保障的分配必须引用一个有效预占，并经 ConsumeCapacity 同笔转换数量。分配仍是执行意图，不加任何「已装载」字段。

## 要裁的

哪些线路/班次的分配必须有容量保障（CONTEXT 原话是「需要容量保障时」）：做成内置策略供租户选，还是作租户取值。

## 既有票

none（operator-channel/50、tf-face-algebra/02 只管重放代数）
