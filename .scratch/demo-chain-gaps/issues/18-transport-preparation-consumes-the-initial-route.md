# 18 运输准备结果与初始路由消费者（01·三）

Category: enhancement
Status: ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [tf-chain-plan](../tf-chain-plan.md)。
Blocked by: [16](./16-transport-schedule-templates-and-concrete-schedules-are-registered.md)；软依赖 [17](./17-capacity-pools-are-registered.md)（没有池时答 `容量待确认`，不挡落地）
地盘：TF：新 `domain/transport_opportunity.go`；新 `application/prepare_opportunity_on_planned_legs.go`；新 `adapters/networkrouting/planned_leg_source.go`；新 `adapters/parcelshipment/declared_demand_source.go`；新包 `adapters/inbox/`；`postgres` 新 `transport_opportunity.go` + `review_catalogue.go`；迁移 TF 0028；`cmd/parcel-dispatch/assemble.go`(+_test)。
出处：[01](./01-transport-opportunity-has-no-production-entry.md)；方案「01 运输机会与班次」D01-1、D01-5、D01-6、D01-7、D01-8、D01-10、D01-11，「跨票通则」X3、X4、X6，「需要的 ADR」ADR-0201。

## 做什么

- TF 消费 `network-routing.initial-route.formed`：路由表改成 `FanOut(VE, TF)`，各自 inbox、各列未决哨兵（方案 D01-1）。
- 候选与择优只按方案 D01-8 的那一种内置形；不预占，不建占位班次。
- 空册与无候选分格答，`未配置` 入账不重投（方案 D01-5、X6）。
- 收寄前需求量经 TF 消费侧适配器取 PS 申报测量并标来源，维度与换算照方案 D01-6；新端口另立文件（方案 X4）。
- 结果唯一键、重放与冲突照方案 D01-10；不入队准备结果信封（方案 X3）。
- 改路后的新计划不在本票，完成记录登这个缺口，指向 [34](./34-network-routing-replanned-route-envelope-and-tf-preparation-consumer.md)（方案 D01-7）。
- 进 main 的令牌次序与波次见方案「并行车道」。

## 完成判据

- 无路线判断零写入。
- `未配置`、`无当前运输机会`、`容量待确认`、`当前机会不可用`、`已准备` 各有真库测试。
- 重放答 `已有结果`。
- TF 未决重投时 VE inbox 不重复投影。
- 路由测试改成两路。
- 完成记录登改路缺口（→[34](./34-network-routing-replanned-route-envelope-and-tf-preparation-consumer.md)）。
- 演示：提交一单后，`registry=transport-preparation` 列出三段，至少一段 `运输机会已准备`；窗口对不上就调 SYN 模板时刻，不改匹配规则。

## 规模

L
