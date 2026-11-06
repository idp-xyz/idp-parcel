# 07 network-routing 其余四册迁到 ADR-0144

Category: enhancement
Status: in-progress——修订轮：照派单说明「02 的非作者评审拦下过的」逐条自查，缺口已补，待非作者评审（2026-11-06 通道 1，分支 `cursor/crpr-07-network`，基 `d48c5b130`；完成记录与修订记录见文末）。迁移编号 network-routing `0034`（进 main 时按 tip 重核）
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

见 spec「每张迁移子票（02–13）的做法与判据」。本票特有：初始路线按包裹计量，是 spec「`total` 一律给精确值」点名的大册；组合键各段都进行标识。

## 完成记录（2026-11-06，分支 `cursor/crpr-07-network`，基 `d48c5b130`）

**落点**（`ports/`、`adapters/` 都在 `internal/networkrouting/` 下）

| 笔 | 文件 | 做了什么 |
|---|---|---|
| `3dc84a8b2` | `ports/catalogue_pages.go`（新）；`ports/route_plan_read.go`、`reroute_decision.go`、`initial_route_tie_decision.go` | 本票各册的声明；各读端口收查询对象、答 `cataloguepage.Result`；建议行多带 `RecordedAt`。首批的 `CatalogPage` 未动 |
| `3dc84a8b2` | `adapters/postgres/route_plan_catalogue.go`、`reroute_suggestion_catalogue.go`、`initial_route_tie_decision.go` | 落列声明取页，`total` 与本页同一组条件；建议与并列的 `state` 在公用表表达式里导出，筛选与本页读同一列；游标位置取自扫描回来的排序列；jsonb 只对本页的行解 |
| `3dc84a8b2` | `adapters/http/query_route_plans.go`、`query_reroute_suggestions.go`、`decide_initial_route.go`、`catalogue_page.go`（新） | 先 Intake 后解码，`/route-plans` 的 `register` 也挪到 Intake 之后；坏参数答 400 带 `detail`；答复加 `page`；建议答复体多带 `recordedAt` |
| `3dc84a8b2` | `adapters/http/testdata/route_plans_initial_route.json`、`route_plans_reassessment.json` | 契约夹具多一格 `page`（`-update` 重生） |
| `3dc84a8b2` | `adapters/http/route_catalogue_pages_test.go`、`adapters/postgres/route_catalogue_pages_test.go`（新）；既有用例 | 端点与真库用例；既有用例与替身改走新签名 |
| `3dc84a8b2` | `migrations/network_routing/0034_catalogue_read_pagination_indexes.sql`（新） | 每册一条与缺省序同形的索引 |
| `3dc84a8b2` | `cmd/parcel-api/unwired_orchestration.go`（`unwiredRoutePlans` 两个方法）、`endpoints_reroute_decision_test.go`（`unwiredRerouteSuggestions`）、`endpoints_initial_route_tie_test.go`（`unwiredInitialRouteTies`） | 占位只改签名，邻行未动 |
| `4e3c13494` | 本票 | 完成记录 |
| 修订轮（本笔） | `adapters/http/query_route_plans_test.go`、`route_catalogue_pages_test.go`；`adapters/postgres/route_catalogue_pages_test.go`、`route_plan_catalogue_test.go`；`ports/catalogue_pages.go`、`adapters/http/query_route_plans.go`、`adapters/postgres/route_plan_catalogue.go`、`reroute_suggestion_catalogue.go`、`initial_route_tie_decision.go` 与 `0034` 的注释；本票 | 未配置用例补坏选择器与坏参数，配置用例补 `register=` 空值；真库新用例的夹具一律 `SYN-`，复核插行助手拆出可指定判断键的一支，复核册与建议册补「服务目的不进 q」；注释与票面不数别处、引 spec 用原句；完成记录按本笔实测重写。逐条见文末「修订记录」 |

**各册声明**（声明在 `ports/catalogue_pages.go`，落列在各读面）

