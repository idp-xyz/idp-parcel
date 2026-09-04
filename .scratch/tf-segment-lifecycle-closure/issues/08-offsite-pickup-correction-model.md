# 揽收登记的更正：新版本还是失效 + 替代

Category: enhancement
Status: ready-for-agent——领域问题已裁（见「裁决」节，MCP-3 2026-09-04，owner 授权自决）：**A 新版本**，段侧参与关系重派生不在本票
Blocked by: 无（不阻塞 04–07）

## 裁决（MCP-3，2026-09-04）

**取 A「新版本」。** 更正形成一条新的 `OffsitePickup` 版本，回指被更正版本（`Corrects`），原版本与原判断不动；
`OffsitePickupRegistry` 仍**只插不改**；parcel-shipment 按版本幂等采用，新版本再采用一次是**正确行为**——PS 的
责任起点与正式承诺生效时间锚在 `OccurredAt` 上（`OffsitePickup.OccurredAt` 自注），发生时刻被更正时 PS 必须再判一次。

依据三条，都是已经写在仓里的话，不是新裁量：

1. `domain.PickupResultVersion` 的注释原句「**来源更正形成新版本不覆盖本版**」；`register_offsite_pickup.go` 对同键不同
   内容的处置注释原句「首登不顶替，**来源更正走新版本**」。B 路等于推翻这两句。
2. 本上下文已有五处更正形状同为 `Corrects` 回指 + 新版本新登记（`TransportHandover.Correct`、`EffectiveDelivery.CorrectProof`、
   `TransportMovementFact`、`LoadAssignment`、`TransportChargeOccurrence`），登记册一律只插不改（ADR-0097 窄口的同一结构
   判据：「回写为未发生」在这个口上表达不出来）。B 路要给登记册开改写口，是本上下文第一处例外，而例外没有理由。
3. CONTEXT 生命周期⑤「保留原段、原参与关系和原判断，形成失效或替代关系并重新派生」——「失效」在版本链里由「被后续版本
   回指」表达（`TransportHandover` 的汇总注释原话：被回指的那一代不计，它留在册上），不需要一个可改写的失效位。

**更正携带什么、不携带什么，照 `HandoverCorrection` 的取法**：只带「证据说了什么」——地点、控制依据、执行方、发生时刻
四格，加新版本号与更正时刻；**不带**「这是哪一次」——对象、任务、尝试三格沿用被更正版本，改了它们就是另一次揽收而不是更正。
每格完备性同首登（控制依据仍必备：更正不能把一次揽收更正成一次失败到访——那是另一种事实，走别的口）。更正时刻不得早于
被更正版本的登记时刻；沿用原版本号即覆盖，构造期拒绝。

**附问：更正改了控制证据或发生时刻，参与起点是同段新起点还是新段？——同段，但这半边不在本票。** 段是共同控制责任范围，
「承运责任变了才是另一段」（ADR-0103 主体判据；CONTEXT「可验证的实际承运责任或运输控制边界发生变化时，结束原参与并形成
下一段」）——更正证据或时刻不改变谁在控制，所以不是新段。参与关系今天以 `OFFSITE-PICKUP/<版本>` 为 `entryBasis`、以
`OccurredAt` 为 `enteredAt`（`JoinWithPickup`），按 CONTEXT ⑤ 应保留原参与、形成替代参与回指原参与并按新版本重派生起点；
**但这一层机制今天对交接更正同样没有**——`RegisterTransportHandoverHandler.Correct` 只落新版本并重交意图，不碰段登记册。
只在揽收一侧补它会让两种来源的更正在段上行为不一致，因此**段侧「来源更正 → 参与关系重派生」另立一票，覆盖交接与揽收
两种来源**；本票的 Correct 编排照交接那一侧的现状：落新版本 + 重交 PS 采认意图，不进段。

**本票范围（裁后重述）**：领域 `OffsitePickup.Correct(PickupCorrection)` → 端口 `OffsitePickupRegistry` 以新版本键 `Save`
（不加改写口）→ 编排 `RegisterOffsitePickupHandler.Correct`（读回前版 → 领域 Correct → 提交 → 重交意图；无前版则未受理，
更正不出无中生有的揽收）→ 端点 `/transport-fulfillment/offsite-pickup-corrections`（沿票 04 形状，共享接线文件占号）。
四层一次建设，走 TDD。

**能力边界**：读过本票、TF `CONTEXT.md` 全文、`domain/offsite_pickup.go`、`domain/transport_handover.go` 的 `Correct` 与
`HandoverCorrection`、`domain/actual_fulfillment_segment.go` 的 `JoinWithPickup`、`application/register_offsite_pickup.go`、
`application/register_transport_handover.go` 的 `Correct`；grep 过五处 `Corrects` 的落点。**没读** PS 侧采用揽收意图的编排
（`UC-PS-003` 揽收源链）——「新版本再采用一次是正确行为」是按 `PickupResultVersion` 自注与 PS 责任起点锚在 `OccurredAt`
推的，实施时若 PS 采用口对同对象第二版本有别的处置，以 PS 票面为准并回本票追记。裁的是**更正模型的形状**，不含任何取值。

## 从哪里来

票 [03](03-parcel-api-wiring-for-segment-orchestrations.md) 裁决 2 写「揽收含更正口」，票 [04](04-control-fact-entry-endpoints.md)
落地时发现 `RegisterOffsitePickupHandler` 只有 `Register`，应用层没有更正编排，端点表因此只挂了登记口。
MCP-3 2026-09-03 裁：**这不是端点层的事，是领域层尚未答的问题**，另立本票，不阻塞接线四票。

## 要裁的领域问题

CONTEXT 生命周期⑧「来源证据被更正 → 保留原段、形成失效或替代关系并重新派生」。揽收登记的更正
落到哪一种形状：

- **新版本**（同交接那一侧：新版回指前身、原判断不动、每格完备性同首次）——`OffsitePickup` 今天
  有 `Version`（逐成功对象签发的 `PickupResultVersion`），但没有 `Corrects` 链；parcel-shipment 的
  采用判断按版本幂等，新版本意味着下游要再采用一次。
- **失效 + 替代**（原登记标失效、另立一条替代登记并重新派生）——与生命周期⑧的措辞更贴，但
  「失效」在 `OffsitePickupRegistry` 上今天表达不出来（只插不改）。

两条路对段的影响不同：更正若改了控制证据或发生时刻，对象在段里的参与起点要不要跟着动？CONTEXT
「已经成立的实际履约段及履约参与关系不能被取消、删除或回写为未发生」——参与起点的更正是新段还是
同段新起点，与票 [02](02-actual-carrier-judgment-model.md) 的「不追溯覆盖未知期间」同族。

## 裁后要做的

领域（`domain.OffsitePickup` 的更正门）→ 端口（登记册的读回/落新版或失效口）→ 编排
（`RegisterOffsitePickupHandler.Correct` 或独立编排）→ 端点（`/transport-fulfillment/offsite-pickup-corrections`，
沿票 04 的形状）。四层一次建设，走 TDD。

## 边界

裁前不写代码。端点表照今天的样子不挂揽收更正口。
