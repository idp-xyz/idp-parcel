# 46 个 Outbox handoff 的下游消费方向清点

Category: enhancement
Status: superseded

> **已被取代**（MCP-1 记，2026-08-17）：现行权威是
> [`../outbox-handoff-consumption-map/report.md`](../outbox-handoff-consumption-map/report.md)
> （task-8a0c476f，取证于 HEAD `4d57ecd`）。两份底层类型集合一致（55 类、46 适配器），
> 新表判据全部落到 UC 引文，并把本文「判不准，要人定」8 条解决了大半（manifest 归 CC
> 内部、ClaimSettlement 四类按 MAP 单向约束不得开 VE 消费者、statement 三类定为混合）。
> 本文仍然成立且新表未重复的两点：「三条先看的结论」的第一条（按 46 配会漏 9 类）与
> 「适配器注释不是意图的出处，端口接口注释才是」。
>
> 台账勘误：本文即 task-818b6e81 的实际产出（转手后于 2026-08-14 以 cab9d1b 落档，
> 未回报台账）；该票 2026-08-17 被 MCP-1 误以「无任何产出」结为 failed，结论以本头注为准。

只读清点，无代码改动。承自 task-818b6e81（MCP-4 转来）。供派发路由表装配使用。

判据只取三处：[CONTEXT-MAP](../../docs/domain/CONTEXT-MAP.md) 的「关系约束」、各
`docs/domain/<context>/CONTEXT.md` 的所有权声明、以及各 `internal/<context>/ports/ports.go`
上 `*HandoffIntent` 的意图注释（那是发布方自己写下的「交给谁」）。**没有按命名相似推断。**

## 三条先看的结论

**一、路由表要配的是 55 种信封类型，不是 46 个 handoff。** 五个 handoff 一对多：
`GovernanceHandoff` 发 3 种、`OperatingHandoff` 2 种、`ClaimSettlementHandoff` 4 种、
`AdvanceRecoveryHandoff` 2 种、`StatementHandoff` 3 种。按 46 去配会漏 9 种，而按
[ADR-0049](../../docs/adr/0049-publish-channel-is-in-process-delivery-until-load-evidence.md)
漏掉的类型一旦被发出就 `ErrNoSubscriber` 并阻塞分区。

**二、16 种类型的下游在发布方自己的上下文内部，不该配跨上下文订阅者。** 这是本次清点
最要紧的产出——把 46 个全当缺口会造出一批没人需要的消费者。它们集中在 settlement-accounting
（对账单纳入、审核对账、已结视图）与 visibility-exception（客户视图派生、信号链、义务台账），
两处的意图注释都明写下游是本上下文的下一段编排。

**三、真需要跨上下文订阅者的约 30 种，其中 1 种已有实现。** 唯一的消费者是
`internal/networkrouting/adapters/inbox` 的 `AcceptanceConsumer`。全仓再无第二个
——`transportfulfillment/application` 里那个 `Consume` 是容量消耗，不走 `inbox.Store`。

| 格 | 类型数 |
|---|---|
| 已有消费者 | 1 |
| 应有但未开 | 30 |
| 本就不应该有跨上下文消费者 | 16 |
| 判不准，要人定 | 8 |

## 一个通用发现

**适配器注释不是意图的出处，端口接口注释才是。** 46 个 `Outbox*Handoff` 的 Go 注释是
统一的机械句（「把 X 写入 Outbox，实现 ports.Y。入队一步由 outboxintent.EnqueueOnce
承担」），一个下游都没点名。真正写着「交给谁」的是 `ports.go` 上 `*HandoffIntent` 的注释。
装配路由表时按适配器去找下游会一无所获，别绕这个弯。

## 已有消费者（1）

| 信封类型 | 发布方端口 | 消费方 |
|---|---|---|
| `parcel-shipment.acceptance-decision.formed` | PS `AcceptanceDecisionHandoff` | NR `nrinbox.AcceptanceConsumer` |

## 本就不应该有跨上下文消费者（16）

