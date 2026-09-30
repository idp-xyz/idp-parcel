# ADR-0172：冻结边界的首版形态是剩余计划段数，没声明就不是未冻结

Status: Accepted（2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定二、决定七：判断形态归产品，租户选形态、填取值。）
Date: 2026-09-30

## Context

复核里没有冻结判断。已执行前缀必须从计划段链和当前可控节点切出。可控节点只认节点收寄或权威运输交接。改善阈值与自动改路条件不在本记录。

## Decision

**一、首版冻结形态是剩余计划段数。** 剩余段数不超过策略版本登记的段数，即越过冻结边界。段数是租户取值，不预填。

**二、形态和段数都空，判断答未配置。** 未配置不是未冻结。只声明一半拒，不落库。

**三、已执行前缀只认计划节点链上的可控节点。** 节点收寄或权威运输交接才能切前缀。扫描、位置、消息顺序不建立可控节点。节点不在计划上，前缀不成立。

**四、复核在原计划仍可执行时先看冻结。** 未配置则这次复核停在未配置。已越过边界则保留原计划，不因改善改路。硬约束失效仍走失效重判。

## 候选与反方

- **没声明就当未冻结。** 轻微改善会在边界不明时被放行。否决。
- **用扫描位置切前缀。** CONTEXT 禁止。否决。

## Consequences

- 迁移是 `network_routing/0012_route_strategy_freeze_form.sql`。
- 改善阈值与装载并发不在本记录。

## 越权风险点

1. 剩余段数小于或等于限额即越过边界。归 network-routing 的 owner 复核。
2. 未越过边界时，本记录仍不因改善改路；那一格归后续改善阈值。归 network-routing 的 owner 复核。

## Links

- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)
- [network-routing CONTEXT](../domain/network-routing/CONTEXT.md)
- [UC-NR-003](../application/network-routing/UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md)
- 票 `.scratch/routing-first-cut/issues/04-freeze-boundary-and-executed-prefix.md`
