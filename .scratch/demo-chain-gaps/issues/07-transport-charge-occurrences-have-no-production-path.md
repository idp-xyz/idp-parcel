# 07 运输收费发生项没有形成路径，也没有信封

Category: enhancement
Status: needs-triage——2026-10-18 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自同日对演示动线余下各段的只读勘察（隔离子代理，证据 `S`）；下列现象与证据取自勘察结果，本票立时没有逐条复核，接票时先核。
Blocked by: 无
出处：[product-strategy-boundary/05](../../product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md) 2026-10-18 那次重走。

## 现象

【硬停】运输收费发生项没有形成路径，也没有信封：结算无从消费运输履约的成本来源

## 证据（读码）

领域 `ChargeOccurrenceReason` 四格（BOOKING/CANCELLATION/FAILED_ATTEMPT/ACTUAL_FULFILLMENT）里，只有失败尝试有构造路（`domain.ChargeOccurrenceForFailedAttempt`）。`NewRegisterFailedAttemptChargeHandler` 在 cmd/ 零引用，而且它的 `ports.FailedAttemptSource` 只读揽收尝试（PickupAttemptKey），失败派送尝试（AT-TF-095）也没有路。全仓 `Occurrences.Save` 只在 register_failed_attempt_charge.go 出现一次。全仓没有 `transport-fulfillment.*charge*` 事件类型，`ports.ChargeOccurrenceRegistry` 也没有交接口。`domain.ActualFulfillmentSegment` 不带供应商协议/履约条件快照，采购法人、服务方、协议这几格无处可取。PC 种子里的供应商协议正文只有 supplier/legalEntity/scope/purchasePlan，没有发生条件。下游已经在等：parcel-dispatch 用 `tfpostgres.NewChargeOccurrences` 给 `pptransport.NewResolver` 读成员，SA 的 `TriggerBuyEvaluationHandler` 需要调用方给发生项（见 `buildEvaluationRequestOrchestration` 注释）。

## 三类

机制与产品策略为主，另有租户取值。规模 L。

## 最小的诚实修法

机制：新增「实际履约发生项」形成用例——对象在段内的参与因有效交付或下一次交接结束、且该段关联了采用的委托（依赖停点 2）时，按协议形成 ACTUAL_FULFILLMENT；自营照 ADR-0098 决定三答「不适用」。委托/订舱编排形成 BOOKING/CANCELLATION；失败尝试用例扩到派送尝试并接上装配；发生项登记同笔入队一个新信封（如 `transport-fulfillment.charge-occurrence.formed`），供 psb/20 的触发消费。产品策略：「哪些事实满足协议的发生条件」做成内置形态（按段/按件/按尝试）。租户取值：演示两份供应商协议各选一种形态。

## 要裁的

发生条件挂在参与方与商业侧的供应商协议正文上，还是挂在运输履约的采用登记上（需 PC 与 TF 两边 owner）；发生项与段/委托的关联键；psb/20 选「发生项形成即发起」时信封带什么。

## 既有票

none（运输履约侧）；结算消费侧是 psb/20
