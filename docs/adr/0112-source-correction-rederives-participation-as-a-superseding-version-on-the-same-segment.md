# ADR-0112: 来源更正在同段内以「替代参与版本」重派生履约参与关系——参与关系上长回指前版的链、当前参与按链尾派生、与更正编排同事务作派生一侧、段已关闭仍重派生；撤回控制的更正是失效格，形状随本链、实施另票

Status: Accepted
Date: 2026-09-04

## Context

`transport-fulfillment` CONTEXT 生命周期一句：「来源证据被更正或事件有效性变化 → 保留原段、原参与关系和原判断，形成失效或替代关系并重新派生当前有效控制与履约结果」；Rules 一句：「已经成立的实际履约段及履约参与关系不能被取消、删除或回写为未发生。来源证据被更正时，保留原事实和原判断，形成失效、替代及重新派生结果」；段结束那一句给了例外格：「实际履约段结束 → 判断历史封存：不再接受新版本，除来源事实更正引起的重新派生」。

票 [tf-segment-lifecycle-closure/10](../../.scratch/tf-segment-lifecycle-closure/issues/10-source-correction-rederives-participation.md) 量到的事实：这句话的前半今天两种来源都有——`TransportHandover.Correct` 与 `OffsitePickup.Correct` 都形成回指前版的新版本；后半两种来源都没有——`RegisterTransportHandoverHandler.Correct` 与 `RegisterOffsitePickupHandler.Correct` 只落新版本并重交意图，不碰段登记册。而参与关系以来源版本为入场依据（`JoinWithPickup` 取 `OFFSITE-PICKUP/<版本>` 与 `OccurredAt`，`JoinWithHandover` 取 `TransferOutBasis` 与 `JudgedAt`），更正一旦改了控制证据或发生时刻，段里那条参与指着的就是一个已被回指的版本、一个已被更正的起点。票 08 裁决已答：更正改了控制证据或发生时刻是**同段**不是新段——「承运责任变了才是另一段」（ADR-0103 主体判据），更正证据或时刻不改变谁在控制。

库面上 `fulfillment_participation`（迁移 0006）主键取（租户+段+对象），头注写「已结束的参与也不重开——再次进入是新的段，所以这里不需要版本维」；段登记册（ADR-0097）刻意没有通用 Update，只有 `Join` / `EndParticipation` / `CloseSegment` 三个窄写口，「没有任何一条路径能把已发生的写回未发生」。

同族先例：ADR-0117 为 `parcel-shipment` 采用账选了「只插不改地长版本链、当前按链尾派生」；`offsite_pickup`（迁移 0015）与 `effective_delivery` 的更正链同形。

## Decision

**一、同段内的「替代参与」是参与关系上的一条链：新参与关系回指被替代的参与，原参与一字不动，当前参与按链尾派生。** `FulfillmentParticipation` 增加「被替代的入场依据」（`Supersedes`，即前一版参与的入场依据——来源版本引用）；同一对象在同一段里的参与由此成一条链，根是首次入场，此后每一版更正回指前一版。`ParticipationFor(object)` 答链尾（没有任何参与回指它的那一条），`ActiveParticipations` 只数链尾；被回指的参与在聚合内标为已被替代（派生态，不落列），它的 `Active()` 为否——它不再表达当前控制。库面主键从（租户+段+对象）改为（租户+段+对象+入场依据），两条部分唯一索引守形状：每对象一个根（`supersedes IS NULL`）、同一入场依据至多被替代一次；自引用外键守链不悬空、不跨对象。**不采**「段上另立一条参与并把原参与标为被替代」——那要 UPDATE 原参与，违「只插不改」；也不给参与关系另立结果词。

选与 ADR-0117 同形是有意的：两处要表达的是同一件事——原判断不删、替代关系显式、「当前」是派生不是存的；两处读法也一致（`ParticipationFor` 与 `FindResponsibilityStart` 都答链尾）。差别只有一处：PS 那条链的键上带来源版本，TF 这条链的键上带入场依据——入场依据本就是来源版本引用的写法，是同一件东西。

**二、重派生与更正编排同事务，作派生一侧。** 输入全在 TF：更正后的新版本（来源登记册）与该对象在段上的参与（段登记册）。票 09 裁决③否掉同事务的判据是「一条控制事实登记编排要去读三个外部上下文」，这里一个外部上下文都不读，判据不成立；票 06 把「结束前一段参与」放进交接编排同事务的理由（「TF 自己的生命周期规则、输入全在 TF」）逐字适用。与 `enterFulfillmentSegment` 同一条纪律：**派生一侧的失败不得回滚来源更正**——段登记册读不到或写不进只留续办引用；领域正当拒绝不留引用但单开答格（`SegmentEntryRefusal` 加两格：`NO_PARTICIPATION_TO_REDERIVE`、`CORRECTION_WITHDRAWS_CONTROL`）。两条触点 `RegisterTransportHandoverHandler.Correct` 与 `RegisterOffsitePickupHandler.Correct` 共用一处重派生门（`rederiveFulfillmentParticipation`），与两条来源共用 `enterFulfillmentSegment` 同形。段由登记册按对象反查（`FindActiveSegments` 之外多一口按对象取「在场或已离场的当前参与所在段」——更正的对象可能早已离场），更正命令不带段号：更正的输入是「证据说了什么」，段是派生知道的事。

