# 只读勘察：第三个消费者选谁

- 派活：MCP-1，接 `60ea63c` 第二消费者落地之后
- 取证点：HEAD `e1b985a`（代码面与 `60ea63c` 相同，其间只有一份文档提交）
- 执行方：MCP-3（只读；除本文件外未改任何文件，未写任何代码）
- 判据（派活给定）：①推进 SYN 纵向闭环下一段 ②消费侧就绪只差消费门与接线 ③不撞实例半边

## 结论先说

**按判据②「只差消费门与接线」，一条都不剩了。** 第 3 行是最后一条，已在 `60ea63c` 用掉。

这不是说别的方向做不了，是说**下一票必然比上一票大**——每条剩余方向都还差一件消费门与接线
之外的东西。排期请按这个前提做，不要按「再来一次 60ea63c」估。

判据②失效的原因收敛成一句，比逐行清单有用：

> **一条方向能不能今天接上，不取决于消费侧编排在不在，而取决于「提供方那份可重读的记录，
> 是否已经带齐消费侧命令所要的每一维身份」。**

第 3 行能接是因为 `IntakeAdoptionRecord` 自己就带着 `CustomerAccountID` 与
`ShipmentRequestID`——信封给四维键，按键取回整行，命令要的身份全在行里。其余方向大多不是：
事实取得回来，**当事人取不回来**。

## 三挡

### ① 现在就能做（只差消费门与接线）

**空。** 逐条核过的候选见下两挡。

### ②′ 差一件机制活，不需要新 UC（本挡是派活三挡里没有的一格，但剩余方向大多落在这里）

派活给的三挡假定「不在①就是缺领域建模或卡实例半边」。实测有第四类：**领域权威齐备、消费侧
编排齐备、卡在一个缺失的反查读口**。它既不是②也不是③，混进任一挡都会让排期估错，因此单列。

| 方向 | 消费侧现状（取证于 `e1b985a`） | 差什么 |
|---|---|---|
| 8. `node-operations.node-intake.formed` → PS 采认 | `AdoptNetworkIntakeHandler` + `intake_source.go` 齐备 | `AdoptFromNodeIntake` 的第二参 `TargetShipment` 要**完整来源身份**（租户+客户+来源+来源请求键）+ 委托号 + 基线版本。信封与 NO 的收寄记录都给不出后两维 |
| 16. `transport-fulfillment.effective-delivery.registered` → PS 终局 | `FormParcelFinalHandler` + `delivery_outcome.go` 齐备 | 同上，`TargetShipment` 同形 |
| 18/19. `transport-fulfillment.offsite-pickup.formed` / `.registered` → PS 采认 | `OffsitePickupAdapter` + `pickup_source.go` 齐备 | 同上，`TargetShipment` 同形（本包与节点收寄那份是同一个结构体） |
| 37. `visibility-exception.tracking-projection.derived` → VE 客户视图派生（同上下文） | `DeriveCustomerViewHandler` 齐备；载荷 `{tenantId,parcel,versionId}` 配 `Projections.FindCurrent(tenant,parcel)` 能取回投影本体 | `DeriveCustomerViewCommand` 还要 `Customer`（货主客户账户）。投影按包裹立键，不带账户；`CustomerViewStore.FindCurrent` 又要账户才查得动，反查不成环 |

**四条是同一件事的四个面：缺「按包裹/载运对象反查当事人（委托或客户账户）」这一层读。**

这一层不是实例半边：委托的成员包裹集合是接受基线的一部分，PS 自己拥有这层关系；VE 的
包裹与账户关系同理。它也不需要新 UC：UC-PS-003（收寄采认）、UC-PS-004（终局）都已明写这些
采认要发生。

