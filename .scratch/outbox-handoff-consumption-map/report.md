# 只读清点：46 个 outbox handoff 的下游消费方向与缺口

- 任务：`task-8a0c476f-969d-4b82-8ba5-ad18c4c602fc`（取代 `task-818b6e81`，零起点重派）
- 清点基线：HEAD `4d57ecd`（2026-08-17，清点全程未变；全部已推）
- 执行方：MCP-3（只读；除本文件外未改任何文件）

## 方法与判据

- **范围**：`internal/*/adapters/postgres/*handoff*.go` 中实现 `ports.*Handoff` 的 Outbox 发布适配器。共 **46 个适配器、55 个信封类型、9 个发布上下文**（PS5+NR2+NO4+TF9+CC9+SA14+VE8+PP1+PG3）。`network-routing/adapters/postgres/route_handoff_log.go` 不在内——它是 UC-NR-001 步骤 2 的重放指纹册（`ports.RouteHandoffLog`），不发布任何信封。
- **第 3 栏判据**只取三类权威文档：[CONTEXT-MAP](../../docs/domain/CONTEXT-MAP.md) 的边与关系约束、各 `CONTEXT.md` 所有权声明、`UC-*` 正文。端口注释只用作找 UC 的索引；凡判据只剩端口注释而查无文档处，如实标注。
- **第 4 栏四态**：`已有消费者（注明）` / `应有但未开` / `本就不应该有跨上下文消费者（审计/对外也是结论）` / `说不清（写明缺哪份文档）`。
- 现状底帐（`60ea63c`）：组合根（`cmd/parcel-dispatch/assemble.go`）路由表**两条**，都投向 network-routing——`parcel-shipment.acceptance-decision.formed` → `network-routing/initial-route-on-acceptance`（AcceptanceConsumer，64bae12），`parcel-shipment.network-intake.recorded` → `network-routing/reassess-on-network-intake`（NetworkIntakeConsumer，60ea63c）。两条各带各的未决哨兵翻译（`ErrRouteHandoffUndecided` / `ErrReassessmentUndecided`）。未映射类型经 `dispatch.DirectPublisher` 撞 `ErrNoSubscriber`，失败码 `dispatch.no_subscriber`，阻塞该分区（ADR-0049 第三条，有意设计）。

## 总览

| 计数 | 值 |
|---|---|
| Outbox 发布适配器 | 46 |
| 信封类型 | 55 |
| 已有消费者的类型 | 2（`parcel-shipment.acceptance-decision.formed`、`parcel-shipment.network-intake.recorded`） |
| 应有但未开（跨上下文消费） | 28 类（PS3+NR2+NO4+TF9+CC7+VE2+PP1） |
| 应有但未开（**同上下文**下一段编排消费） | 15 类（CC2+SA9+VE4） |
| 混合（内部消费者未开＋对外段） | 5 类（statement×3、customer-view、customer-notification） |
| 本就不应该有跨上下文消费者 | 2 类（cost-allocation、operating-result → 分析/报表，对外） |
| 说不清 | 3 类（pilot-governance 全部） |

**一条在途链要先说**：今天唯一被组合根构造的生产方编排就是 `CreateInitialRouteHandler`（assemble.go），它成功一次就经 `OutboxInitialRouteHandoff` 入队 `network-routing.initial-route.formed`——而该类型无消费者。**首个纵向切片跑通路由的那一刻，派发分区即撞 `dispatch.no_subscriber` 阻塞**。这不与 ADR-0049 冲突（接不住就不登记是对的），但意味着「初始路由的下游消费者（见第 7 行）」在纵向闭环里排在比直觉更靠前的位置。今天它尚未发作，因为上游 PS 侧生产编排（FormAcceptanceDecision 等）也无人构造，整条链是静默的。

## 主表

「同上下文」= 消费者是发布方自己上下文的下一段编排（仍需路由条目，但不跨所有权边界）。

### parcel-shipment（5 适配器 / 5 类型）

