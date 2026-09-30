# ADR-0174：改路与装载并发按业务时间裁决，同一时刻和因果不明都不并进前两格

Status: Accepted（2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定二、决定七：判断方法归产品。）
Date: 2026-09-30

## Context

CONTEXT 已写死两格：实际装载先发生，改路从下一个可控节点生效；改路先有效，其后按旧计划发生的装载保留为真实事实并形成路由偏离。不按消息到达顺序。同一时刻、因果不明，CONTEXT 与 `AT-NR-047` 都没写。复核今天只消费节点收寄和运输交接作为控制依据，没有装载事实。

## Decision

**一、只比权威业务发生时间与改路生效边界。** 装载或交接事实带来发生时间、节点、所依计划版本，以及因果是可比还是不明。消息到达时间不进入裁决。

**二、装载时间严格早于改路生效，且因果可比、所依计划就是被离开的计划。** 改路从该装载节点在计划链上的下一个节点生效。已经是最后一节点时，没有更后的可控节点，不另造一个。节点不在计划上，输入不成立。

**三、改路生效时间严格早于装载时间，且因果可比、计划相符。** 装载保留为真实事实，裁决标明形成路由偏离。偏离之后的处置归 visibility-exception，本记录不做。

**四、两个时间都在、且相等。** 结果是同一时刻。不改到下一节点，也不形成偏离。到达顺序不许用来打破平局。

**五、因果不明是单独一格。** 来源标明不明、任一时间为空，或装载所依计划不是这次改路离开的计划，结果都是因果不明。不并进装载先，也不并进改路先。因果陈述的零值是输入错误，不是这一格的默认。

## 候选与反方

- **同一时刻算装载先。** 会把打平的时间当成装载已经占住。否决。
- **同一时刻算改路先。** 会把同时的装载记成偏离。否决。
- **因果没写就当不明。** 可比的权威时间本身就是先后依据。没写不等于不明。否决。
- **时间缺了就用消息到达时间补。** CONTEXT 禁止。否决。

## Consequences

- 判断在领域 `ArbitrateLoadingConcurrency`。没有新迁移。
- 复核编排还不调用它：没有装载事实可交。接线去处写在票 `routing-first-cut/06`。

## 越权风险点

1. 同一时刻与因果不明都不产生下一节点或偏离。归 network-routing 的 owner 复核。
2. 路由偏离只被标明，不在本上下文打开异常处置。归 network-routing 的 owner 复核。

## Links

- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)
- [network-routing CONTEXT](../domain/network-routing/CONTEXT.md)
- [UC-NR-003](../application/network-routing/UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md)
- 票 `.scratch/routing-first-cut/issues/06-reroute-loading-concurrency-arbitration.md`
