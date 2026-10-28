# ADR-0333：节点监管协作的「待补充」是落库的一轮决定，补充后的决定是同一事项的下一轮——与 TF 的待补充答复形不同

Status: Accepted（2026-10-28，Claude Code 会话代 NO owner 决（代决）：用户常设授权「你作为业务和系统专家自决」；票 docs-consistency/code/18，评审要求补 ADR。待 NO owner 复核。不改 ADR-0265 正文。）
Date: 2026-10-28

## Context

- NO CONTEXT 与 GLOSSARY「节点监管协作承接决定」：决定可以是已承接、部分承接、待补充或拒绝；UC-NO-001 把「节点协作待补充」列为结果，要求保留已知范围、缺口、责任方与补充入口，`AT-NO-003` 要求可核对。代码原先只有接受、拒接、部分承接三格，主键为（租户+事项），同一事项只决定一次。
- [ADR-0265](./0265-a-regulatory-disposition-supplement-required-is-an-answer-not-a-recorded-decision.md) 把 TF 的待补充定为答复、不落记录，理由是缺的东西（移动授权、对象、动作、起止责任）都在关务一侧，TF 落一条记录没有自己能推进的下一步。

## Decision

**一、NO 的待补充是落库的一轮决定。**（机制）

待补充记录缺口清单（对象、动作、节点、授权依据、必要作业条件五类，至少一项，封闭），不带承接范围，不形成承接；其上不能登记执行事实。与 ADR-0265 不同的理由：UC-NO-001 要求待补充可数、可查、带补充入口；缺口里的节点与必要作业条件归 NO 自有（必要作业条件的取值归租户节点登记，本记录只指名缺哪一类），本上下文有自己能推进的下一步。

**二、轮次链。**（机制）

承接决定主键取（租户+事项+轮次），迁移 `0024`。补充到位后的决定是下一轮，回指上一轮（上一轮必为待补充），原行不改写。「同一事项只决定一次」放宽为「同一事项同一轮只决定一次；待补充除外，可有下一轮」。当前轮已是非待补充决定时，不同内容仍是冲突。读回交最新一轮。

**三、发布意图带轮次。**（机制）

事件 ID 与载荷带轮次，待补充与其后的新决定各发一份；载荷只是门铃，下游按（租户+事项）读最新一轮。

## 候选与反方

- **照 ADR-0265 做成答复。** 否决：UC-NO-001 要求待补充可查带入口，且节点与作业条件缺口归 NO 自己补。
- **原行就地改成新决定。** 否决：改写历史，待补充不可数。

## Consequences

- `CollaborationAcceptanceRecord` 带 `Round`；`FindByKey` 取最新一轮；新增结果格 `PENDING_SUPPLEMENT` 与 `ITEM_PENDING_SUPPLEMENT`。
- 载荷规范化的 `gaps` 带 `omitempty`，其余三值的 NOC-1 文档不变。

## 越权风险点

1. **整件待补**（NO owner）。待补充不带承接范围，所以 `AT-NO-002`「P1 已承接、P2 待补充」在一轮里表达不了：要么整件待补，要么部分承接并把 P2 记作未承接原因；后者一经落下，补齐后不能再改（与 ADR-0265 越权风险点 3 同形）。
2. **重放旧轮**。当前轮已是非待补充决定时，重放旧轮的待补充答冲突，不答已有决定。
3. **并发补充**。两份补充后决定并发，败者读回赢家直接作答，不比内容。
4. **责任方未建模**。缺口类别指向去哪补，不另存责任方。

## Links

- [ADR-0265](./0265-a-regulatory-disposition-supplement-required-is-an-answer-not-a-recorded-decision.md)：TF 待补充为答复——对照
- [UC-NO-001](../application/node-operations/UC-NO-001-ACCEPT-AND-EXECUTE-CUSTOMS-NODE-COLLABORATION.md)；票 `.scratch/docs-consistency/code/18-node-customs-collaboration-acceptance-lacks-pending-supplement.md`
