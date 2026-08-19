# NO-CTRL-TRANSFER-SURVEY：按权威交接推进节点控制转出的勘察

只读勘察，不含实现代码。切片 PN-06，协调岗 MCP-1。

**取证基准**：`7d7e138`（`feat(dispatch): 交接登记只投 VE 投影不 FanOut PS`），自建 detached worktree，未在共享树 `D:\tops\idp-parcel` 上取证或落文件。全部跨文件引用用符号名或引文，不用行号。

---

## Q1 `ControlTransferAdapter.TransferOut` 今天还是不是纯领域函数

**是，仍然是纯领域函数；非测试调用点为零。**

完整签名（`internal/nodeoperations/adapters/transportfulfillment/control_transfer.go`）：

```go
func (adapter *ControlTransferAdapter) TransferOut(
	control nodomain.PhysicalControl,
	handover tfdomain.TransportHandover,
) (nodomain.PhysicalControl, error)
```

入参两个都是**领域值**：`nodeoperations/domain.PhysicalControl` 与 `transportfulfillment/domain.TransportHandover`。没有 `context.Context`、没有仓储、没有事务句柄，函数体内只调 `handover.TransferOutBasis()`、`nodomain.NewTransferOutReference`、`control.TransferOut(reference, handover.JudgedAt())`，一次 I/O 都没有。

**接收者本身就装不下依赖**：`type ControlTransferAdapter struct{}` 是空结构体，`NewControlTransferAdapter()` 不接任何参数。要给它接仓储，得先改构造函数的形状。

**调用点**（全 worktree 检索 `NewControlTransferAdapter` 与包路径 `internal/nodeoperations/adapters/transportfulfillment`）：

| 出现处 | 性质 |
|---|---|
| `control_transfer.go` 的定义 | 定义 |
| `control_transfer_test.go`（外部测试包，`adapter "…/adapters/transportfulfillment"`） | 测试 |
| `.scratch/outbox-handoff-consumption-map/next-consumer-survey.md` 第 20 行条目 | 文档 |

**非测试调用点：0。** 该包没有被 `internal/` 或 `cmd/` 下任何生产代码 import。早前在 `e1b985a` 上的判断在 `7d7e138` 上仍然成立。

顺带一条对排期有用的：它内部调的 `nodomain.PhysicalControl.TransferOut` 有第二个非测试调用点——`nopostgres.rebuildControl`，在读回时按 `controlRow.ReleasedBy`/`ReleasedAt` 重放已转出状态。**读回侧已经能表达"控制已转出"，只是没有任何路径产生过这个状态。**

---

## Q2 现有 UC-NO-* 有没有写下「按上游权威交接结果推进本节点实物控制」

**没有。三份 UC-NO-* 都没有写下这件事；三份都只写了它的反面——这件事不归本用例。**

- **UC-NO-001（承接并执行关务节点协作）**：正文写「物理装载、节点侧交出或接收观察仍是节点事实；跨节点或运输方的控制转移只能由 `transport-fulfillment` 的权威交接结果形成」。应用流程里"形成权威运输交接与后续移动事实"那一步的责任方写的是 `transport-fulfillment`，不是 NO。`AT-NO-007` 同调：「只形成装载与节点侧证据；控制是否转移由运输履约形成权威交接结果」。全篇规定 NO **不做**什么，没有一步以交接结果为输入。
- **UC-NO-002（接收客户送达节点的作业实物）**：范围硬排除——「本用例不执行：……场外揽收、节点与运输方之间的权威交接、实际运输或交付」，并加了一句「实物由运输方交给节点时，节点只形成节点侧证据……不得调用本用例冒充客户送站」。「控制、身份与后续作业」一节确实复述了「只有明确节点收寄或权威运输交接的"已交接"结果可以建立节点控制」，但那是**控制成立（转入）**侧的规则复述，且本用例的应用流程八步全部走客户送站一条线。
- **UC-NO-003（执行正常节点作业、集运与封签）**：把「节点已经通过客户直接收寄或权威运输交接取得明确实物控制」写成**启动条件**（前置，不是本用例产生的）；「交接准备」一节写「PN-04 的 `transport-fulfillment` 必须基于同一对象范围形成唯一权威交接结果；已拒收或待确认不转移控制」——仍是对提供方的要求。范围硬排除「节点收寄或运输方到站时的权威交接结论」。

