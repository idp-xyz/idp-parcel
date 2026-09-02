# 把 TF 七条支路接起来

Category: enhancement
Status: in-progress——MCP-3 占 internal/transportfulfillment/ 与 migrations/transport_fulfillment/

## 范围来自哪条裁决

owner 于 2026-09-02 裁定：生产接线棘轮基线上那 32 条**算当前范围内**，按切片分工接起来，
差量口径同步下调（已落 `84c2eed`）。依据是
[32/32 逐条取证](../mechanism-executor-triage/spec.md)。本批做其中 `transport-fulfillment`
那 7 条。

## 地基实测（锚 `9d6063c`，`go build ./...` 绿）

`internal/transportfulfillment/ports/ports.go` 已有十二组端口（揽收尝试、派送尝试、有效交付、
场外揽收、运输交接、运输委托、订舱、订舱应答、替代旅程、班次、容量池、处置承接），
`migrations/transport_fulfillment/` 有五份迁移。

**七条要的东西一个都不在其中**：实际履约段、履约参与关系、装载分配、派送任务、实际移动事实、
交接范围汇总、运输收费发生项——**均无表、无端口**。

所以「支路未接」这个分类在**用例层面**成立（上游编排在、这一步没有调用方），落到**实现**
则是每条一次四层建设，不是加一行调用。本批按这个实际体量分票。

## 七票与阻塞边

| 票 | 内容 | 体量 | 阻塞 |
|---|---|---|---|
| [01](issues/01-actual-fulfillment-segment-registry.md) | 实际履约段登记册（表 + 端口 + 适配器） | 大 | — |
| [02](issues/02-establish-segment-from-pickup-and-handover.md) | 两条立段入口接进现有两例编排 | 中 | 01 |
| [03](issues/03-handover-scope-summary-read-face.md) | 交接范围汇总读面 | 小 | — |
| [04](issues/04-transport-charge-occurrence-registry.md) | 运输收费发生项登记册与失败尝试入口 | 中 | — |
| [05](issues/05-dispatch-task-and-load-assignment-bodies.md) | 派送任务与装载分配本体 | 大 | — |
| [06](issues/06-movement-fact-registry.md) | 实际移动事实登记册 | 中 | 01 |
| [07](issues/07-fulfillment-participation-per-object.md) | 履约参与关系逐对象成立与结束 | 大 | 01, 02 |

**票 03 与 04 无阻塞边且互不交叉，可与 01 并行。** 票 03 最小（只读面、不建表，`SummarizeHandovers`
是纯派生，交接册已存在），适合作为本批第一刀验证形状。

## 为什么 01 是关键路径

CONTEXT 这句是实际履约段成立的定义性边界：

> 载运对象通过有效收寄或权威交接进入运输方控制时，其履约参与关系和适用实际履约段才成立。
> 扫描、订舱确认、承运接受、列入总单或舱单、车辆到场、装载分配和物理装载中的任一单项均不能
> 替代该边界。

今天收寄登记得进去、交接登记得进去，而**这条边界在生产路径上跨不过去**——实际履约段、履约
参与关系、实际承运商一个都形成不了。票 01 建册、票 02 接入口、票 07 补逐对象关系，三票合起来
才让这句话有执行器。

## 红线（本批共用）

- **不改现有编排的导出签名。** 七条都是新增符号与新增调用点；若某票确需改现有导出签名，
  先在频道报「我要改 X 的签名」再动（`docs/agents/parallel-sessions.md`）。
- **新开迁移序号，不改已施加的迁移。** `transport_fulfillment` 当前最大 `0005`。
- **实例取值一律留空并拒绝默认值。** 本批建的是机制：登记这些事实的能力。
- **`CONTEXT` 的分立纪律不得合并**：CONTEXT 反复要求「达成计划、中断、折返、提前终止和异常
  转交必须保留不同结果，不能全部写成『已完成』」「申请量、接受量、预占量、分配量、释放量和
  实际装载量必须分别保存」——建表时不要为了少几列把它们压成一个状态或一个数字。
- 每票走 TDD：先写钉住 CONTEXT 硬句的测试，再实现。

## 完工判据

七票全 resolved 时，`production_wiring_baseline.txt` 里 `transport-fulfillment` 那 7 行应当
**全部被剪掉**（棘轮门禁会自己发现它们有了生产调用点）。剪的时候按基线要求逐条分成因：
两个名字在全仓是否各只有一处声明，排除「别处同名声明造成误判」那一种。
