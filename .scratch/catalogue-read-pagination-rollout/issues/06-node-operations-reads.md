# 06 node-operations 八册迁到 ADR-0144

Category: enhancement
Status: in-progress——分支 `cursor/crpr-06-node`，基 `3a20b2bcc`；迁移编号预留 node-operations `0027`（进 main 时按 tip 重核）
Blocked by: 01
地盘：`internal/nodeoperations/{ports,adapters/http,adapters/postgres}` 里下表各册的读端口、端点与读面及其测试；`cmd/parcel-api` 里这些读口的占位；
`migrations/node_operations` 的一条新迁移（索引）。
出处：ADR-0144 决定三至七；spec「横贯各子票的取舍」。

## 本票的册（今天的次序钉 `a1274a6b8`）

| 路由（选择器） | 读端口方法 | 今天的 `ORDER BY` |
|---|---|---|
| `/node-operations-records`（六种） | `ReviewCatalogueRead.ListReceptions` / `ListUnidentifiedItems` / `ListConsolidationUnits` / `ListConsolidationMembers` / `ListIdentityReviewUnits` / `ListPhysicalControlTransitions` | 见 `adapters/postgres/review_catalogue*.go` |
| `/node-operations-delivery-pre-advices` | `DeliveryScopeCatalogueRead.ListDeliveryPreAdvices` | `expected_arrival_at DESC, pre_advice_id` |
| `/node-operations-delivery-handover-notes` | `DeliveryScopeCatalogueRead.ListDeliveryHandoverNotes` | `note.opened_at DESC, note.note_id` |

两条按标识取单条的路由（`…/{preAdviceId}`、`…/{noteId}`）不在内。

## 做法与判据

见 spec「每张迁移子票（02–13）的做法与判据」。本票特有：收寄、集包成员等按实物计量，是取舍 4 点名的大册；聚合导出的册（如身份复核单元按 `latest` 排）键集落在导出列上。