| # | 端口 | 信封类型 | 谁应该消费（判据） | 现状 |
|---|---|---|---|---|
| 1 | `SourceDataVersionHandoff` | `parcel-shipment.source-data-version.formed` | CC（UC-CC-002/003/007）、NR、NO、SA——UC-PS-002 步骤 9 明写「将版本引用交给适用下游：UC-CC-002、UC-CC-003、UC-CC-007、路由、节点或结算分别重新判断」，另有「下游交接边界」整节；CONTEXT-MAP 边 PS→CC「版本化客户原始资料」 | 应有但未开 |
| 2 | `AcceptanceDecisionHandoff` | `parcel-shipment.acceptance-decision.formed` | NR——CONTEXT-MAP 边 PS→NR「接受后……提供接受基线，网络与路由……形成初始路由」；UC-NR-001 | **已有消费者：NR `AcceptanceConsumer`（路由表唯一条目）**。注：MAP 另有 PS→NO/CC/VE 提供接受基线的边，但以事件还是查询口交付无 UC 明文，未计入缺口 |
| 3 | `NetworkIntakeHandoff` | `parcel-shipment.network-intake.recorded` | NR 复核——CONTEXT-MAP 关系约束「网络与路由据已知实际接货位置及当时有效证据复核」（NO/TF→NR 节）；UC-PS-003 步骤 8 明写经 UC-NR-003 重新校验路由 | **已有消费者：NR `NetworkIntakeConsumer`（路由表第二条，`60ea63c`）**。消费门四条同 AcceptanceConsumer，消费者名 `network-routing/reassess-on-network-intake` 与之分账；`reassess_on_network_intake.go` 按采用键四维取回记录后交 `ReassessOnIntakeAdapter` 翻译 |
| 4 | `ParcelCancellationHandoff` | `parcel-shipment.parcel-cancellation.recorded` | NR、NO、TF、CC、SA——UC-PS-006 步骤 5「向路由、节点、运输、关务、面单或结算责任方提出范围化请求，每个下游独立承接」 | 应有但未开。注：SA 的控制释放另有同步直调链（PS 适配器→`ReleasePreAcceptanceControlHandler`），事件消费与直调的分工 UC 未明文，接消费者时需先裁 |
| 5 | `FinalOutcomeHandoff` | `parcel-shipment.final-outcome.formed` | VE——CONTEXT-MAP 边 PS→VE「包裹身份谱系、客户承诺与终局结果」；SA——边 PS→SA「计费来源」 | 应有但未开 |

### network-routing（2 / 2）

| # | 端口 | 信封类型 | 谁应该消费（判据） | 现状 |
|---|---|---|---|---|
| 6 | `ReachabilityJudgmentHandoff` | `network-routing.reachability-judgment.formed` | PS——CONTEXT-MAP 边 NR→PS「返回可达性判断……参与委托接受判断」 | 应有但未开。注：PS 已有同步读口（`reachability.go` 适配器直调 NR 编排），事件半边对应 PS 的续办推进（`AdvanceAcceptanceJudgmentHandler`，今天零构造） |
| 7 | `InitialRouteHandoff` | `network-routing.initial-route.formed` | NO——边 NR→NO「当前路由、计划节点与版本化路由指令」；TF——边 NR→TF「计划履约段与时间窗口」；VE——边 NR→VE「路由版本、选择依据与计划偏离基准」 | 应有但未开。**唯一已被生产构造的发布方**（见总览的在途链说明） |

### node-operations（4 / 4）

