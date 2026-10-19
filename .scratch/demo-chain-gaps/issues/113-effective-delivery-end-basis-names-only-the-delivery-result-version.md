# 113 有效交付结束参与的终点依据只带交付结果版本：同一对象两次有效交付用同一个版本号时，自动形成整封弃置

Category: bug
Status: needs-triage——2026-10-19 票 [111](./111-actual-fulfillment-formation-matches-participation-ends-by-version-only.md) 的作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [ADR-0221](../../../docs/adr/0221-a-handover-participation-basis-is-minted-from-the-handover-key.md) 越权风险点 5。
Blocked by: 无
地盘：TF `domain/actual_fulfillment_formation.go`（`EffectiveDeliveryEndBasis`）、`domain/actual_fulfillment_segment.go`（`EndParticipationWithDelivery`）、`application/form_actual_fulfillment_on_participation_end.go`（`OnDelivery`）。

## 现象（读码，钉票 111 的分支）

- 有效交付的键是（租户，对象，尝试），每一版交付结果另有版本。结束参与时记的终点依据是 `EFFECTIVE-DELIVERY/<版本>`，不带尝试。
- 实际履约发生项自动形成那一路（ADR-0218）按这一串找结束的参与，逐字对上多于一条答 `ErrParticipationEndedInSeveralSegments`，信封烧完重投预算落 `ABANDONED`。
- 同一对象先后两次有效交付（例如退运交回发货人，两次各在自己的派送段），登记方都把交付结果版本写成 `v1` 时，两段的终点依据相同，正是票 111 在交接一侧撞上的那一格。
- 演示动线今天只交付一次，没撞上；读码可构造。

## 三类

- 机制：照 ADR-0221 的做法，终点依据由交付的完整键铸（对象、尝试、版本三节长度前缀），另起一个起头，存量照读、按版本加终点时刻认；或在找参与那一步另比终点时刻。
- 产品策略：无。
- 租户取值：无。

## 要裁的

- 走哪条。改拼法的话，`OnDelivery` 的命令里已经有尝试（`FormActualFulfillmentOnDeliveryCommand.Attempt`，今天只校验、不进依据），结束参与那一侧 `EndParticipationWithDelivery` 也拿得到交付的尝试，不必改信封。

## 规模

S