**但 CONTEXT 层已经写下了，这一点要跟 UC 层分开记。** `docs/domain/node-operations/CONTEXT.md` 的 Lifecycles「节点控制」有一条完整的转出转换：「节点控制 → 控制转出：`transport-fulfillment` 针对明确对象形成权威运输交接的"已交接"结果；已拒收或待确认不转出控制，备货、装载或节点侧交出扫描本身也不结束控制。」下一条还写了逐对象纪律：「部分交接时，各对象分别结束或保留节点控制；批次、集运单元或车辆范围不形成一刀切的控制结果。」Rules 的「接收、控制与交接」首条同调。`docs/domain/CONTEXT-MAP.md` 的 `node-operations ↔ transport-fulfillment` 关系约束也写明「只有"已交接"结果转移控制，已拒收或待确认均保持原控制」。

**结论：CONTEXT 能力在位且措辞完整，UC 级缺一份。** 领域函数 `ControlTransferAdapter.TransferOut` 正是照 CONTEXT 那两条写出来的，所以它在位而没人调——写它的那张票只走到 CONTEXT，没走到 UC。

---

## Q3 提供方那一侧的可重读性

分两层答，因为判据的答案和"能不能接上"的答案在这一票里不同。

### 提供方过关

`tfpostgres.TransportHandovers.FindByKey` 按（租户 + 对象 + 范围 + 版本）取回 `ports.TransportHandoverRecord`，读回经 `domain.RehydrateTransportHandover` 逐格复验（注释：「读回经 RehydrateTransportHandover 复验逐格完备性与版本链，坏行在这里暴露」）。NO 命令要的每一维都带得出：

| NO 侧要的维 | 提供方给得出的符号 |
|---|---|
| 租户 | `TransportHandover.TenantID()` |
| 实物 / 载运对象 | `TransportHandover.Object()` |
| 控制依据 | `TransportHandover.TransferOutBasis()`，返回 `"TRANSPORT-HANDOVER/" + version`，且只有`已交接`给得出 |
| 业务时间 | `TransportHandover.JudgedAt()` |

取回这份记录所需的四维键也已经在信封里：`veinbox.decodeRegisteredTransportHandover` 译 `tenantId` / `object` / `scope` / `version`，注释写明「四维（含 version）缺一即毒丸——处理方按（租户+对象+范围+版本）取回登记」。NO 侧的消费门可以照同一份载荷译，不需要提供方补发任何字段。

**节点维不在这份记录上**（`ReleasedBy`/`ReceivedBy` 是 `HandoverPartyReference`，`Scope` 是 `HandoverScopeReference`，都不是 `NodeReference`），但这不构成缺口：`ControlTransferAdapter.TransferOut` 不要节点，节点随已在场的 `PhysicalControl` 一起来。

**按本仓判据——「能不能接上取决于提供方那份可重读的记录是否带齐了消费侧命令要的每一维身份」——提供方这一侧过关。**

### 缺口换了个位置：在消费方自己的存储形状上

两条，都是硬的，都不在提供方。

**一、从（租户 + 实物）读不回当前在身的控制。** `PhysicalControl` 没有自己的表，它是 `node_operations.reception` 的 `control` jsonb 列（迁移 `0001_reception.sql`，`CONSTRAINT reception_pkey PRIMARY KEY (tenant_id, source_id)`，全表无第二索引）。`ports.ReceptionStore` 的读口只有 `FindByKey(ctx, key ReceptionKey)`，而 `ReceptionKey` 是 `{TenantID, SourceID}`。交接事件带的是实物，不是收寄来源标识；`unit` 埋在 jsonb 里且没索引。**今天没有任何读路径能从实物找到它的控制。**

