# 07 network-routing 其余四册迁到 ADR-0144

Category: enhancement
Status: ready-for-agent——迁移编号预留 network-routing `0034`（进 main 时按 tip 重核）
Blocked by: 01
地盘：`internal/networkrouting/{ports,adapters/http,adapters/postgres}` 里下表各册的读端口、端点与读面及其测试（网络目录七族已迁，不在内）；`cmd/parcel-api` 里这些读口的占位；
`migrations/network_routing` 的一条新迁移（索引）。
出处：ADR-0144 决定三至七；spec「横贯各子票的取舍」。

## 本票的册（今天的次序钉 `a1274a6b8`）

| 路由（选择器） | 读端口方法 | 今天的 `ORDER BY` |
|---|---|---|
| `/route-plans`（两种） | `RoutePlanCatalogueRead.ListInitialRoutes` / `ListRouteReassessments` | `recorded_at DESC, customer_account_id, shipment_request_id, acceptance_baseline, declared_parcel_id, service_purpose` / `recorded_at DESC, correlation_id` |
| `/route-reroute-suggestions` | `RerouteSuggestionCatalogueRead.ListRerouteSuggestions` | `recorded_at DESC, correlation_id` |
| `/route-initial-route-ties` | `InitialRouteTieCatalogueRead.ListInitialRouteTies` | `settled_at DESC, declared_parcel_id, customer_account_id, shipment_request_id, acceptance_baseline, service_purpose` |

## 做法与判据

见 spec「每张迁移子票（02–13）的做法与判据」。本票特有：初始路线按包裹计量，是取舍 4 点名的大册；组合键各段都进行标识。