| # | 端口 | 信封类型 | 谁应该消费（判据） | 现状 |
|---|---|---|---|---|
| 8 | `NodeIntakeHandoff` | `node-operations.node-intake.formed` | PS——边 NO→PS「节点收寄、实测与物理拆合事实」；UC-PS-003（收寄采认链） | 应有但未开；PS 消费侧 `AdoptNetworkIntakeHandler` 与 `intake_source.go` 已成型，未接线 |
| 9 | `ExecutionFactHandoff` | `node-operations.execution-fact.recorded` | CC——边 NO→CC「实测、查验协作与处置执行事实」；CONTEXT-MAP NO→CC 关系约束（UC-NO-001 节） | 应有但未开 |
| 10 | `SealedSnapshotHandoff` | `node-operations.sealed-snapshot.recorded` | TF——边 NO→TF「可交接集运单元、物理装卸和节点侧交接证据」 | 应有但未开 |
| 11 | `CollaborationAcceptanceHandoff` | `node-operations.collaboration-acceptance.decided` | CC——CONTEXT-MAP NO→CC 关系约束：节点经 UC-NO-001 独立形成承接决定，关务据此更新协作事项 | 应有但未开 |

### transport-fulfillment（9 / 9）

| # | 端口 | 信封类型 | 谁应该消费（判据） | 现状 |
|---|---|---|---|---|
| 12 | `TransportCommissionHandoff` | `transport-fulfillment.transport-commission.submitted` | SA——边 TF→SA「运输收费发生项和履约成本依据」（供应商预期成本上游，UC-SA-004 输入） | 应有但未开 |
| 13 | `RegulatoryAcceptanceHandoff` | `transport-fulfillment.regulatory-acceptance.recorded` | CC——CONTEXT-MAP CC→NO/TF 关系约束：TF 经 UC-TF-001 拥有监管运输承接，回执给关务协作链（UC-CC-008） | 应有但未开 |
| 14 | `CapacityConsumptionHandoff` | `transport-fulfillment.capacity-consumption.recorded` | NR——边 TF→NR「班次容量信号、分配结果与运输控制边界」 | 应有但未开。**口径出入**：端口注释称消费方为「装载分配链」（TF 内部），与 MAP 的 TF→NR 边不同向；本表按 MAP 判，出入记入矛盾清单第 4 条 |
| 15 | `DispositionExecutionHandoff` | `transport-fulfillment.disposition-execution.recorded` | CC——边 TF→CC 与 CC 的处置执行核对（CONTEXT.md：CC 拥有「处置执行核对判断」） | 应有但未开 |
| 16 | `EffectiveDeliveryHandoff` | `transport-fulfillment.effective-delivery.registered` | PS——边 TF→PS「场外揽收、实际承运收寄与履约事实」+ UC-PS-004（终局判断） | 应有但未开；PS 消费侧 `FormParcelFinalHandler` 与 `delivery_outcome.go` 已成型，未接线 |
| 17 | `ExceptionJourneyHandoff` | `transport-fulfillment.exception-journey.recorded` | VE——边 TF→VE「运输履约事实」 | 应有但未开 |
| 18 | `OffsitePickupHandoff` | `transport-fulfillment.offsite-pickup.formed` | PS——边 TF→PS + CONTEXT-MAP TF→PS 关系约束（场外揽收统一解释为有效网络收寄）；UC-TF-002 步骤 6A | 应有但未开；PS 消费侧 `pickup_source.go` 已成型，未接线 |
| 19 | `OffsitePickupRegistrationHandoff` | `transport-fulfillment.offsite-pickup.registered` | PS——同上（UC-PS-003 揽收源采认链，与 18 是同链两拍：登记与形成） | 应有但未开 |
| 20 | `TransportHandoverRegistrationHandoff` | `transport-fulfillment.transport-handover.registered` | NO——边 TF→NO「权威交接结果」（CONTEXT-MAP NO↔TF 约束：只有已交接转移控制）；NR——边 TF→NR「实际移动事实」触发重判 | 应有但未开 |

### customs-compliance（9 / 9）

