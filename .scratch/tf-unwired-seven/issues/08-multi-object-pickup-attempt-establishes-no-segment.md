# 多对象到访不立段，而剪完棘轮之后没有任何信号会提醒它

Category: enhancement
Status: resolved（2026-09-03，MCP-1，`80e46a0`）
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

## Comments

- 2026-09-03 · MCP-1：**做了四件里的三件；第四件（「一并处置的已知缺口」）取证后作废，它的
  前提不成立。**

  **做了什么**：`PerformOffsitePickupCommand` 增 `Segment`（整次到访共用），
  `ObjectPickupSubmission` 增 `PlannedSegment`（逐对象各一个），`PerformOffsitePickupDeps` 增
  可缺席的 `Segments`；成功对象逐个过 `enterFulfillmentSegment`，第一个立段、其余加入同一个段。
  全是新增字段与新增方法，**没改任何既有导出签名**，不给段引用就不立段，现有调用点一行未改。

  **票面第 4 点那一格按逐对象裁**：新增 `ObjectSegmentEntry` 与
  `PerformOffsitePickupResult.SegmentEntries()`，**只列真有欠账的对象**。理由就是票面自己写的
  那条 CONTEXT——任务汇总只能由对象结果派生，整批一个 `SegmentContinuationReference()` 说不出
  「哪几个没进去」。两条单对象入口那边保持单个字符串，它们本来就只有一个对象。

  ---

  **第四件为什么作废，以及它是怎么差点被做出来的。**

  票面写着：「段引用格式不合法时，两条单对象入口今天**静默不立段**……『这个入参没被受理』从
  结果上看不出来」，并要求裁一个口径、把两条单对象入口一起改齐。

  我照着做了：给 `enterFulfillmentSegment` 换了返回类型（续办引用 / 共用引用形不成 / 逐对象
  计划段形不成三格），给三个结果类型各加了 `SegmentInputNotAccepted()`，红也写好了。**然后那
  两条 red 转不成 green**，去看构造器才发现前提是假的：

  `NewFulfillmentSegmentReference` 与 `NewPlannedSegmentReference` 都只调 `newRequiredValue`，
  而它**唯一的失败是去空白后为空**。进段门在调构造器之前就有一道缺席判据
  （`strings.TrimSpace(segmentReference) == ""` 与 `plannedReference != ""`）把那一种筛走了。
  **于是非空的引用一定构造得出来，那两处 `err != nil` 今天一行也走不到。**

  所以「静默不立段」这个缺口不存在：能走到那里的只有空白串，而空白串等同于「没给段引用」
  ——那正是它应得的答案。**我全部回退了那一套三格**，只留一条测试
  （`TestABlankSegmentReferenceMeansNotRequestedNotMalformed`）把这条判定钉住，并在
  `enterFulfillmentSegment` 那两处 `err != nil` 上写明它们今天不可达、引用日后加了格式规则才会
  真的失败。

  **这一格值得留档，因为它是本仓反复在记的那个形状的又一面**：票面那句话读起来像取证过的
  ——它具体、有位置、有后果，唯独没有人去看一眼构造器。而**照着它做，会得到一个永远答不出来
  的答案格**：一个不可达的状态被建成可观察的答案，比缺一格更坏——下一个人会以为「没报输入
  未受理」意味着引用是好的。

  ---

  **未做且要说清：**

  - **内容指纹不含段字段。** `pickupContentDigest` 与单对象那边的 `pickupRegistrationDigest`
    一样，不把 `Segment` / `PlannedSegment` 计入。**这是照 MCP-3 在票 02 立下的先例，不是我另
    裁的**：来源身份说的是物理事实，改一个段引用不该把同一次到访判成 `SOURCE_CONFLICT`。
    连带后果是**同一来源带着改好的段引用重投会得到 `EXISTING_RESULT`，段不会被补进去**。两条
    单对象入口有同一个缺口，本票没有扩大它，也没有修它——那是「已登记的事实事后补进段」，
    另一件事。
  - **重放不重进段。** `existingResult` 那条路不碰段登记册，与单对象入口同形。
  - **棘轮零改动**，本票量不到，票面已写明。

  **验证**（提交前工作树，DSN 已设、库可达）：`gofmt -l internal/transportfulfillment/` 为空、
  `go build ./...` 退 0、`go vet ./internal/transportfulfillment/...` 退 0、
  `go test -count=1 ./internal/transportfulfillment/...` 四包全 ok。新增 8 个用例在 `-v` 下逐条
  `PASS`。真库那一层本票零改动。`-race` 未跑。
