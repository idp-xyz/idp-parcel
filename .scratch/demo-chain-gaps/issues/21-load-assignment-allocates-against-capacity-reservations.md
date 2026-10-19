# 21 装载分配的容量关系（03b）

Category: enhancement
Status: ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [tf-chain-plan](../tf-chain-plan.md)。
Blocked by: [19](./19-load-assignment-checks-the-schedule-is-registered.md)、[20](./20-capacity-reservation-and-release-decision-faces.md)
地盘：`domain/load_assignment_movement.go`、`load_assignment_rehydration.go`、`transport_schedule_capacity.go`；`ports/load_assignment.go`（不改 `ports.go` 签名）；`postgres/load_assignment_registry.go`、`capacity_pool.go`；迁移 TF 0031；tfhttp `form_load_assignment.go`；`assemble_segment_operations.go`；UC-TF-004 AT。
出处：[03](./03-load-assignment-does-not-check-the-schedule.md)；方案「03 装载分配与容量」D03-1、D03-3、D03-4、D03-5，「需要的 ADR」ADR-0203。

## 做什么

- 保障判据：班次登记了容量池，装载分配就须对每个池引用一份当前有效预占并声明分配量；没登池的班次不要求（方案 D03-3）。
- 预占上新增分配关系，守恒、归还与「不调 `Consume`」照方案 D03-4；池所在班次须等于分配所在班次。
- 分配量逐池逐维度声明并带来源，成员数的用法照方案 D03-5。
- UC-TF-004 补 AT 行；新格名照方案「需要的 ADR」ADR-0203。
- 进 main 的令牌次序与波次见方案「并行车道」。

## 完成判据

- 三个新格：`CAPACITY_RESERVATION_REQUIRED`、`RESERVATION_NOT_ON_SCHEDULE`、`ALLOCATION_EXCEEDED`（带余量）。
- 撤回或改版归还，守恒有真库测试。
- 不调 `Consume`。
- 无池班次照旧形成。
- 演示：带预占引用的分配答 201，超量答 `ALLOCATION_EXCEEDED`。

## 规模

L