| # | 端口 | 信封类型 | 谁应该消费（判据） | 现状 |
|---|---|---|---|---|
| 21 | `CustomsCaseHandoff` | `customs-compliance.customs-case.established` | VE——边 CC→VE「关务事实」（案件视图）；CC 申报链（同上下文） | 应有但未开 |
| 22 | `DeclarationSubmissionHandoff` | `customs-compliance.declaration-submission.formed` | 对外发送通道（监管方向，不是内部上下文）＋ CC 外部结果核对（同上下文） | 应有但未开（同上下文核对段）；对外段不构成跨上下文消费者 |
| 23 | `ExternalResultHandoff` | `customs-compliance.external-result.received` | CC 判断与核对（同上下文）；MAP 另有 CC→NO/TF「监管结果」与 CC→VE「关务事实」边 | 应有但未开 |
| 24 | `FollowUpHandoff` | `customs-compliance.follow-up.recorded` | CC 申报执行方（同上下文）；VE（生效消费，边 CC→VE） | 应有但未开 |
| 25 | `GateVerificationHandoff` | `customs-compliance.gate-verification.recorded` | NO、TF——CONTEXT-MAP CC→NO/TF 关系约束：只消费与当前对象、拟执行动作和监管边界匹配的门禁，只阻断方向性动作 | 应有但未开 |
| 26 | `ManifestHandoff` | `customs-compliance.manifest.recorded` | CC 案件视图与申报链（同上下文，UC-CC-012 接受外部舱单引用后本上下文自消费） | 应有但未开 |
| 27 | `RestrictionHandoff` | `customs-compliance.regulatory-restriction.changed` | NO、TF（门禁执行方）——边 CC→NO/CC→TF；PS——边 CC→PS「关务限制及解除结果」＋「各限制源→PS」约束（新交易/重开资格）；NR——边 CC→NR「合规候选口岸、申报路径与限制」 | 应有但未开（本表覆盖上下文最多的一类） |
| 28 | `VerificationHandoff` | `customs-compliance.disposition-verification.recorded` | CC 案件关闭核对（同上下文）；VE 处置协调（边 CC→VE，UC-VE-001） | 应有但未开 |
| 29 | `CaseClosureHandoff` | `customs-compliance.case-closure.recorded` | VE 案件视图（边 CC→VE）；「治理审计」段无文档判据（见矛盾清单第 5 条） | 应有但未开 |

### settlement-accounting（7 适配器 / 14 类型）

SA 的事件大半供**自己上下文的下一段编排**（UC-SA-003 对账单纳入、UC-SA-004 审核、UC-SA-005 核销）消费——UC-SA-003 明写费用明细来自 UC-SA-002、金额调整来自 UC-SA-001/002/004/007「等金额创建用例」，只纳入已达确认条件者。

| # | 端口 | 信封类型 | 谁应该消费（判据） | 现状 |
|---|---|---|---|---|
| 30 | `SupplierBillHandoff` | `settlement-accounting.supplier-bill.received` | SA 审核与对账（UC-SA-004，同上下文） | 应有但未开（同上下文） |
| 31 | `ChargeConfirmationHandoff` | `settlement-accounting.charge-confirmation.formed` | SA 对账单纳入（UC-SA-003 费用明细源，同上下文） | 应有但未开（同上下文） |
| 32 | `AdvanceRecoveryHandoff` | `settlement-accounting.advance-recovery.formed`、`settlement-accounting.recovery-adjustment.formed` | SA 对账单纳入（UC-SA-003 引 UC-SA-001 金额，同上下文） | 应有但未开（同上下文） |
| 33 | `ClaimSettlementHandoff` | `settlement-accounting.claim-amount.formed`、`recovery-receivable.formed`、`recovery-acknowledgement.formed`、`claim-adjustment.formed` | SA 对账单纳入与追偿链（UC-SA-007、UC-SA-003，同上下文）。MAP 约束 VE→SA 单向、「不把金额结果回写为异常事实」——**不得**给 VE 开消费者 | 应有但未开（同上下文） |
| 34 | `StatementHandoff` | `settlement-accounting.statement.published`、`statement.voided`、`statement.included` | SA 外部资金核销目标索引（UC-SA-005，同上下文）；客户通知段对外（消息能力） | 混合：同上下文消费者未开；对外段不构成跨上下文消费者 |
| 35 | `SettlementApplicationHandoff` | `settlement-accounting.settlement-application.applied` | SA 对账单与应付的已结视图（UC-SA-005 后段，同上下文） | 应有但未开（同上下文） |
| 36 | `OperatingHandoff` | `settlement-accounting.cost-allocation.formed`、`operating-result.derived` | 分析/报表消费（UC-SA-006）。CONTEXT-MAP 无 SA 向任何内部上下文供指标的边，经营指标是 SA 终点 | **本就不应该有跨上下文消费者**（对外分析口） |

