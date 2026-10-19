# 33 移动事实已形成信封与 VE 投影

Category: enhancement
Status: ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [tf-chain-plan](../tf-chain-plan.md)。
Blocked by: [32](./32-movement-facts-declare-segment-and-record-departure-gate-reading.md)
地盘：TF 新 `postgres/movement_fact_handoff.go` 与端口、`record_movement_fact.go`；VE 新 inbox 消费者与派生；`cmd/parcel-dispatch/assemble.go`(+_test)；`cmd/parcel-api/assemble_movement_fact.go`；种子 visibility 映射（参考配置 + SYN 采用）。
出处：[05](./05-movement-facts-are-unlinked-and-departure-gate-is-self-reported.md)；方案「05 移动事实与出发门禁」D05-1、D05-10，「跨票通则」X3，「需要的 ADR」ADR-0205。

## 做什么

- `transport-fulfillment.movement-fact.recorded` 与 VE 消费者、路由行、里程碑映射参考配置在同一个变更上线（方案 D05-10、X3）。
- 阻断读数随信封交 VE，派生异常信号（方案 D05-1）。
- 演示种子：visibility 映射的参考配置与 SYN 采用。
- NR 路由偏离不在本票，归 [35](./35-network-routing-route-deviation-consumes-movement-facts.md)。
- 进 main 的令牌次序与波次见方案「并行车道」。

## 完成判据

- 生产方与消费者在同一个变更。
- 哨兵登列。
- 阻断读数派生异常。
- 真库测试。
- 演示：出发后 `/tracking-projections` 出现 DEPARTED。

## 规模

M
