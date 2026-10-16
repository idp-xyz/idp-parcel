# 集运加成员不核任何方向性依据：已取消、已终局、无路由、身份未定的实物都装得进袋

Category: bug
Status: needs-triage——2026-10-16 Claude Code 会话立（票 [operator-channel/25](../../operator-channel/issues/25-node-reception-carries-parcel-shipment-service-result-markers.md) 实施中量到）
Blocked by: 无
出处：UC-NO-003 的业务规则「方向性作业必须取得当前有效路由指令；集运还必须取得成员级集运兼容判断」、输入表「包裹与服务」一行（`parcel-shipment` 的当前取消/终局结果）、`AT-NO-031` 与 `AT-NO-042`；NO CONTEXT「候选尚未确认时不得据此执行方向性作业」与「已取消、已经终局或当前无有效路由的包裹仍然到站时……」两条。

取证于 `0214a62a`（2026-10-16）。

## 事实

`internal/nodeoperations/application/consolidate_parcels.go` 的 `ConsolidateParcelsHandler.AddMember` 在幂等闸之后只问两件事：单元在不在（`Store.FindByID`），成员此刻是否已被别的单元直接包含（`Containment.CurrentParent`）。`ConsolidateParcelsDeps` 里没有路由指令、集运兼容、服务结果或身份的读口。于是：

- 收寄记录上带 `CANCELLED/…` 或 `FINAL/…` 标记的实物（票 25 落的），加进袋照常 `MEMBER_ADDED`——标记今天没有任何消费方。
- 没有当前有效路由指令的实物照常进袋（`AT-NO-031` 要求阻止集包）。
- 待识别或身份冲突的实物照常进袋（ps-port-remainder/05 的完成记录已记过「`AddMember` 不核成员是否已识别」，那一票只为阶段读面如实答`不可归属`，没有补门禁）。

票 25 的完成记录说「标记限制后续方向性作业」此刻只体现在记录与复核目录上，缺的就是这一半。

## 要裁的问题

1. **门禁放在哪一步。** 加成员、封装、交出各自核，还是只在加成员核（封装与交出继承成员已过门）。
2. **依据从哪里来。** 服务结果：读收寄记录上的标记（收寄那一刻的快照，之后 PS 新形成的取消读不到），还是加成员时再经票 25 的 `ParcelServiceResultView` 现问。路由指令与集运兼容：network-routing 今天有没有可消费的读口，没有时是本票阻断还是答`未决`。
3. **挡下来答什么。** UC-NO-003 结果契约里的「方向性作业受阻」（对象、拟执行动作、缺少依据、当前安全作业和续办入口），与现有 `ConsolidationNotAccepted` / `ConsolidationUndecided` 两格怎么对上；依赖读不动与「依据明确不成立」要分开。
4. **已取消或终局的实物要处置时怎么走。** `AT-NO-042`「方向性处置等待授权」：授权从哪来（PS 的收寄后服务处置决定？），本票接不接。

## 不做

- 不改「标记不阻止接收」（UC-NO-002）。
- 不在 NO 判断取消或终局是否成立——那是 PS 的事，NO 只消费结果。
