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

**端口 + 适配器 + 编排**：把失败入口接进 `perform_offsite_pickup.go`——那里已经有
`AttemptObjectResult`，判 `Outcome().Failed()` 后登记。

## 陷阱

- **失败尝试费是费用的「发生」不是「金额」。** 登记它不表示要收多少钱，也不表示应收应付成立；
  金额与责任由 `settlement-accounting` 依据它形成。迁移与端口注释都要说清，否则下游会把它
  当账。
- 成功尝试不形成失败尝试费——这一条领域已经拒了（`ErrNotAFailedAttempt`），库面也要镜像，
  否则绕过构造门的写入路径能落进一条不该存在的行。

## 完工判据

`ChargeOccurrenceForFailedAttempt` 有生产调用路径，棘轮基线那一行可剪。
