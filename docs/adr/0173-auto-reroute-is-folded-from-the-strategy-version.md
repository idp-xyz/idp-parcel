# ADR-0173：自动改路由策略版本折出，事实目录不再是判断权威

Status: Accepted（2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定二、决定七：判断形态归产品，租户选形态、填取值。）
Date: 2026-09-30

## Context

`EvaluateAutoRerouteConditions` 收的是已经折好的布尔。事实目录把那组布尔存成按判断键的陈述，注释曾把折法记成实例半边。ADR-0146 之后折法是产品策略。目录若继续被复核读取，策略版本和目录就是两处权威。

## Decision

**一、首版自动改路形态是成本改善。** 新候选成本低于当前选中候选，差额严格大于策略版本登记的阈值，并且位于可控节点、只改未执行部分、没有未解除硬限制和既有责任，才允许自动改路。阈值是租户取值，不预填。

**二、形态和阈值都空，只形成建议。** 阻塞是 `AUTO_REROUTE_UNCONFIGURED`。未声明不是允许，也不是不允许。只声明一半拒，不落库。

**三、原计划仍可执行时，先看冻结。** 已越过冻结边界则保留原计划，不因改善改路。未越过且改善超过阈值，才自动切换。硬约束失效仍走失效重判；失效后同样按本形态折自动条件。

**四、自动改路事实目录退场为判断权威。** 表和登记口保留历史陈述，复核不读它。限制与既有责任随这次判断的证据进入，不从目录折政策。

## 候选与反方

- **没声明就当不允许，连建议都不形成。** 说不出为什么没自动，和「未配置」不是同一件事。否决。
- **没声明就当允许。** 会把空册当成租户已经选了自动。否决。
- **目录继续当政策权威，策略版本只存阈值。** 两处都能改「是否允许自动」。否决。

## Consequences

- 迁移是 `network_routing/0013_route_strategy_auto_reroute.sql`。
- 装载并发不在本记录。

## 越权风险点

1. 成本改善必须严格大于已登记阈值才自动切换。归 network-routing 的 owner 复核。
2. 事实目录不再被复核读取。归 network-routing 的 owner 复核。

## Links

- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)
- [ADR-0172](./0172-route-freeze-is-remaining-segment-count.md)
- [network-routing CONTEXT](../domain/network-routing/CONTEXT.md)
- [UC-NR-003](../application/network-routing/UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md)
- 票 `.scratch/routing-first-cut/issues/05-auto-reroute-conditions-folded-from-strategy.md`