配了反而错：这些意图的下游是发布方自己的下一段编排，或是系统外的对方（外部通道、门户、
分析面），都不是另一个限界上下文。

| 信封类型 | 发布方端口 | 意图注释点名的下游 | 何以判定为内部 |
|---|---|---|---|
| `transport-fulfillment.capacity-consumption.recorded` | TF `CapacityConsumptionHandoff` | 「交给装载分配链」 | CONTEXT-MAP `node-operations ↔ transport-fulfillment`：「运输履约唯一拥有……班次、装载分配和节点间移动」——装载分配是 TF 自己的 |
| `customs-compliance.external-result.received` | CC `ExternalResultHandoff` | 「交给判断与核对消费」 | 两者都是 CC 自己的编排 |
| `customs-compliance.declaration-submission.formed` | CC `DeclarationSubmissionHandoff` | 「交给发送通道与外部结果核对消费」 | 发送通道是系统外能力；外部结果核对是 CC 自己的 |
| `settlement-accounting.supplier-bill.received` | SA `SupplierBillHandoff` | 「交给审核与对账消费」 | 两者都在 SA 内 |
| `settlement-accounting.charge-confirmation.formed` | SA `ChargeConfirmationHandoff` | 「交给对账单纳入消费（UC-SA-003 只纳已确认费用）」 | 对账单属 SA |
| `settlement-accounting.settlement-application.applied` | SA `SettlementApplicationHandoff` | 「交给下游（对账单与应付的已结视图）」 | 两者都在 SA 内 |
| `settlement-accounting.advance-recovery.formed` | SA `AdvanceRecoveryHandoff` | 「交给对账单纳入消费（UC-SA-003 上游）」 | 同上 |
| `settlement-accounting.recovery-adjustment.formed` | SA `AdvanceRecoveryHandoff` | 同上 | 同上 |
| `settlement-accounting.cost-allocation.formed` | SA `OperatingHandoff` | 「交给分析消费」 | 「分析」不是本仓的任何一个限界上下文 |
| `settlement-accounting.operating-result.derived` | SA `OperatingHandoff` | 同上 | 同上 |
| `visibility-exception.tracking-projection.derived` | VE `ProjectionHandoff` | 「客户视图派生正是消费者」 | 客户视图属 VE |
| `visibility-exception.customer-view.published` | VE `CustomerViewHandoff` | 「门户展示、通知判断的输入」 | 门户在系统外；通知判断属 VE |
| `visibility-exception.eta-prediction.formed` | VE `ETAHandoff` | 「客户视图链的重派生输入」 | 属 VE |
| `visibility-exception.visibility-gap.formed` | VE `VisibilityGapHandoff` | 「交给信号链（缺口是否命中异常规则由分诊判断）」 | 信号与分诊都属 VE |
| `visibility-exception.signal-triage.concluded` | VE `TriageHandoff` | 「建案、复核队列的输入」 | 案件属 VE |
| `visibility-exception.customer-notification.formed` | VE `NotificationHandoff` | 「义务台账、升级判断的输入」 | 都属 VE |

**这一格对派发装配的含义要说清楚**：它们仍然入 Outbox、仍然要被派发，只是订阅者与发布方
同属一个上下文。**不是「不用配」，是「配的是本上下文自己的消费者」。** 若装配时认定进程内
同上下文调用不必过 Outbox，那是另一个决定（要不要保留这些 handoff），本清点不替它拍。

## 应有但未开（30）

按 CONTEXT-MAP 的关系约束确有跨上下文接收方，而代码里没有消费者。

### parcel-shipment 发出（4）

