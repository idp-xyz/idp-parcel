# 68 移动命令退役 `gateRequired`

Category: enhancement
Status: ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [handover-dispatch-plan](../handover-dispatch-plan.md)。
Blocked by: 无
地盘：TF `application/record_movement_fact.go`(+test)、tfhttp `record_movement_fact.go`(+test)、`work_fact_intake_test.go`；`cmd/parcel-api/assemble_movement_fact_test.go`、`transport_fulfillment_command_lines_test.go`；文档 ADR-0205 复核记录一行、UC-TF-005 `AT-TF-113`。
出处：[tf-chain-plan](../tf-chain-plan.md)「等 peer 的」删掉移动命令与载荷里的 `gateRequired` / `gateClearance`；方案「代决」C16。

## 做什么

- 应用命令删掉 `GateRequired`；HTTP 载荷照收照弃，作退役键注明；`gateClearance` 保留作证据（方案 C16）。
- 不另立 ADR，只在 ADR-0205 复核记录里加一行。

## 完成判据

- 载荷带 `true`、带 `false`、不带：读数与内容摘要逐字相同，均 201。
- `gateClearance` 照存。
- 真库测试。
- 演示：出发载荷带 `"gateRequired":false` → 201 `MOVEMENT_FACT_RECORDED`，`gateReading` 与不带时相同。

## 规模

S
