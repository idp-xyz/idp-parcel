# 机制半边清点（生成物，勿手改）

由 `tools/mechanism-inventory` 生成。本文只有数，没有定级——「达标／部分／未开始」与留待裁定在开发主线正文里。

## 逐上下文文件面

| 上下文 | 生产 | 测试 | 应用编排 | postgres 适配器 | 其中 Outbox 投递 | http 适配器 |
|---|---|---|---|---|---|---|
| accessidentity | 7 | 3 | 0 | 1 | 0 | 0 |
| architecture（非业务） | 0 | 13 | 0 | 0 | 0 | 0 |
| collectionremittance | 22 | 10 | 4 | 6 | 0 | 3 |
| customscompliance | 95 | 95 | 18 | 41 | 10 | 13 |
| networkrouting | 59 | 57 | 8 | 13 | 2 | 5 |
| nodeoperations | 31 | 26 | 3 | 10 | 4 | 6 |
| parcelpricing | 99 | 94 | 10 | 14 | 1 | 16 |
| parcelshipment | 187 | 181 | 20 | 34 | 10 | 17 |
| partycommercial | 149 | 157 | 12 | 39 | 1 | 35 |
| pilotgovernance | 21 | 19 | 4 | 6 | 1 | 4 |
| platform（非业务） | 22 | 21 | 0 | 0 | 0 | 0 |
| settlementaccounting | 101 | 82 | 14 | 43 | 9 | 9 |
| transportfulfillment | 143 | 130 | 25 | 36 | 11 | 26 |
| visibilityexception | 98 | 93 | 11 | 30 | 8 | 10 |
| **合计** | 1034 | 981 | 129 | 273 | 57 | 144 |

业务上下文 12 个，非业务目录 2 个。`cmd/` 生产 68、测试 99。

## 跨上下文消费缝：25 组，74 个生产文件

| 消费方 | 提供方 | 文件 |
|---|---|---|
| customscompliance | settlementaccounting | 2 |
| networkrouting | parcelshipment | 3 |
| networkrouting | partycommercial | 3 |
| nodeoperations | transportfulfillment | 1 |
| parcelpricing | settlementaccounting | 2 |
| parcelshipment | customscompliance | 1 |
| parcelshipment | networkrouting | 1 |
| parcelshipment | nodeoperations | 4 |
| parcelshipment | parcelpricing | 3 |
| parcelshipment | partycommercial | 20 |
| parcelshipment | pilotgovernance | 2 |
| parcelshipment | settlementaccounting | 2 |
| parcelshipment | transportfulfillment | 5 |
| settlementaccounting | customscompliance | 2 |
| settlementaccounting | parcelpricing | 2 |
| settlementaccounting | partycommercial | 3 |
| transportfulfillment | networkrouting | 1 |
| transportfulfillment | parcelshipment | 1 |
| transportfulfillment | partycommercial | 2 |
| visibilityexception | customscompliance | 2 |
| visibilityexception | networkrouting | 1 |
| visibilityexception | nodeoperations | 1 |
| visibilityexception | parcelshipment | 4 |
| visibilityexception | partycommercial | 1 |
| visibilityexception | transportfulfillment | 5 |

## 迁移：12 个模块共 182 份 SQL

| 模块 | 份数 |
|---|---|
| access_identity | 1 |
| collection_remittance | 1 |
| customs_compliance | 23 |
| network_routing | 11 |
| node_operations | 4 |
| parcel_pricing | 10 |
| parcel_shipment | 22 |
| party_commercial | 36 |
| pilot_governance | 7 |
| settlement_accounting | 21 |
| transport_fulfillment | 20 |
| visibility_exception | 26 |

## 接线面：接入面端点 132 个，消费适配器 33 个生产文件，直投路由表 24 条

接入面端点按 `cmd/` 生产文件里 `[]httpapi.BusinessEndpoint` 字面量的条目数，按端点构造函数所在的 `internal/<上下文>/adapters/http` 归属；不按 `adapters/http/` 的文件数——一个处理器可挂多个端点。