| 信封类型 | 应消费方 | 依据 |
|---|---|---|
| `parcel-shipment.source-data-version.formed` | CC | CONTEXT-MAP `parcel-shipment → customs-compliance`：「小包托运提供包裹身份、接受基线和 `UC-PS-002` 形成的版本化客户原始资料；关务只读引用当前适用范围」。端口注释刻意不逐下游拆口：「哪些下游该重新判断，取决于范围、阶段与各自的门禁，那是下游自己的判断」——所以路由表可挂多个订阅者 |
| `parcel-shipment.network-intake.recorded` | NR | 端口注释：「network-routing 的复核触发正是它的消费者」。NR 侧的 `ReassessOnIntakeAdapter` 已在位，缺的只是信封消费者这一层 |
| `parcel-shipment.final-outcome.formed` | VE、SA | 端口注释：「追踪、异常、结算消费稳定引用，不回写终局」 |
| `parcel-shipment.parcel-cancellation.recorded` | NR、SA | 端口注释：「路由释放、财务控制释放等各自独立承接」 |

### network-routing 发出（2）

| 信封类型 | 应消费方 | 依据 |
|---|---|---|
| `network-routing.reachability-judgment.formed` | PS | CONTEXT-MAP `network-routing → parcel-shipment`：「返回可达性判断及其判断标识……标准网络服务的可达结果参与委托接受判断」 |
| `network-routing.initial-route.formed` | NO、TF、VE | CONTEXT-MAP `network-routing → node-operations / transport-fulfillment`：「路由计划、计划节点、计划履约段、时间窗口和路由指令表达版本化执行意图」；`network-routing → visibility-exception`：「提供路由版本、选择依据、无当前有效路由和路由偏离结果」 |

### parcel-pricing 发出（1）

| 信封类型 | 应消费方 | 依据 |
|---|---|---|
| `parcel-pricing.evaluation.recorded` | SA | 端口注释：「费用采用在 settlement-accounting，本上下文只交结果不造费用」；CONTEXT-MAP `parcel-pricing → settlement-accounting` 同向 |

### node-operations 发出（4）

| 信封类型 | 应消费方 | 依据 |
|---|---|---|
| `node-operations.node-intake.formed` | PS | 端口注释：「parcel-shipment 的采用判断正是消费者」 |
| `node-operations.execution-fact.recorded` | CC | 端口注释：「交给 customs-compliance 的处置执行核对消费（CC 侧 ExecutionFactView 的上游源）」 |
| `node-operations.collaboration-acceptance.decided` | CC | 端口注释：「交回 customs-compliance——承接结果是协作链的回执信号」 |
| `node-operations.sealed-snapshot.recorded` | TF | 端口注释：「装载与交接按封装快照对货」；CONTEXT-MAP `node-operations ↔ transport-fulfillment` 把装载与权威交接判给 TF |

### transport-fulfillment 发出（8）

| 信封类型 | 应消费方 | 依据 |
|---|---|---|
| `transport-fulfillment.offsite-pickup.formed` | PS | 端口注释：「交给 parcel-shipment 判断有效网络收寄（UC-TF-002 步骤 6A）」 |
| `transport-fulfillment.offsite-pickup.registered` | PS | 端口注释：「交给 parcel-shipment 采认（既有 OffsitePickupAdapter 的上游，UC-PS-003 揽收源链）」。**与上一条同去向，见下方存疑项** |
| `transport-fulfillment.effective-delivery.registered` | PS | 端口注释：「交给 parcel-shipment 终局判断消费（既有 DeliveryOutcomeAdapter 的上游）」 |
| `transport-fulfillment.transport-handover.registered` | NO、NR | 端口注释：「一份意图，消费方自分——node-operations 的控制转移只认得出 TransferOutBasis 的已交接，network-routing 以 TransportHandoverControl 证据种类触发重判」 |
| `transport-fulfillment.transport-commission.submitted` | SA | 端口注释：「交给 settlement-accounting 作供应商成本预期的上游来源」 |
| `transport-fulfillment.exception-journey.recorded` | VE | 端口注释：「交给 visibility-exception 异常链——监管与非监管来路都要让异常侧看见」 |
| `transport-fulfillment.disposition-execution.recorded` | CC | 端口注释：「交给 customs-compliance 作处置执行事实源」，且限定「只有 RegulatoryOrigin 的旅程走这条链」 |
| `transport-fulfillment.regulatory-acceptance.recorded` | CC | 端口注释：「回执给关务协作链（`UC-CC-008` 据以更新事项交接结果）」 |

