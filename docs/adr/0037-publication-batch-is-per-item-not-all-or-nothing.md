# ADR-0037: 发布批次是逐对象折叠，不是全有或全无的聚合

Status: Accepted  
Date: 2026-08-12

## Context

[UC-PC-001](../application/party-commercial/UC-PC-001-MAINTAIN-AND-PUBLISH-COMMERCIAL-AUTHORITY.md) `AT-PC-011`：「一个批次中产品合法、合同冲突 → 产品可以发布，合同冲突；批次不全量回滚。」

同用例写明：「发布批次不是聚合……各自拥有独立结果」；步骤 6：「批次中其他对象失败不回滚合法版本」。

今天领域只有单对象 `Publish` + `Register`。若编排层用「任一项失败则撤销本批已写入」表达批次，就直接违背上述句子。需要一处机制半边的结果代数，把「逐项独立」钉死，而不是靠调用方自觉。

## Decision

**一、`CommercialRegistry.PublishBatch` 是对「Publish 然后 Register」的有序折叠。** 每一项各自得到 `PublicationBatchItemResult`（发布错误或登记结果）；批次本身不是聚合根，也没有批次级成功/失败枚举。

**二、已成功 `CREATED`（或 `REPLAY`）的项不得因后续项失败被移出登记册。** 全量回滚不是合法实现。

**三、指名引用存续可随批次推进：lookup 未给出时，以登记册当前已发布对象作答。** 这样同一批次内「先产品后合同」能看见刚写入的产品；不引入 Outbox / 事务适配器。

**四、本记录不覆盖事件投递续办（`AT-PC-016`）。** 那条仍受 Outbox 闸门阻断。

## Consequences

- AT-PC-011 由红转绿：产品 `CREATED` 与合同 `CONFLICT` 可同批并存。
- 调用方必须按项处理结果，不能只看「批次 error」。

## Alternatives considered

- **批次级事务 / 全有或全无。** 否决：与 UC「不全量回滚」直接冲突。
- **只在 application 层循环、领域不露面。** 否决：没有具名结果代数时，「不回滚」无法被领域测试钉住，编排一换就丢。

## Links

- [UC-PC-001](../application/party-commercial/UC-PC-001-MAINTAIN-AND-PUBLISH-COMMERCIAL-AUTHORITY.md)：`AT-PC-011`
- [ADR-0036](./0036-publication-requires-named-references-published.md)：单项发布的引用闸门；批次折叠仍逐项遵守
