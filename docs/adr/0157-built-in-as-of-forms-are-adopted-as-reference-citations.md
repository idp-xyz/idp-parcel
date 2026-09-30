# ADR-0157：内置时点形态以参考配置发布，租户在时点语义格用引用选用

Status: Accepted（2026-09-30 用户授权通道 1 自决：「你是业务和系统专家，你能帮我来做决策吗」。通道 2 起草。裁决原文在票 `product-strategy-boundary/06` Comments「裁决 ← 通道 1」。）
Date: 2026-09-30

## Context

「提交接收」的折法在 `parcel-shipment`，租户在 `party-commercial` 的规则包里选用。形态清单若放在消费方，登记时要校验就得让商业上下文反向依赖托运上下文。`referenceconfig` 两边都能引，也不属于任何一方。

ADR-0147 的采用写在登记依据格。时点语义格不是依据格：它选的是哪一种时点形态。上一笔把引用串写进语义格，但登记只过非空构造门，写错版本也能落库，判断时才静默答`未配置`。

## Decision

**一、内置时点形态的对外声明以参考配置发布。** 折法仍在认得它的执行器里，参考配置不另写一份折法。首份的键是 `parcel-shipment/as-of-semantics/submission-receipt`，版本 1，作为 ADR-0147 越权风险点 3 的 owner 定键。前缀取 `parcel-shipment`：折法与执行器在那里。

**二、选用写在时点语义格，不写在依据格。** 接单规则包的逐项时点策略，以及价格政策汇率口径的时点语义，租户要选内置形态就写 `REFCFG-1:` 引用。这把 ADR-0147 的「采用」扩到形态选择格。不带此前缀的值仍是不透明引用。

**三、带 `REFCFG-1:` 的值，登记时必须能解析且该版已发布，否则拒。** 不把一个到判断时才答`未配置`的坏引用落进去。不带前缀的值不在这里解释；没有执行器认得它，形成时点时答`未配置`。

**四、执行器不按判断类别设限。** 租户在哪一格采用，就在哪一格形成。没采用的那一格答`未配置`，是因为那一格没写这个引用，不是因为财务控制这一类永远形不成。

## 候选与反方

- **内置形态码，登记与执行在同一上下文核对封闭词表。** 路由的排序形态可以这么做，因为登记和执行都在 network-routing。这里两边分开，商业上下文要校验就得依赖托运上下文。否决。
- **语义格只存不透明串，执行器认一个产品常量。** 写错版本落得进去，判断时才变成未配置，登记方看不到拒因。否决。

## Consequences

- 首份参考配置不带折法字段。已发布版本的摘要按删字段后的原文重算；该版尚未进 main，这次改内容不是改写已发布版本。
- 登记门在 party-commercial 构造时点语义引用的地方：带前缀的值不过门就立不起来。发布翻译、管理台草稿与重建都走这一处。
- 价格政策汇率格可以选用同一引用。计价侧没有汇率时点执行器，那一格仍答`未配置`，不在本记录里补执行器。

## 越权风险点

1. 把「采用」扩到形态选择格。归 PC owner 复核，与 ADR-0146 越权风险点 4 同类。
2. 标识前缀归 parcel-shipment 而非 party-commercial。归 PS、PC owner 复核。
3. 汇率格可以选用，计价侧没有执行器。归 PP owner 复核。

## Links

- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)：判断方法归产品，租户选形态
- [ADR-0147](./0147-reference-configuration-ships-embedded-and-is-adopted-through-ordinary-registration.md)：引用串形状与越权风险点 3；本记录把采用扩到时点语义格
- 票 [`product-strategy-boundary/06`](../../.scratch/product-strategy-boundary/issues/06-ps-acceptance-and-label-selection-judgment-methods.md)、[`23`](../../.scratch/product-strategy-boundary/issues/23-pc-as-of-semantics-cells-gate-reference-citations.md)
