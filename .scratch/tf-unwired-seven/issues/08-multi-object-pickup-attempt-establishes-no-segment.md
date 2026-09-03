# 多对象到访不立段，而剪完棘轮之后没有任何信号会提醒它

Category: enhancement
Status: ready-for-agent
Blocked by: 无（02 已 resolved，进段门与两条单对象入口都在）

## 缺什么

`application/perform_offsite_pickup.go` 的 `PerformOffsitePickupHandler.Handle` 把一次实际到访
推进到逐对象揽收判断：成功的对象形成 `domain.OffsitePickup` 并交意图，失败的只存尝试结果。
**它不立段。**

票 02 接的是两条**单对象**入口（`register_transport_handover.go` 与 `register_offsite_pickup.go`），
两者共用 `application/enter_fulfillment_segment.go` 那道进段门。多对象到访这一条当时没接，理由
是形状不同（见下），不是漏了。

## 为什么它不是「再接一条」

**CONTEXT 要求逐对象分别关联自己的计划履约段**：

> 一个实际履约段可以承载多个载运对象，每个对象必须通过履约参与关系分别关联自己的计划履约段、
> 控制起止和结果。

两条单对象入口各自只带一个 `PlannedSegment` 就够了。多对象到访不行——`PerformOffsitePickupCommand`
的 `Objects []ObjectPickupSubmission` 里**每一项都要能带自己的计划段引用**，而段引用本身是整次
到访共用的（同一次到访取得控制的对象进同一个共同控制范围）。所以字段要分两层加：命令上一个
`Segment`，逐对象提交上各一个 `PlannedSegment`。

## 为什么现在没有信号

**棘轮门禁只量导出工厂。** `EstablishSegmentWithPickup` 已随票 02 出名单
（`internal/architecture/production_wiring_baseline.txt`），此后无论多少条编排还没接它，那份名单
都不会再红。本票就是那个提醒——同一句也写在基线该组的注释里，因为读那份名单的人比读本票面的多。

## 要做的

1. `PerformOffsitePickupCommand` 增 `Segment`；`ObjectPickupSubmission` 增 `PlannedSegment`。
   **两者都可缺席，缺席时不立段也不报错**——与票 02 裁定的形状一致（段身份由谁铸出仍未裁决，
   编排不拿到访任务或尝试引用顶替）。
2. `PerformOffsitePickupDeps` 增可缺席的 `Segments`。
3. 逐个成功对象过 `enterFulfillmentSegment`：**第一个立段、其余加入同一个段**。那道门自己会问
   登记册，不必在这条编排里记「段立了没有」。
4. 结果上交回段那一半的欠账。**这里比单对象入口多一格要想清楚**：一次到访里可能只有部分对象
   进段失败，`RegisterOffsitePickupResult` 那种单个 `SegmentContinuationReference()` 表达不了
   「哪几个没进去」。照 CONTEXT「每个对象必须分别保存成功、失败、拒收、待确认或其他适用结果，
   任务汇总只能由对象结果派生」，续办引用大概率也要逐对象，而不是整批一个。

## 陷阱

- **失败结果不制造段。** CONTEXT：「客户不在、货物未备好、包装不合格或其他失败结果不制造实际
  履约段」。这条今天由构造门守着（形不成 `OffsitePickup` 就走不到进段那一步），接线时别绕过它
  自己判一遍。
- **别让进段失败翻掉尝试登记。** 与票 02 同一条：来源保全一侧照常成立，派生一侧的失败只留续办
  引用。
- **别整批一个结果。** 上面第 4 点那一格是本票与票 02 真正的差别，不是顺手能抄的。

## 一并处置的已知缺口（来自票 02）

段引用格式不合法时，两条单对象入口今天**静默不立段**：既不留续办引用，也不改 `outcome`。
重试一个写坏的引用不会变好，所以它不算欠账；但「这个入参没被受理」从结果上看不出来。本票决定
一个口径并把两条单对象入口一起改齐，别只改新写的这一条。

## 完工判据

`PerformOffsitePickupHandler` 走通「一次到访多个对象进同一个段」，且逐对象结果分得开。
**棘轮不会为本票红，也不会为本票绿**——它量不到这一格，完工判据只能靠测试与票面。