| 册（路由 → 读端口方法） | 缺省序 | 可排维 | 筛选维 | q 覆盖 | 行标识 | 索引（`0034`） |
|---|---|---|---|---|---|---|
| 初始路由判断（`/route-plans?register=initial-route` → `ListInitialRoutes`） | `-recordedAt` | recordedAt | conclusion | 客户账户、委托、受理基线、包裹、计划版本 | 判断键去掉租户后余下的各段 | `initial_route_by_tenant_recorded` |
| 路由复核（`/route-plans?register=reassessment` → `ListRouteReassessments`） | `-recordedAt` | recordedAt | conclusion | 触发关联、客户账户、委托、受理基线、包裹、被复核计划 | 触发关联 | `route_reassessment_by_tenant_recorded` |
| 改路建议（`/route-reroute-suggestions` → `ListRerouteSuggestions`） | `-recordedAt` | recordedAt | state（导出列） | 同复核册 | 触发关联 | 部分索引 `route_reassessment_suggestion_by_tenant_recorded`（`suggestion IS NOT NULL`） |
| 初始路由并列（`/route-initial-route-ties` → `ListInitialRouteTies`） | `-settledAt` | settledAt | state（导出列） | 客户账户、委托、受理基线、包裹 | 同初始路由判断，包裹打头 | 部分索引 `initial_route_not_formed_tied_by_tenant_settled`（`reason = 'CANDIDATES_TIED'`） |

`register` 登为 `/route-plans` 两种册的选择器（`Spec.Selectors`），保持原义、取值进游标摘要；两种册的册名不同，别册的游标答 400。服务目的不进 q：它是目的词，不是哪个对象的标识，搜它只会命中整册。这几张表上原本没有以「租户 + 排序维」打头的索引，各条都是新增，没有换掉旧索引。排序列与标识列都非空（`recorded_at`、`settled_at` 为 `NOT NULL`，标识是主键段；`state` 只作筛选维）。

**完成判据**（钉本笔）

- ✅ 真库用例（`SYN-` 夹具，带 DSN，`adapters/postgres/route_catalogue_pages_test.go`）：
  - 每册各过一遍「翻页期间落新行」：空册答空页、`total` 为 0；落五行后两行一页地翻，翻完第一页再落一新一旧两行，翻完恰是旧五行加较旧那一行，较新那一行落在已翻过的那一端、不混进后面的页，翻页途中 `total` 为 7。
  - 「组合键各段都进行标识」：初始路由册、并列册各在同一时刻落一组行，两两只差判断键的一段，判断键里租户以外的每一段都有一对把关；一行一页翻出的次序即标识倒序，每页都核 `total`。
  - 每册一组筛选与 q 用例：维内为或、q 不分大小写、q 与筛选同时、`%` 按字面、服务目的不进 q；每条都断言 `total` 等于该条件下的行数。
- ✅ 端点用例（`adapters/http/route_catalogue_pages_test.go`，按册表驱动，每册都过）：
  - 合规的一问带 `page`，`size` 取 Intake 页大小；末页 `next` 为 `null`，空册 `total` 为 0。
  - 解码原样交读口：排序方向、维内多值、q 去首尾空白、游标往返。
  - 配置了的渠道上，自报租户、自报页大小、集外排序维、词表外筛选值、换条件后的旧游标、别册的游标、超长 q 各答 400 带 `detail`、不带 `outcome`，读口没被调用。
  - 渠道未配置时，合规与写错的查询串同答 403、答复体逐字节相同、读口没被调用；`/route-plans` 另过缺 `register` 与 `register` 不在集内。`query_route_plans_test.go` 的未配置用例另过 `register` 缺席、为空、不在集内，以及带集外键、集外排序维；它的配置用例里 `register` 缺席、为空、不在集内照旧答 400，读口没被调用。
  - 端点次序：各路由都是方法门、Intake、再读查询串；`register` 的分派与校验都在 Intake 之后（判断项「`register` 的坏请求挪到 Intake 之后」）。
