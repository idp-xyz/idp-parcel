# 12 pilot-governance 四册迁到 ADR-0144

Category: enhancement
Status: in-progress——作者完成，待非作者评审（分支 `cursor/crpr-12-governance`，基 `3a20b2bcc`；迁移用了预留的 pilot-governance `0009`，进 main 时按 tip 重核）
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

## 完成记录

### 落点

| 笔 | 文件 | 做了什么 |
|---|---|---|
| `ed7178386` | `internal/pilotgovernance/ports/catalogue_pages.go`（新） | 声明四册 |
| | `internal/pilotgovernance/ports/registry_read.go` | `GovernanceRegistryRead` 四个方法改收 `cataloguepage.Query`、答 `cataloguepage.Result[Row]`；仍不收租户，`limit` 非正仍拒 |
| | `internal/pilotgovernance/adapters/postgres/registry_read.go` | 四册落列；共用 `registryPage` 取页（总数一句、本页一句、同一组条件），游标位置取扫描回来的列值；权威区间的 `interval_id` 只进游标 |
| | `internal/pilotgovernance/adapters/http/query_governance_registers.go`、`catalogue_page.go`（新） | `register` 照旧先于 Intake 判；Intake 之后按选中那一册的声明解码，坏参数答 400 `MALFORMED_REQUEST` 带 `detail`（本包新增 `writeProblemWithDetail`、`problemDetail.Detail` 带 `omitempty`）；四册答复加 `page` |
| | `cmd/parcel-api/unwired_orchestration.go` | 只改 `unwiredGovernanceRegisters` 四个方法的签名 |
| | `migrations/pilot_governance/0009_catalogue_read_pagination_indexes.sql`（新） | 四册各一条与缺省序同形的索引 |
| | 两个适配器包的 `*_test.go` | 旧用例改调用点（`firstPage`/`rowsOf`，读面夹具多带 `pool`）；新增真库与端点用例 |
| 本笔 | 本票面 | 完成记录与 Status |

### 声明

四册都在 `GET /governance-registers` 下，选择器 `register`；都不放筛选维；可排维只放缺省那一维；取页条件里没有租户段。

| 册（`register`） | 缺省序 | 可排维 | 行标识 | `q` 列 | 索引（迁移 0009） |
|---|---|---|---|---|---|
| `governance-registers/authority-interval` | `-insertedAt` | `insertedAt` | `interval_id`（主键） | 对象范围、能力、事实类型、权威方 | `authority_interval_by_inserted (inserted_at DESC, interval_id DESC)` |
| `governance-registers/suspension` | `-occurredAt` | `occurredAt` | `suspension_id`（主键） | 暂停标识、触发来源、暂停范围、执行人 | `suspension_decision_by_occurred (occurred_at DESC, suspension_id DESC)` |
| `governance-registers/resumption` | `-decidedAt` | `decidedAt` | `suspension_id`（主键） | 暂停标识、决定人 | `resumption_decision_by_decided (decided_at DESC, suspension_id DESC)` |
| `governance-registers/takeover` | `-effectiveAt` | `effectiveAt` | 对象范围、能力、事实类型、权威方、`from_at`、`COALESCE(to_at, from_at)` | 对象范围、能力、事实类型、权威方 | `takeover_record_by_effective (effective_at DESC, 四维 DESC, from_at DESC, (COALESCE(to_at, from_at)) DESC)` |

### 完成判据