**但它有存储代价，不是加个方法就完事。** `parcel_shipment.shipment_request` 整份聚合存一列
`jsonb`，主键是四维来源身份，另有 `(tenant_id, shipment_request_id)` 唯一约束——**没有任何
包裹维度的列或索引**，成员包裹埋在 `snapshot` 文档里（`migrations/parcel_shipment/0002`）。
按包裹反查要么走 jsonb 表达式索引，要么另立投影表，两条路都要迁移，都要 PS owner 定形状。

**这一挡的高杠杆点：一件работ解四条方向。** 若排第三消费者，建议先把这个反查口单开一票
（PS 一票、VE 一票），做完之后 8/16/18/19 与 37 才真的退化成「只差消费门与接线」。

### ② 差领域建模（缺 UC/CONTEXT 能力）

| 方向 | 缺什么 |
|---|---|
| 7. `network-routing.initial-route.formed` → NO | NO 全包搜不到任何接路由指令的东西（`initial-route`/`路由指令`/`计划节点` 零命中），三个编排 `ReceiveDeliveredUnit`/`AcceptCollaboration`/`ConsolidateParcels` 无一对口。要先立「节点接收版本化路由指令」的 UC/CONTEXT 能力。**上一票已报，MCP-1 已入未决清单** |
| 20. `transport-fulfillment.transport-handover.registered` → NO 控制转出 | `ControlTransferAdapter.TransferOut(control, handover)` 在位，但它是**纯领域函数**：不读库、不写库、没有应用编排包着它。NO 侧缺一个「按权威交接结果推进实物控制」的编排（受理、幂等、提交），那是 UC 级的活 |
| 6. `network-routing.reachability-judgment.formed` → PS 续办推进 | `parcelshipment/adapters/networkrouting/` 只有同步读口适配（`ReachabilityAssessor`/`ReachabilityRevalidator`），**没有事件侧适配器**；PS 的 `AdvanceAcceptanceJudgmentHandler` 零引用。事件半边与同步半边的分工 UC 未明文（同步链已经能拿到判断，事件链要做什么需要先裁），属建模先行 |

### ③ 卡实例半边

| 方向 | 等什么 |
|---|---|
| 46. `pilot-governance.suspension/resumption/takeover.recorded` | 不是实例半边，是**缺档**：pilot-governance 不在 CONTEXT-MAP、无 `CONTEXT.md`、无 UC-PG-*。消费方向在权威文档层面不可判（清点表矛盾清单第 2 条），维持「说不清」 |
| 14. `transport-fulfillment.capacity-consumption.recorded` | 口径未裁：端口注释说消费方是 TF 内部装载分配链，CONTEXT-MAP 边是 TF→NR。接之前要 TF owner 裁（清点表矛盾清单第 4 条）。不是实例参数，是所有权口径 |

**没有一条剩余方向是卡 `PAR-*` 的。** 派活预设的第三挡实测基本落空——实例半边今天卡的是
**接入渠道与价卡那一侧**（`PAR-INT-01`/`PAR-INT-03`，已由 ADR-0055 收成未配置格），不是消费
侧接线。消费侧的卡点是反查读口与领域能力，两样都在机制半边。

## 覆盖声明（重要）

**本勘察不是逐条走完 53 类。** 我按判据②反向筛：先列出全部跨上下文消费方适配器包
（`internal/<消费方>/adapters/<提供方>/`，九个），逐个读其入口签名判断「消费侧是否已有落点」，
再对有落点的逐条核实身份维度是否取得回来。因此：

- **逐条读过并有取证的**：3、6、7、8、16、18/19、20、37，加 PS 委托表的存储形状。
- **按同类归并、未逐条读的**：SA 的九类同上下文事件（30–36）与 VE 其余链内事件（38–42）。
  它们的消费方都是本上下文下一段编排，形状与 37 同类，**我判断多半同样卡在「事实取得回、
  当事人取不回」那一格，但这一句是归纳不是取证**，排到它们时要重取一次。
- **未看的**：CC 九类（21–29）、TF 其余类、NO 其余类、PP 45 类的消费侧落点。