**二、转出后的控制写不回去。** `ReceptionStore` 只有 `FindByKey` 与 `Save`，`Receptions.Save` 走 `ON CONFLICT DO NOTHING`、撞键答`已有记录`，没有 `Update`（对照同文件的 `ConsolidationStore` 是有 `Update` 的，注释还专门解释了为什么状态推进要走 `Update`）。行模型 `controlRow` 已经有 `releasedBy` / `releasedAt` 两格，`marshalPresence` 已经在写它们，`rebuildControl` 已经能重放——**列在、算子不在。**

### 一条要 NO owner 拍板、不属编排的（只建议，不替 owner 定）

`ControlTransferAdapter.TransferOut` 用 `handover.Object().String() != control.Unit().String()` 做**字符串比对**跨类型对身份。但两边的自述对不齐：

- `tfdomain.CarriedObjectReference`：「指名一个载运对象。它是对正式包裹身份或集运单元的引用——两者分别属 parcel-shipment 与 node-operations，本上下文不铸造它们。」
- `nodomain.HandlingUnitID`：「节点现场能区分的作业实物标识。它是本上下文的作业身份，不是客户服务身份——正式包裹身份属 parcel-shipment，作业实物记录不能替代它。」

也就是说交接对象可能指**正式包裹身份**（PS 拥有）或**集运单元**（NO 拥有的 `ConsolidationUnitID`），而控制挂在**作业实物**（`HandlingUnitID`）上——三个概念今天靠字符串相等碰运气。合成夹具里两边同名所以测试是绿的；真实数据下 TF 对一个正式包裹身份形成的交接，取不中 NO 的作业实物控制。这属 NO 与 TF 两位 owner 的身份对应关系，不是编排能定的，建议在开票前先裁。

---

## Q4 可排期的判断

**主体是 (a)，但前置一个 (b) 的小项；不是 (c)。**

一句话：**CONTEXT 能力与领域函数都在位，提供方记录也带齐了身份维，因此不卡在 (c)；但 UC 级缺一份（(b) 的那一小半），且 (a) 的工作量比「只差应用层 + 消费门 + 接线」多两项 NO 侧存储改造——读路径与写算子今天都不存在。**

### 要新建 / 要动的文件

| # | 文件 | 性质 | 说明 |
|---|---|---|---|
| 1 | `docs/application/node-operations/UC-NO-004-*`（新） | 新建 | 建议独立 UC，不塞进 UC-NO-002/003——那两份都把权威运输交接**硬排除**在「本用例不执行」里，往里塞会跟它们自己的范围声明打架。写不写、怎么写由 NO owner 定 |
| 2 | `migrations/node_operations/0003_*.sql`（新） | 新建 | 让（租户 + 实物）能定位当前在身控制。加索引还是另立控制表由 NO owner 定形状；`reception` 今天只有 `(tenant_id, source_id)` 主键 |
| 3 | `internal/nodeoperations/ports/ports.go` | 改 | 加按（租户 + 实物）找当前在身控制的读端口，加转出的写算子。端口用 NO 自己的语言（ADR-0025：提供方类型不得进消费方 `ports`） |
| 4 | `internal/nodeoperations/adapters/postgres/`（改 `reception.go` 或新文件） | 改/新建 | 上述端口的 SQL 实现。`controlRow` 的 `releasedBy`/`releasedAt` 已在，只差算子 |
| 5 | `internal/nodeoperations/application/transfer_out_control.go`（新） | 新建 | 编排：受理、幂等、提交。幂等键候选（租户 + 实物 + 交接版本）——留票内决定，不在此定死 |
| 6 | `internal/nodeoperations/adapters/inbox/transport_handover_consumer.go`（新） | 新建 | **NO 的第一个 inbox 消费门**：`internal/nodeoperations/` 下今天没有 `adapters/inbox/` 目录。消费者名必须与 `visibility-exception/derive-projection-from-transport-handover` 分家（VE 侧注释：「共名会让一路把另一路的投递当重复跳过」） |
| 7 | `cmd/parcel-dispatch/assemble.go` | **改，且是共享接线** | 见下 |

