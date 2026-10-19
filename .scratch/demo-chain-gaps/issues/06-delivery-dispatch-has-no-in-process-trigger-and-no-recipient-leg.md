# 06 进入派送段之后没有进程内的派送发起；计划里没有末端到收件人那一段

Category: enhancement
Status: needs-triage——2026-10-19 会话 idp-parcel-9b 已交还，归本会话；待 TF 链第一波后接（见 [tf-chain-plan](../tf-chain-plan.md)「等 peer 的」）。此前 needs-triage——2026-10-18 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自同日对演示动线余下各段的只读勘察（隔离子代理，证据 `S`）；下列现象与证据取自勘察结果，本票立时没有逐条复核，接票时先核。
Blocked by: 无
出处：[product-strategy-boundary/05](../../product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md) 2026-10-18 那次重走。

## 现象

【软停】进入派送段之后没有进程内的派送发起；派送窗口缝在演示计划上没有诚实可引的段

## 证据（读码）

交接带 segmentServiceAction=FINAL_DELIVERY 立起派送段之后，进程内没有任何东西调用 `TriggerDeliveryDispatchHandler`：交接信封只投 VE；`/transport-fulfillment-delivery-dispatch-triggers` 是作业事实面，只能由 OFFSITE 设备手打一拍。`buildDeliveryDispatchTrigger` 注释写「谁按拍调这一口、拍频多大，是租户运营节拍的实例半边」（psb/05 格 13）。窗口问题：NR 的 `plannedLeg`（calendar_projection.go）只从网络连接生成段，演示计划三段止于 SYN-NODE-SIN-LM，没有「末端→收件人」这一段。若声明 `RPV-000000000001#3`，`DeliveryWindows.LoadDeliveryWindow` 交回的是 SIN-HUB→SIN-LM 干线段的窗口；不声明则答 REQUIREMENT_MISSING [DELIVERY_WINDOW]，只能退到运营决定 `/transport-fulfillment-dispatch-task-registrations` 手填地点、窗口、条件。

## 三类

机制与产品策略为主，另有租户取值。规模 M。

## 最小的诚实修法

产品策略：内置「随交接进入派送段、逐对象发起」的策略——parcel-dispatch 把交接信封另投一个运输履约消费者，按键重读交接与段，只在派送段内调用触发执行器；「要求缺失」是业务答案、不重投，「未决」登哨兵。按拍批量的形态和拍频作参考配置供租户选（psb/09 第 2 项）。窗口那一半先如实答缺失，走手工建任务，等 NR owner 裁定。

## 要裁的

发起策略的内置默认是事件驱动还是按拍，演示采用哪一种；派送窗口从哪来——NR 计划是否要含末端交付段/交付窗口（ADR-0131 只裁了按引用取，没裁派送段对应哪个计划段）。

## 既有票

psb/09 第 2 项（窗口那一半 none）
