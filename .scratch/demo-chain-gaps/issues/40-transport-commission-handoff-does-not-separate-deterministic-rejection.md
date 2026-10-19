# 40 运输委托交接口不分确定性拒收：分区键越限时委托落册，意图却永远入不了队

Category: enhancement
Status: needs-triage——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [23](./23-transport-commission-submission-face.md) 的隔离评审（非阻断项，评审未要求进 main 前修）。
Blocked by: 无
出处：envelope-id-bounds/01「残余」那条：「可读的分区键与 Subject 仍随各维长度涨……越过仍会被确定性拒收」，以及同票「运输履约那一半收口时如再碰到，再议」；[ADR-0202](../../../docs/adr/0202-transport-commission-booking-and-carrier-answer-entry-families-and-supplier-agreement-check.md) 决定六「意图与委托同一笔入队」。

## 现象

委托提交口（`POST /transport-fulfillment-transport-commission-submissions`）的信封 ID 已改为指纹，长度固定；分区键仍是可读的「租户/委托/口名」，Subject 是委托标识，两者都随标识变长。租户标识加委托标识长到分区键越过 `eventing.MaxPartitionKeyLength` 时：

- 框架在入队前的信封校验就拒收了，不是 SQL 错误，事务没有中止；
- 编排的 `handOff` 把任何交信封失败都折成续办引用，照答 `COMMISSION_SUBMITTED`；
- 装配那一笔照常提交：委托落了册，答 201 并带 `handoffReference`，「委托已提交」意图却不在 Outbox 里；
- 登记方照续办引用原样重交，答 `EXISTING_COMMISSION`，重发的仍是同一份越限信封，每次原样再失败。

结果是 ADR-0202 决定六写的「同一笔」被打破，而且看不出是坏数据。今天委托信封没有消费者（ADR-0066「缓」），所以眼下丢的只是一封没人收的信封；等后继消费者接上，丢的就是它要认领的那份意图。

## 证据（读码）

- `internal/transportfulfillment/adapters/postgres/transport_commission_handoff.go`：`transportCommissionPartitionKey` 原样串接；`HandOffTransportCommission` 把 `outboxintent.EnqueueOnce` 的错误原样包一层交回，不分 `eventing.ErrInvalidEnvelope`。
- `internal/transportfulfillment/application/commission_transport.go` 的 `handOff`：任何交信封失败都留续办引用。
- `cmd/parcel-api/assemble_transport_commission.go` 的 `transactionalTransportCommissionSubmitter`：只在编排答`未决`或交回 error 时回滚。
- 用例 `TestATransportCommissionEnqueuesPastTheConcatenatedIDLimit` 只证到信封 ID 上限那一档，不证分区键上限。
- 对照：SELL 那一路已经分开——`ports.ErrSellEvaluationRequestHandoffRejected` 由交接口包住 `eventing.ErrInvalidEnvelope`，编排见到它整笔交回 error。BUY 那一路的同类问题见 [39](./39-buy-evaluation-request-handoff-does-not-separate-deterministic-rejection.md)。

## 三类

机制（失败按恢复动作分格，ADR-0029；同笔不变量）。规模 S。若选「在草稿门限委托标识长度」，上限值是产品策略，不是租户取值。

## 最小的诚实修法

二选一：

1. 照 SELL 的做法：TF 交接口遇 `eventing.ErrInvalidEnvelope` 包一枚确定性哨兵；`handOff` 见到它不再留续办引用，编排整笔交回 error，委托不落册，线上答 500。委托册里就不会出现没有意图的委托。
2. 在委托草稿门限委托标识长度，越限答 `INPUT_NOT_ACCEPTED`。登记方拿到的是可改的输入问题，而不是 500。但上限要按租户标识的上限反推，口名、分隔符也算在里面，所以要在一处定义，且与分区键的拼法同源。

## 要裁的

- 选 1 还是选 2，还是两者都做：2 让登记方能改，1 兜住将来别的越限来源。
- TF 其余带可读分区键的交接口要不要一并处理，还是等各口第一次有生产方时逐口收。归 TF owner。

## 既有票

- envelope-id-bounds/01（登记了这项残余，没有给它去处）
- [39](./39-buy-evaluation-request-handoff-does-not-separate-deterministic-rejection.md)（SA 侧同类）