- ✅ 迁移按字节无 CR、无 BOM：`grep -c $'\r'` 为 0、首三字节不是 `EF BB BF`；`./migrations/` 的行尾守卫过。
- ✅ 门（作者自验，钉本笔，WSL go1.26.8）：
  - 改过的 `.go` 文件 `gofmt -l` 为空；全仓 `go build ./...`、`go vet ./...` 退 0。
  - 带 DSN `go test -count=1 -p 1 -v`，范围是改过的包按 `go list -test -deps` 反查出的全部反向依赖，外加 `./internal/architecture/...`：69 包 ok、0 FAIL，合计 6814 PASS、1 SKIP。那一个 SKIP 是 `internal/platform/pgtest` 的 `TestHelperTemplateOwnerProcess`，它是子进程助手，平常就跳过。
  - 分包：`internal/networkrouting/adapters/postgres` 195 PASS / 0 SKIP，`adapters/http` 110 / 0，`application` 336 / 0，`ports` 10 / 0，`cmd/parcel-api` 654 / 0，`internal/architecture` 174 / 0，`internal/platform/migrate` 8 / 0，`migrations` 1 / 0。
  - 全跑之后只在 `adapters/http/query_route_plans_test.go` 的配置用例里补了 `register=` 一问，该包单独重跑（`gofmt`、`go vet`、`-v`）仍是 110 PASS / 0 SKIP；改的是测试文件，别的包不受影响。
  - 计数取证：`grep -c -- '--- PASS'` 与 `grep -c -- '--- SKIP'` 打在 `-v` 输出上，分包按 `ok` 行切段计。

**判断项**

1. **决胜键改同向**：各册迁移前都是首维倒序、标识段正序；现在首维同值的几行改按标识倒序排。第一页与今天只在首维完全相同的行之间次序不同。
2. **大册**：初始路由按包裹计量、复核按事件计量，是 spec「`total` 一律给精确值」点名的那一类；建议是复核的子集，并列按包裹计、只含停在并列的。`total` 照给精确值，超出租户级规模时回 ADR-0144 改给 `null`（ADR-0144 越权风险点 3）。也因此各册都只放出缺省序一维：另放的维没有同形索引，每取一页都要把本租户整册排一遍。
3. **管理台与脚本带的参数**：
   - 管理台 `api.ts` 带 `register=initial-route`、`register=reassessment`；`scripts/demo-seeds/demo-run/chain.sh` 与 `cmd/parcel-api` 的探针带 `register=initial-route`。`register` 登为选择器。
   - 建议与并列两个路由今天不带参数。
   - 答复只多一格 `page`（建议另多 `recordedAt`），运行时管理台照旧能用。
4. **`register` 的坏请求挪到 Intake 之后**（spec「端点先 Intake、后解码查询参数」；ADR-0055 决定三）：渠道未配置时，`register` 缺席、为空或取值不在集内从前先答 400，现在与一切请求同答 403。配置了的渠道上照旧答 400，`register=` 传空值也照旧答 400（空值从来不是哪一册），与 `family` 同为不带 `detail` 的选择器拒绝。同包的 `/network-catalog`（首批，不在本票）仍是先解码，由 15 改齐。
5. **管理台契约用例会红**：`apps/admin-web/src/pages/network/route-plans.contract.test.ts` 读本包的契约夹具、按 TS 响应类型核键集。夹具多了 `page`，读这些夹具的用例都红，报「响应体 有 TS 类型不认识的键 page」（作者在本分支上单跑该用例实测）。管理台归 14，本票不改；14 给 `/route-plans` 的响应类型补上 `page` 之前，main 上的管理台测试是红的，推送方排序时要让 14 的这一处先行或同批。
6. **建议多一格 `recordedAt`**：ADR-0144 决定三要求可排维取答复体里的字段名。建议今天只答 `suggestedAt`（住在建议 jsonb 里）与 `triggerOccurredAt`（可空），都不宜作键集列，所以取那次复核的落册时刻并在答复体里带出。管理台的建议类型要跟（归 14）。
7. **管理台的客户端缺省序与读口不同列**：复核与建议两页今天在客户端按 `reassessedAt` / `suggestedAt` 倒序排，读口的缺省序是 `recordedAt`；初始路由与并列两页与读口同列。14 接管理台时决定改按 `recordedAt` 下推，还是在这两册另放那一维（那时要补索引）。
8. **并列的排序列可变**：未形成登记同一判断键再停一次就改写那一行，`settled_at` 前移，`reason` 也可能改走。翻页期间被改写的那一行可能挪进已翻过的区间，或离开本册。其余各册的排序列落册后不变（写入都是 `ON CONFLICT DO NOTHING`）。建议与并列的 `state` 是现算的，翻页期间状态变了的行在按 `state` 筛时同理可能出入。
9. **行标识都取主键去掉租户，并列只改段序**（spec「行标识取登记表主键去掉租户那几段」）：初始路由与并列的主键都是判断键，标识取其中租户以外的各段；复核与建议的主键是租户加触发关联，标识取触发关联。并列的段集合与主键相同，段序照迁移前的决胜次序以包裹打头、不照主键列序；索引同此。
10. **改路建议用部分索引**：建议住在复核行上。复核那条同形索引也能用，但要滤掉不带建议的行；部分索引只收带建议的行，写入代价只落在带建议的复核上。
11. **没有计数队列，q 不落 JSON**：本票各册都不带计数与 `truncated`，读端口都答 `cataloguepage.Result`，没有自带 `Next` / `Total` 的结构。q 落的都是文本列；计划、候选与建议住在 jsonb 里，都不进 q。