### visibility-exception（8 / 8）

VE 链内事件（37–41）按 CONTEXT.md「拥有：……投影、ETA、缺口、信号、分诊」，消费者是本上下文下一段派生。

| # | 端口 | 信封类型 | 谁应该消费（判据） | 现状 |
|---|---|---|---|---|
| 37 | `ProjectionHandoff` | `visibility-exception.tracking-projection.derived` | VE 客户视图派生（同上下文，`DeriveCustomerViewHandler`） | 应有但未开（同上下文） |
| 38 | `CustomerViewHandoff` | `visibility-exception.customer-view.published` | VE 通知判断（同上下文）；门户展示段对外 | 混合：同上下文消费者未开；对外段不构成跨上下文消费者 |
| 39 | `ETAHandoff` | `visibility-exception.eta-prediction.formed` | VE 客户视图重派生（同上下文；CONTEXT 语义「ETA 新版本必须重新派生客户视图」） | 应有但未开（同上下文） |
| 40 | `VisibilityGapHandoff` | `visibility-exception.visibility-gap.formed` | VE 信号链分诊（同上下文） | 应有但未开（同上下文） |
| 41 | `TriageHandoff` | `visibility-exception.signal-triage.concluded` | VE 发作期/案件链（同上下文） | 应有但未开（同上下文） |
| 42 | `NotificationHandoff` | `visibility-exception.customer-notification.formed` | VE 义务台账与升级判断（同上下文）；消息能力段对外（MAP「VE↔消息能力」约束） | 混合：同上下文消费者未开；对外段不构成跨上下文消费者 |
| 43 | `DispositionHandoff` | `visibility-exception.disposition-request.sent` | NR、NO、TF、CC——CONTEXT-MAP 边 VE→四上下文「异常处置请求；不直接改写源事实」＋约束「发送、接受和实际完成分别记录」 | 应有但未开 |
| 44 | `LiabilityHandoff` | `visibility-exception.claim-liability.concluded` | SA——边 VE→SA「责任、外部响应与索赔追偿依据」；UC-SA-007 赔付金额链上游 | 应有但未开 |

### parcel-pricing（1 / 1）

| # | 端口 | 信封类型 | 谁应该消费（判据） | 现状 |
|---|---|---|---|---|
| 45 | `EvaluationHandoff` | `parcel-pricing.evaluation.recorded` | SA——边 PP→SA「不可变评价、版本清单与解释」；NR——边 PP→NR（BUY 候选评价并入择优） | 应有但未开 |

### pilot-governance（1 适配器 / 3 类型）

| # | 端口 | 信封类型 | 谁应该消费（判据） | 现状 |
|---|---|---|---|---|
| 46 | `GovernanceHandoff` | `pilot-governance.suspension.recorded`、`resumption.recorded`、`takeover.recorded` | **说不清**——pilot-governance 不在 CONTEXT-MAP（十个上下文里没有它）、无 `docs/domain/pilot-governance/CONTEXT.md`、无 UC-PG-*。端口注释称「受影响上下文的准入闸消费」，但那不是本表许可的判据 | 说不清（缺 pilot-governance 的 CONTEXT.md 或 CONTEXT-MAP 条目；全库仅产品基线提及该词） |

## 路由表核对（`60ea63c`）

