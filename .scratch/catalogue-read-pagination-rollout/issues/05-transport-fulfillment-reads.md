# 05 transport-fulfillment 各册迁到 ADR-0144

Category: enhancement
Status: ready-for-agent——迁移编号预留 transport-fulfillment `0058`（进 main 时按 tip 重核）
Blocked by: 01
地盘：`internal/transportfulfillment/{ports,adapters/http,adapters/postgres}` 里下表各册的读端口、端点与读面及其测试；`cmd/parcel-api` 里这些读口的占位；
`migrations/transport_fulfillment` 的一条新迁移（索引）。
出处：ADR-0144 决定三至七；spec「横贯各子票的取舍」。

## 本票的册（今天的次序钉 `a1274a6b8`）

`/transport-fulfillment-records` 按 `kind` 分派的十三种（`ReviewCatalogueRead` 的 `ListTransportSchedules`、`ListScheduleTemplates`、`ListCapacityPools`、`ListTransportHandovers`、
`ListEffectiveDeliveries`、`ListCarrierMasterDocuments`、`ListTransportPreparations`、`ListDeliveryDispatchPendings`、`ListDeliveryDispatchReaskCadences`、
`ListActualFulfillmentUnformed`、`ListBookingRequests`、`ListBookingAnswerBounces`、`ListPendingAttemptResults`），外加：

| 路由 | 读端口方法 | 今天的 `ORDER BY` |
|---|---|---|
| `/transport-fulfillment-external-tracking-facts` | `ExternalTrackingFactReviewRead.ListCurrentExternalTrackingFacts` | `occurred_at, fact_ref, version` |

各种今天的次序见 `adapters/postgres/review_catalogue.go` 等读面；开工先核 `/transport-fulfillment-carrier-first-effective-pickups` 是否按租户上列（读端口不收 `limit`），
按租户上列则一并迁、按对象取链则属单条读取不在内。

## 做法与判据

见 spec「每张迁移子票（02–13）的做法与判据」。本票特有：交接、有效妥投、订舱等按包裹或事件计量，是取舍 4 点名的大册。