**未验**

- 管理台未接（归 14），契约用例的红见判断项「管理台契约用例会红」；全量 `scripts/test-full.sh` 由推送方在重放后的 tip 上跑。
- 索引是否被选中没有在租户规模的数据上看过执行计划；真库用例只证次序、键集与 `total`。

## 修订记录

**修订轮 ← 派单方 · 通道 1**（对照派单说明「02 的非作者评审拦下过的」逐条自查；本票此前未经非作者评审）

| 自查项 | 处理 |
|---|---|
| 端点先 Intake，迁移前就有的选择器校验也算；未配置时坏选择器、坏参数都答 403 | 各端点在 `3dc84a8b2` 已是 Intake 在前，`register` 的分派与校验在其后，本笔无需对调；`query_route_plans_test.go` 的未配置用例原只问两种合规的 `register`，补上 `register` 缺席、为空、不在集内与集外键、集外排序维 |
| 每册真库用例：翻页途中插入新行、空册 `total` 为 0、筛选或 q 改变 `total` | `3dc84a8b2` 已有，逐册核过：插入用例每册各跑一遍、先证空册；筛选与 q 的每条都断言 `total` |
| 端点表驱动：合规一问带 `page` 且 `size` 取 Intake 页大小，集外参数 400 带 `detail`，未配置时合规与不合规都 403，换条件拿旧游标 400 | `3dc84a8b2` 已有，每册都过，旧游标每册都证 |
| 注释与票面不引 spec 的取舍序号，不数别处的东西 | 票面「做法与判据」与判断项里按序号引的取舍改引原句；声明、端点、读面与用例的注释和迁移注释里的册数、段数、表数、维数改为不计数的说法；`Covers` 引 spec 原句「端点先 Intake、后解码查询参数」；本包网络目录端点的先例不再数族 |
| 新夹具一律 `SYN-` | 真库新用例经旧插行助手落下的服务目的 `DELIVERY` 改为 `SYN-PURPOSE-1`：复核插行助手拆出可指定判断键的 `insertReassessmentRowOn`，初始路由的筛选用例改走 `insertJudgmentRow`；旧用例的夹具不动。端点用例新补的坏 `register` 用 `SYN-NOT-A-REGISTER` |
| JSON 数组上的 q 不用 `::text` | 不适用：各册 q 只落文本列（判断项「没有计数队列，q 不落 JSON」） |
| 既有参数传空值的旧行为保住 | `register=` 传空值在配置了的渠道上迁移前后都答 400，`query_route_plans_test.go` 的配置用例补上这一问；建议与并列迁移前不收参数 |
| 计数队列自带 `Next` / `Total`、行标识不取主键去租户的，写进判断项 | 本票没有计数队列；行标识都取主键去租户，并列只改段序——判断项「行标识都取主键去掉租户，并列只改段序」「没有计数队列，q 不落 JSON」 |
