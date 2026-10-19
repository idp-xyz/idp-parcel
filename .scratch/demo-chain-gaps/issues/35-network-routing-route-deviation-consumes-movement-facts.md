# 35 NR：路由偏离消费移动事实

Category: enhancement
Status: needs-triage——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [tf-chain-plan](../tf-chain-plan.md)。后继票，不在演示主链上；地盘与完成判据分诊时定稿。
Blocked by: [33](./33-movement-fact-recorded-envelope-and-tracking-projection.md)
地盘：NR 新消费者（路由偏离）；`cmd/parcel-dispatch/assemble.go`（派发令牌）。
出处：[05](./05-movement-facts-are-unlinked-and-departure-gate-is-self-reported.md)；方案「05 移动事实与出发门禁」D05-10，「票拆分」后继票表。

## 做什么

- NR 消费 [33](./33-movement-fact-recorded-envelope-and-tracking-projection.md) 上线的 `transport-fulfillment.movement-fact.recorded`，据此判路由偏离（方案 D05-10）。
- 派发令牌次序见方案「并行车道」。

## 完成判据

- NR 消费者进路由表并列未决哨兵。
- 移动事实与当前有效计划不符时形成路由偏离判断（真库测试）。

## 规模

M
