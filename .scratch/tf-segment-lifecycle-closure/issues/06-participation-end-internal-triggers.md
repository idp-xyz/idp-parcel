# 结束参与的两处内部触发：挂在交付与交接落库之后

Category: enhancement
Status: ready-for-agent
Blocked by: 无。MCP-1 2026-09-03 答 A（票 16 只加新文件，不碰 `register_effective_delivery.go` 与
`register_transport_handover.go`），开工前置已满足；票 16 两刀已落 `a3e28ff`、`65b369f`。
两问 MCP-3 同日已答（见「裁决」节），可开工；MCP-3 排的队列是 05 → 07 → 06。

## 裁决（MCP-3，2026-09-03，原文追录）

> - **段引用从哪来：(a)** 按对象找在场参与的读口。交付是关于对象的事实，回传方不知道段；CONTEXT
>   生命周期③就是按对象说的。零个在场参与 → 不是 error，结果里透一格 NO_ACTIVE_PARTICIPATION（可观察，
>   不静默）；多于一个在场参与 → 响亮 error（一对象同时只能在一个共同控制范围里，那是库面不一致，
>   不挑一个）。命令不加段字段。
> - **触发在哪一层：(i) 编排 Deps，但必填不可缺席。**「有效交付→结束参与」是 TF 自己的生命周期规则，
>   归编排不归组合根；可缺席会造出 ADR-0098 那一族「静默不发生」，必填则没接线在构造时就看得见
>   （测试用替身）。同事务；结束参与失败则整笔不落（error → 5xx），不要「交付落了参与没结」的半成品。
> - **「下一段已关闭」同答 SEGMENT_CLOSED，且前一段参与照常结束**：`已交接` 已使控制转入接收方，前段参与
>   结束与下一段收不收无关；下一段拒收透出，与票 04 那一格同形。

**落法上的一处待核**：`RegisterEffectiveDeliveryDeps` / `RegisterTransportHandoverDeps` 加必填字段会让所有
既有构造点（`cmd/parcel-api` 装配、各测试夹具）在编译期红——那正是「必填」要的可见性，但意味着本票
要一次改完所有构造点，开工前在频道开窗。

## 从哪里来

票 [03](03-parcel-api-wiring-for-segment-orchestrations.md) 决策简报第 5 行（MCP-3 2026-09-03 同意）：
`EndFulfillmentParticipation` 对应 UC-TF-005 步骤 7，责任方是 `transport-fulfillment` **自己**，
不该有外部端点。三路分置：

- **交付一路**：挂在 `RegisterEffectiveDelivery` 落库之后。
- **下一次交接一路**：挂在 `RegisterTransportHandover` 落库之后。
- **明确终止一路**：走 admin 写面，归票 [07](07-admin-write-faces-segment-closure-dispatch-task-load-assignment.md)。

把 5 做成端点会让 UC 步骤 7 的责任方从 TF 变成接入方——这是本票存在的全部理由。

## 拆票时列出的两问（已由上面「裁决」节答，留作理由记录）

**段引用从哪来。** `EndFulfillmentParticipationCommand` 要 `Segment`：

- 交接一路容易：`RegisterTransportHandoverCommand.Segment` 就是对象进的那个段；结束的是它在
  **前一段**的参与——前一段是哪个，交接命令今天不带。
- 交付一路更难：`RegisterEffectiveDeliveryCommand` 根本没有段字段。

两条路都要回答「对象此刻在哪个段」。可选的形状（列出供裁，不在此定）：
(a) 段登记册加一个按（租户 + 对象）找在场参与的读口；(b) 交接 / 交付命令加可缺席的
`EndsParticipationIn` 一类字段由调用方指名；(c) 两者都要。这是领域形状，**MCP-3 裁**；涉及
`ports.ActualFulfillmentSegmentRegistry` 与两条编排的 Deps，都在 MCP-1 票 16 的邻域。

**触发在哪一层。** 两种落法：
(i) 编排内——`RegisterEffectiveDeliveryHandler` / `RegisterTransportHandoverHandler` 的 Deps 加一个
可缺席的 `ParticipationEnds`，落库后同事务调 `End`；(ii) 组合根——`cmd/parcel-api` 的事务包装在
inner 成功后调 `EndFulfillmentParticipationHandler.End`。(ii) 不碰 TF application，但它把「交付
结束参与」这条 CONTEXT 规则放进了装配点——装配点换一个进程（CLI、消费者）这条规则就丢了。
倾向 (i)，由 MCP-3 与 MCP-1 定。

**下一段已关闭要不要答格。** 票 04 随裁决 3 让进段门交回 `SegmentEntryRefusal`（`SEGMENT_CLOSED`），
三条控制事实入口都透出了；`EndFulfillmentParticipationHandler.enterNextSegment` 走同一道门但**暂只取
续办引用**，那一格没透出。它与「下一段由谁指名」是同一次裁决的两半，本票一并答。

## 红线（沿票 03 与 tf-unwired-seven）

- 不改既有导出签名；Deps 新字段一律可缺席，缺席时行为与今天逐字节同形。
- **不自动关段**：结束最后一条参与不顺手关段（CONTEXT 生命周期④后半句是一个决定）。
- 派生一侧失败不翻来源保全：`End` 的失败进 `SegmentContinuationReference` 一类的欠账格，交付 /
  交接照登。
- 交付与终止之后没有「下一段」（`carriesControlOnward`）——交付一路不许收 `NextSegment`。

## 完工判据

两处触发各有测试钉住：交付落库后同对象在段内的参与关系已结束；交接落库后对象在前一段的参与
已结束、在新段的参与已成立；`End` 依赖故障时交付 / 交接结果不变而欠账格非空。装配点接真、临时
worktree 全绿、提交带 pathspec、向 MCP-3 报 SHA 与验证种类。
