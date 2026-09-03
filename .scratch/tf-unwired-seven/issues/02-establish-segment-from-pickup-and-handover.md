# 两条立段入口接进现有两例编排

Category: enhancement
Status: resolved——两条立段入口都上了生产调用路径，棘轮两行都剪了；多对象到访那一条
另立 [08](./08-multi-object-pickup-attempt-establishes-no-segment.md)，理由见文末末条
Blocked by: 01（已 resolved）

## 要接的两条

- `EstablishSegmentWithPickup`（自注「由首个对象的有效收寄成立段（CONTEXT 生命周期①）」）
  → 接进 `application/register_offsite_pickup.go` 与/或 `perform_offsite_pickup.go`
- `EstablishSegmentWithHandover`（自注「由首个对象的『已交接』权威交接成立段」）
  → 接进 `application/register_transport_handover.go`

两例编排今天都在造领域对象（`FormOffsitePickup` / `FormTransportHandover`），只是不立段。

## 三处必须想清楚的

**一、首个对象与后续对象走不同的路。** 两个函数名里都写着「由**首个**对象成立段」，后续对象
是 `JoinWith*` 加入既有段。所以编排要先判「这个范围的段是否已成立」——**这一判要落在段登记册
的取回上，不能靠编排自己记**。

**二、交接三裁决里只有「已交接」立段。** CONTEXT 与既有交接登记编排都把三裁决分开
（已交接 / 拒收 / 待确认），而`拒收`与`待确认`**不给转出引用**。立段同理：只有`已交接`能成立
段，另两格不成立也不报错——那是正当的业务结果，不是失败。

**三、这一步不能让登记失败。** 收寄登记与交接登记本身是控制事实的保全，CONTEXT 把接货时间
称为「责任起点锚」。**立段失败不得回滚登记**——否则一次段登记故障会抹掉一条已经发生的物理
事实。处置形状照本仓既有纪律：来源保全一侧的失败上抛，派生一侧的失败形成本上下文自己的
未决结果并带续办引用。

## 陷阱

- 别把「段已成立」缓存在编排里。并发两个对象同时到达时，那个缓存就是一次竞态。
- 别用「登记成功即立段」的隐式耦合表达 CONTEXT 那条边界；边界的判断要显式，因为 CONTEXT
  专门列举了七种**不能**替代它的单项事实。

## 完工判据

`EstablishSegmentWithPickup` 与 `EstablishSegmentWithHandover` 出现在非测试生产调用路径上，
棘轮基线两行可剪；剪前按基线要求核全仓同名声明。

## Comments

- 2026-09-03 · MCP-3：**阻塞已清，写口已备，编排未接。本票仍 `draft`，激活是 owner 的事。**

  **`Blocked by: 01` 已解**：票 01 于 2026-09-02 resolved（四层齐，重建门那条已剪）。

  **段成立之后的演进写口已落库面**（`0d8500a`）：按
  [ADR-0097](../../../docs/adr/0097-segment-evolution-writes-through-narrow-doors-not-a-general-update.md)
  在 `ports/fulfillment_segment.go` 的 `ActualFulfillmentSegmentRegistry` 上开了 `Join` /
  `EndParticipation` / `CloseSegment` 三个窄口，真库适配器与四个真库测试同笔。**没有通用
  Update**，被禁的「回写为未发生」「整段覆盖成员差异」在这个口上表达不出来；`Save` 仍只用于
  首登。那三个文件是接手时已躺在共享树上的 TF 线在途改动，一字未改入库，来历写在提交信里。

  **这对本票意味着**：第一个对象走 `EstablishSegmentWith*` → `Save` 首登；后续对象走领域
  `JoinWith*` → `Join`。「这个范围的段是否已成立」这一判照票面正文落在 `FindByKey` 上，不缓存
  在编排里。ADR-0097 决定五那条靠纪律的约束在本票编排里要守：关段前先读回整段、走领域
  `CloseSegment`，不得直接调写口——那属票 07，但入口形状本票就要留对。

  **棘轮**：本笔一行未动，`EstablishSegmentWith*` 两条仍在名单上，等本票的编排来剪。

