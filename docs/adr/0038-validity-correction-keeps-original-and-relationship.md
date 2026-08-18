# ADR-0038: 有效性更正保留原版本与更正关系，不改正文

Status: Accepted  
Date: 2026-08-12

## Context

[UC-PC-001](../application/party-commercial/UC-PC-001-MAINTAIN-AND-PUBLISH-COMMERCIAL-AUTHORITY.md) `AT-PC-013`：「外部源更正历史有效区间 → 保留原版本和更正关系，推进修订标识。」

同用例：「正文变化创建新版本；有效性更正必须保留原版本、原判断和更正关系。」

今天 `Revise` 只改草稿正文；已发布走 `ErrCommercialContentIsFixed`。`Register` 把区间算进「同一次发布」——改区间会被读成 `CONFLICT` 并拒绝覆盖。`SupersededBy` 是后继版本替代，不是同源区间更正。缺少一条「区间变了、正文与批准仍是原来那次发布」的关系通道，修订标识也就无从因有效性更正而推进。

## Decision

**一、有效性更正是登记册上的独立事实，不改写原 `CommercialVersion` 键下的正文、批准与原区间。** 原版本继续 `Lookup` 可得。

**二、更正携带 `ValidityCorrectionReference` 与新的 `EffectiveInterval`，并显式指向被更正的对象版本。** 选用解析候选时，有更正则按更正后区间判断适用；无更正则仍用原区间。

**三、接纳更正必须使该范围的 `ViewRevision` 变化。** 消费方据此检测旧解析是否因有效性更正而失效。

**四、与 `Revise` / `SupersededBy` 分清。** 改正文另起版本；替代是后继版本关系；本记录只谈区间更正。

## Consequences

- AT-PC-013 由红转绿。
- `applicable` 读选用区间时必须问登记册有没有更正，不能只看版本值对象上的原区间。

## Alternatives considered

- **原地改 `effective`。** 否决：覆盖历史，与「保留原版本」冲突。
- **当新版本号发布。** 否决：正文未变却逼出新版本身份，和「正文变化创建新版本」抢语义；既有引用的版本号也对不上。
- **复用 `SupersededBy`。** 否决：替代结束旧版参与新选择；更正是同源区间修订，旧正文判断仍在。

## Links

- [UC-PC-001](../application/party-commercial/UC-PC-001-MAINTAIN-AND-PUBLISH-COMMERCIAL-AUTHORITY.md)：`AT-PC-013`
- [ADR-0037](./0037-publication-batch-is-per-item-not-all-or-nothing.md)：同属维护/发布机制半边，互不替代
- [ADR-0056](./0056-validity-correction-append-only-serialized-by-version-row.md)：补充多条只增与同版本写串行化；本记录各条不变
