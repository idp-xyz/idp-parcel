# 04 parcel-pricing 各册迁到 ADR-0144

Category: enhancement
Status: in-progress——分支 `cursor/crpr-04-pricing`，基 `d48c5b130`；迁移编号预留 parcel-pricing `0015`（进 main 时按 tip 重核）
Blocked by: 01
地盘：`internal/parcelpricing/{ports,adapters/http,adapters/postgres}` 里下表各册的读端口、端点与读面及其测试；`cmd/parcel-api` 里这些读口的占位；
`migrations/parcel_pricing` 的一条新迁移（索引）。
出处：ADR-0144 决定三至七；spec「横贯各子票的取舍」。

## 本票的册（今天的次序钉 `a1274a6b8`）

| 路由 | 读端口方法 | 今天的 `ORDER BY` |
|---|---|---|
| `/pricing-evaluations` | `EvaluationCatalogueRead.ListEvaluations` | `recorded_at DESC, evaluation_id` |
| `/pricing-price-cards` | `PriceCardCatalogueRead.ListPriceCards` | `registered_at DESC, plan_id, plan_version` |
| `/pricing-reference-series` | `ReferenceSeriesCatalogueRead.ListReferenceSeries` | `registered_at DESC, series_id, series_version` |
| `/pricing-reference-series-coverage` | `ReferenceSeriesCoverageRead.ListReferenceSeriesCoverage` | `series_id, kind` |
| `/pricing-price-card-draft-views` | `PriceCardDraftRead.ListPriceCardDrafts` | `submitted_at DESC, plan_id, plan_version` |

开工先核 `/pricing-pending-series-evaluations`：它的读端口不收 `limit`，判它是「有意不设上限」（写明依据）还是漏网，漏网则一并迁。

## 做法与判据

见 spec「每张迁移子票（02–13）的做法与判据」。本票特有：评价按请求计量，是取舍 4 点名的大册。
