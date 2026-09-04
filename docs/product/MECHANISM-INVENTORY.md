# 机制半边清点（生成物，勿手改）

由 `tools/mechanism-inventory` 生成。本文只有数，没有定级——「达标／部分／未开始」与留待裁定在开发主线正文里。

## 逐上下文文件面

| 上下文 | 生产 | 测试 | 应用编排 | postgres 适配器 | 其中 Outbox 投递 | http 适配器 |
|---|---|---|---|---|---|---|
| accessidentity | 5 | 1 | 0 | 0 | 0 | 0 |
| architecture（非业务） | 0 | 13 | 0 | 0 | 0 | 0 |
| collectionremittance | 22 | 10 | 4 | 6 | 0 | 3 |
| customscompliance | 74 | 75 | 16 | 34 | 9 | 8 |
| networkrouting | 52 | 48 | 6 | 13 | 2 | 5 |
| nodeoperations | 29 | 24 | 3 | 9 | 4 | 5 |
| parcelpricing | 57 | 60 | 5 | 8 | 1 | 12 |
| parcelshipment | 121 | 121 | 17 | 22 | 7 | 11 |
| partycommercial | 81 | 81 | 8 | 25 | 0 | 14 |
| pilotgovernance | 19 | 17 | 3 | 6 | 1 | 4 |
| platform（非业务） | 15 | 15 | 0 | 0 | 0 | 0 |
| settlementaccounting | 77 | 56 | 12 | 37 | 7 | 7 |
| transportfulfillment | 100 | 89 | 19 | 30 | 10 | 16 |
| visibilityexception | 97 | 91 | 11 | 30 | 8 | 10 |
| **合计** | 749 | 701 | 104 | 220 | 49 | 95 |

业务上下文 12 个，非业务目录 2 个。`cmd/` 生产 42、测试 61。

## 跨上下文消费缝：16 组，47 个生产文件

| 消费方 | 提供方 | 文件 |
|---|---|---|
| networkrouting | parcelshipment | 3 |
| networkrouting | partycommercial | 3 |
| nodeoperations | transportfulfillment | 1 |
| parcelshipment | networkrouting | 1 |
| parcelshipment | nodeoperations | 3 |
| parcelshipment | parcelpricing | 3 |
| parcelshipment | partycommercial | 11 |
| parcelshipment | pilotgovernance | 2 |
| parcelshipment | settlementaccounting | 2 |
| parcelshipment | transportfulfillment | 4 |
| settlementaccounting | partycommercial | 1 |
| visibilityexception | customscompliance | 2 |
| visibilityexception | networkrouting | 1 |
| visibilityexception | nodeoperations | 1 |
| visibilityexception | parcelshipment | 4 |
| visibilityexception | transportfulfillment | 5 |

## 迁移：11 个模块共 128 份 SQL

| 模块 | 份数 |
|---|---|
| collection_remittance | 1 |
| customs_compliance | 16 |
| network_routing | 9 |
| node_operations | 3 |
| parcel_pricing | 4 |
| parcel_shipment | 14 |
| party_commercial | 22 |
| pilot_governance | 5 |
| settlement_accounting | 16 |
| transport_fulfillment | 12 |
| visibility_exception | 26 |

## 接线面：接入面端点 90 个，消费适配器 24 个生产文件，直投路由表 15 条

接入面端点按 `cmd/` 生产文件里 `[]httpapi.BusinessEndpoint` 字面量的条目数，按端点构造函数所在的 `internal/<上下文>/adapters/http` 归属；不按 `adapters/http/` 的文件数——一个处理器可挂多个端点。

| 上下文 | 端点 |
|---|---|
| collectionremittance | 1 |
| customscompliance | 10 |
| networkrouting | 9 |
| nodeoperations | 2 |
| parcelpricing | 8 |
| parcelshipment | 8 |
| partycommercial | 19 |
| pilotgovernance | 1 |
| settlementaccounting | 4 |
| transportfulfillment | 15 |
| visibilityexception | 13 |
| **合计** | 90 |

消费适配器按 `internal/<消费方>/adapters/` 下 `inbox`、`adoptconsume`、`finalconsume`、`veconsume` 四类目录的生产文件数。它与上面的「跨上下文消费缝」是两种东西：那一栏数的是消费方为某个提供方写的防腐层，这一栏数的是接进程内直投信封的消费门。

| 消费方 | inbox | adoptconsume | finalconsume | veconsume | 合计 |
|---|---|---|---|---|---|
| networkrouting | 2 | 0 | 0 | 0 | 2 |
| parcelshipment | 6 | 1 | 1 | 0 | 8 |
| visibilityexception | 12 | 0 | 0 | 2 | 14 |
| **合计** | 20 | 1 | 1 | 2 | 24 |

直投路由表按 `cmd/` 生产文件里 `map[eventing.EventType]dispatch.Consumer` 字面量的条目数，按条目键（事件类型常量）所属的消费门包归属。路由表只随消费者一起长（ADR-0049 第三条），本表只报它此刻多长。

| 事件类型所属消费方 | 条目 |
|---|---|
| networkrouting | 2 |
| parcelshipment | 5 |
| visibilityexception | 8 |
| **合计** | 15 |

## 端口：声明 325 个；基线口径缺 16，精确口径缺 13

基线口径缺（名字未在任何适配器/平台生产文件出现）：

- `nodeoperations.ParcelIdentityView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/cmd/parcel-api.unconfiguredParcelIdentityView）
- `parcelshipment.ContinuedAttemptRegisterView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres.ContinuedAttemptRegisters）
- `parcelshipment.LabelChannelGateway` 
- `parcelshipment.LabelValidityRuleView` 
- `parcelshipment.SourceDataAmendmentAuthorizer` 
- `parcelshipment.SourceDataRuleDeclaration` 
- `partycommercial.ServiceProductFormRegistry` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres.CommercialPublications）
- `settlementaccounting.BuyEvaluationView` 
- `settlementaccounting.ClaimAmountRuleView` 
- `settlementaccounting.ConfirmedChargeFactsView` 
- `settlementaccounting.ContractResponsibilityView` 
- `settlementaccounting.SupplierAuditAuthorityView` 
- `settlementaccounting.SupplierPayableAccountView` 
- `transportfulfillment.EffectiveTimeRules` 
- `transportfulfillment.FailedAttemptSource` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres.PickupAttempts）
- `visibilityexception.NotificationChannelGateway` 

精确口径缺（无具体类型完整实现）：

- `parcelshipment.LabelChannelGateway` 
- `parcelshipment.LabelValidityRuleView` 
- `parcelshipment.SourceDataAmendmentAuthorizer` 
- `parcelshipment.SourceDataRuleDeclaration` 
- `settlementaccounting.BuyEvaluationView` 
- `settlementaccounting.ClaimAmountRuleView` 
- `settlementaccounting.ConfirmedChargeFactsView` 
- `settlementaccounting.ContractResponsibilityView` 
- `settlementaccounting.SupplierAuditAuthorityView` 
- `settlementaccounting.SupplierPayableAccountView` 
- `transportfulfillment.EffectiveTimeRules` 
- `transportfulfillment.TrackingSource` （虚高：名字出现过，但无人实现）
- `visibilityexception.NotificationChannelGateway` 
