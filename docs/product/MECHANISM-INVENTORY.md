# 机制半边清点（生成物，勿手改）

由 `tools/mechanism-inventory` 生成。本文只有数，没有定级——「达标／部分／未开始」与留待裁定在开发主线正文里。

## 逐上下文文件面

| 上下文 | 生产 | 测试 | 应用编排 | postgres 适配器 | 其中 Outbox 投递 | http 适配器 |
|---|---|---|---|---|---|---|
| accessidentity | 5 | 1 | 0 | 0 | 0 | 0 |
| architecture（非业务） | 0 | 13 | 0 | 0 | 0 | 0 |
| collectionremittance | 22 | 10 | 4 | 6 | 0 | 3 |
| customscompliance | 83 | 83 | 16 | 37 | 9 | 11 |
| networkrouting | 55 | 50 | 6 | 14 | 2 | 5 |
| nodeoperations | 30 | 25 | 3 | 10 | 4 | 5 |
| parcelpricing | 88 | 86 | 9 | 13 | 1 | 16 |
| parcelshipment | 176 | 169 | 20 | 32 | 10 | 17 |
| partycommercial | 123 | 138 | 9 | 33 | 1 | 27 |
| pilotgovernance | 20 | 18 | 3 | 6 | 1 | 4 |
| platform（非业务） | 17 | 16 | 0 | 0 | 0 | 0 |
| settlementaccounting | 83 | 64 | 12 | 40 | 8 | 7 |
| transportfulfillment | 140 | 128 | 25 | 36 | 11 | 24 |
| visibilityexception | 98 | 92 | 11 | 30 | 8 | 10 |
| **合计** | 940 | 893 | 118 | 257 | 55 | 129 |

业务上下文 12 个，非业务目录 2 个。`cmd/` 生产 60、测试 83。

## 跨上下文消费缝：23 组，67 个生产文件

| 消费方 | 提供方 | 文件 |
|---|---|---|
| customscompliance | settlementaccounting | 2 |
| networkrouting | parcelshipment | 3 |
| networkrouting | partycommercial | 3 |
| nodeoperations | transportfulfillment | 1 |
| parcelshipment | customscompliance | 1 |
| parcelshipment | networkrouting | 1 |
| parcelshipment | nodeoperations | 4 |
| parcelshipment | parcelpricing | 3 |
| parcelshipment | partycommercial | 20 |
| parcelshipment | pilotgovernance | 2 |
| parcelshipment | settlementaccounting | 2 |
| parcelshipment | transportfulfillment | 4 |
| settlementaccounting | parcelpricing | 1 |
| settlementaccounting | partycommercial | 2 |
| transportfulfillment | networkrouting | 1 |
| transportfulfillment | parcelshipment | 1 |
| transportfulfillment | partycommercial | 2 |
| visibilityexception | customscompliance | 2 |
| visibilityexception | networkrouting | 1 |
| visibilityexception | nodeoperations | 1 |
| visibilityexception | parcelshipment | 4 |
| visibilityexception | partycommercial | 1 |
| visibilityexception | transportfulfillment | 5 |

## 迁移：11 个模块共 163 份 SQL

| 模块 | 份数 |
|---|---|
| collection_remittance | 1 |
| customs_compliance | 17 |
| network_routing | 9 |
| node_operations | 4 |
| parcel_pricing | 9 |
| parcel_shipment | 21 |
| party_commercial | 32 |
| pilot_governance | 6 |
| settlement_accounting | 18 |
| transport_fulfillment | 20 |
| visibility_exception | 26 |

## 接线面：接入面端点 117 个，消费适配器 29 个生产文件，直投路由表 20 条

接入面端点按 `cmd/` 生产文件里 `[]httpapi.BusinessEndpoint` 字面量的条目数，按端点构造函数所在的 `internal/<上下文>/adapters/http` 归属；不按 `adapters/http/` 的文件数——一个处理器可挂多个端点。

| 上下文 | 端点 |
|---|---|
| collectionremittance | 1 |
| customscompliance | 13 |
| networkrouting | 9 |
| nodeoperations | 2 |
| parcelpricing | 11 |
| parcelshipment | 15 |
| partycommercial | 24 |
| pilotgovernance | 1 |
| settlementaccounting | 4 |
| transportfulfillment | 24 |
| visibilityexception | 13 |
| **合计** | 117 |

消费适配器按 `internal/<消费方>/adapters/` 下 `inbox`、`adoptconsume`、`finalconsume`、`veconsume` 四类目录的生产文件数。它与上面的「跨上下文消费缝」是两种东西：那一栏数的是消费方为某个提供方写的防腐层，这一栏数的是接进程内直投信封的消费门。

| 消费方 | inbox | adoptconsume | finalconsume | veconsume | 合计 |
|---|---|---|---|---|---|
| customscompliance | 1 | 0 | 0 | 0 | 1 |
| networkrouting | 2 | 0 | 0 | 0 | 2 |
| parcelshipment | 10 | 1 | 1 | 0 | 12 |
| visibilityexception | 12 | 0 | 0 | 2 | 14 |
| **合计** | 25 | 1 | 1 | 2 | 29 |

直投路由表按 `cmd/` 生产文件里 `map[eventing.EventType]dispatch.Consumer` 字面量的条目数，按条目键（事件类型常量）所属的消费门包归属。路由表只随消费者一起长（ADR-0049 第三条），本表只报它此刻多长。

| 事件类型所属消费方 | 条目 |
|---|---|
| customscompliance | 1 |
| networkrouting | 2 |
| parcelshipment | 9 |
| visibilityexception | 8 |
| **合计** | 20 |

## 端口：声明 390 个；基线口径缺 14，精确口径缺 7

基线口径缺（名字未在任何适配器/平台生产文件出现）：

- `nodeoperations.ParcelIdentityView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/cmd/parcel-api.unconfiguredParcelIdentityView）
- `parcelshipment.ContinuedAttemptRegisterView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres.ContinuedAttemptRegisters）
- `parcelshipment.CurrentFinalView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres.FinalOutcomes）
- `parcelshipment.LabelChannelGateway` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/cmd/parcel-api.unconfiguredLabelChannelGateway）
- `parcelshipment.ResponsibilityStartView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres.IntakeAdoptions）
- `partycommercial.ApprovalDutyRuleView` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres.ApprovalDutyRules）
- `partycommercial.ServiceProductFormRegistry` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres.CommercialPublications）
- `settlementaccounting.ClaimAmountRuleView` 
- `settlementaccounting.ConfirmedChargeFactsView` 
- `settlementaccounting.ContractResponsibilityView` 
- `settlementaccounting.SupplierAuditAuthorityView` 
- `settlementaccounting.SupplierPayableAccountView` 
- `transportfulfillment.FailedAttemptSource` （虚低：精确口径已实现，实现者 go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres.PickupAttempts）
- `visibilityexception.NotificationChannelGateway` 

精确口径缺（无具体类型完整实现）：

- `settlementaccounting.ClaimAmountRuleView` 
- `settlementaccounting.ConfirmedChargeFactsView` 
- `settlementaccounting.ContractResponsibilityView` 
- `settlementaccounting.SupplierAuditAuthorityView` 
- `settlementaccounting.SupplierPayableAccountView` 
- `transportfulfillment.TrackingSource` （虚高：名字出现过，但无人实现）
- `visibilityexception.NotificationChannelGateway` 
