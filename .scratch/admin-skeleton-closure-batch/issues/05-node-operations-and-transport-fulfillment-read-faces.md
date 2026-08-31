# 节点作业与运输履约两张查阅页——包在、`query_*` 为零

Category: feature
Status: in-progress——MCP-5（阶段一已交付自验绿，候票 07 装配广播进阶段二；基线 5aca747，工作树 idp-parcel-mcp5）
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

### 两页与表的对应：定稿（MCP-5，复核过两份 CONTEXT.md、迁移 0001/0002 与 0001/0003/0004/0005、两张页骨架）

**节点侧四区**，页面五块（含待识别实物）对表：

| 页面区 | 登记册 | 说明 |
|---|---|---|
| 节点收寄 | `reception` | 只列带收寄与控制在场的两格（`INTAKE_FORMED` / `PENDING_IDENTIFICATION`，0001 的在场规则）；另两格是提交处理结果，没有收寄事实可列，也不是 CONTEXT「节点收寄」词条——到站扫描、卸载不等于节点收寄。 |
| 待识别实物 | `reception`（`PENDING_IDENTIFICATION` 格的身份视角） | 候选、冲突标、正式关联引用照登记转写；识别成功不回写原行（永久作业记录），关联缺席说的是「登记那一刻还没有」。 |
| 集运单元 | `consolidation_unit` | 三相封闭词照转写；成员数照 `members` 计数，最近封签取快照末元素。 |
| 实测 | **无登记册（本批无端点）** | 0002 的 `execution_fact` 主键含协作事项（`item_ref`），是海关协查的执行事实，不是通用实测登记——挪来供数会把协查演成日常作业。 |
| 交接证据 | **无登记册（本批无端点）** | `collaboration_acceptance` 同理是协查承接决定；CONTEXT 的「节点侧交接证据」在存储上还没有登记格。 |

**履约侧五区**对表：

| 页面区 | 登记册 | 说明 |
|---|---|---|
| 班次 | `transport_schedule` | 行上只登身份、方向、出发时刻——执行准备与实际执行两组（开放/关闭订舱、暂停、出发、到达…）无登记格，读面不代填「未出发」一类派生状态词。 |
| 容量池 | `capacity_pool` + `capacity_reservation` | 四量分别维护照 CONTEXT：有效容量照行，已预占/已释放/实际使用按预占子表逐维求和，不互相抵扣、不代算可用量；适用期间池级无登记格。 |
| 权威交接结果 | `transport_handover` | 一行一判断版本，更正是新行指回前版，版本链在册面完整可见；`basis` 只在拒收/待确认在场。 |
| 交付证明 | `effective_delivery` | 只列当前版（`is_current`）；`proof` 是证明引用不是证据内容。 |
| 承运总单与运输舱单 | **无登记册（本批无端点）** | `transport_commission` 是委托订舱应答，不是总单/舱单；册名封闭集刻意没有这格。 |

三处「无登记册」的空态都属「无处可登」，不是「登记册为空」——端点不设恒空册，页面接真时该三区保持骨架说明文案。

### 阶段一交付（MCP-5）

两包各四类文件：`domain/operations_query_scope.go`（本上下文作用域，租户单维授权边界）、
`ports/catalogue_read.go`（伴生读端口，不扩写侧接口）、`adapters/postgres/review_catalogue.go`
（真库读适配器 + 真库测试，测试内经写侧适配器插行再读回）、`adapters/http` 里
`catalogue_intake.go` + `query_*_records.go` + `isolated_read_intake.go`（命令处理器零改动，
`UnconfiguredIntake` 补查阅面同堵）。

端点：`GET /node-operations-records?registry=reception|unidentified-item|consolidation-unit`、
`GET /transport-fulfillment-records?registry=transport-schedule|capacity-pool|transport-handover|effective-delivery`。
容量四量以十进制计数串转写（2^53 之上 JSON number 失真，判据同 settlementhttp minorAmount）。
