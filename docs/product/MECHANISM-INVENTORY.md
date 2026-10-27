# 机制半边清点（生成物，勿手改）

由 `tools/mechanism-inventory` 生成。本文只有数，没有定级——「达标／部分／未开始」与留待裁定在开发主线正文里。

## 逐上下文文件面

| 上下文 | 生产 | 测试 | 应用编排 | postgres 适配器 | 其中 Outbox 投递 | http 适配器 |
|---|---|---|---|---|---|---|
| accessidentity | 39 | 46 | 0 | 6 | 0 | 2 |
| architecture（非业务） | 0 | 17 | 0 | 0 | 0 | 0 |
| collectionremittance | 24 | 12 | 4 | 6 | 0 | 3 |
| customscompliance | 126 | 130 | 24 | 47 | 10 | 18 |
| networkrouting | 94 | 108 | 11 | 19 | 2 | 10 |
| nodeoperations | 111 | 119 | 15 | 29 | 4 | 17 |
| parcelpricing | 147 | 135 | 20 | 20 | 1 | 25 |
| parcelshipment | 241 | 242 | 24 | 45 | 10 | 25 |
| partycommercial | 173 | 193 | 15 | 43 | 1 | 40 |
| pilotgovernance | 23 | 19 | 5 | 6 | 1 | 4 |
| platform（非业务） | 26 | 24 | 0 | 0 | 0 | 0 |
| settlementaccounting | 233 | 185 | 38 | 69 | 13 | 10 |
| transportfulfillment | 289 | 304 | 52 | 61 | 16 | 50 |
| visibilityexception | 121 | 122 | 12 | 31 | 8 | 13 |
| **合计** | 1647 | 1656 | 220 | 382 | 66 | 217 |

业务上下文 12 个，非业务目录 2 个。`cmd/` 生产 230、测试 384。

## 跨上下文消费缝：50 组，164 个生产文件

| 消费方 | 提供方 | 文件 |
|---|---|---|
| accessidentity | pilotgovernance | 1 |
| collectionremittance | accessidentity | 1 |
| customscompliance | accessidentity | 4 |
| customscompliance | parcelshipment | 1 |
| customscompliance | settlementaccounting | 2 |
| networkrouting | accessidentity | 3 |
| networkrouting | customscompliance | 1 |
| networkrouting | parcelpricing | 3 |
| networkrouting | parcelshipment | 4 |
| networkrouting | partycommercial | 4 |
| networkrouting | transportfulfillment | 1 |
| nodeoperations | accessidentity | 6 |
| nodeoperations | networkrouting | 1 |
| nodeoperations | parcelshipment | 5 |
| nodeoperations | partycommercial | 1 |
| nodeoperations | transportfulfillment | 1 |
| parcelpricing | accessidentity | 2 |
| parcelpricing | parcelshipment | 1 |
| parcelpricing | settlementaccounting | 4 |
| parcelpricing | transportfulfillment | 1 |
| parcelshipment | accessidentity | 5 |
| parcelshipment | customscompliance | 1 |
| parcelshipment | networkrouting | 1 |
| parcelshipment | nodeoperations | 5 |
| parcelshipment | parcelpricing | 2 |
| parcelshipment | partycommercial | 26 |
| parcelshipment | pilotgovernance | 2 |
| parcelshipment | settlementaccounting | 4 |
| parcelshipment | transportfulfillment | 7 |
| partycommercial | accessidentity | 2 |
| pilotgovernance | accessidentity | 1 |
| settlementaccounting | accessidentity | 3 |
| settlementaccounting | customscompliance | 2 |
| settlementaccounting | parcelpricing | 7 |
| settlementaccounting | parcelshipment | 2 |
| settlementaccounting | partycommercial | 5 |
| settlementaccounting | transportfulfillment | 2 |
| transportfulfillment | accessidentity | 5 |
| transportfulfillment | customscompliance | 3 |
| transportfulfillment | networkrouting | 2 |
| transportfulfillment | nodeoperations | 1 |
| transportfulfillment | parcelshipment | 2 |
| transportfulfillment | partycommercial | 3 |
| visibilityexception | accessidentity | 4 |
| visibilityexception | customscompliance | 3 |
| visibilityexception | networkrouting | 1 |
| visibilityexception | nodeoperations | 2 |
| visibilityexception | parcelshipment | 4 |
| visibilityexception | partycommercial | 1 |
| visibilityexception | transportfulfillment | 9 |

## 迁移：12 个模块共 323 份 SQL

| 模块 | 份数 |
|---|---|
| access_identity | 24 |
| collection_remittance | 2 |
| customs_compliance | 27 |
| network_routing | 31 |
| node_operations | 20 |
| parcel_pricing | 14 |
| parcel_shipment | 30 |
| party_commercial | 42 |
| pilot_governance | 8 |
| settlement_accounting | 46 |
| transport_fulfillment | 49 |
| visibility_exception | 30 |

## 接线面：接入面端点 194 个，消费适配器 57 个生产文件，直投路由表 40 条

接入面端点按 `cmd/` 生产文件里 `[]httpapi.BusinessEndpoint` 字面量的条目数，按端点构造函数所在的 `internal/<上下文>/adapters/http` 归属；不按 `adapters/http/` 的文件数——一个处理器可挂多个端点。

| 上下文 | 端点 |
|---|---|
| accessidentity | 2 |
| collectionremittance | 1 |
| customscompliance | 21 |
| networkrouting | 13 |
| nodeoperations | 17 |
| parcelpricing | 20 |
| parcelshipment | 15 |
| partycommercial | 37 |
| pilotgovernance | 1 |
| settlementaccounting | 8 |
| transportfulfillment | 44 |
| visibilityexception | 15 |
| **合计** | 194 |

