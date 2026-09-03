# TF 七条：32 条里最大的一簇，而它记的是「达标」不是「留待」

Category: chore
Status: resolved——七条逐条分类已出（2026-09-02），处置已由 owner 裁定并兑现：计入差量（`84c2eed`），
七条随 `tf-unwired-seven` 八票全部接上生产调用方，棘轮该组清空（`0c8d65d`）
Blocked by: 无

## 为什么单开一票

`transport-fulfillment` 在基线名单上占 7 条，是七组里最大的一簇，而它在
[开发主线](../../../docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)
「机制半边现状」表里记的是 **达标**（不带「显式留待」注），差量列写的是 **「无」**。

PN-04 那一行还写着「TF 编排八例对应七个 UC 全触」「端口已接满（第二十七轮：TF 24 口两口径
0 缺）」。**端口 0 缺与七条工厂零调用点并不矛盾**——那正是[票 01](./01-skeleton-criterion-and-its-instruments-measure-different-things.md)
说的：端口口径量的是「声明的端口 → 实现」，量不到「规则 → 端口」。

## 七条

`internal/transportfulfillment/domain` 下，`production_wiring_baseline.txt` 冻着：

    ChargeOccurrenceForFailedAttempt
    EstablishSegmentWithHandover
    EstablishSegmentWithPickup
    FormLoadAssignment
    OpenDispatchTask
    RecordMovementFact
    SummarizeHandovers

基线对这一组只有一句笼统的组注释：「以下各条均产出带不变式校验的领域对象，包外零引用」。

## 已核的那一条，与它揭出的形状

`EstablishSegmentWithPickup` 逐条核过（锚 `9d6063c`）：

编排 `internal/transportfulfillment/application/perform_offsite_pickup.go` **存在**，且确实在
造领域对象——`FormFulfillmentAttempt`、`FormAttemptObjectResult`、`OffsitePickup`，以及一串
`NewXxxReference`。它**不调** `EstablishSegmentWithPickup`。

也就是说：**揽收尝试成了事实，但没有人由这个事实立起实际履约段。** 这不是「切片还没开工」
（编排在、适配器在、端口满），也不是「等租户」（缺的是调用点不是数据），是**同一用例里
下游那一环没接上**。

`OpenDispatchTask` 旁证同形：编排里用着 `NewDispatchTaskReference`（任务的**引用**），而
任务**本体**的开启入口零调用点。引用得到一个从来没被开出来过的东西。

## 本票要做的

逐条核余下六条，每条答四问，**不按组作答**：

1. 该工厂产出什么，`docs/domain/transport-fulfillment/CONTEXT.md` 哪条硬句要求它（摘原句）；
2. 该能力的端口 / 迁移表 / 应用编排 / 适配器四层各自在不在；
3. 若编排在而不调它，编排实际调的是什么——**「接错」与「没接」要分开**；
4. 归到哪一格：支路未接 / 整能力未接 / 死码。

## 为什么这一组的结论对全局最要紧

TF 是**达标且差量为「无」**的四个切片之一（另三个是 PN-01、PN-05、PN-08）。带「显式留待」
注的那四个至少还有一份认可清单可查；**差量为「无」的这一个，名单上却躺着七条**。若这七条里
有任何一条是支路未接，那么「差量：无」这句话本身要改。

反过来，若七条逐条核下来都有正当理由（例如都等某个尚未到来的上游事实来源），那就把理由
逐条写进基线注释——基线自己要求的正是这个：「加一条时把理由写在它上面」。今天这一组只有
一句组注释，而基线在 2026-08-24 逐条补核时已经吃过组注释的亏。

## 边界

本票**不接线、不改代码、不改基线名单**。产出是七行带举证的分类，交人裁。

## Comments