保质期同清点表：现状断言取证于 `e1b985a`，HEAD 前进后「消费侧在不在」这一类结论要重取证；
判据栏引用的文档边不受影响。

## 给排期的一句

第三消费者若要复现第二个那种「一票做完、真库绿」的体量，**得先花一票把反查读口做出来**。
直接排 8/16/18/19 或 37 会在写到一半时撞上迁移与 owner 裁定，那时再拆票比现在拆贵。

## 失效注记 as-of `3b9f212`（2026-08-20，MCP-4）

本勘察取证于 `e1b985a`，以下按新 tip 逐挡核对结论存亡。

**②′ 挡四条全部被后续工作推翻（按本勘察自己的建议路径推翻的）**：

- 「按包裹/载运对象反查当事人」那层读口已建成——[ADR-0060](../../docs/adr/0060-parcel-lookup-uses-current-snapshot-projection.md)（包裹反查走当前快照投影列）、迁移 `migrations/parcel_shipment/0006_current_accepted_parcel_projection.sql`、适配器 `parcelshipment/adapters/postgres/current_accepted_parcel_target.go`。**第 8/16 两条的 `TargetShipment` 完整来源身份反查缺口不缺了**：`adopt_on_node_intake.go` / `adopt_on_effective_delivery.go` / `adopt_on_offsite_pickup.go` 三个消费适配器都经它取回完整目标，反查不着有专格哨兵 `ErrParcelTargetNotFound`（登记为可重试未决）。
- 第 8/16/19 三条消费者已接线（路由表 FanOut：先 VE 投影再 PS 采用/终局），第 37 条已接线（`tracking-projection.derived` → 客户视图派生，账户维经 PS 反查填上——正是本勘察说的「反查不成环」被 WIRE-CUSTOMER-VIEW 解开）。
- 第 18 条（尝试级 `offsite-pickup.formed`）**有意不接**：装配注释「两条都登记会让同一份揽收结果被采用两次」。它从「差反查口」变成「口径上不该接」，与清点表判据栏的出入已记入 report.md 刷新记录第 18 行。

**② 挡三条仍成立（逐条重验于 `3b9f212`）**：

- 第 7 条（`initial-route.formed` → NO）：NO 全包对 `initial-route`/`路由指令`/`计划节点` 仍零命中，`docs/application/node-operations/` 仍只有 UC-NO-001/002/003，无「节点接收版本化路由指令」用例。**开发计划把它点名为首个 A/B 候选的前置判断维持：不能直接开工，先立 UC/CONTEXT 能力。** 注意信封本身已有 VE 投影消费者（report.md 刷新记录第 7 行），但那不是本条说的 NO 消费。
- 第 20 条（`transport-handover.registered` → NO 控制转出）：`ControlTransferAdapter.TransferOut` 仍是纯领域函数（`nodeoperations/adapters/transportfulfillment/`），NO 侧仍无「按权威交接结果推进实物控制」的应用编排。VE 投影腿已接，不改变本条。
- 第 6 条（`reachability-judgment.formed` → PS 续办）：`AdvanceAcceptanceJudgmentHandler` 仍零生产构造（仅 application+测试命中），该类型仍未登记路由；事件半边与同步半边的分工仍无 UC 明文。

**③ 挡两条仍成立**：第 46 条 pilot-governance 仍无 `docs/domain/pilot-governance/`（glob 零文件）、无 UC-PG-*；第 14 条容量消耗的口径出入无新 ADR 裁决（`docs/adr/README.md` 对「容量」零命中）。

**「①挡空、一条都不剩」的结论已部分反转**：当时压在②′的四条今天全部落地，说明高杠杆判断正确且已被执行。①挡（只差消费门与接线）现在是否非空，需按判据②对剩余方向（CC 21–29 余量、SA 30–36、VE 38–44、PP 45）重勘——本注记只核对旧结论存亡，不重做勘察。