- ✅ 翻页期间插入新行不重出、不漏行，空册答空页 `total` 为 0：`TestGovernanceRegistersPageByCursorWithoutRepeatsOrGapsWhileRowsAreRegistered`，四册各一遍——翻完第一页后在已翻那头与未翻那头各登一行，第二页接着 `P03`、`P02`，末页带上新登在后头的 `P00`，`total` 5 → 7。
- ✅ `total` 随 `q` 变、`%` 与 `_` 按字面：`TestGovernanceRegistersTotalFollowsTheKeyword`，四册各一遍。四册都没有筛选维，「随筛选变」一格不适用。
- ✅ 决胜键同向、跨页续得上：`TestGovernanceRegistersBreakTiesInTheSortDirection`（四册，同刻四行跨页，正倒序各走完全册，时刻带微秒）；接管册第六段：`TestTakeoverRegistryKeepsOpenAndClosedTwinsApart`（前五段相同、一开一闭，一行一页翻，各出现一次）。
- ✅ 端点：未知键（`tenant`、`limit`）、未声明的筛选键、集外排序维与别册的排序维、解不开的游标、换 `q` 拿旧游标、拿暂停册的游标翻恢复册，各答 400 带 `detail`；渠道未配置时选定了册的各变体同答 403、答复体逐字相同；四册的 `page` 三格，`size` 取 Intake 页大小：`TestGovernanceRegistersRefuseQueriesOutsideTheDeclaration`、`TestGovernanceRegistersCarryThePageAndPassTheDecodedQuery`。
- ✅ 迁移按字节无 CR、无 BOM：`0009` 与票面已量。
- ✅ 自验：`go build ./...`、`go vet ./...` 全仓过。`go test -count=1 -p 1 -v <包>`（带 `IDP_PARCEL_POSTGRES_DSN`）逐包数 `grep -c -- '--- PASS'` 与 `'--- SKIP'`（PASS/SKIP，均 FAIL 0）：
  `cmd/parcel-api` 654/0、`cmd/parcel-governance-register` 21/0、`internal/architecture` 174/0、`internal/accessidentity/adapters/pilotgovernance` 2/0、
  `internal/parcelshipment/adapters/accessidentity` 26/0、`internal/parcelshipment/adapters/pilotgovernance` 58/0、`internal/pilotgovernance/adapters/http` 21/0、
  `.../adapters/postgres` 64/0、`.../application` 36/0、`.../domain` 12/0、`internal/platform/migrate` 8/0、`migrations` 1/0。
  反向依赖由 `go list -f '{{.ImportPath}}{{range .Deps}} {{.}}{{end}}' ./...` 反查含 `internal/pilotgovernance/` 者。

### 判断项

- **租户键形状不另裁**：frontier/03 的「不接线」裁决已由 ADR-0083 取代（见该票 Comments）。照 ADR-0083 决定二，四个方法仍只收 `(ctx, limit, query)`，声明、取页条件与索引都没有租户段。
- **决胜键改向**：暂停册、恢复册迁移前同刻按 `suspension_id` 正序，接管册同刻按四维与起点正序；迁移后与时刻倒序同向（倒序）。只在排序时刻完全相同时次序与迁移前相反。权威区间册迁移前已是 `interval_id DESC`，不变。
- **接管册没有主键**：身份是唯一约束 `takeover_record_interval_identity`，含可为空的 `to_at`（`NULLS NOT DISTINCT`）。行标识取六段，末段在读面导出成 `COALESCE(to_at, from_at)`（取舍 6）。不撞值的依据是约束 `takeover_record_bounds`：闭区间终点晚于起点，开放区间同维同起点至多一行。索引末段是同一表达式，两处须逐字一致，读面落列处有注释。
- **`interval_id` 进游标**：它是权威区间的主键与迁移前的收尾列，但答复体不透出（行注「代理键不透出」）。现在它在游标里，游标对调用方是不透明串。
- **`register` 仍先于 Intake**：缺席或词外的 `register` 照旧答 400（不带 `detail`），渠道未配置时也是。这条行为迁移前就有，parcel-shipment 渠道择优册的 `view` 是同一做法。取舍 8 管的是解码的参数：`register` 合法时，其余参数写对写错，未配置渠道都同答 403。
- **不是按包裹或事件计量的大册**：治理登记册是产品级脱敏登记，量级远小于租户级，`total` 给精确值。
- **管理台与脚本带的参数**：`apps/admin-web/src/pages/governance/api.ts` 只带 `?register=` 四词，`cmd/parcel-api` 的探针是 `?register=authority-interval`，`scripts/` 只在 README 提到 `?register=takeover`。四册都把 `register` 登为选择器，取值进游标摘要。

### 未验

- 没有跑 `EXPLAIN` 看四条索引（尤其接管册的表达式末段）是否被缺省序选用；表是产品级小表，只由语句与索引同形支撑。
- 管理台不改（归 14）：今天它只取第一页（Intake 页大小 200）。