**三、段已关闭仍重派生。** 这正是 CONTEXT 例外格说的那件事：判断历史封存「不再接受新版本，**除来源事实更正引起的重新派生**」。替代参与照插进已关闭的段，段不重开、不再关一次；替代参与继承原参与的离场三件（对象的控制终点是有效交付 / 下一次交接 / 明确终止那个事实，更正入场不改它），更正后的起点不得晚于继承的终点——晚于就是「先结束再进入」，是领域正当拒绝。`CloseSegment` 那条「仍有在场参与关不上」与「关闭不早于任何终点」两条不变量对链尾成立即可，重建门照此复核。

**四、更正撤回了控制转移（`已交接` 被更正为拒收或待确认）是失效格，本记录只定它属于同一条链，实施另票。** 撤回控制的版本没有入场依据可立（`TransferOutBasis` 不给），形状是链上一条回指前版、标「失效」的版本：链尾失效即该对象在本段当前无有效参与；原参与仍一字不动。列、读法与它对关段不变量的影响随实施票 [tf-segment-lifecycle-closure/11](../../.scratch/tf-segment-lifecycle-closure/issues/11-control-withdrawing-correction-voids-participation.md) 落地；本记录的实施对这一格如实答 `CORRECTION_WITHDRAWS_CONTROL`，不猜也不静默。

## Consequences

- 两种来源的更正在段上行为一致（票 08 裁决原句）：都经同一道重派生门，都形成替代参与版本。
- `Participations()` 交回全部版本（历史），`ParticipationFor` 交回链尾（当前）；段上按对象折叠的读法（成立时刻取最早入场、在场计数）都按链尾或全集各取所需，本记录逐处点名不留默认。
- 段登记册的两个窄口 `EndParticipation` 与 `FindActiveSegments` 的「在场」判据从 `ended_at IS NULL` 变为「`ended_at IS NULL` 且无人回指」——被替代的版本永远不会被结束（它不再是当前控制），也不算在场。
- 代价：TF 迁移 `0016`（参与表加 `supersedes_entry_basis`、主键换四元、两条部分唯一索引、自引用外键）；`ActualFulfillmentSegmentRegistry` 多一个窄写口 `Supersede`（只插不改，与 `Join` 同形）与一个按对象的读口；`RegisterTransportHandoverDeps` / `RegisterOffsitePickupDeps` 不加依赖——重派生用既有 `Segments`。
- 不在本记录内：实际承运商判断按更正关系重新派生是 `ActualCarrierJudgment` 自己的下一版，走它自己的口；参与关系失效格（决定四）的落地；`network-routing` / `parcel-shipment` 对更正后参与的消费。

## Alternatives considered

- **段上另立参与、原参与标被替代（UPDATE 原行）。** 否决：违「只插不改」；且 ADR-0097 把段登记册做成没有通用 Update 正是为了让这类改写表达不出来。
- **更正即结束原参与再新入场（`End` + `Join`）。** 否决：结束需要一个控制终点事实（有效交付 / 下一交接 / 明确终止），更正不是其中任何一种，硬造一格「因更正结束」就是给「说不清为什么结束」开路（迁移 0006 头注明拒的那一格）；而且已离场的原参与无法再「结束」。
- **异步执行器消费更正信封再重派生。** 否决：输入全在 TF，跨拍只换来一段「更正在册、参与仍指旧版」的窗口与一个新的失败预算消费者；票 09 那条走异步是因为要读三个外部上下文。
- **段已关闭时拒绝重派生。** 否决：CONTEXT 例外格逐字写着更正引起的重新派生不受封存约束；拒绝会让已关闭段上的参与永远指着被回指的版本。

## Links

- [TF CONTEXT](../domain/transport-fulfillment/CONTEXT.md)：「履约参与关系」词条、Rules「不能被取消、删除或回写为未发生……形成失效、替代及重新派生结果」、生命周期「来源证据被更正或事件有效性变化」「判断历史封存……除来源事实更正引起的重新派生」
- [ADR-0097](./0097-segment-evolution-writes-through-narrow-doors-not-a-general-update.md)：段登记册窄写口、没有通用 Update
- [ADR-0103](./0103-actual-carrier-judgment-is-a-versioned-record-per-segment-with-pending-as-a-value.md)：段的主体判据「承运责任变了才是另一段」
- [ADR-0117](./0117-same-source-correction-forms-a-superseding-adoption-version-chained-to-the-current-responsibility-start.md)：PS 采用账的同形链与选形理由
- [票 tf-segment-lifecycle-closure/08](../../.scratch/tf-segment-lifecycle-closure/issues/08-offsite-pickup-correction-model.md)：「同段不是新段」的裁决附问
- [票 tf-segment-lifecycle-closure/10](../../.scratch/tf-segment-lifecycle-closure/issues/10-source-correction-rederives-participation.md)：本记录的实施

## owner 复核记录

- owner 复核 2026-09-09 认可（用户 2026-09-09 12:3x 经 IDP 队列通道 1 授权「你自决，目标是全部解决」，通道 1 代裁）：随 tf/10、tf/11 进 main 复核——决定三「替代版本继承离场三件」自 tf/11 起同样适用于**失效**版本（失效与替代同是链上一个版本，规则说的是版本不是替代）；`SegmentEntryRefusal` 自 tf/10 起多出本记录未点名的第三格 `CORRECTED_START_AFTER_INHERITED_END`（更正后的起点晚于继承的终点），与决定五同族；决定正文字面不改，以本条为准。tf/11 退掉的 `CORRECTION_WITHDRAWS_CONTROL` 一格同认可（撤控制的更正自 tf/11 起使参与失效而非被拒，那一格没有对象了）。