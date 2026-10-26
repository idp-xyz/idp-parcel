# 101 PS 取消与终局形成不看委托生命周期，取消命令也不核目标委托属信封账户：取消口一接上就可达

Category: bug
Status: ready-for-agent——R1（取消口上线前必修）：首方客户 API 的取消口（`/parcel-cancellations`）要接上（用户定的上线范围，见 [first-party-api 五口上线计划](../../first-party-api/spec.md)）。取消编排 `CancelParcelHandler` 与终局形成 `FormParcelFinalHandler` 对委托生命周期没有闸门：已撤回、已拒绝、只是已提交（未接受）的委托下的包裹，都不会被拦，取消照样成立、终局照样形成；取消命令不带客户账户，应用层也不核目标委托属哪个账户，隔离只靠 Intake 构造身份。今天走不到（取消授权角色生产未配置、首方取消 Intake 是 `UnconfiguredIntake{}`），接口一通就可达，所以是取消口的硬前置。2026-10-26 票 [code/36](./36-withdrawn-shipment-requests-cannot-be-read-back-from-the-store.md) 作者核查所得（事实①），首方五口摸底（`.scratch/first-party-api/customer-faces-scoping-2026-10-26.md` 事实①与闸门票 G）立；用户「你作为业务和系统专家自决」授权，协调方分级 R1。
Blocked by: [code/36](./36-withdrawn-shipment-requests-cannot-be-read-back-from-the-store.md)（已撤回委托读得回之后，闸门才有状态可看）
出处：code/36 作者核查；first-party-api 五口摸底。

## 现象

- `application.CancelParcelHandler.Handle`：先 `Requests.FindBySourceIdentity` 按来源身份加委托编号加成员核对（`memberOfCurrentVersion`）找委托，找不到答统一不可见结果；找到之后直接进取消幂等读、边界锁与取消形成，**不看委托的生命周期状态**。已撤回、已拒绝（自 ADR-0188 起可读回）、只是已提交的委托，其包裹都能被取消。
- `application.FormParcelFinalHandler`（以及经它的收寄采用、面单渠道服务终局判断路径）同样没有委托生命周期闸门：从未被接受的委托，也能为它的包裹形成终局。
- `CancelParcelCommand` 只有 `Identity`（来源身份）、`ShipmentRequestID`、包裹、请求方、原因、时刻，不带客户账户；`cancel_parcel.go` 没有客户账户引用。委托属不属信封账户，今天只能靠 Intake 把身份构造成"账户内唯一"。首发 1:1 下跨账户目标天然查无，但应用层没有第二道。

## 证据

- `internal/parcelshipment/application/cancel_parcel.go` 的 `Handle`、`CancelParcelCommand`、`memberOfCurrentVersion`；`form_parcel_final.go` 的 `FormParcelFinalHandler`。
- 文档：UC-PS-006（取消提交）、ADR-0254（取消边界）、ADR-0255 决定一（撤回后以撤回重提方向提交关联新委托——被撤回的委托不再承载包裹的后续动作）、PS CONTEXT 委托生命周期（`已撤回`、`已拒绝`之后原委托不再推进）。
- 摸底报告「取消」一行：命令不带账户，须保证目标委托属信封账户；闸门票 G 立即做，是取消口硬前置。

## 三类

机制。无产品策略；无租户取值。

## 规模

S–M：两只编排各加一道生命周期闸门（读委托状态，按状态分格）；取消加账户核（委托的客户账户等于信封账户，不等答统一不可见结果，不泄露存在）；补各状态与跨账户的真库用例。

## 最小的诚实修法

1. 取消：找到委托之后、取边界锁之前，核委托生命周期——只有`已接受`的委托才能取消其包裹；`已撤回`、`已拒绝`、`已提交`各自答一个确定性业务格（客户可行动：已撤回/已拒绝无事可取消；仅已提交的应改用撤回），不进未决（ADR-0029）。
2. 取消：核目标委托的客户账户等于调用方信封账户（需要端口读委托的客户账户；PS 自己的委托对象带客户账户，核对放应用层，别靠 Intake 一处）；不等与查无同答统一不可见结果（AT-PS-090 的隔离面）。
3. 终局形成：对从未被接受的委托（`已撤回`、`已拒绝`、`已提交`）拒绝形成终局，答确定性格；不改`已接受`之后的既有路径。
4. 用例：各状态一例（取消、终局各一）；跨账户取消答不可见且与查无逐字相同；闸门在边界锁之前；并发撤回与取消（取消读到`已接受`、撤回随后提交——这一格由 ADR-0254 与撤回/接受链的现有串行保证，核一遍，不够就记进越权风险点）。

## 要裁的

- 三个非`已接受`状态各答什么格、是否要新的结果格：倾向复用`请求未受理`以外的新业务格（名字照仓内构词），因为 UC-PS-006 的`请求未受理`只说字段缺。落地时由作者定，写进票面。
- 终局形成的闸门放在 `FormParcelFinalHandler` 里，还是放在调它的三条采用路径各自入口：倾向放在它里面（单点），核它的所有调用方（含 `AdoptOnEffectiveDelivery` 那一路）不会被误拦。
