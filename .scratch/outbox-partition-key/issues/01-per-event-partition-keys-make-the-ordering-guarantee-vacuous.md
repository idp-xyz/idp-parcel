# 逐事件分区键让框架的排序保证落空，而乱序投递不报任何错

Category: bug
Status: needs-triage

发现于清点 46 个 handoff 时（`cab9d1b`）。**只写票，不动代码**——涉及的 handoff 分属
MCP-2、MCP-5 与本通道三块地盘，逐个该用什么分区键是各自主人的判断。

MCP-2 已把它列为派发接线的**并列前置**：订阅者补齐不会顺带修好它，而
[缺订阅者会响亮地报 `ErrNoSubscriber`，乱序投递不报任何错、只是结果错](../../handoff-consumer-inventory/report.md)
——两类缺陷的发现成本差一个量级。

## 结论句

**全仓 46 个 handoff 里有 34 个把 `PartitionKey` 设成逐事件唯一值，于是这 34 类信封的
排序保证是空的。** 框架的顺序保证就是靠分区序列（`finalize.go` 的 `advancePartition` 与
`setPartitionClaimable` 都带 `sequence`），而分区里只有一条时，那个保证没有任何内容。

## 两套约定并存

### 逐事件唯一值（34 个）

`PartitionKey: eventID`、`shape.eventID`，或 `tenant + "/" + eventID`：

- parcel-shipment：`network_intake`、`final_outcome`、`parcel_cancellation`
- network-routing：`initial_route`
- settlement-accounting：`supplier_bill`、`charge_confirmation`、`settlement_application`、
  `operating`、`claim_settlement`、`advance_recovery`、`statement`
- node-operations：`node_intake`、`execution_fact`、`sealed_snapshot`、`collaboration_acceptance`
- transport-fulfillment：`offsite_pickup`、`offsite_pickup_registration`、`effective_delivery`、
  `transport_handover_registration`、`transport_commission`、`capacity_consumption`、
  `exception_journey`、`disposition_execution`、`regulatory_acceptance`
- parcel-pricing：`evaluation`
- pilot-governance：`governance`
- customs-compliance：`customs_case`、`declaration_submission`、`gate_verification`、
  `verification`、`manifest`、`follow_up`、`case_closure`、`external_result`

### 业务主体键（12 个）

- visibility-exception 八个全部如此：`tenant/包裹`（`projection`、`eta`、`visibility_gap`、
  `triage`）、`tenant/客户`（`customer_view`、`notification`）、`tenant/批次`（`liability`）、
  `tenant/案件`（`disposition`）
- customs-compliance：`restriction` 用 `tenant/范围`
- parcel-shipment：`acceptance_decision` 用 `tenant/委托`、`source_data` 用 `tenant/来源请求键`
- network-routing：`reachability` 用 `tenant/请求关联`

## 「保证为空」不等于「有害」——判据在这里

多数逐事件分区无害：那些 `eventID` 本身就派生自业务幂等键，一个业务对象一辈子只发一条
信封，分区是单条属于事实而不是缺陷。例如 network-routing 的 `initial_route`，信封 ID 由
判断键加类型段认领，而复核路径（`reassess_route.go`）根本不发布意图（它的 deps 里没有
Downstream），所以同一判断键确实只会有一条。

**有害的判据只有一条：两条或更多信封是否描述同一个业务对象的先后状态。** 是则乱序会
改变结果，否则分区单条无所谓。

## 首例：恢复赶在暂停前送到

`OutboxGovernanceHandoff` 一个 handoff 发三种类型——`pilot-governance.suspension.recorded`、
`.resumption.recorded`、`.takeover.recorded`——而 `PartitionKey` 是 `eventID`。

暂停与恢复是同一个治理对象的先后两拍。逐事件分区下它们落在两个分区，没有任何东西保证
恢复不会先于暂停送达。**后果是受影响上下文永远停在暂停态**（或反过来，被恢复到一个它
从未进入过的状态）。

这一例值得放在最前，因为它的故障现象是「某个上下文的准入闸莫名一直关着」，而**没有人会
想到去查派发日志**——那里什么错都没报。

同一形状的另外几处（各自是否真有因果先后要问地盘主人，见下节）：
`StatementHandoff` 发 published / voided / included 三种，`ClaimSettlementHandoff` 发四种，
`AdvanceRecoveryHandoff` 与 `OperatingHandoff` 各发两种，全部共用逐事件分区。

## 有一处做对了，而且它就是现成的修法

`OutboxRestrictionHandoff` 用的是 `tenant + "/" + 限制范围`。于是同一范围的「建立」与
「解除」落在同一分区、保序——而这正是最不能乱的一对：解除先于建立到达，等于一条从未
生效过的限制被解除，执行方从此不再阻断。

**这一条的意义超出它自己**：它证明两套约定不是随机差异，而是有人想过、有人没想过。
所以修法不必从头发明——照 `restriction_handoff.go` 那一行的做法（把分区键定在业务对象上
而不是事件上）即可。

## 需要各地盘主人回答的问题

本票不定方案。要修必须先由拥有该 handoff 的人回答「这些信封之间有没有因果先后」，那是
业务判断，不是能从代码读出来的。

| 归属 | 要回答的 |
|---|---|
| pilot-governance（当前无主） | 暂停/恢复/接管三种是否同一治理对象的先后拍。若是，分区键应取什么业务维（被暂停的范围？租户？） |
| settlement-accounting（MCP-5） | `statement` 三种（发布/作废/后续纳入）有无先后；`claim_settlement` 四种、`advance_recovery` 两种、`operating` 两种同问 |
| customs-compliance（当前无主） | 同一案件的建立 → 申报提交 → 核对 → 关闭是否需要保序。注意它们分属**不同 handoff**，即便各自改用业务键，也要用**同一个键公式**才会落进同一分区 |
| node-operations（MCP-5） | 同一集运单元的封装快照与执行事实有无先后 |
| transport-fulfillment（本通道） | 同一对象的揽收登记与交接登记、同一容量池的多次消耗有无先后。我会在拿到判据后自查并回报 |
| parcel-shipment（MCP-2） | `final_outcome` 与 `parcel_cancellation` 对同一包裹有无先后；`acceptance_decision` 与 `source_data` 已用业务键，确认键公式是否需要对齐 |

**跨 handoff 那一条最容易漏**：分区由键值决定，不由 handoff 决定。两个 handoff 只要算出
同一个键字符串，它们的信封就进同一分区并因此保序；算不出同一个键，改成业务键也白改。

## 本票不做的事

- 不定任何 handoff 的分区键取值。
- 不改代码。发现虽出自本通道，但改动跨三块地盘。
- 不判断「是否该先接派发线」——那是 MCP-2 已经作出的决定（不接），本票只是它列出的并列
  前置之一。

## 一条给写代码的人的提醒

改分区键会改变已入队信封的分区归属。若在有存量 PENDING 条目时改，新旧键算出的分区不同，
**存量条目仍留在旧分区里**。今天没有派发器在跑、也没有存量，所以现在改是最便宜的时刻；
一旦接线并开始积累，同样的改动就要连带考虑迁移。
