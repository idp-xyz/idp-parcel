# ADR-0043: 发布意图由结果标识认领，重放重发同一份——事务发布落地前的统一缝形

Status: Accepted  
Date: 2026-08-12

## Context

Bento 持久化闸门挡住事务发布与 outbox（[ADR-0017](./0017-admission-gates-judged-by-blocking-cause.md)），但「业务结果要通知适用下游」的机制半边不必等它。第一个样本是 `UC-PS-002` 步骤 9 的 `SourceDataVersionHandoff`（`AT-PS-031`）；第二个是 `UC-PS-001` 决定发布的 `AcceptanceDecisionHandoff`（`AT-PS-013`，落于 `d551872`）；第三个是 `UC-NR-002` 判断发布的 `ReachabilityJudgmentHandoff`（`AT-NR-030` 的意图半边，与本记录同笔落地）。

三个样本一字未改地共享同一形状，且已跨出单一上下文。[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)的「无事件机制」一行两次把「是否升为口径」向后递延——第一样本时留待第二个用例，第二样本时留待第三个上下文。第三个到了：要么定成口径，要么承认三处同形只是巧合并放任第四处漂移。

## Decision

**在事务发布落地之前，任何「业务结果需要通知适用下游」的用例照此办理：**

1. **一份结果发一份意图，意图由结果标识认领**（资料版本号、决定标识、请求关联）。重放重发同一份，不是第二份——下游按标识认领即天然去重。
2. **本上下文不记意图完没完成。** 那份状态要与业务结果同一事务落库才算数；闸门解除前记一个证明不了原子性的完成标志，比不记更糟——它看起来像发过了。
3. **首次交付失败不改写业务结果。** 结果照常交回，另留一条**发布续办引用**，与「结果未形成」的续办分开——一个要重放发布，一个要重判结果，合成一格调用方分不清该做哪件。
4. **端口接口属机制半边先定**（ADR-0017 口径），唯一实现是确定性测试替身；实际投递、outbox 与事务发布仍归闸门。

## Consequences

- Outbox 落地时，各上下文的实现点收敛为同一形状：意图表按结果标识幂等，无需逐用例重新设计。
- 每个采纳点付的是重发成本：重放路径每次都重发同一意图，由下游按标识去重。
- 若某个用例需要「确定只发一次」的语义，那是对本记录的推翻信号——回来改这里，不要在消费侧偷偷记完成标志。
- 三处既有样本即本记录的实例；新用例采纳时引用本记录，不再各自在注释里复述理由。

## Alternatives considered

- **逐用例即兴模仿（现状）。** 否决：三处已同形靠的是模仿，第四处开始漂移时没有任何东西会红；散在各注释里的口径正是红线「单一权威」要消除的。
- **等 Outbox 一起定。** 否决：闸门何时解除不由本仓决定，而采纳点在持续增加；机制半边不挂在外部工件上（ADR-0017 已就同类死锁裁过）。
- **消费侧记「已发布」完成标志。** 否决：无事务时标志与结果可各自成败，半真状态会让一份从未送达的结果看起来发过了。

## Links

- [ADR-0017：实现准入闸门按阻断理由分别裁决](./0017-admission-gates-judged-by-blocking-cause.md)：投递/事务半边仍阻断的出处
- [ADR-0031：自有仓储端口的写入结果是封闭代数而不是 error](./0031-owned-repository-write-outcome-is-a-closed-algebra-not-an-error.md)：与本记录在同一条提交边界上互补
- [UC-PS-002](../application/parcel-shipment/UC-PS-002-AMEND-CUSTOMER-SOURCE-DATA.md)（`AT-PS-031`）、[UC-PS-001](../application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md)（`AT-PS-013`）、[UC-NR-002](../application/network-routing/UC-NR-002-ASSESS-PARCEL-REACHABILITY.md)（`AT-NR-030`）：三个样本的验收出处
- [开发主线：机制半边现状](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md#横切缺口不属任何单一切片)：「无事件机制」一行随本记录更新
