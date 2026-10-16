# 机制半边清点（生成物，勿手改）

由 `tools/mechanism-inventory` 生成。本文只有数，没有定级——「达标／部分／未开始」与留待裁定在开发主线正文里。

## 逐上下文文件面

| 上下文 | 生产 | 测试 | 应用编排 | postgres 适配器 | 其中 Outbox 投递 | http 适配器 |
|---|---|---|---|---|---|---|
| accessidentity | 29 | 22 | 0 | 5 | 0 | 1 |
| architecture（非业务） | 0 | 13 | 0 | 0 | 0 | 0 |
| collectionremittance | 22 | 10 | 4 | 6 | 0 | 3 |
| customscompliance | 106 | 105 | 20 | 42 | 10 | 15 |
| networkrouting | 74 | 72 | 9 | 13 | 2 | 7 |
| nodeoperations | 36 | 29 | 4 | 10 | 4 | 7 |
| parcelpricing | 130 | 117 | 17 | 16 | 1 | 23 |
| parcelshipment | 200 | 193 | 20 | 36 | 10 | 18 |
| partycommercial | 159 | 169 | 13 | 40 | 1 | 38 |
| pilotgovernance | 22 | 20 | 5 | 6 | 1 | 4 |
| platform（非业务） | 25 | 23 | 0 | 0 | 0 | 0 |
| settlementaccounting | 178 | 124 | 29 | 59 | 11 | 9 |
| transportfulfillment | 153 | 139 | 27 | 36 | 11 | 29 |
| visibilityexception | 100 | 97 | 11 | 30 | 8 | 11 |
| **合计** | 1234 | 1133 | 159 | 299 | 59 | 165 |

业务上下文 12 个，非业务目录 2 个。`cmd/` 生产 80、测试 130。

## 跨上下文消费缝：40 组，104 个生产文件

| 消费方 | 提供方 | 文件 |
|---|---|---|
| accessidentity | pilotgovernance | 1 |
| customscompliance | accessidentity | 3 |
| customscompliance | settlementaccounting | 2 |
| networkrouting | accessidentity | 2 |
| networkrouting | customscompliance | 1 |
| networkrouting | parcelpricing | 2 |
| networkrouting | parcelshipment | 3 |
| networkrouting | partycommercial | 4 |
| nodeoperations | accessidentity | 1 |
| nodeoperations | parcelshipment | 1 |
| nodeoperations | transportfulfillment | 1 |
| parcelpricing | accessidentity | 2 |
| parcelpricing | parcelshipment | 1 |
| parcelpricing | settlementaccounting | 2 |
| parcelpricing | transportfulfillment | 1 |
| parcelshipment | accessidentity | 1 |
| parcelshipment | customscompliance | 1 |
| parcelshipment | networkrouting | 1 |
| parcelshipment | nodeoperations | 4 |
| parcelshipment | parcelpricing | 2 |
| parcelshipment | partycommercial | 23 |
| parcelshipment | pilotgovernance | 2 |
| parcelshipment | settlementaccounting | 3 |
| parcelshipment | transportfulfillment | 5 |
| partycommercial | accessidentity | 2 |
| settlementaccounting | accessidentity | 1 |
| settlementaccounting | customscompliance | 2 |
| settlementaccounting | parcelpricing | 5 |
| settlementaccounting | partycommercial | 3 |
| transportfulfillment | accessidentity | 3 |
| transportfulfillment | networkrouting | 1 |
| transportfulfillment | parcelshipment | 1 |
| transportfulfillment | partycommercial | 2 |
| visibilityexception | accessidentity | 1 |
| visibilityexception | customscompliance | 2 |
| visibilityexception | networkrouting | 1 |
| visibilityexception | nodeoperations | 1 |
| visibilityexception | parcelshipment | 4 |
| visibilityexception | partycommercial | 1 |
| visibilityexception | transportfulfillment | 5 |

## 迁移：12 个模块共 213 份 SQL

| 模块 | 份数 |
|---|---|
| access_identity | 6 |
| collection_remittance | 1 |
| customs_compliance | 23 |
| network_routing | 18 |
| node_operations | 4 |
| parcel_pricing | 12 |
| parcel_shipment | 23 |
| party_commercial | 37 |
| pilot_governance | 8 |
| settlement_accounting | 35 |
| transport_fulfillment | 20 |
| visibility_exception | 26 |

