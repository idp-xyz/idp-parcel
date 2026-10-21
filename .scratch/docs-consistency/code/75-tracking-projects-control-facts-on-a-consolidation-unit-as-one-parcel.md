# 75 交接、揽收、交付、外部承运追踪四路仍把集运单元当一件追踪包裹投

Category: enhancement
Status: needs-triage——(a) R2（上线后尽快）：整袋交接是常态，追踪上每交接一袋就多一份不是包裹的追踪投影，违背 VE CONTEXT「集运、装载或同批关系不合并包裹身份和客户轨迹」。(b) 归后续。2026-10-21 协调方据票 demo-chain-gaps/123 隔离评审立（作者子代理代笔）。
Blocked by: (a) 无；(b) [demo-chain-gaps/80](../../demo-chain-gaps/issues/80-a-consolidation-unit-handover-does-not-move-member-control.md)（ADR-0263 决定四）
上下文：VE（全程追踪）。
出处：票 [demo-chain-gaps/123](../../demo-chain-gaps/issues/123-tracking-projects-a-departing-bag-as-one-parcel-and-its-members-get-no-departure.md) 完成记录越权风险点 3；其真库补派生用例里，袋经交接进段时交接那一封给袋立了一条交接条目。

## 现象

票 123 只改了移动事实那一路与补派生。其余四路按控制事实或轨迹事实的对象引用直接立追踪包裹，对象是集运单元时照旧把它当一件包裹投：

- `internal/visibilityexception/adapters/transportfulfillment/derive_on_transport_handover.go`（运输交接）
- `internal/visibilityexception/adapters/transportfulfillment/derive_on_offsite_pickup.go`（对象级场外揽收）
- `internal/visibilityexception/adapters/transportfulfillment/derive_on_effective_delivery.go`（有效交付）
- `internal/visibilityexception/adapters/transportfulfillment/derive_on_external_carrier_tracking.go`（外部承运轨迹）

证据只记 `S`（读码）。

## 拆两半

### (a) 不再把单元当追踪包裹投——R2，机制，现在就能做

- 四路在立追踪包裹之前，经票 123 的 `veports.ConsolidationMembershipView` 问一次「这一件那一刻是不是集运单元」（时点取那份事实的业务发生时间）；是单元就入账不派生，不为它立投影；读不动记未决，答复不一致记发布失败，未决名单各路自己登。
- 不在这一半把事实投到成员上：交接与交付对成员意味着控制转移，ADR-0263 决定四在单元控制规则立起来之前不为成员形成参与或控制判断，投影若先投给成员，追踪上就出现一份运输履约自己都没认的成员控制。
- 规则句补进 VE CONTEXT「追踪投影与里程碑」票 123 那一条旁边。存量袋投影不清理（同票 123 越权风险点 2）。

### (b) 把交接、交付投到成员上——Blocked by 票 80

- 票 80 定下成员随单元受控的派生规则之后，交接、交付按那条规则投到成员上；揽收与外部承运轨迹看那时的规则是否同样适用。

## 三类

(a) 机制。(b) 机制，口径随票 80。

## 既有票

- [demo-chain-gaps/123](../../demo-chain-gaps/issues/123-tracking-projects-a-departing-bag-as-one-parcel-and-its-members-get-no-departure.md)；[demo-chain-gaps/80](../../demo-chain-gaps/issues/80-a-consolidation-unit-handover-does-not-move-member-control.md)
