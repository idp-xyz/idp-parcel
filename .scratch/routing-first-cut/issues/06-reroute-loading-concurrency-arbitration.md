# 06 改路与装载 / 交接并发时按业务时间与生效边界裁决

Category: enhancement
Status: resolved——2026-10-01 进 main（`b0baf6c1→6f264661`，补提交 `18df9762→8f599cd2`，ADR-0174）。Blocked by 04 已在 main
Blocked by: 04
父票：[psb/04](../../product-strategy-boundary/issues/04-routing-product-strategy-first-cut.md)「路由策略族」那一步
地盘：network-routing 领域与复核编排；若要消费装载或交接结果，只在 NR 侧的消费方适配器里加（[ADR-0025](../../../docs/adr/0025-cross-context-adapters-live-on-the-consumer-side.md)）。
出处：[ADR-0146](../../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定七；network-routing [`CONTEXT.md`](../../../docs/domain/network-routing/CONTEXT.md)「改路与装载或交接并发时，按权威业务发生时间、改路生效边界和明确因果关系裁决」一句；[UC-NR-003](../../../docs/application/network-routing/UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md) `AT-NR-047`。

## 做什么

1. **判断结构**：给定改路生效边界与一条装载或交接事实（权威业务发生时间、节点、所依计划版本），裁出「实际装载先发生 → 改路从下一个可控节点生效」或「改路先有效 → 其后按旧计划发生的装载保留为真实事实并形成路由偏离」；不按消息到达顺序。
2. **复核接上**：开工先查 NR 今天消费了 TF / NO 的哪些结果（取证于 `64b37f27`：复核只把运输交接当作控制依据消费，没有装载事实）。没有所需事实时，判断结构先落领域并由用例覆盖，接线缺口写进票面并指去处，不在本票里新开跨上下文订阅。

## 不做

- 不拥有装载、交接事实（归 TF / NO）；路由偏离之后的异常处置归 visibility-exception。

## 完成判据

- [x] 领域用例：装载先、改路先、同一时刻、因果不明各一格。
- [x] `AT-NR-047` 在复核用例里有覆盖，或票面写明接线缺口与去处。

## 接线缺口

复核今天没有装载事实。`ReassessOnIntakeAdapter` 只把 parcel-shipment 的节点收寄和场外揽收译成控制依据，运输交接在这里是控制种类，不是装载发生时间、节点和所依计划版本。NR 不订阅 transport-fulfillment 或 node-operations。

去处：装载或交接事实由那两个上下文拥有。NR 要裁 `AT-NR-047` 时，在消费方适配器里把权威业务发生时间、节点、所依计划版本和因果陈述交给 `ArbitrateLoadingConcurrency`。不在本票新开订阅。路由偏离之后的处置去 visibility-exception。

## Comments

**评审 ← 通道 1 · 钉 `b0baf6c1` · 2026-10-01**

- **阻断**：架构门禁红。`CausalityStatement`、`LoadingConcurrencyJudgment`、`LoadingConcurrencyOutcome`、`LoadingFact`、`RerouteEffectBoundary` 不在类型可达性基线；`ArbitrateLoadingConcurrency` 不在接线基线。判断本身不改。退回原分支补基线，不改写 `b0baf6c1`。

**补提交评审 ← 通道 1 · 钉 `18df9762`**

- 只改两份基线。理由写明等 NR 消费方适配器把装载事实交来。`go test -count=1 -p 1 ./internal/architecture/` 与领域包 PASS。
- **阻断**：无。`AT-NR-047` 没有复核用例，票面接线缺口写了去处，判据允许。非阻断。

**进 main 记录（2026-10-01，通道 1）**

重放到 `721d5874` 之上，零冲突：`b0baf6c1→6f264661`，`18df9762→8f599cd2`，清点 `710733d9`。没有新迁移。全量 `go test -p 1 -count=1 ./...`：135 ok / 0 FAIL。分支作封存出处。
