# 节点作业与运输履约两张查阅页——包在、`query_*` 为零

Category: feature
Status: ready-for-agent——MCP-5
Blocked by: 无

## 现状（取证于 `65b6cf2`）

两个上下文都**有** `adapters/http` 包，但包里 `query_*` 数为 **0**——只有命令端点
（`/node-operations/receptions`、`/transport-fulfillment/deliveries`、
`/transport-fulfillment/delivery-proof-corrections`），没有任何查阅面。这正是两页接不了线的
直接原因。

表都在且有租户维：`node_operations` 5 表 18 处 `tenant_id`（`reception`、`consolidation_unit`、
`containment_current`、`execution_fact`、`collaboration_acceptance`）；`transport_fulfillment`
16 表 59 处（`transport_schedule`、`capacity_pool`、`capacity_reservation`、
`transport_commission`、`transport_handover`、`effective_delivery`、`delivery_attempt` 等）。
当前全部 0 行。**ADR-0077 形状逐字成立，不需要新 ADR。**

## 页面定位的一条硬约束

这两页是**治理查阅面，不是一线作业端**。实时现场作业（扫描、点验、装卸）按
[ADR-0021](../../../docs/adr/0021-frontline-operations-client-is-part-of-the-product.md)
属一线作业端，不在管理台。两页现有骨架的页头已注明这一点——**接真时保留该表述**，不要因为
加了查阅端点就把它读成作业入口。查阅面**零登记零编辑动作**。

## 表会一直是空的，这是预期不是缺陷

两上下文的写入方是那三个命令端点，而它们全部装着 `UnconfiguredIntake{}`——包裹进不来，作业
事实就不会产生。见 [spec 事实基线](../spec.md)。两页接完**仍是空册**。

**完成判据只能写成「空态文案说的是『读取入口已配置、登记册为空』，而不是『尚未接线』」，
不得写成「页面有数据」。** 不要造收寄或交接的种子——那是运行时业务事实不是主数据。读适配器的
真库测试照常写（测试内插行再读回）。

## 做什么

对两页各自：读端口（`ports`）→ 真库读适配器（`adapters/postgres`）+ 真库测试 → 在**既有**
`adapters/http` 包里加 `query_*.go` 与隔离读准入入格。**不动包里既有的命令处理器**。

两页现有骨架的栏目已按 CONTEXT 语义搭好（节点侧四区：收寄、集运单元、实测、交接证据；履约侧
班次执行准备与实际执行分列、容量维度不折算、权威交接结果按共享词表着色）。**读面按现有栏目
供数，不要重新设计页面**；栏目与表对不上时以 CONTEXT 与现有栏目为准，并在 Comments 记明。

## 两阶段与次序

- **阶段一**：上述全部。自验绿后向频道交**已验 SHA** 与端点行；**不自改** `cmd/parcel-api`
  装配四件（占号在票 07）。
- **阶段二**：收到 MCP-1「已装配」广播后，两页接真，`liveIds` 加两行——**只加自己那两行，
  不动邻行**。

## 完成判据

两页转 live；空态文案说「读取入口已配置、登记册为空」而非「尚未接线」；页头 ADR-0021 表述
保留；含真库全仓绿（注明）；两包既有命令端点回归无变化。

## Comments
