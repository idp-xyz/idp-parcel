# 19 装载分配核班次在册（03a）

Category: enhancement
Status: ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [tf-chain-plan](../tf-chain-plan.md)。
Blocked by: 无（演示那一步要 [16](./16-transport-schedule-templates-and-concrete-schedules-are-registered.md) 或 [18](./18-transport-preparation-consumes-the-initial-route.md) 的在册班次）
地盘：`application/form_load_assignment.go`(+_test)；`cmd/parcel-api/assemble_segment_operations.go`(+_test)、`load_assignment_single_chain_test.go`、`storable_values_channel_test.go`；tfhttp `form_load_assignment_test.go`、`segment_operations_test.go`；UC-TF-004 AT 行。
出处：[03](./03-load-assignment-does-not-check-the-schedule.md)；方案「03 装载分配与容量」D03-1、D03-2，「已不成立或已修」03。

## 做什么

- 装载分配在版本重放与链头判定之后按租户读班次登记，不在册与读失败分格答（方案 D03-2）。
- 落在 UC-TF-004 步 1 现行文字之内，不需要 ADR，只补 AT 行（方案 D03-2，及「需要的 ADR」里「不需要新 ADR」那段）。
- 不碰 `OpenDispatchTaskDeps`；与 peer 同函数相邻，落地前 rebase（方案「等 peer 的」）。

## 完成判据

- 新格 `SCHEDULE_NOT_FOUND`（200），放在重放与链头判定之后。
- 读失败答 `UNDECIDED`。
- 他租户引用落同一格。
- 真库测试先补登班次行。
- 不碰 `OpenDispatchTaskDeps`。
- 演示：对物化班次分配答 201，对乱填班次答 `SCHEDULE_NOT_FOUND`。

## 规模

S
