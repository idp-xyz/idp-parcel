# 28 订舱与取消发生项；实际履约经段采用取采购上下文

Category: enhancement
Status: ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [tf-chain-plan](../tf-chain-plan.md)。
Blocked by: [24](./24-booking-references-a-schedule-and-carrier-answers-via-integration-client.md)、[25](./25-segment-adopts-transport-commission-and-cancellation-faces.md)、[27](./27-transport-charge-occurrences-are-registered-and-handed-off.md)
地盘：TF `domain/transport_charge_occurrence.go`；`register_charge_occurrence.go`；新读端口文件；tfhttp `register_charge_occurrence.go`。
出处：[07](./07-transport-charge-occurrences-have-no-production-path.md)；方案「07 运输收费发生项」D07-2、D07-3，「需要的 ADR」ADR-0207，「已不成立或已修」07。

## 做什么

- 同一登记用例补订舱与取消两种原因，各自的事实依据照方案「需要的 ADR」ADR-0207。
- 段已采用委托（[25](./25-segment-adopts-transport-commission-and-cancellation-faces.md)）时，实际履约的采购上下文从委托快照取，与调用方值的冲突答法照方案 D07-2。

## 完成判据

- 订舱的依据是已接受的答复；取消的依据是未开始范围。
- 段已采用委托时可以省略采购上下文，给了但不一致答冲突。
- 真库测试。
- 演示：干线订舱被接受后登订舱费发生项。

## 规模

M