- 2026-09-02 · MCP-3：**七条逐条核完，全部归「支路未接」，无一例外。** 方法是把 TF 八例编排
  逐个摘出它调的领域构造，再与七条对表。八例编排全部存在且都在造领域对象——**没有一例调过
  这七条里的任何一条**。

  | 条目 | 该由谁调 | 那个编排实际调的 |
  |---|---|---|
  | `EstablishSegmentWithPickup` | `register_offsite_pickup.go` / `perform_offsite_pickup.go` | `FormOffsitePickup`、`FormFulfillmentAttempt`、`FormAttemptObjectResult` |
  | `EstablishSegmentWithHandover` | `register_transport_handover.go` | `FormTransportHandover` |
  | `SummarizeHandovers` | `register_transport_handover.go` | 同上，汇总从不派生 |
  | `ChargeOccurrenceForFailedAttempt` | `perform_offsite_pickup.go` | `FormAttemptObjectResult`——**它的入参就在这个 handler 里造出来，然后没被用** |
  | `OpenDispatchTask` | 无 | `perform_offsite_pickup.go` 造 `NewDispatchTaskReference` |
  | `FormLoadAssignment` | 无 | `prepare_transport_opportunity.go` 造 `NewLoadAssignmentReference` |
  | `RecordMovementFact` | 无 | 八例编排里没有任何一例处理实际移动 |

  **重复出现三次的形状：引用造得出，本体造不出。** `NewDispatchTaskReference` 与
  `NewLoadAssignmentReference` 都在编排里被调用，而 `OpenDispatchTask` 与 `FormLoadAssignment`
  零调用点——生产代码里流转着指向从未被创建过的东西的引用。CONTEXT 明写「运输委托、订舱、
  容量预占、承运接受和装载分配分别拥有业务身份、对象范围、数量、条件和生命周期」，而装载
  分配今天只有引用没有身份。

  **最重的一条是前两条合起来。** CONTEXT 这句是实际履约段成立的定义性边界：

  > 载运对象通过有效收寄或权威交接进入运输方控制时，其履约参与关系和适用实际履约段才成立。
  > 扫描、订舱确认、承运接受、列入总单或舱单、车辆到场、装载分配和物理装载中的任一单项均
  > 不能替代该边界。

  两条成立入口（`EstablishSegmentWithPickup` 注「由首个对象的有效收寄成立段（CONTEXT
  生命周期①）」、`EstablishSegmentWithHandover` 注「由首个对象的『已交接』权威交接成立段」）
  **都没有生产调用方**。收寄登记得进去、交接登记得进去，而**那条 CONTEXT 称为边界的边界，
  生产路径上跨不过去**。实际履约段、履约参与关系、实际承运商——CONTEXT 用整整一节写的这些，
  今天在真进程上一个都形成不了。

  `ChargeOccurrenceForFailedAttempt` 是最容易修也最刺眼的一条：它要的 `AttemptObjectResult`
  就在 `perform_offsite_pickup.go` 里由 `FormAttemptObjectResult` 造出来，隔几行就没人再用。
  它守的是 `AT-TF-094`（失败尝试形成发生项，第二次成功不覆盖第一次），验收判据编号都在
  代码注释里写着。

  **结论对 PN-04 定级的影响**：开发主线 PN-04 行记「达标」、差量列写「无」。按本轮取证，
  这七条都是机制半边的支路未接，**「差量：无」这句话与名单上的七条对不上**，二者必有一句
  要改。改哪一句是产品判断，本票不裁——但两句同时留着不成立。

  取证锚 `9d6063c`（编排文件与领域文件在 `9d6063c..0c4b5a1` 间未变动）。本笔只读，未改
  任何代码。

- 2026-09-03 · MCP-1：**转 resolved，本票要做的（分类、交人裁）两件都已闭环。** 裁：owner
  于 09-02 取「计入机制半边差量」一路（`84c2eed`，开发主线那节「差量：无」被更正），本票末段
  提出的「两句同时留着不成立」由此消解。兑现：七条按切片分成
  [`tf-unwired-seven`](../../tf-unwired-seven/spec.md) 八票，09-03 全部 resolved，
  `production_wiring_baseline.txt` 的 `transport-fulfillment` 组由 7 条清空到 0（`0c8d65d`）。

  **一句要如实**：清空量的是「导出工厂有了生产调用方」——调用方落在
  `internal/transportfulfillment/application/` 的编排层，`cmd/parcel-api` 对这批新编排尚无
  装配与端点。本票开头那句「收寄登记得进去、交接登记得进去，而那条边界生产路径上跨不过去」
  今天改成：**编排层跨得过去，端点层还没有路走到编排**。那件不在本票也不在 tf-unwired-seven，
  另立于 [`tf-segment-lifecycle-closure`](../../tf-segment-lifecycle-closure/spec.md)。
