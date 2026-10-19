# 38 BUY 评价请求交接口不分确定性拒收与暂时故障：发生项已形成那一路会重投到失败预算耗尽

Category: enhancement
Status: needs-triage——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [26](./26-settlement-consumes-charge-occurrence-formed-to-request-buy-evaluation.md) 的隔离评审（非阻断项，评审建议留给 SA owner）。
Blocked by: 无
出处：[ADR-0208](../../../docs/adr/0208-buy-evaluation-is-requested-from-the-charge-occurrence-formed-envelope.md) 越权风险点 6。

## 现象

发生项已形成那一路（ADR-0208）在消费事务里登记 BUY 评价请求并交信封。交信封一失败，触发编排一律答`未决`、整笔回滚，派发把它落 `dispatch.consumer_undecided` 并重投。框架对信封的确定性拒收（`eventing.ErrInvalidEnvelope`，例如租户标识超长时作用域或分区键过不了上限）重投同一份永远同一个结果，于是一直重投到失败预算耗尽，才以未决之名落下，看不出是坏数据。

## 证据（读码）

- `internal/settlementaccounting/adapters/postgres/evaluation_request_handoff.go` 的 `HandOffEvaluationRequest`：`EnqueueOnce` 的错误原样包一层交回，不分 `eventing.ErrInvalidEnvelope`。
- `internal/settlementaccounting/application/request_buy_evaluation.go` 的 `handOff`：任何交信封失败都留续办引用、照答已请求。
- `internal/settlementaccounting/application/trigger_buy_evaluation_on_occurrence_formed.go` 的 `Trigger`：见到续办引用答`未决`（`BuyEvaluationTriggerRequestHandoffUnavailable`）。
- 对照：SELL 那一路已经分开——`ports.ErrSellEvaluationRequestHandoffRejected` 由交接口包住 `eventing.ErrInvalidEnvelope`，`RequestSellEvaluationOnParcelMomentHandler` 见到它整笔交回 error。

## 三类

机制（失败按恢复动作分格，ADR-0029）。规模 S。

## 最小的诚实修法

照 SELL 的做法：BUY 交接口遇 `eventing.ErrInvalidEnvelope` 包一枚确定性哨兵（例如 `ports.ErrEvaluationRequestHandoffRejected`）；发生项已形成那一路见到它原样上抛、落 `publish_failed`，不答未决。

## 要裁的

哨兵从哪一层透出来。`RequestBuyEvaluationHandler` 同时服务 ADR-0164 那条由调用方交齐的触发面与 `cmd/parcel-api` 的评价请求端点，那两处今天对任何交信封失败都答「已请求、留续办引用」。改成确定性拒收整笔交回 error，会改变那两处的答法（端点从「已形成答案」变成报错）；只在触发编排里另认，又要编排能看见交接口的错误。归 SA owner。

## 既有票

none
