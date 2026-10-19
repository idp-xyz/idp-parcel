# 32 移动事实声明段与出发门禁读数

Category: enhancement
Status: ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [tf-chain-plan](../tf-chain-plan.md)。
Blocked by: [31](./31-customs-action-admission-judgement-and-node-customs-zones.md)；[operator-channel/48](../../operator-channel/issues/48-cmd-parcel-api-tests-and-comments-say-what-the-code-does.md)（同一测试文件）
地盘：TF `domain/load_assignment_movement.go`、`movement_fact_rehydration.go`、`payload_canonicalization.go`、新 `departure_gate_reading.go`；`application/record_movement_fact.go`；新 `ports/departure_gate.go`；新 `adapters/customscompliance/departure_gate.go`；`postgres/movement_fact_registry.go`；迁移 TF 0026；tfhttp `record_movement_fact.go`；`cmd/parcel-api/assemble_movement_fact.go`(+_test)、`movement_fact_same_identity_test.go`、`transport_fulfillment_command_lines_test.go`、`storable_values_channel_test.go`；ADR-0205、TF CONTEXT、UC-TF-005。
出处：[05](./05-movement-facts-are-unlinked-and-departure-gate-is-self-reported.md)；方案「05 移动事实与出发门禁」D05-1、D05-2、D05-6、D05-7、D05-8、D05-9，「跨票通则」X4、X5，「需要的 ADR」ADR-0205，「已不成立或已修」05。

## 做什么

- 出发一律落册，同一笔存关务门禁读数（封闭集照方案 D05-1）；退役「`DEPARTURE_GATE_BLOCKED`、不落库」那一格（方案 D05-1、X5）。
- TF 消费侧端口 `DepartureGateView` 在写事务开始前同步问 [31](./31-customs-action-admission-judgement-and-node-customs-zones.md) 的准入判断，TF 只翻译（方案 D05-2、X4）。
- 移动事实带登记方声明的段引用，不加外键；对象取段的当前参与；段不在册也照记（方案 D05-6）。不核班次在册（方案 D05-9）。
- 设备的 `gateRequired` 失去权威但字段暂留，`gateClearance` 只作证据（方案 D05-8）。
- 外包干线的跨境门禁不在本票（方案 D05-7）；不改 `work_fact_intake*.go`、`assemble_operator_decisions.go`（方案「等 peer 的」）。
- 起草 ADR-0205（要点见方案「需要的 ADR」）；TF CONTEXT Rules 补门禁读数；UC-TF-005。
- 进 main 的令牌次序与波次见方案「并行车道」。

## 完成判据

- 出发一律落库。
- 五格读数与事实同一笔（真库）。
- CC 读不动也落库，记 `读不动`。
- `gateRequired=false` 照样问 CC。
- 段不在册照记。
- 不改 `work_fact_intake*.go`、`assemble_operator_decisions.go`。
- 演示：自营 SHA→SZX 出发答 201，读数 `不受门禁`。

## 规模

L