| 上下文 | 端点 |
|---|---|
| collectionremittance | 1 |
| customscompliance | 16 |
| networkrouting | 9 |
| nodeoperations | 2 |
| parcelpricing | 11 |
| parcelshipment | 15 |
| partycommercial | 32 |
| pilotgovernance | 1 |
| settlementaccounting | 6 |
| transportfulfillment | 24 |
| visibilityexception | 15 |
| **合计** | 132 |

消费适配器按 `internal/<消费方>/adapters/` 下 `inbox`、`adoptconsume`、`finalconsume`、`veconsume` 四类目录的生产文件数。它与上面的「跨上下文消费缝」是两种东西：那一栏数的是消费方为某个提供方写的防腐层，这一栏数的是接进程内直投信封的消费门。

| 消费方 | inbox | adoptconsume | finalconsume | veconsume | 合计 |
|---|---|---|---|---|---|
| customscompliance | 1 | 0 | 0 | 0 | 1 |
| networkrouting | 2 | 0 | 0 | 0 | 2 |
| parcelpricing | 1 | 0 | 0 | 0 | 1 |
| parcelshipment | 11 | 1 | 1 | 0 | 13 |
| settlementaccounting | 2 | 0 | 0 | 0 | 2 |
| visibilityexception | 12 | 0 | 0 | 2 | 14 |
| **合计** | 29 | 1 | 1 | 2 | 33 |

直投路由表按 `cmd/` 生产文件里 `map[eventing.EventType]dispatch.Consumer` 字面量的条目数，按条目键（事件类型常量）所属的消费门包归属。路由表只随消费者一起长（ADR-0049 第三条），本表只报它此刻多长。

| 事件类型所属消费方 | 条目 |
|---|---|
| customscompliance | 1 |
| networkrouting | 2 |
| parcelpricing | 1 |
| parcelshipment | 10 |
| settlementaccounting | 2 |
| visibilityexception | 8 |
| **合计** | 24 |

## 端口：声明 422 个；基线口径缺 17，精确口径缺 7

基线口径缺（名字未在任何适配器/平台生产文件出现）：

- `networkrouting.CustomsApplicabilitySource` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/networkrouting/application.CustomsApplicabilityNotConnected）
- `networkrouting.InitialRouteEvidenceView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/networkrouting/application.CatalogInitialRouteEvidence）
- `networkrouting.NetworkEvidenceView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/networkrouting/application.CatalogNetworkEvidence）
- `nodeoperations.ParcelIdentityView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/cmd/parcel-api.unconfiguredParcelIdentityView）
- `parcelpricing.PricingInputResolver` 
- `parcelshipment.ContinuedAttemptRegisterView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres.ContinuedAttemptRegisters）
- `parcelshipment.CurrentFinalView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres.FinalOutcomes）
- `parcelshipment.LabelChannelGateway` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/cmd/parcel-api.unconfiguredLabelChannelGateway）
- `parcelshipment.ResponsibilityStartView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres.IntakeAdoptions）
- `partycommercial.ApprovalDutyRuleView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres.ApprovalDutyRules）
- `partycommercial.ServiceProductFormRegistry` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres.CommercialPublications）
- `settlementaccounting.ClaimAmountRuleView` 
- `settlementaccounting.ConfirmedChargeFactsView` 
- `settlementaccounting.SupplierAuditAuthorityView` 
- `settlementaccounting.SupplierPayableAccountView` 
- `transportfulfillment.FailedAttemptSource` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres.PickupAttempts）
- `visibilityexception.NotificationChannelGateway` 

精确口径缺（无具体类型完整实现）：

- `parcelpricing.PricingInputResolver` 
- `settlementaccounting.ClaimAmountRuleView` 
- `settlementaccounting.ConfirmedChargeFactsView` 
- `settlementaccounting.SupplierAuditAuthorityView` 
- `settlementaccounting.SupplierPayableAccountView` 
- `transportfulfillment.TrackingSource` （虚高：名字出现过，但无人实现）
- `visibilityexception.NotificationChannelGateway` 