### assemble.go 要动，排期时须独占

`transport-fulfillment.transport-handover.registered` 这一路今天在 `NewDirectPublisher` 的路由表里只挂了 `veHandoverRouted` **一个**消费者。加 NO 要把它改成 `dispatch.FanOut(veHandoverRouted, <NO 侧带哨兵的消费者>)`，照 `nodeIntakeFan` / `pickupFan` / `deliveryFan` 的既有写法。连带要改的还有：

- `deriveHandoverConsumer` 上方那句「本路不接 PS：终局只认有效交付」——**这句本身仍然对，不要动它的裁定**，但要补上 NO 已在本路。
- `veHandoverUndecidedSentinels` 的注释「本路不 FanOut 给 PS……NO/NR 控制转移不在本票」——NO 落地后前半句仍成立、后半句失效。
- `wireDispatcher` 的路由表注释「第六条只接 `transport-handover.registered`，不接 PS」同上。
- NO 侧的未决哨兵要**自己包一份**再进 FanOut：既有注释已定纪律「各路先包自己的哨兵再 FanOut：禁止把两路哨兵合成一份再包 FanOut。FanOut 不翻译。」

**`assemble.go` 是共享接线，同一时间只允许一张票占它。这张票要占。**

### 不建议做的（已裁定，本报告不挑战）

不建议把交接那一路改成 FanOut 给 PS——终局只认有效交付，这是已裁定的。本票只在这一路上加 NO，PS 仍然不接。

---

## 能力边界

**读过**：`AGENTS.md`；`docs/domain/CONTEXT-MAP.md`（`node-operations` 段与全部关系约束）；`docs/domain/node-operations/CONTEXT.md` 全文；`docs/application/node-operations/` 下全部三份 UC-NO-*（001 检索式读、002/003 全文）；`docs/adr/0025-cross-context-adapters-live-on-the-consumer-side.md` 全文；`internal/nodeoperations/` 的 `domain/physical_control.go`、`ports/ports.go`、`adapters/postgres/reception.go`、`adapters/transportfulfillment/control_transfer.go`；`internal/transportfulfillment/domain/transport_handover.go`、`adapters/postgres/transport_handover_registry.go`；`internal/visibilityexception/adapters/inbox/transport_handover_consumer.go`；`migrations/node_operations/0001_reception.sql` 全文与 `0002_*.sql` 的表清单；`cmd/parcel-dispatch/assemble.go` 的路由表与 `deriveHandoverConsumer` 段。

**没读**：`docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md`（PN-06 切片定义未复核，切片编号取自派工票）；`docs/design/` 下 PN-06 的 handoff 工作包（未定位，因此本报告**没有回答"现在允许做到哪一层"**——骨架 / 隔离 `S` / PN-08 候选的准入判断不在本报告结论内）；`docs/domain/transport-fulfillment/CONTEXT.md` 全文（只经 UC-TF-* 与代码注释间接取证）；UC-TF-005 全文（只检索了 `node-operations` 相关行）；`internal/transportfulfillment/application/register_transport_handover.go` 全文；ADR-0017 / ADR-0031 / ADR-0043 / ADR-0049（只经代码注释间接引用）。

**没做**：没有跑 `go build` / `go test`——本票为只读勘察，Q1–Q4 的结论全部来自静态取证，未经编译或运行验证。

**没定**：未替 NO owner 定任何不变式。Q3 末尾的身份类型对不齐、Q4 表中第 1、2、5 行的形状选择，均为建议，决定权在 NO owner。
