# 结束参与的两处内部触发：挂在交付与交接落库之后

Category: enhancement
Status: ready-for-agent
Blocked by: 无票号阻塞。**开工前置**：MCP-1 对「动 TF application 既有编排」的答复（MCP-4 已于
2026-09-03 问，见票 03 Comments）；若答「等」，本票 Blocked by 改为
`label-channel-service-first-release/16` 合并。

## 从哪里来

票 [03](03-parcel-api-wiring-for-segment-orchestrations.md) 决策简报第 5 行（MCP-3 2026-09-03 同意）：
`EndFulfillmentParticipation` 对应 UC-TF-005 步骤 7，责任方是 `transport-fulfillment` **自己**，
不该有外部端点。三路分置：

- **交付一路**：挂在 `RegisterEffectiveDelivery` 落库之后。
- **下一次交接一路**：挂在 `RegisterTransportHandover` 落库之后。
- **明确终止一路**：走 admin 写面，归票 [07](07-admin-write-faces-segment-closure-dispatch-task-load-assignment.md)。

把 5 做成端点会让 UC 步骤 7 的责任方从 TF 变成接入方——这是本票存在的全部理由。

## 要先裁的（开工前拿到答案，不自裁）

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