- 第一条 `acceptance-decision.formed → AcceptanceConsumer` 与 CONTEXT-MAP 边 PS→NR、UC-NR-001 一致，**无冲突**。
- 第二条 `network-intake.recorded → NetworkIntakeConsumer` 与 UC-PS-003 步骤 8、UC-NR-003 一致，**无冲突**。
- 未决哨兵翻译逐条对应（第一条包 `ErrRouteHandoffUndecided`，第二条包 `ErrReassessmentUndecided`）：分设而不合用，运维据失败码分得出两条链各等哪个依赖。
- 其余 53 类未登记本身不是冲突：ADR-0049 第三条明写登记接不住的类型比不登记更糟。缺口在消费者侧（本表第 4 栏），不在路由表侧。
- 唯一的时间性风险是总览所述在途链：已接线的 NR 生产方成功即产出无订阅者类型。

## 矛盾与陈旧口径（只记不修）

1. **十处端口注释称「今天没有实现，唯一实现是测试替身」，但 Outbox 适配器已存在**：PS `SourceDataVersionHandoff`、PS `AcceptanceDecisionHandoff`、CC `ExternalResultHandoff`、SA 全部七口（`SupplierBill`/`ChargeConfirmation`/`AdvanceRecovery`/`Statement`/`SettlementApplication`/`Operating`/`ClaimSettlement`）。注释归因于 ADR-0017 的 Bento/Outbox 闸门——该闸门解除、持久化批落地后注释未回写。
2. **pilot-governance 无领域文档**：见第 46 行。三类治理事件的消费方向在权威文档层面不可判。
3. **collection-remittance 反向缺口**：CONTEXT-MAP 有 SA→CR「COD 服务费和合法抵扣金额」与 TF→CR「履约侧代收证据」两条边，但 `internal/` 下无 collection-remittance 实现，也没有任何指向它的 handoff——是「有边无发布面」，与本表「有发布面无消费者」相反的一类，列此备档。
4. **CapacityConsumption 口径出入**：端口注释称消费方为「装载分配链」（TF 内），CONTEXT-MAP 边为 TF→NR「班次容量信号、分配结果」。本表按 MAP 判 NR；接消费者前需 TF 所有者裁定。
5. **CaseClosure 的「治理审计」消费段**查无文档出处（CONTEXT-MAP/CC CONTEXT.md/UC 均无此语），仅端口注释有；若确指 pilot-governance，则并入第 2 条的缺档。

## 附带 a：「领域件在位、无任何生产调用」入口清单

**原票三例重核（HEAD `4d57ecd`）——维持成立**：`RegisterValidityCorrection` / `RegisterPricePolicy` / `RegisterSettlementPolicy`（`partycommercial/domain/commercial_registry.go`）在非测试代码中零调用；`commercial_authority.go` 注释自认「这三册今天都还没有持久化面——本仓也没有任何生产代码调用」，与 grep 结果一致（仅域内声明、测试与该注释命中）。

**全量扫描口径**：58 个应用层处理器（`internal/*/application` 的 `New*Handler`），查非测试代码中 application 包之外的引用。三档：

