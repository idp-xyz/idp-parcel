# 01 余下各口的信封 ID 仍原样串接实例引用：标识一长就入不了队，交接只留没人重发的续办引用

Category: bug
Status: ready-for-agent——2026-10-18 Claude Code 会话立（用户「你作为业务和系统专家自决」授权），出自 [routing-first-cut/22](../../routing-first-cut/issues/22-initial-route-envelope-id-exceeds-the-event-id-limit.md) 的取证。
Blocked by: 无
出处：`outboxintent.FingerprintEventID` 头注（票 sa-cc/34 裁决 2）：信封 ID 上限是 `eventing.MaxEventIDLength`，租户、委托、包裹这类引用多长归实例半边，本仓给不出上界，原样串接就是把上限押在别人的长度上。

## 现状（读码，钉 `45b9a088`）

关务各口与结算部分口已改指纹形，初始路由随 routing-first-cut/22 改。下列构造函数仍原样串接：

- node-operations：`collaborationAcceptanceEventID`、`executionFactEventID`、`nodeIntakeEventID`、`sealedSnapshotEventID`
- parcel-shipment：`continuedAttemptDecisionEventID`、`finalOutcomeEventID`、`labelTransactionJudgmentEventID`、`manualReviewCompletedEventID`、`networkIntakeEventID`、`parcelCancellationEventID`、`shipmentRequestSubmittedEventID`、`submissionVersionFormedEventID`
- party-commercial：`operatorRegistrationCompletedEventID`
- pilot-governance：`resumptionEventID`、`suspensionEventID`、`takeoverEventID`
- settlement-accounting：`chargeConfirmationEventID`、`supplierBillEventID`
- transport-fulfillment：`capacityConsumptionEventID`、`carrierFirstEffectivePickupEventID`、`dispositionExecutionEventID`、`effectiveDeliveryEventID`、`exceptionJourneyEventID`、`externalTrackingFactEventID`、`offsitePickupEventID`、`offsitePickupRegistrationEventID`、`regulatoryAcceptanceEventID`、`transportCommissionEventID`、`transportHandoverRegistrationEventID`
- visibility-exception：`triageEventID`、`visibilityGapEventID`

不经具名函数、在信封构造处原地串接的（全在 settlement-accounting 的 postgres 适配器里）：

- `operating_handoff.go`：分摊、经营结果两种信封
- `claim_settlement_handoff.go`：索赔金额、应收、确认、索赔调整
- `advance_recovery_handoff.go`：调整、追偿
- `statement_handoff.go`：纳入；另一处 ID 直接取分区键
- `supplier_bill_handoff.go`：应付、贷项（同文件只有一种形状走 `supplierBillEventID`）
- `settlement_application_handoff.go`：ID 直接取分区键
- `evaluation_request_handoff.go`：分区键加 `/submitted`

取证方法：按 `eventing.Envelope` 字面量里 `ID:` 那一格的表达式扫，不只扫具名的 `…EventID` 函数。

演示数据下哪些已越过上限，本票没逐个核。初始路由那一口是实测越过的。

## 做什么

- 逐口改走 `FingerprintEventID`：口名前缀加该口认领键的全部维。分区键不动；原先与 ID 共用一串的，分区键取那一串、一字不变（先例 `customsCasePartitionKey`）。原先就有独立分区键的口（如 `carrierFirstEffectivePickupPartitionKey`、`takeoverPartitionKey`）照旧。
- 每口补一条用真实长度标识的入队用例（先例 `TestAnInitialRouteWithFullLengthIdentifiersStillEnqueues`）。
- 要定的一格：交接因信封校验被拒是确定性失败，重发同一份照样被拒。今天它只化成一条续办引用，不报日志。要不要在交接口把这类失败另作一格响亮报出来，由本票定。

## 不做

- 不改分区键的语义，不改载荷形状。
