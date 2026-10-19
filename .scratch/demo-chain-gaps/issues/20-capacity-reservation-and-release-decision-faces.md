# 20 容量预占与释放运营决定面

Category: enhancement
Status: ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [tf-chain-plan](../tf-chain-plan.md)。
Blocked by: [17](./17-capacity-pools-are-registered.md)
地盘：`internal/accessidentity/operation_decision.go`(+_test)；tfhttp `operator_decision_intake.go`(+test)、新 `reserve_capacity.go`/`release_capacity.go`；`application` 的 Reserve/Release 带需求来源；新 `cmd/parcel-api/assemble_capacity_reservation.go` + `endpoints.go` 行；种子 access 02/03、governance 新区间；ADR-0203、TF CONTEXT 一句。
出处：[03](./03-load-assignment-does-not-check-the-schedule.md)；方案「03 装载分配与容量」D03-1、D03-3、D03-6，「跨票通则」X2，「需要的 ADR」ADR-0203。

## 做什么

- 预占与释放各是一种运营决定种类，授予、准入映射与权威区间演示用种子登 `SYN-`（方案 D03-6、X2）。
- 预占必须声明需求量与来源；触硬上限的答法照方案 D03-6。
- 起草 ADR-0203（要点见方案「需要的 ADR」）；TF CONTEXT Rules 补保障判据（方案 D03-3、「随票同步的文档」）。
- 进 main 的令牌次序与波次见方案「并行车道」。

## 完成判据

- 两种决定种类。
- 预占必须带需求量与来源。
- `CAPACITY_EXCEEDED` 带余量，没有审批越过（AT-TF-031）。
- 重放幂等。
- 真库测试。
- 演示：配置员对 SYN 池预占答 201。

## 规模

M