- 2026-09-03 · MCP-3：**本票 resolved。** 四笔：`a318f0e`（交接侧首个对象立段）、`f680e1e`
  （交接侧补加入/无控制转移/失败续办三格 + 棘轮剪 Handover）、`36aed67`（收寄侧接线 + 进段门
  抽成一处 + 棘轮剪 Pickup）。

  **票面正文点名的三处，逐条交代：**

  **一、首个与后续走不同的路，判据落在登记册取回上。** `enterFulfillmentSegment` 每次都
  `FindByKey`，没找到走 `EstablishSegmentWith*`，找到走 `JoinWith*` 再落窄写口 `Join`。**没有
  缓存在编排里**——票面陷阱那条说的竞态是真的：两个对象并发到达时，各自那份缓存都会说「还没
  成立」，同一个段会被立两回。

  **二、交接三裁决只有`已交接`立段，另两格不成立也不报错。** 这一格**没有红过**：领域的
  `TransferOutBasis` 早已把门，编排走的就是它，所以那条测试是回归守卫不是红→绿。如实标出，
  免得下一个人以为它证明了本票新写的代码。它不空——同一组里`已交接`那格实测段登记册被写一次、
  这格零次。

  **三、立段失败不回滚登记。** 结果各增一个 `SegmentContinuationReference()`，非空表示来源已
  登记、段那一半还欠着。**领域拒绝与登记册故障分成两种答案**：拒收立不起段不留引用（留了会让
  调用方反复重试一件本就不该发生的事），只有读不到或写不进才是欠账。

  **段身份从哪来——本票遇到的真正卡点，由 owner 裁。** 两个命令都不带段引用，而
  `EstablishSegmentWith*` 要一个；全仓除测试与 postgres 重建行外没有任何地方产出过
  `FulfillmentSegmentReference`，**没有现成的铸号路径**。CONTEXT 只说实际履约段「不等同于计划
  履约段、班次、运输委托、订舱、总单或舱单」并列举七种不能替代该边界的事实，**没说它的身份由
  什么铸出**；交接范围算不算「同一共同运输控制范围」同样没裁。owner 2026-09-03 在四个候选里
  裁定：**命令新增显式入参，缺席时不立段也不报错**——机制半边现在做完，段号那个实例值留空并
  拒绝默认。因此今天全部调用方行为不变（都不给段号）。

  **进段门先抽后接。** 收寄与交接两侧当时逐字同形，照抄第二份就是为同一形状立第二个口径。
  共有判断收在 `application/enter_fulfillment_segment.go`，两侧各自只提供 `establish` / `join`
  两道领域门。**分两个门而不是一个「进段」门**，因为领域本来就分两个：段由首个对象的控制事实
  成立，后续对象是加入既有段——合成一个会把这条 CONTEXT 边界藏进实现。

  **`PerformOffsitePickupHandler`（多对象到访）没接，另立 08。** 它不是顺手能带的：一次到访覆盖
  多个对象，而 CONTEXT 要求**每个对象分别关联自己的计划履约段**，所以逐对象提交那一层要加字段，
  与本票这两条单对象入口的形状不同。**而它剪完两行之后不会再有任何信号提醒**——本门禁只量导出
  工厂，`EstablishSegmentWithPickup` 已经出名单了。这一句同时写进了基线注释，因为下一个读那份
  名单的人比下一个读本票面的人多。

  **一处已知缺口，08 一并处置**：段引用格式不合法时静默不立段，既不留续办引用也不改 `outcome`。
  重试一个写坏的引用不会变好，所以它不算欠账；但「这个入参没被受理」从结果上看不出来。

  **验证**：`go test ./internal/transportfulfillment/... -count=1` 四包 ok，`go vet` 退 0，
  `go build ./...` 退 0，架构门禁（含棘轮）ok，`gofmt -l` 为空。**未跑全仓、未跑真库、未跑
  `-race`**——本票没有适配器层改动，真库那一层无从验；`-race` 仍归收尾那一批，与票 03 注记的
  裁定同一条。
