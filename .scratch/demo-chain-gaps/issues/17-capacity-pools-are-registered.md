# 17 容量池登记（01·二）

Category: enhancement
Status: ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [tf-chain-plan](../tf-chain-plan.md)。
Blocked by: [16](./16-transport-schedule-templates-and-concrete-schedules-are-registered.md)
地盘：TF：`domain` 池与模板容量声明；`application/prepare_transport_opportunity.go` 的 `EstablishPool`；`postgres/capacity_pool.go`、`review_catalogue.go`；迁移 TF 0027；tfhttp 新 `register_capacity_pool.go`；CLI 加一个 kind；装配与 `endpoints.go` 行；种子。
出处：[01](./01-transport-opportunity-has-no-production-entry.md)；方案「01 运输机会与班次」D01-3、D01-9、D01-11，「03 装载分配与容量」D03-6，「需要的 ADR」ADR-0201。

## 做什么

- 容量池走登记册配置写，池登记口与 CLI 同一用例（方案 D01-3）。
- `EstablishPool` 改为核对班次在册（方案 D03-6）。
- 周期安排上可选的逐维容量声明随物化建池（方案 D01-9）。
- 演示种子登物化班次上的 SYN 池。
- 进 main 的令牌次序与波次见方案「并行车道」。

## 完成判据

- 池登记口与 CLI。
- `EstablishPool` 对不在册班次答 `SCHEDULE_NOT_FOUND`。
- 模板的逐维容量随物化建池且幂等。
- 多维分存（AT-TF-030）。
- 真库测试。
- 演示：查阅口 `capacity-pool` 列出物化班次上的 SYN 池。

## 规模

M