消费适配器按 `internal/<消费方>/adapters/` 下 `inbox`、`adoptconsume`、`finalconsume`、`veconsume` 四类目录的生产文件数。它与上面的「跨上下文消费缝」是两种东西：那一栏数的是消费方为某个提供方写的防腐层，这一栏数的是接进程内直投信封的消费门。

| 消费方 | inbox | adoptconsume | finalconsume | veconsume | 合计 |
|---|---|---|---|---|---|
| customscompliance | 1 | 0 | 0 | 0 | 1 |
| networkrouting | 3 | 0 | 0 | 0 | 3 |
| nodeoperations | 5 | 0 | 0 | 0 | 5 |
| parcelpricing | 2 | 0 | 0 | 0 | 2 |
| parcelshipment | 13 | 1 | 1 | 0 | 15 |
| settlementaccounting | 8 | 0 | 0 | 0 | 8 |
| transportfulfillment | 5 | 0 | 0 | 0 | 5 |
| visibilityexception | 16 | 0 | 0 | 2 | 18 |
| **合计** | 53 | 1 | 1 | 2 | 57 |

直投路由表按 `cmd/` 生产文件里登记的不同事件类型数（`routeLeg` 字面量的 `event` 与 `map[eventing.EventType]dispatch.Consumer` 字面量的键并认，同一事件类型扇出给几路只算一条），按事件类型常量所属的消费门包归属。路由表只随消费者一起长（ADR-0049 第三条），本表只报它此刻多长。

| 事件类型所属消费方 | 条目 |
|---|---|
| customscompliance | 1 |
| networkrouting | 2 |
| nodeoperations | 4 |
| parcelpricing | 2 |
| parcelshipment | 12 |
| settlementaccounting | 4 |
| transportfulfillment | 2 |
| visibilityexception | 12 |
| （未归类） | 1 |
| **合计** | 40 |

## 端口：声明 610 个；基线口径缺 39，精确口径缺 2

基线口径缺（名字未在任何适配器/平台生产文件出现）：

- `customscompliance.RestrictionScopeView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres.Restrictions）
- `networkrouting.InitialRouteEvidenceView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/networkrouting/application.CatalogInitialRouteEvidence）
- `networkrouting.NetworkEvidenceView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/networkrouting/application.CatalogNetworkEvidence）
- `parcelpricing.PriceCardApprovalDutyRuleView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres.PriceCardApprovalDutyRules）
- `parcelpricing.ReferenceReviewDutyRuleView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres.ReferenceReviewDutyRules）
- `parcelshipment.ContinuedAttemptRegisterView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres.ContinuedAttemptRegisters）
- `parcelshipment.LabelChannelGateway` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/cmd/parcel-api.unconfiguredLabelChannelGateway）
- `parcelshipment.ResponsibilityStartView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres.IntakeAdoptions）
- `partycommercial.ApprovalDutyRuleView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres.ApprovalDutyRules）
- `partycommercial.ServiceProductFormRegistry` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres.CommercialPublications）
- `settlementaccounting.AmountGrammarRegister` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres.AmountGrammars）
- `settlementaccounting.AmountGrammarView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres.AmountGrammars）
- `settlementaccounting.AuditEscalationCeilingRegister` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres.AuditEscalationCeilings）
- `settlementaccounting.AuditEscalationCeilingView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres.AuditEscalationCeilings）
- `settlementaccounting.ChargeAttributionDateRecorder` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres.CustomerCharges）
- `settlementaccounting.ChargeConfirmationFactRegister` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres.SettlementCatalogues）
- `settlementaccounting.ClaimAmountRuleRegister` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres.SettlementCatalogues）
- `settlementaccounting.ClaimAmountRuleView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres.SettlementCatalogues）
- `settlementaccounting.ConfirmedChargeFactsView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres.SettlementCatalogues）
- `settlementaccounting.EstimatedCustomerChargeStore` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres.CustomerCharges）
- `settlementaccounting.PeriodicFeeRegister` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres.PeriodicFees）
- `settlementaccounting.PeriodicFeeView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres.PeriodicFees）
- `settlementaccounting.SellEvaluationTriggerRegister` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres.SellEvaluationTriggers）
- `settlementaccounting.SellEvaluationTriggerView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres.SellEvaluationTriggers）
- `settlementaccounting.SettlementAccountRegister` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres.SettlementAccounts）
- `settlementaccounting.SupplierAuditAuthorityRegister` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres.SettlementCatalogues）
- `settlementaccounting.SupplierAuditAuthorityView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres.SettlementCatalogues）
- `settlementaccounting.SupplierPayableAccountRegister` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres.SettlementCatalogues）
- `settlementaccounting.SupplierPayableAccountView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres.SettlementCatalogues）
- `transportfulfillment.ActualCarrierJudgmentView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres.ActualCarrierJudgments）
- `transportfulfillment.BookingAnswerView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres.BookingAnswers）
- `transportfulfillment.BookingView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres.BookingRequests）
- `transportfulfillment.DepartureSegmentView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres.FulfillmentSegments）
- `transportfulfillment.FailedAttemptSource` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres.PickupAttempts）
- `transportfulfillment.FulfillmentSegmentView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres.FulfillmentSegments）
- `transportfulfillment.HandoverGatePredecessorView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres.TransportHandovers）
- `transportfulfillment.SegmentCommissionAdoptionView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres.SegmentCommissionAdoptions）
- `transportfulfillment.TransportCommissionView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres.TransportCommissions）
- `visibilityexception.NotificationChannelGateway` 

精确口径缺（无具体类型完整实现）：

- `transportfulfillment.TrackingSource` （虚高：名字出现过，但无人实现）
- `visibilityexception.NotificationChannelGateway` 
