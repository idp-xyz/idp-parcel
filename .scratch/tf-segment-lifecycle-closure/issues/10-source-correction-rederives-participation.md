# 来源更正 → 参与关系重派生：交接与揽收两种来源的更正今天都不进段

Category: enhancement
Status: draft——MCP-3 2026-09-04 随票 08 收口立票；票 08 裁决附问已答「同段、不是新段」，本票只承接那半边的机制，形状待裁
Blocked by: 无

## 缺口

CONTEXT 生命周期一句：「来源证据被更正或事件有效性变化 → 保留原段、原参与关系和原判断，形成失效或替代关系并重新
派生当前有效控制与履约结果。」这句话的**前半**（保留原判断、形成替代版本）今天两种来源都有——`TransportHandover.Correct`
与 `OffsitePickup.Correct` 都形成回指前版的新版本；**后半**（参与关系跟着重派生）两种来源都没有：

- `RegisterTransportHandoverHandler.Correct` 只落新版本并重交意图，不碰段登记册。
- `RegisterOffsitePickupHandler.Correct`（票 [08](08-offsite-pickup-correction-model.md)）照交接那一侧的现状，同样不进段。

而参与关系今天以来源版本为 `entryBasis`、以来源业务时间为 `enteredAt`（`JoinWithPickup` 取 `OFFSITE-PICKUP/<版本>` 与
`OccurredAt`；`JoinWithHandover` 同形），更正一旦改了控制证据或发生时刻，段里那条参与关系指着的就是一个已被回指的版本、
一个已被更正的起点。

## 票 08 已裁的那一半

> 附问：更正改了控制证据或发生时刻，参与起点是同段新起点还是新段？——**同段**，但这半边不在本票。段是共同控制责任
> 范围，「承运责任变了才是另一段」（ADR-0103 主体判据；CONTEXT「可验证的实际承运责任或运输控制边界发生变化时，结束原
> 参与并形成下一段」）——更正证据或时刻不改变谁在控制，所以不是新段。

所以本票不必再裁「新段还是同段」，要裁的是**同段内怎么表达「替代参与」**：原参与关系不能被取消、删除或回写为未发生
（CONTEXT 硬句），新起点又要按新版本重派生——是参与关系上长出版本链（形照来源那一侧的 `Corrects`），还是段上另立一条
参与关系回指原参与并把原参与标为被替代，两条路对 `ActualFulfillmentSegment.ParticipationFor`、`Active()`、
`SummarizeHandovers` 式的折叠读法影响不同。

## 为什么一票覆盖两种来源

只在揽收一侧补它会让两种来源的更正在段上行为不一致——票 08 裁决原句。触发点是两条：`RegisterTransportHandoverHandler.Correct`
与 `RegisterOffsitePickupHandler.Correct`；进段那道门今天两侧共用 `enterFulfillmentSegment`，重派生那道门也该共用一处。

## 先答再开工

- 参与关系的「替代」在领域上长什么样（见上）——`/domain-modeling`，落 CONTEXT「履约参与关系」词条。
- 重派生与更正编排是否同事务：票 06 把「结束前一段参与」放进交接编排同事务的理由是「输入全在 TF」；重派生的输入
  （新版本、原参与）也全在 TF，但票 09 裁决③给过反例的判据，要对着再走一遍。
- 段已关闭（`ActualFulfillmentSegment.CloseSegment` 之后）时更正来源的参与怎么办：CONTEXT「实际履约段结束 → 判断历史封存：不再接受
  新版本，除来源事实更正引起的重新派生」——这一句正是本票的例外格。

## 红线

- 原参与关系与原段一字不动（只插不改，与两本登记册同一条纪律）。
- 不自动关段，不改承运主体判断的现有版本——判断按更正关系重新派生是 `ActualCarrierJudgment` 自己的下一版，走它自己的口。
- 实例值留空拒默认；SYN 夹具只记 `S`。

## 参照

票 [08](08-offsite-pickup-correction-model.md) 裁决附问；`internal/transportfulfillment/domain/actual_fulfillment_segment.go`
的 `JoinWithPickup` / `JoinWithHandover`；`internal/transportfulfillment/application/enter_fulfillment_segment.go`；
`docs/domain/transport-fulfillment/CONTEXT.md` 生命周期节；ADR-0103。

## Comments

- 2026-09-04 · MCP-3：立票。起因是票 08 裁决把段侧重派生划出更正票之外并要求一票覆盖两种来源。**只写票面，未动代码。**