### customs-compliance 发出（6）

| 信封类型 | 应消费方 | 依据 |
|---|---|---|
| `customs-compliance.regulatory-restriction.changed` | TF、NO、PS | 端口注释：「TF/NO 的门禁执行方消费——它们只执行不豁免」；CONTEXT-MAP 另有 `各限制源 → parcel-shipment`：「小包托运只据此判断新交易或重开的适用资格」 |
| `customs-compliance.gate-verification.recorded` | TF、NO | 端口注释：「TF/NO 的动作执行方消费——门禁满足不生成放行」；CONTEXT-MAP `customs-compliance → node-operations / transport-fulfillment` 同向 |
| `customs-compliance.customs-case.established` | VE | 端口注释：「申报链与 VE 案件视图消费」——申报链属 CC 内部，VE 是跨上下文那一半 |
| `customs-compliance.disposition-verification.recorded` | VE | 端口注释：「案件关闭核对与 VE 的处置协调消费它」——同上，前者内部后者跨界 |
| `customs-compliance.follow-up.recorded` | VE | 端口注释：「申报执行方消费目标，VE 与案件视图消费生效」 |
| `customs-compliance.case-closure.recorded` | VE | 端口注释：「VE 的案件视图与治理审计消费它」 |

### pilot-governance 发出（3）

| 信封类型 | 应消费方 | 依据 |
|---|---|---|
| `pilot-governance.suspension.recorded` | 受影响上下文的准入闸 | 端口注释：「交给受影响上下文的准入闸消费（暂停生效、恢复生效、权威切换）」 |
| `pilot-governance.resumption.recorded` | 同上 | 同上 |
| `pilot-governance.takeover.recorded` | 同上 | 同上 |

**这三条的订阅者清单定不下来**：意图注释说的是「受影响上下文」，而哪些上下文受影响取决于
暂停的范围，是运行期数据不是装配期常量。`pilot-governance` 也不在 CONTEXT-MAP 的九个上下文
之列。路由表要一份静态清单，所以这三条需要一个决定——见下节。

### visibility-exception 发出（2）

| 信封类型 | 应消费方 | 依据 |
|---|---|---|
| `visibility-exception.claim-liability.concluded` | SA | 端口注释：「交给结算侧（`UC-SA-007` 赔付金额链的上游源——金额由结算形成，这里只交结论）」 |
| `visibility-exception.disposition-request.sent` | NR、NO、TF、CC | CONTEXT-MAP `visibility-exception → 业务上下文`：「异常处置只能提出带目标、范围和依据的改路、重作业、重新履约或重新申报等请求；发送、接受和实际完成分别记录」。四种请求对应四个上下文 |

## 判不准，要人定（8）

### 1. `customs-compliance.manifest.recorded`

端口注释：「交给适用下游（案件视图与申报链消费）」。申报链明确属 CC 内部；**「案件视图」
归谁不确定**——CC 自己有案件视图，VE 也有案件视图（`CaseClosureHandoff` 注释里的「VE 的
案件视图」写得很明确，这一条却没带 VE 前缀）。两种读法一个落第二格一个落第三格。

### 2–5. `ClaimSettlementHandoff` 的四种类型

`settlement-accounting.claim-amount.formed`、`.recovery-receivable.formed`、
`.recovery-acknowledgement.formed`、`.claim-adjustment.formed`。

端口注释：「把赔付金额与调整交给对账单纳入、把应追偿与认可交给追偿链。各携其记录，
消费方自分」。**对账单纳入属 SA 内部**（第三格），**追偿链归谁则不确定**——CONTEXT-MAP
的上下文节写着「`visibility-exception` 拥有……客户索赔项和追偿事项」，但
`visibility-exception → settlement-accounting` 一条又说结算「独立形成客户赔付、应追偿金额
和追偿认可金额」。也就是说追偿金额在 SA、追偿事项在 VE，这四种类型分别落哪边要逐条判。

一句提醒：**这四种是同一个 handoff 发出的四种类型，而路由表按类型建**，所以它们完全可以
分别落不同的格，不必统一。

