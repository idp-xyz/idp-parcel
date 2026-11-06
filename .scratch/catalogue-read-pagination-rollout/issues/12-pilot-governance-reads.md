# 12 pilot-governance 四册迁到 ADR-0144

Category: enhancement
Status: in-progress——2026-11-06 作者开工，分支 `cursor/crpr-12-governance`，基 `3a20b2bcc`。迁移编号预留 pilot-governance `0009`（进 main 时按 tip 重核）
Blocked by: 01
地盘：`internal/pilotgovernance/{ports,adapters/http,adapters/postgres}` 里治理登记册的读端口、端点与读面及其测试；`cmd/parcel-api` 里这个读口的占位；
`migrations/pilot_governance` 的一条新迁移（索引）。
出处：ADR-0144 决定三至七；spec「横贯各子票的取舍」。

## 本票的册（今天的次序钉 `a1274a6b8`）

| 路由（选择器） | 读端口方法 | 今天的 `ORDER BY` |
|---|---|---|
| `/governance-registers`（四种） | `GovernanceRegistryRead.ListAuthorityIntervals` / `ListSuspensions` / `ListResumptions` / `ListTakeovers` | `inserted_at DESC, interval_id DESC` / `occurred_at DESC, suspension_id` / `decided_at DESC, suspension_id` / `effective_at DESC, object_scope, capability, fact_kind, authority, from_at` |

## 做法与判据

见 spec「每张迁移子票（02–13）的做法与判据」。本票特有：治理读面的租户键形状以票 admin-web-page-wiring-frontier/03 的裁决为准，不在本票另裁。
