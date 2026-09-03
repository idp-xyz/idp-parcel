# 关段声明口：`CloseFulfillmentSegment` 编排

Category: enhancement
Status: resolved——`2b4f6d7` + 补刀 `722e846`（MCP-1，2026-09-03）；完工判据三条各有测试，见文末 Comments
Blocked by: 无

## CONTEXT 要求什么

> 全部有效参与关系已经结束且不再接受新对象 → 实际履约段结束；各对象可以具有不同结果。

> 已成立的实际履约段不存在“取消回未开始”转换。没有有效控制结束事实时，停止移动、异常案件或
> 计划取消均不结束该段。

## 为什么它单独一票，而不是随结束参与顺手做

票 [tf-unwired-seven/07](../../tf-unwired-seven/issues/07-fulfillment-participation-per-object.md)
明确未做并给了理由：那句话的后半「**且不再接受新对象**」是一个决定，不是能从参与关系状态推导
出来的东西。结束最后一条参与时自动关段等于替人做了那个决定——而段一关，后来的兼容对象就进不
了它，得另立新段。本票建的就是那个让人做决定的口。

## 三层都在，缺的只是编排

- 领域：`ActualFulfillmentSegment.CloseSegment(at)` 已守「未成立关不上」「已关不重关」
  「仍有在场参与关不上」（`ErrSegmentStillActive`）。
- 端口：`ActualFulfillmentSegmentRegistry.CloseSegment` 是 ADR-0097 第三个窄口，只填关闭两列、
  `WHERE closed = false`。
- 适配器：postgres 实现与真库测试都在（重复关段答 `SegmentAlreadyClosed`）。

端口注释里那句话是本票的形状：**「编排必须先读回整段、走领域的 `CloseSegment` 再落库，不得直接
调本口关段」**——`ErrSegmentStillActive` 是跨行条件，窄口的前置条件表达不了，那是 ADR-0097 里
唯一一条靠纪律而非结构的约束。本票把那条纪律落成唯一的生产调用方。

## 接缝（测试只在这里写）

`application.CloseFulfillmentSegmentHandler.Close(ctx, CloseFulfillmentSegmentCommand)`。
命令只带租户、段引用、关闭时刻——关闭时刻由调用方给（决定是何时做的不由写库那一刻定），与
终止那一路的 `EndedAt` 同理。**不带依据字段**：领域与表都没有它，加它是改模型不是加编排。

结果代数封闭：`已关闭` / `早已关闭` / `仍有在场参与` / `段不在册` / `输入不受理` / `未决`。
「仍有在场参与」单独成格而不并进「不受理」：续办动作不同——前者去结束剩下的参与，后者改输入。

## 要做的

- 编排：读回整段 → 领域 `CloseSegment` → 窄口 `CloseSegment`；并发下另一方先关是业务答案不是失败。
- 测试替身：`segmentRegistryDouble.CloseSegment` 此前是返回 `Invalid` 的占位，`segmentRowsDouble`
  没有关闭两列、`FindByKey` 装回时不带 `Closed`——**替身若不带关闭状态，「关段后进不了」这一类
  永远测不出来**，同票 07 补 `EndParticipation` 那一格的理由。

## 陷阱

- **不自动关段**——本票不改 `EndFulfillmentParticipationHandler`。
- 关闭时刻早于最后一条参与的终点：领域今天不查这一条（`CloseSegment` 只看 `at` 非零）。**本票
  不加**——那是领域不变量的变化，要在领域层与重建门同时落，另议；票面如实记它没守。

## 完工判据

三条各有测试：段可经编排关闭；关闭后新对象经生产进段路径（`RegisterTransportHandover` →
`enterFulfillmentSegment`）进不了该段；仍有在场参与的段关不上且登记册一动不动。

## Comments

- 2026-09-03 · MCP-1：**本票 resolved，一刀 `2b4f6d7`。** 新增
  `application/close_fulfillment_segment.go` 与其测试；改 `establish_segment_on_handover_test.go`
  只动 `segmentRegistryDouble` 的 `CloseSegment` 与 `segmentRowsDouble` 的关闭两列。

  **完工判据三条对应的用例**：`TestASegmentWhoseParticipationsHaveAllEndedClosesAndAcceptsNoNewObjects`
  （可关，且关后 parcel-3 凭正当交接来到同段进不去、成员数不动、关闭状态不动）、
  `TestASegmentWithAnActiveParticipationCannotBeClosed`（仍有在场参与关不上、登记册一动不动）。
  另钉：重放答`早已关闭`且第一次的关闭时刻不被改写；段不在册；无时刻 / 无段引用不受理；登记册读或写
  失败为`未决`带续办引用；结果集封闭。

  **替身补真那一格有变异实测**：把 `FindByKey` 里带回 `Closed`/`ClosedAt` 的两行去掉，「可关且关后
  进不了」与「重放不改写时刻」两条当场红——这两条测的确实是关闭状态往返，不是替身自己的返回值。

  **一条走读时核出、本票没动的**：`arriveByHandover` 里 parcel-3 的交接**本身登记成功**（交接是控制
  事实的保全，进段是派生一侧，`enterFulfillmentSegment` 对领域拒绝交回空串不留续办）。所以「段已关、
  对象来了」在生产上的可观察结果只有「交接在册、段里没它」，**没有任何一格告诉调用方它该另立新段**。
  这是既有裁定（领域拒绝不留引用）的直接后果，不是本票引入的；要不要为`段已关闭`单开一格是票 03
  接端点时的问题，记在这里免得丢。

  **未做**（票面「陷阱」已记）：关闭时刻早于最后一条参与终点，领域不查，本票不加。
  → **已随补刀 `722e846` 做掉**，见下一条。

  **验证**：在 `2b4f6d7` 的干净检出上 `gofmt -l` 空、`go build ./...` 退 0、`go vet` TF 与
  architecture 退 0、`go test -count=1` TF 五包 + architecture 全 ok。未跑真库（无适配器层改动），
  `-race` 未跑。

- 2026-09-03 · MCP-1：**补刀 `722e846`——关闭时刻不得早于任何参与终点，转换门与重建门各守一次。**
  owner 「继续」后接的第一件。它是 CONTEXT「一个仍在控制中的对象足以让段继续存在」的时序面：关闭
  早于某成员离场，等于段在那个对象仍受控时已经结束。`CloseSegment` 在无在场参与之后再比一次时刻，
  早于任一终点拒 `ErrInvalidFulfillmentSegment`（与「终点早于起点」同错同族）；**同刻允许**——最后一个
  对象交出去那一刻关段是正当的，测试两边都钉了。重建门加同一条行间核对，仍只比行上两个时刻，不重放
  `CloseSegment`（ADR-0028 那条分界）。

  无签名变更；编排不改（它已把领域其余错误映到`不受理`）。`ports/fulfillment_segment.go` 头注里
  「ADR-0097 唯一一条靠纪律」的计数改成「这一类」并点名生产调用方；ADR 正文不动。

  **陷阱一节那条随之失效**，票面正文不改，此处记：它说的「本票不加」已由本条推翻。

  **验证**：在 `722e846` 的干净检出上 `gofmt -l` 空、`go build ./...` 退 0、`go vet` TF 退 0、
  `go test -count=1` TF 六包 + architecture 全 ok；**TF postgres 包带 DSN 实跑**（本机 55432，
  `-v` 下单条 `PASS` 非 `SKIP`），全包 124 PASS / 0 SKIP / 0 FAIL。`-race` 未跑。
