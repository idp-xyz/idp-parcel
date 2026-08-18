# ADR-0061: 已接受委托的重建门按快照表达能力开门

Status: Accepted
Date: 2026-08-18

## Context

[ADR-0030](./0030-rehydration-admits-one-state-at-a-time-by-snapshot-expressiveness.md) 把重建门的准入判据定在快照表达能力上，并写明**今天只开`已提交`**。`已接受`当时进不来，因为顶层六个产物字段里，决定、基线、承诺与客户资料版本都没有快照表达：读回一行已接受委托，会静默交出一份次零基线，后续资料更正全部落到基线外。

本期补齐了`已接受`可达字段的表达。按 ADR-0030 的两步顺序——先补齐表达，再从拒绝名单里放出来——现在可以开这一格。`已拒绝`与`已撤回`仍缺主动拒绝与撤回的完整表达，继续挡在门外。

本记录不改写 ADR-0030 的 Status，也不改它「按状态逐个开门」的判据；它只回答「`已接受`这一格现在开不开」。

## Decision

**一、重建门开到`已提交`与`已接受`。** `已拒绝`、`已撤回`仍由入口以 `ErrRehydrationStateNotSupported` 拒绝。

**二、`已接受`的快照表达如下；重建只校验、不重算 Decide / 基线 / 承诺。**

| 聚合字段 | 快照表达 | 备注 |
|---|---|---|
| `decision` | `RehydrateAcceptanceDecisionSpec` | 产物，无公开构造器。`Checks` 与 `Basis` 收成品类型（有公开构造器） |
| `decisionFormed` | `bool` | 接受路径为真；与决定在场同真 |
| `baseline` | `RehydrateAcceptanceBaselineSpec` | 产物。成员集合、提交版本、固定时刻 |
| `commitment` | `RehydrateExpectedCommitmentSpec` | 产物。`Basis` 收成品 |
| `sourceDataVersions` | `[]CustomerSourceDataVersion` | 成品类型；`已接受`下可空 |
| `withdrawal` / 主动拒绝 | **无** | 故`已拒绝`/`已撤回`仍不开 |

**三、半截的已接受快照是坏数据，不是「本期不支持」。** 状态列被推进到`已接受`而文档仍是已提交形状，走 `ErrInvalidRehydratedShipmentRequest`。门已经开了，缺产物就是这一行立不起来。

**四、PostgreSQL 快照文档按上表增设字段；读回只经公开访问器摊出、经领域构造函数重建。** 写入投影（INSERT/UPDATE 列）与迁移不在本记录范围内。

## Consequences

- `ShipmentRequestRepository.FindBySourceIdentity` 对已接受行交回带决定、基线、承诺的聚合；NR 接受决定消费者可以按引用读回委托，不再在重建门前失败。
- SYN-V0 已决定路径的停点从 ADR-0030 重建门推进到生产装配的 `nil` 适用性映射（`ROUTING_APPLICABILITY_UNAVAILABLE` / `dispatch.consumer_undecided`）。未决仍回滚 inbox，不写 `initial_route`。
- `已拒绝`/`已撤回`仍会让运维看到「等这扇门开到那个状态」，而不是去查一行其实完好的数据。
- 重建面新增三个受限标识：`RehydrateAcceptanceDecisionSpec`、`RehydrateAcceptanceBaselineSpec`、`RehydrateExpectedCommitmentSpec`。

## Alternatives considered

- **连`已拒绝`/`已撤回`一起开。** 否决：撤回与主动拒绝仍无快照表达。先放进来再补字段，正是 ADR-0030 要修的「宣称四个、其实一个」。
- **已接受缺产物时仍报 `ErrRehydrationStateNotSupported`。** 否决：违反 ADR-0029 分格。门已经开到这个状态，缺的是这一行的数据，恢复动作是查库或查适配器，不是等下一期开门。
- **读回时按当前版本重跑 Decide 以补产物。** 否决：ADR-0028 把重建定为只校验不重算。重跑会把当时的商业依据换成此刻的，接受基线不再是接受时固定的那一份。

## Links

- [ADR-0030：聚合重建按状态逐个开门](./0030-rehydration-admits-one-state-at-a-time-by-snapshot-expressiveness.md)：本记录推进它「今天只开`已提交`」那一格；判据与其余各条不变
- [ADR-0028：聚合的重建与构造分属两扇门](./0028-aggregate-rehydration-is-a-separate-door-that-validates-without-recomputing.md)：重建仍只校验不重算
- [ADR-0029：按标识取回原状态失败时按恢复动作分格](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：已接受缺产物与门外状态分两个哨兵
