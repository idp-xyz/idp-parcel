# 05 移动事实与包裹不相连、没有下游；出发门禁由设备自报，不问关务

Category: enhancement
Status: needs-triage——2026-10-18 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自同日对演示动线余下各段的只读勘察（隔离子代理，证据 `S`）；下列现象与证据取自勘察结果，本票立时没有逐条复核，接票时先核。
Blocked by: 无
出处：[product-strategy-boundary/05](../../product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md) 2026-10-18 那次重走。

## 现象

【软停】实际移动事实能登，但只挂在班次上，与包裹不相连、没有下游；外包干线的移动没有合法入口；出发门禁不问关务

## 证据（读码）

`/transport-fulfillment/movement-facts` 能答 MOVEMENT_FACT_RECORDED，但 `RecordMovementFactCommand` 只挂 Schedule（不必在册）；`domain.RecordMovementFact` 不读段也不读对象。`buildMovementFactOrchestration` 注释写「不向任何下游交意图」，NR 和 VE 都看不到。一线面的来源写死为 `frontlineMovementSource = "SELF_OPERATED"`，外包干线（`SYN-PARTY-CARRIER-TRUNK-01`）的移动只能走外部轨迹：`ports.TrackingSource` 没有生产实现，`AdoptTrackingMaterialHandler` 没有生产入口，集成客户端面在演示上配不了（psb/31）。出发门禁的 GateRequired/GateClearance 由设备自报，放行引用是自由串；运输履约没有读关务的端口，CC 的 `NewVerifyReleaseGateHandler` 在 cmd/ 零引用，`customs-compliance.gate-verification.recorded` 没有消费者。结果是跨境段 SZX-GATE→SIN-HUB 的出发不受门禁约束。

## 三类

机制与产品策略为主，另有租户取值。规模 L。

## 最小的诚实修法

机制：出发门禁改由运输履约的消费侧适配器向关务问当前放行结果，不再收设备自报的放行串；移动事实登记同笔入队一份意图，消费者就位之前不登路由。产品策略：TrackingSource 至少一种连接器形态对合成源跑通（psb/09 第 1 项）。租户取值：演示干线定为自营或外包，决定移动走一线面还是外部轨迹。

## 要裁的

演示干线是自营还是外包；门禁由运输履约拉关务，还是关务把放行推给运输履约；移动事实怎样关联到对象（经装载分配成员，还是经段）。

## 既有票

psb/09 第 1 项；psb/31；门禁那一半 none
