# 机制半边清点（生成物，勿手改）

由 `tools/mechanism-inventory` 生成。本文只有数，没有定级——「达标／部分／未开始」与留待裁定在开发主线正文里。

## 逐上下文文件面

| 上下文 | 生产 | 测试 | 应用编排 | postgres 适配器 | 其中 Outbox 投递 | http 适配器 |
|---|---|---|---|---|---|---|
| accessidentity | 5 | 1 | 0 | 0 | 0 | 0 |
| architecture（非业务） | 0 | 10 | 0 | 0 | 0 | 0 |
| collectionremittance | 22 | 10 | 4 | 6 | 0 | 3 |
| customscompliance | 69 | 68 | 13 | 32 | 9 | 8 |
| networkrouting | 52 | 47 | 6 | 13 | 2 | 5 |
| nodeoperations | 29 | 24 | 3 | 9 | 4 | 5 |
| parcelpricing | 45 | 46 | 3 | 6 | 1 | 8 |
| parcelshipment | 102 | 99 | 14 | 20 | 7 | 10 |
| partycommercial | 65 | 66 | 7 | 20 | 0 | 12 |
| pilotgovernance | 19 | 17 | 3 | 6 | 1 | 4 |
| platform（非业务） | 12 | 11 | 0 | 0 | 0 | 0 |
| settlementaccounting | 73 | 50 | 10 | 35 | 7 | 7 |
| transportfulfillment | 53 | 44 | 8 | 22 | 9 | 5 |
| visibilityexception | 87 | 84 | 9 | 26 | 8 | 10 |
| **合计** | 633 | 577 | 80 | 195 | 48 | 77 |

业务上下文 12 个，非业务目录 2 个。`cmd/` 生产 37、测试 55。

## 跨上下文消费缝：16 组，42 个生产文件

| 消费方 | 提供方 | 文件 |
|---|---|---|
| networkrouting | parcelshipment | 3 |
| networkrouting | partycommercial | 3 |
| nodeoperations | transportfulfillment | 1 |
| parcelshipment | networkrouting | 1 |
| parcelshipment | nodeoperations | 3 |
| parcelshipment | parcelpricing | 1 |
| parcelshipment | partycommercial | 10 |
| parcelshipment | pilotgovernance | 1 |
| parcelshipment | settlementaccounting | 2 |
| parcelshipment | transportfulfillment | 4 |
| settlementaccounting | partycommercial | 1 |
| visibilityexception | customscompliance | 2 |
| visibilityexception | networkrouting | 1 |
| visibilityexception | nodeoperations | 1 |
| visibilityexception | parcelshipment | 4 |
| visibilityexception | transportfulfillment | 4 |

## 迁移：11 个模块共 101 份 SQL

| 模块 | 份数 |
|---|---|
| collection_remittance | 1 |
| customs_compliance | 13 |
| network_routing | 9 |
| node_operations | 3 |
| parcel_pricing | 3 |
| parcel_shipment | 10 |
| party_commercial | 16 |
| pilot_governance | 5 |
| settlement_accounting | 15 |
| transport_fulfillment | 5 |
| visibility_exception | 21 |

## 端口：声明 274 个；基线口径缺 9，精确口径缺 7

基线口径缺（名字未在任何适配器/平台生产文件出现）：

- `nodeoperations.ParcelIdentityView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/cmd/parcel-api.unconfiguredParcelIdentityView）
- `parcelshipment.SourceDataAmendmentAuthorizer` 
- `parcelshipment.SourceDataRuleDeclaration` 
- `partycommercial.ServiceProductFormRegistry` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres.CommercialPublications）
- `settlementaccounting.ClaimAmountRuleView` 
- `settlementaccounting.ConfirmedChargeFactsView` 
- `settlementaccounting.ContractResponsibilityView` 
- `settlementaccounting.SupplierAuditAuthorityView` 
- `visibilityexception.NotificationChannelGateway` 

精确口径缺（无具体类型完整实现）：

- `parcelshipment.SourceDataAmendmentAuthorizer` 
- `parcelshipment.SourceDataRuleDeclaration` 
- `settlementaccounting.ClaimAmountRuleView` 
- `settlementaccounting.ConfirmedChargeFactsView` 
- `settlementaccounting.ContractResponsibilityView` 
- `settlementaccounting.SupplierAuditAuthorityView` 
- `visibilityexception.NotificationChannelGateway` 