## 接线面：接入面端点 140 个，消费适配器 35 个生产文件，直投路由表 26 条

接入面端点按 `cmd/` 生产文件里 `[]httpapi.BusinessEndpoint` 字面量的条目数，按端点构造函数所在的 `internal/<上下文>/adapters/http` 归属；不按 `adapters/http/` 的文件数——一个处理器可挂多个端点。

| 上下文 | 端点 |
|---|---|
| accessidentity | 1 |
| collectionremittance | 1 |
| customscompliance | 16 |
| networkrouting | 9 |
| nodeoperations | 2 |
| parcelpricing | 17 |
| parcelshipment | 15 |
| partycommercial | 32 |
| pilotgovernance | 1 |
| settlementaccounting | 6 |
| transportfulfillment | 25 |
| visibilityexception | 15 |
| **合计** | 140 |

消费适配器按 `internal/<消费方>/adapters/` 下 `inbox`、`adoptconsume`、`finalconsume`、`veconsume` 四类目录的生产文件数。它与上面的「跨上下文消费缝」是两种东西：那一栏数的是消费方为某个提供方写的防腐层，这一栏数的是接进程内直投信封的消费门。

| 消费方 | inbox | adoptconsume | finalconsume | veconsume | 合计 |
|---|---|---|---|---|---|
| customscompliance | 1 | 0 | 0 | 0 | 1 |
| networkrouting | 2 | 0 | 0 | 0 | 2 |
| parcelpricing | 1 | 0 | 0 | 0 | 1 |
| parcelshipment | 13 | 1 | 1 | 0 | 15 |
| settlementaccounting | 2 | 0 | 0 | 0 | 2 |
| visibilityexception | 12 | 0 | 0 | 2 | 14 |
| **合计** | 31 | 1 | 1 | 2 | 35 |

直投路由表按 `cmd/` 生产文件里 `map[eventing.EventType]dispatch.Consumer` 字面量的条目数，按条目键（事件类型常量）所属的消费门包归属。路由表只随消费者一起长（ADR-0049 第三条），本表只报它此刻多长。

| 事件类型所属消费方 | 条目 |
|---|---|
| customscompliance | 1 |
| networkrouting | 2 |
| parcelpricing | 1 |
| parcelshipment | 12 |
| settlementaccounting | 2 |
| visibilityexception | 8 |
| **合计** | 26 |

## 端口：声明 473 个；基线口径缺 29，精确口径缺 2

基线口径缺（名字未在任何适配器/平台生产文件出现）：

- `networkrouting.InitialRouteEvidenceView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/networkrouting/application.CatalogInitialRouteEvidence）
- `networkrouting.NetworkEvidenceView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/networkrouting/application.CatalogNetworkEvidence）
- `nodeoperations.ParcelIdentityView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/cmd/parcel-api.unconfiguredParcelIdentityView）
- `parcelpricing.PriceCardApprovalDutyRuleView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres.PriceCardApprovalDutyRules）
- `parcelshipment.ContinuedAttemptRegisterView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres.ContinuedAttemptRegisters）
- `parcelshipment.LabelChannelGateway` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/cmd/parcel-api.unconfiguredLabelChannelGateway）
- `parcelshipment.ResponsibilityStartView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres.IntakeAdoptions）
- `partycommercial.ApprovalDutyRuleView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres.ApprovalDutyRules）
- `partycommercial.ServiceProductFormRegistry` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres.CommercialPublications）
- `settlementaccounting.AmountGrammarRegister` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres.AmountGrammars）
- `settlementaccounting.AmountGrammarView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres.AmountGrammars）
- `settlementaccounting.AuditEscalationCeilingRegister` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres.AuditEscalationCeilings）
- `settlementaccounting.AuditEscalationCeilingView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres.AuditEscalationCeilings）
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
- `transportfulfillment.FailedAttemptSource` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres.PickupAttempts）
- `visibilityexception.NotificationChannelGateway` 

精确口径缺（无具体类型完整实现）：

- `transportfulfillment.TrackingSource` （虚高：名字出现过，但无人实现）
- `visibilityexception.NotificationChannelGateway` 
