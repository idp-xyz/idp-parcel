# 77 发生项登记的事实读口不分重建失败：读回的坏行也答成可重试的未决

Category: enhancement
Status: needs-triage——2026-10-19 Claude Code 作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [28](./28-booking-and-cancellation-occurrences-and-procurement-context-from-adoption.md) 的隔离评审（非阻断项，评审未要求进 main 前修）。号取协调方给票 28 预分配的号段。
Blocked by: 无
出处：[ADR-0029](../../../docs/adr/0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)（失败按恢复动作分格）；[ADR-0207](../../../docs/adr/0207-transport-charge-occurrences-are-registered-by-an-operation-decision-and-handed-off-in-the-same-transaction.md) 正文与文末补记（发生项登记取事实、经委托取采购上下文）。

## 现象

运输收费发生项登记（`POST` 发生项登记口）取事实与采购上下文时，履约段、订舱、承运答复、运输委托、段采用连线这几个读口只要交回错误，用例一律答 `UNDECIDED`（`FACT_SOURCE_UNAVAILABLE` 或 `PROCUREMENT_SOURCE_UNAVAILABLE`）并给续办引用，叫登记方原样重试。

TF 的 postgres 读适配器在读回时重建领域对象。重建失败——列值过不了领域构造门、成对的列只剩一列（库约束之前写下的行，或约束没覆盖到的组合）——与连接断、超时一样只包一层普通错误交回。这一类重试永远不会变：登记方照续办引用重交，每次得到同一个未决，看不出是库里的坏数据。

段读口是票 27 起就这样，票 28 加的订舱、答复、委托、采用连线照同一做法，不是票 28 新引入的分类缺口。

## 证据（读码）

- `internal/transportfulfillment/application/register_charge_occurrence.go`：`actualFulfillmentFact` 读段、`commissionAdoptedBy` 读采用连线与委托，err 非空即答未决（后者经 `errProcurementSourceUnreadable`）。
- `internal/transportfulfillment/application/register_booking_charge.go`：`bookingFact` 读订舱、委托、答复，同上。
- `internal/transportfulfillment/adapters/postgres/transport_commission.go` 的重建（协议对象与版本只存了一列时的那条错误、`domain.RehydrateTransportCommission` 交回的错误）都是 `fmt.Errorf` 包一层，没有可识别的哨兵；其余几册的读适配器同形。应用层因此分不出。
- 对照：SA 立了 `saports.ErrSupplierAgreementScopeUntranslatable`，`internal/settlementaccounting/adapters/partycommercial/supplier_agreement_scope.go` 包住它，SA 编排凭它与读不动分开。
- 票 [67](./67-actual-fulfillment-occurrences-form-from-handover-and-delivery-envelopes.md)（ADR-0218）的自动形成编排，`internal/transportfulfillment/application/form_actual_fulfillment_on_participation_end.go`，在交给登记用例之前自己也读一遍，同形：
  - `OnHandover` 读交接；
  - `formFor` 按对象列段、按键读整段（`ParticipationSegmentsView` 两口），读采用连线，读旅程册（postgres `AlternateJourneys.FindByMember`）。
  - 任一口交回错误都答本编排的未决，经 `ErrActualFulfillmentFormationUndecided` 登进派发的未决名单。读回的坏行同样被当成读不动。

## 自动形成之后的分量（2026-10-19 票 67 评审补记）

以前这一类只在人工口上出现，登记方看到一次未决、照续办引用重交。ADR-0218 之后，同一个登记用例也由派发进程随交接与有效交付信封调用。读回坏行答成未决，就会以 `consumer_undecided` 挂在交接或交付分区的队头，烧完失败预算才落 `ABANDONED`。在它之前，同一对象后面的交接与交付信封（追踪投影、派送发起、包裹终局）都跟着等（ADR-0218 越权风险点 8，票 [89](./89-actual-fulfillment-formation-blocks-later-envelopes-in-the-handover-and-delivery-partitions.md)）。修法不变：自动形成编排那几口见到同一枚确定性哨兵，上抛，不答未决。

## 三类

机制（失败按恢复动作分格）。规模 S–M。

## 最小的诚实修法

在 TF `ports` 立一枚「读回的行重建不了」的确定性哨兵，postgres 读适配器重建失败时包住它；发生项登记见到它整笔交回 error（装配点回滚，线上答 500），不给续办引用，与已有的数据违约（`ErrAdoptionWithoutCommission`、`ErrBookingWithoutCommission`）同样处理。

## 要裁的

- 哨兵的粒度：TF 一枚通用的「行不可重建」，还是逐册各一枚。归 TF owner。
- 同一批读口的其余编排（承运编排读委托、订舱、答复，段采用读段与委托）要不要同票一起分。归 TF owner。自动形成编排那几口（见上「证据」）建议同票分，理由见「自动形成之后的分量」。
- 与 [54](./54-charge-occurrence-registration-does-not-separate-deterministic-agreement-check-failures.md)（协议核对的确定性失败）并成一票还是分开：同是发生项登记把确定性失败说成未决，修法同形（`ports` 立哨兵、适配器包、编排上抛）。

## 既有票

- [54](./54-charge-occurrence-registration-does-not-separate-deterministic-agreement-check-failures.md)：同一用例里协议核对那一步的同类问题。
- [39](./39-buy-evaluation-request-handoff-does-not-separate-deterministic-rejection.md)、[41](./41-transport-commission-handoff-does-not-separate-deterministic-rejection.md)：交接口的同类问题。
