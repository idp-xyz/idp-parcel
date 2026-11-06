# 03 visibility-exception 十四册迁到 ADR-0144

Category: enhancement
Status: in-progress——2026-11-06 作者会话在分支 `cursor/crpr-03-visibility` 上做（worktree `idp-parcel-crpr-03`，基 `d48c5b130`）；迁移编号预留 visibility-exception `0034`（进 main 时按 tip 重核）
Blocked by: 01
地盘：`internal/visibilityexception/{ports,adapters/http,adapters/postgres}` 里下表各册的读端口、端点与读面及其测试；`cmd/parcel-api` 里这些读口的占位；
`migrations/visibility_exception` 的一条新迁移（索引）。
出处：ADR-0144 决定三至七；spec「横贯各子票的取舍」。

## 本票的册（今天的次序钉 `a1274a6b8`）

| 路由（选择器） | 读端口方法 | 今天的 `ORDER BY` |
|---|---|---|
| `/tracking-projections` | `OperationsProjectionRead.ListCurrent` | `derived_at DESC, parcel_ref DESC` |
| `/exception-case-records` | `CaseReviewRead.ListExceptionCases` | `established_at DESC, case_id` |
| `/exception-triage-records`（两种） | `TriageReviewRead.ListSignalEpisodes` / `ListDispositionRequests` | `started_at DESC, seq DESC` / `sent_at DESC, request_id` |
| `/claims-recovery-records`（三种） | `ClaimsRecoveryReviewRead.ListCustomerNotifications` / `ListClaimItems` / `ListRecoveryMatters` | `decided_at DESC, notification_id` / `submitted_at DESC, batch_ref, item_id` / `opened_at DESC, matter_id` |
| `/visibility-catalogues`（八种） | `CatalogueListRead.ListMilestoneMappings` … `ListConflictSignalRules` | 版本册按 `effective_from DESC, <版本>`；声明册按各自的键 |

## 做法与判据

见 spec「每张迁移子票（02–13）的做法与判据」。本票特有：

- 选择器分派的每种各是一册（声明各有册名），选择器键在声明里登为选择器，不兼作筛选维（决定四）。
- 追踪投影按包裹计量，是取舍 4 点名的大册。
