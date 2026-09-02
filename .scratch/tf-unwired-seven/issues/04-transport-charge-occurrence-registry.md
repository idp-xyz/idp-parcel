# 运输收费发生项登记册与失败尝试入口

Category: enhancement
Status: draft
Blocked by: 无

## 这一票为什么刺眼

`ChargeOccurrenceForFailedAttempt` 要的入参 `AttemptObjectResult`，**就在
`application/perform_offsite_pickup.go` 里由 `FormAttemptObjectResult` 造出来**，隔几行没人
再用。它守的是 `AT-TF-094`（失败尝试形成发生项，第二次成功不覆盖第一次），验收判据编号写在
函数注释里。

## CONTEXT 与既有领域约束

领域侧 `TransportChargeOccurrence` 已实现：**无金额字段**（金额归 `settlement-accounting`）、
原/替代旅程不合并、事实依据与业务时间取自结果本身、**揽收到手的结果被拒**（只有失败结果能
形成失败尝试费）。

开发主线 PN-04 把「收费发生项（无金额字段、原/替代旅程不合并）」列为已收口的领域件——领域件
确实在，**入口不在**。

## 现状（锚 `9d6063c`）

领域 `domain/transport_charge_occurrence.go` 有 `TransportChargeOccurrence`、
`TransportChargeOccurrenceSpec` 与两个构造入口（常规一个、失败尝试一个）。**无表、无端口。**

## 要做的

**表**：新开迁移。**不得有金额列**——这条要写进迁移注释，因为下一个人很容易「顺手」加一个
`amount_minor`，而那会把 `settlement-accounting` 的所有权搬过来。

**约束**：
- 同一尝试的失败发生项只登一次，第二次成功**不覆盖**第一次（`AT-TF-094` 的库面镜像）。
- 原旅程与替代旅程的发生项分别成立、不合并（封闭的旅程归属列）。

**端口 + 适配器 + 编排**：把失败入口接进 `perform_offsite_pickup.go`。

> **⚠ 上面这句话是错的，2026-09-02 动笔时核出来，见文末 Comment。** 手上有
> `AttemptObjectResult` 只满足事实依据那一格，其余七格都不在那个 handler 里。这一票**不是
> 机械活**，接线前需要一次裁决。

## 陷阱

- **失败尝试费是费用的「发生」不是「金额」。** 登记它不表示要收多少钱，也不表示应收应付成立；
  金额与责任由 `settlement-accounting` 依据它形成。迁移与端口注释都要说清，否则下游会把它
  当账。
- 成功尝试不形成失败尝试费——这一条领域已经拒了（`ErrNotAFailedAttempt`），库面也要镜像，
  否则绕过构造门的写入路径能落进一条不该存在的行。

## 完工判据

`ChargeOccurrenceForFailedAttempt` 有生产调用路径，棘轮基线那一行可剪。

## Comments

- 2026-09-02 · MCP-3：**动笔时核出本票不是机械活，且我票面正文那句接线方案是错的。已就地
  标注作废，未改代码。**

  **一、`perform_offsite_pickup.go` 手上没有形成发生项所需的东西。**
  `PerformOffsitePickupCommand` 的字段是租户、来源、任务、尝试、执行方、地点、计划窗口、
  到场时刻、证据、改约来源、对象清单——**没有旅程、没有责任法人、没有服务提供方、没有协议
  快照、没有范围、没有数量单位、没有有效性版本**。而 `FormTransportChargeOccurrence` 这些
  全是必备（缺一即 `ErrInvalidChargeOccurrence`）。

  我正文里写「那里已经有 `AttemptObjectResult`，判一下失败就能登记」——**那只满足了事实
  依据一格**，把「入参之一在场」当成了「入参齐备」。

  **二、再往下想一层，这一票的范围本身要收窄。** CONTEXT 说自营履约「不虚构外部供应商、
  供应商协议或外部运输委托」，而发生项的定义是「**可能依据供应商协议形成外部运输成本**的
  业务事实范围」。两句合起来：**自营揽收失败不该形成发生项**——没有外部成本可言。

  所以失败尝试费只在**外包揽收**下成立。而「这次揽收是不是外包、依哪份协议」这件事，
  **在整条揽收路径上今天不存在**：`OffsitePickup` 与 `FulfillmentAttempt` 都不带采购上下文，
  `PickupAttemptStore` 也不存。这是[分类表](../../mechanism-executor-triage/spec.md)里同一族
  的第四例——**有语言无形状**：CONTEXT 用整节写了采购责任与协议快照，而揽收这条路上没有它的
  落点。

  **三、因此本票要先裁一件**：失败尝试费发生项的采购上下文从哪来。三条路各有代价：

  - **扩 `PerformOffsitePickupCommand`**：加七个字段。代价是把采购事实塞进一条本来只报
    物理事实的命令，而调用方（节点作业侧）多半不知道协议快照。
  - **开一个采购上下文读口**（按揽收任务或旅程解析出法人/提供方/协议）。代价是多一个端口，
    但它与 CONTEXT 的所有权划分一致——采购责任本就不属揽收登记方。**我倾向这条。**
  - **不在揽收编排里形成，另开一条编排**（由持有采购上下文的一方在失败结果之后形成）。
    代价是多一次跨编排的触发，好处是两种事实各归其位。

  **裁之前不动代码。** 猜错的代价不是返工，是给一批自营揽收凭空造出外部成本来源。

  **四、体量也比票面写的大**：成员是列表（`Members []CarriedObjectReference`），要第二张
  表；另有有效性版本链（`corrects` / `revisionKind` / `revisionBasis` / `revisedAt`）四件
  成组。按票 01 的实测，这一票接近「大」而不是「中」。
