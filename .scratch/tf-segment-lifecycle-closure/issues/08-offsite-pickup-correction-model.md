# 揽收登记的更正：新版本还是失效 + 替代

Category: enhancement
Status: draft——领域问题未裁，MCP-3 下一轮裁；裁前不转 ready-for-agent
Blocked by: 无（不阻塞 04–07）

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
