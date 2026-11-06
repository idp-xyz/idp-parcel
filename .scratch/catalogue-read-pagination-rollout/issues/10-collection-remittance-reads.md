# 10 collection-remittance 一册迁到 ADR-0144

Category: enhancement
Status: in-progress——2026-11-06 作者开工，分支 `cursor/crpr-10-collection`，基 `3a20b2bcc`。迁移编号预留 collection-remittance `0005`（进 main 时按 tip 重核）
Blocked by: 01
地盘：`internal/collectionremittance/{ports,adapters/http,adapters/postgres}` 里代收分户账的读端口、端点与读面及其测试；`cmd/parcel-api` 里这个读口的占位；
`migrations/collection_remittance` 的一条新迁移（索引）。
出处：ADR-0144 决定三至七；spec「横贯各子票的取舍」。

## 本票的册（今天的次序钉 `a1274a6b8`）

| 路由 | 读端口方法 | 今天的 `ORDER BY` |
|---|---|---|
| `/collection-subledgers` | `CodSubledgerCatalogueRead.ListCodSubledgers` | `s.customer_ref, s.legal_entity_ref, s.currency, s.channel_ref` |

## 做法与判据

见 spec「每张迁移子票（02–13）的做法与判据」。本票特有：分户账的余额是按过账现算的侧连接，键集落在分户账四段键上、余额只随本页行算。