### 6–8. `StatementHandoff` 的三种类型

`settlement-accounting.statement.published`、`.voided`、`.included`。

端口注释：「交给下游（客户通知、外部资金核销的目标索引）」。**外部资金核销指向系统外**
（第三格那一半），**客户通知归 VE**（CONTEXT-MAP：VE 拥有「客户披露及通知决定」）——若
这一半成立，三种类型都要挂 VE 订阅者。但注释没说三种是否都要通知客户（`voided` 作废是否
通知客户是业务问题），所以三条分别判。

## 另有两条需要注意，不属上面任何一格

**一、`transport-fulfillment.offsite-pickup.formed` 与 `.registered` 同去向 PS，值得确认是不是
真要两条链。** 两个端口的意图注释分别指 `UC-TF-002 步骤 6A`（尝试提交那条链上的揽收）与
`UC-PS-003 揽收源链`（对象级登记入口），幂等键也不同（来源身份 对 「对象+尝试」）。所以
它们确实是两件事，不是重复——但 PS 侧要不要两个消费者、还是一个消费者认两种类型，是装配
决定。我在 `a5095ae` 里为同样的理由把它们的登记表分了开（合表必坏其一的重放判定），这条
理由在消费侧同样成立。

**二、四个 handoff 的端口注释仍写着「今天没有实现，唯一实现是测试替身」，而适配器已经落地。**
`ExternalResultHandoff`、`SupplierBillHandoff`、`ChargeConfirmationHandoff`、
`AdvanceRecoveryHandoff`、`StatementHandoff`、`SettlementApplicationHandoff`、
`OperatingHandoff`、`ClaimSettlementHandoff`、`ReachabilityJudgmentHandoff` 等的注释是
Bento 闸门解除前写的，现在都有 `Outbox*Handoff` 实现了。注释过期不影响本清点的判定（我取的
是「交给谁」那半句，不是「有没有实现」那半句），但**它们该被清理**，否则下一个人会以为这些
口还没接。这属各自地盘，本清点只报不改。

## 本清点没做的事

- 没有开任何消费者、没有碰 `cmd/`、没有改任何代码。
- 没有判定「第三格那 16 种要不要继续走 Outbox」——那是装配决定，不是方向判定。
- `pilot-governance` 那三条与「判不准」那 8 条需要人定，路由表在它们定下来之前不完整。

## 底数注记 as-of `1665fdb`（2026-09-01，MCP-3，批务票 admin-remainder-mechanism-batch/05 第 1 项）

本文已被取代（见头注），本节不复活它，只把**它与现行权威共用的那个底数**钉一次时点——
头注写着「两份底层类型集合一致（55 类、46 适配器）」，那句话锚在 2026-08-17，今天不再是当前值。

实测于 `1665fdb`（口径同现行权威表，命令见
[T2 量尺重核 `1665fdb`](../admin-remainder-mechanism-batch/t2-remeasure-1665fdb.md)）：
**Outbox 发布适配器 47、信封类型 59**（`4d57ecd` 上按同一命令回跑得 46 / 55，与头注一致，
所以这是树在动不是量法在动）。新增四类是
`parcel-shipment.shipment-request.submitted`、`customs-compliance.follow-up.replacement-proposed`、
`customs-compliance.follow-up.replacement-effective`、`settlement-accounting.settlement-application.reversed`，
**四类都不在本文那 46 / 55 的任何一格里**——本文的四格分类（已有消费者 1 / 应有但未开 30 /
本就不应该有 16 / 判不准 8）因此对它们无话可说，别把 55 当全集去减。

本文头注列为「仍然成立且新表未重复」的两点：

- **「按 46 配会漏 9 类」这条结论仍成立，数变了**：`1665fdb` 上是 47 个适配器发 59 类，
  按适配器去配会漏 **12** 类。哪几个适配器一对多、各发几类，本轮未重数。
- 「适配器注释不是意图的出处，端口接口注释才是」不是计数断言，不随 HEAD 过期，原样成立。
