# 34 NR：复评新计划已形成信封 + TF 准备消费者（TF 先登路由）

Category: enhancement
Status: needs-triage——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [tf-chain-plan](../tf-chain-plan.md)。后继票，不在演示主链上；地盘与完成判据分诊时定稿。
Blocked by: [18](./18-transport-preparation-consumes-the-initial-route.md)
地盘：NR 生产方（复评新计划已形成信封）；TF 准备消费者；`cmd/parcel-dispatch/assemble.go`（派发令牌）。
出处：[01](./01-transport-opportunity-has-no-production-entry.md)；方案「01 运输机会与班次」D01-7，「票拆分」后继票表。

## 做什么

- NR 为复评形成的新计划出一封已形成信封；TF 消费者照 ADR-0049 先登路由，复用 [18](./18-transport-preparation-consumes-the-initial-route.md) 的准备用例（方案 D01-7）。
- TF 不轮询 NR 计划目录（方案 D01-7）。
- 派发令牌次序见方案「并行车道」。

## 完成判据

- 改路后形成的新计划各段有运输准备结果。
- 消费者先于生产方进路由表。
- [18](./18-transport-preparation-consumes-the-initial-route.md) 完成记录里登的改路缺口关闭。

## 规模

M