- **被组合根实际构造：2 个**（`60ea63c`）——`CreateInitialRouteHandler` 与 `ReassessRouteHandler`（同在 `cmd/parcel-dispatch/assemble.go`）。后者只写不发：它没有 handoff 端口，因此进组合根不改变「生产方」那一侧的计数。
- **被跨上下文适配器按具体类型引用（适配器自身未接入 cmd）：10 个**——`AdjudicateCommercialAuthorization`、`AdoptNetworkIntake`、`ApplyPreAcceptanceControl`、`AssessParcelReachability`、`FormJudgmentAsOf`、`FormParcelFinal`、`ReleasePreAcceptanceControl`、`ResolveCommercialBasis`、`ValidateCommercialBasis`、`ValidateReachabilityJudgment`。
- **零非测试引用：46 个**，其中再分两档：
  - **有 `adapters/http` 端点按自声明接口引用、等 `PAR-INT-01` Intake（endpoints.go 固化的装配缝，非缺口）：6 个**——`SubmitShipmentRequest`、`WithdrawShipmentRequest`（PS）、`ReceiveDeliveredUnit`（NO）、`RegisterEffectiveDelivery`（TF）、`HandleClaim`（VE receive_claim）、`ReceiveExternalResult`（CC）。（VE 的 `query_customer_tracking_view.go` 是查询端点，不构造派生处理器。）
  - **连消费面都没有：40 个**，按上下文——PS：`AdvanceAcceptanceJudgment`、`AdvanceFinancialControlJudgment`、`AmendCustomerSourceData`、`CancelParcel`、`FormAcceptanceDecision`、`FormNewSubmissionVersion`、`RejectShipmentRequest`；NO：`AcceptCollaboration`、`ConsolidateParcels`；TF：`AcceptRegulatoryDisposition`、`CommissionTransport`、`PerformOffsitePickup`、`PrepareTransportOpportunity`、`RegisterOffsitePickup`、`RegisterTransportHandover`、`StartAlternateJourney`；CC：`CloseCustomsCase`、`EstablishCase`、`ManageFollowUp`、`ManageRestriction`、`ReceiveManifest`、`SubmitDeclaration`、`VerifyDisposition`、`VerifyReleaseGate`；SA：`AllocateCosts`、`AssessAdvanceRecovery`、`ConfirmCharge`、`CutOffPublishStatement`、`MapExternalFunds`、`ReceiveSupplierBill`、`SettleClaimAmounts`；VE：`DeriveCustomerView`、`DeriveProjection`、`FormETA`、`NotifyCustomer`、`RaiseSignal`、`SendDispositionRequest`；PP：`EvaluatePricing`；PG：`GovernIncident`、`RecordStageReview`。

**两端关系（原票的猜测成立）**：55 类事件里 53 类无消费者，58 个处理器里 56 个无生产构造（`60ea63c`；清点时为 54 与 57）——同一条「装配纵深缺失」的两个测量面。多数「应有但未开」的消费者，其对应处理器就在上面 40 个零面清单里（如 initial-route 的消费者要做的事对应 NO/TF 的接收编排，final-outcome 的消费者对应 VE 投影链的 `DeriveProjection`）；填路由表格子与给处理器接生产调用，多数格子是同一笔工作。

## 附带 b：party_commercial 迁移号现状（HEAD `4d57ecd` + 工作树）

| 文件 | 字节 | 状态 |
|---|---|---|
| 0001_commercial_publication.sql | 2310 | 已入库 |
| 0002_commercial_resolution.sql | 1424 | 已入库 |
| 0003_authorization_grant.sql | 2726 | 已入库，**不空** |
| 0004_commercial_version_kind_range.sql | 1545 | 已入库 |
| 0005_as_of_policy_declaration.sql | 2763 | 已入库 |
| 0006_acceptance_content_declaration.sql | 4965 | 已入库 |
| 0007_pre_acceptance_control_declaration.sql | 2983 | **在途草稿**（未跟踪，归 MCP-2 接手，勿动） |

`migrations/migrations.go` 以 `//go:embed all:party_commercial` 整目录嵌入：0007 随提交自动带入，**无需改接线文件**（「同笔提交」纪律只约束新模块目录）。

## 保质期声明

本文「现状」断言的取证点分两批：主表判据栏与首轮清点取证于 HEAD `4d57ecd` 的提交内容＋当时工作树；**第 4 栏「已有消费者」、总览计数、路由表核对与附带 a 的三档构造清单已按 `60ea63c` 重取证**（第 3 行由「应有但未开」改为已有消费者，随之改动的计数在各处标了 `60ea63c`）。

共享树上此类断言有保质期。消费本表前若 HEAD 已前进，上述两类结论应按新 HEAD 再取一次证；判据栏引用的文档边（CONTEXT-MAP、各 `CONTEXT.md`、`UC-*`）不随代码变化，不受影响。
