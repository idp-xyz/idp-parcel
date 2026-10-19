# 113 有效交付结束参与的终点依据只带交付结果版本：同一对象两次有效交付用同一个版本号时，自动形成整封弃置

Category: bug
Status: wontfix——前提不成立，不是缺陷（2026-10-19 票 [111](./111-actual-fulfillment-formation-matches-participation-ends-by-version-only.md) 隔离评审指出、作者子代理读码核实，见下「核实」）。原由票 111 的作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权），出自 [ADR-0221](../../../docs/adr/0221-a-handover-participation-basis-is-minted-from-the-handover-key.md) 草稿的一条越权风险点；那一条已在进 main 之前删去，ADR-0221 Context 改记「别的控制事实作依据也只带版本，却不在此列」。
Blocked by: 无
地盘：TF `domain/actual_fulfillment_formation.go`（`EffectiveDeliveryEndBasis`）、`domain/actual_fulfillment_segment.go`（`EndParticipationWithDelivery`）、`application/form_actual_fulfillment_on_participation_end.go`（`OnDelivery`）。

## 核实（2026-10-19，票 111 评审之后，读码）

- 交付结果版本不由登记方给。`RegisterEffectiveDeliveryHandler` 首登与 POD 更正两条路都向 `ports.DeliveryIdentityFactory` 要新版本（`NextDeliveryResultVersion`）；PG 实现（`ResultVersions`）推进全局序列 `transport_fulfillment.delivery_result_version_seq`，拼成「前缀-十二位序号」。
- 生产代码里 `domain.NewDeliveryResultVersion` 只在读回处由外部串构造：更正命令的前版（`PredecessorVersion`，指名已在册的那一版）、读回信封、读回登记行。没有一处让登记方给出一个新版本。
- 所以 `EFFECTIVE-DELIVERY/<版本>` 在库内唯一，同一对象两次有效交付各有各的版本；下面「现象」第三条「登记方都把交付结果版本写成 `v1`」那一格走不到，自动形成那一路不会因此对上两段。交接一侧不同：交接版本是登记方给的，只在（对象，范围）之内唯一，那才是票 111 的缺口。
- 下文原样留作立票时的推理记录，不再成立。

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
