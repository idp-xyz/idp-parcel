# 11 改路与装载 / 交接并发时按业务时间与生效边界裁决

Category: enhancement
Status: draft
Blocked by: 09
父票：[04](./04-routing-product-strategy-first-cut.md)「路由策略族」那一步
地盘：network-routing 领域与复核编排；若要消费装载或交接结果，只在 NR 侧的消费方适配器里加（[ADR-0025](../../../docs/adr/0025-cross-context-adapters-live-on-the-consumer-side.md)）。
出处：[ADR-0146](../../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定七；network-routing [`CONTEXT.md`](../../../docs/domain/network-routing/CONTEXT.md)「改路与装载或交接并发时，按权威业务发生时间、改路生效边界和明确因果关系裁决」一句；[UC-NR-003](../../../docs/application/network-routing/UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md) `AT-NR-047`。

## 做什么

1. **判断结构**：给定改路生效边界与一条装载或交接事实（权威业务发生时间、节点、所依计划版本），裁出「实际装载先发生 → 改路从下一个可控节点生效」或「改路先有效 → 其后按旧计划发生的装载保留为真实事实并形成路由偏离」；不按消息到达顺序。
2. **复核接上**：开工先查 NR 今天消费了 TF / NO 的哪些结果（取证于 `64b37f27`：复核只把运输交接当作控制依据消费，没有装载事实）。没有所需事实时，判断结构先落领域并由用例覆盖，接线缺口写进票面并指去处，不在本票里新开跨上下文订阅。

## 不做

- 不拥有装载、交接事实（归 TF / NO）；路由偏离之后的异常处置归 visibility-exception。

## 完成判据

- [ ] 领域用例：装载先、改路先、同一时刻、因果不明各一格。
- [ ] `AT-NR-047` 在复核用例里有覆盖，或票面写明接线缺口与去处。
