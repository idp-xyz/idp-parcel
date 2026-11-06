# 13 access-identity 操作者与工作设备两册迁到 ADR-0144

Category: enhancement
Status: in-progress——作者完成，待非作者评审（分支 `cursor/crpr-13-access`，基 `3a20b2bcc`；迁移用了预留的 access-identity `0041`，进 main 时按 tip 重核）
Blocked by: 01
地盘：`internal/accessidentity` 里 `ListOperators`、`ListDevices` 的端口、postgres 实现与两个端点及其测试；`migrations/access_identity` 的一条新迁移（索引）。
出处：ADR-0144 决定三至七；spec「横贯各子票的取舍」。

## 本票的册（钉 `a1274a6b8`）

| 路由 | 读方法 | 今天 |
|---|---|---|
| `/operators` | `OperatorCatalogue.ListOperators` | 不设上限，一条查询取全部操作者连同授予与撤销（ADR-0363 决定二） |
| `/work-devices` | `DeviceCatalogue.ListDevices` | 不设上限 |

这两册今天的问题与其他各册相反：不截断，而是一次全取；租户上了量（快递员、场站人员与手持设备都以千计）时，一页就是全量。

## 做法与判据

见 spec「每张迁移子票（02–13）的做法与判据」。本票特有：

- 开工先读 ADR-0363 决定二：分页后仍须一条查询取「本页操作者连同其授予与撤销」，绑定与授予不取自两个时点；写不通就回 ADR 改，不在代码里另立口径。
- 这两口今天没有 `limit`：页大小按操作者渠道契约取，与各上下文操作者 Intake 同值，写明出处。

## 完成记录

### 落点

| 笔 | 文件 | 做了什么 |
|---|---|---|
| `a3f299094` | `internal/accessidentity/catalogue_pages.go`（新） | 声明两册。本上下文没有 `ports` 子包，端口在包根，声明随端口放 |
| | `internal/accessidentity/operator_catalogue.go`、`device.go` | `OperatorCatalogue.ListOperators`、`DeviceCatalogue.ListDevices` 改收 `limit` 与 `cataloguepage.Query`、答 `cataloguepage.Result[Row]`；租户仍在签名上，`limit` 非正拒；`FindGrant` 不动 |
| | `internal/accessidentity/adapters/postgres/operator_catalogue.go` | 一条查询取本页：操作者表上带 `LIMIT` 的子查询取本页操作者，授予与撤销在同一句里左连接（ADR-0363 决定二）；总数另起一句、同一组条件；游标位置取扫描回来的列值 |
| | `internal/accessidentity/adapters/postgres/device_registry.go` | 按设备标识取页；状态由停用表导出（`CASE`，词取 `DeviceCatalogueStatus` 的常量）作筛选维；总数一句、本页一句 |
| | `internal/accessidentity/adapters/http/catalogue_page.go`（新）、`query_work_devices.go`、`operator_access_administration.go` | 认证之后按声明解码，坏参数答 400 `MALFORMED_REQUEST` 带 `detail`（本包新增 `writeProblemWithDetail`、`problemDetail.Detail` 带 `omitempty`）；两册答复加 `page`；页大小 `operatorCatalogueReadLimit` |
| | `cmd/parcel-api/read_target_guard.go` | `/work-devices`、`/operators` 移出 `targetCheckExemptions`；`unstorableTargetRefusal` 的旧形状一格加 `faceOperatorAccessAdministration` |
| | `cmd/parcel-api/unstorable_read_targets_test.go`、`assemble_work_device_catalogue_test.go`、`assemble_operator_access_administration_test.go` | 豁免对照用例去掉这两口；真库读值用例加 `/work-devices?q=`；装配用例替身改签名，加「自报 `tenant` 认证后答 400」一格 |
| | `migrations/access_identity/0041_catalogue_read_pagination_indexes.sql`（新） | 操作者册一条与缺省序同形的索引 |
| | 两个适配器包的 `*_test.go` | 旧用例改调用点（`firstPage`/`rowsOf`）；新增真库与端点用例 |
| 本笔 | 本票面 | 完成记录与 Status |

### 声明

页大小 200：`identityhttp` 的 `operatorCatalogueReadLimit`。出处是 ADR-0144 决定二（页大小由 Intake 按渠道契约定，调用方不给）；本包没有操作者查阅 Intake，常量定在端点包，与各上下文操作者查阅 Intake 的同名常量同值（注释点名 collectionremittance 那一份）。

| 册（路由） | 缺省序 | 可排维 | 行标识 | 筛选维 | `q` 列 | 索引 |
|---|---|---|---|---|---|---|
| `operators`（`/operators`） | `recordedAt` 正序 | `recordedAt` | （发行方、主体），即 `operator_pkey` | 无 | 主体标识、绑定依据 | 新建 `operator_by_tenant_recorded (tenant_id, recorded_at, issuer, subject)`（迁移 `0041`） |
| `work-devices`（`/work-devices`） | `deviceId` 正序 | `deviceId` | 设备标识 | `status`：`REGISTERED` / `DEACTIVATED` | 设备标识、作业范围引用 | 沿用 `device_pkey (tenant_id, device_id)`，不另建 |

### 完成判据

- ✅ 翻页期间插入新行不重出、不漏行，空册答空页 `total` 为 0：`TestOperatorCataloguePagesByCursorWithoutRepeatsOrGapsWhileOperatorsRegister`——翻完第一页后在已翻那头落 `P00`、未翻那头落 `P06`，第二页接着 `P03`、`P04`，末页 `P05`、`P06`，`total` 5 → 7；本页每个操作者的授予各在各的名下，`P03` 那笔带撤销。`TestDeviceCataloguePagesByCursorWithoutRepeatsOrGapsWhileDevicesRegister` 同一做法，新行经登记口登。
- ✅ ADR-0363 决定二：本页操作者连同授予与撤销出自同一句（落点表）；授予的种类、作业范围、终点与撤销随行回来仍由 `TestListOperatorsListsTheTenantInRecordedOrder` 钉着。
- ✅ `total` 随筛选与 `q` 变、`%` 与 `_` 按字面：操作者册 `TestOperatorCatalogueTotalFollowsTheKeyword`（主体与绑定依据各自命中、不分大小写，带 `q` 跨页）；作业设备册在上一条用例里逐条件一行一页走完——`status` 单值、双值、筛选下倒序，`q` 命中设备标识、命中作业范围引用，筛选与 `q` 并用——每页 `total` 都等于该条件下的行数。操作者册没有筛选维，「随筛选变」一格不适用。
- ✅ 决胜键同向、跨页续得上：`TestOperatorCatalogueBreaksTiesOnIssuerAndSubjectInTheSortDirection`（同刻四个操作者跨两个发行方，一行一页，正倒序各走完全册）。作业设备册的行标识就是排序维，没有同值。
- ✅ 端点：未知键（`tenant`、`limit`）、集外排序维（设备册 `-recordedAt`、操作者册 `subject`）、操作者册未声明的 `status`、设备册词表外的 `status=RETIRED`、解不开的游标、换 `q` 拿旧游标、拿另一册的游标，各答 400 带 `detail`，读口不被调用；渠道未配置时参数写对写错同答 403、答复体逐字相同；两册的 `page` 三格齐、`size` 为 200，解出的查询原样交给读口：`TestCataloguesRefuseQueriesOutsideTheDeclaration`、`TestCataloguesCarryThePageAndPassTheDecodedQuery`。生产装配上：`TestWorkDeviceCatalogueAnswersFromTheOperatorChannel`、`TestOperatorAccessAdministrationAnswersFromTheOperatorChannel` 的 `self-reported tenant is outside the query set`。
- ✅ 迁移按字节无 CR、无 BOM：`0041` 与票面已量。
- ✅ 自验：`go build ./...`、`go vet ./...` 全仓过。`go test -count=1 -p 1 -v <包>`（带 `IDP_PARCEL_POSTGRES_DSN`）逐包数 `grep -c -- '--- PASS'` 与 `'--- SKIP'`（PASS/SKIP，均 FAIL 0）：
  `cmd/parcel-api` 656/0、`cmd/parcel-access-register` 15/0、`internal/architecture` 174/0、`internal/accessidentity` 307/0、`.../adapters/http` 83/0、
  `.../adapters/postgres` 65/0、`.../adapters/oidc` 81/0、`.../adapters/pilotgovernance` 2/0；各上下文的 `adapters/accessidentity`：customscompliance 18/0、
  networkrouting 9/0、nodeoperations 48/0、parcelpricing 15/0、parcelshipment 26/0、partycommercial 8/0、settlementaccounting 4/0、transportfulfillment 21/0、
  visibilityexception 23/0；`internal/platform/migrate` 8/0、`migrations` 1/0。`.../adapters/registrationjson`、collectionremittance 与 pilotgovernance 的
  `adapters/accessidentity` 没有测试文件。反向依赖由 `go list -f '{{.ImportPath}} {{join .Deps " "}} {{join .TestImports " "}} {{join .XTestImports " "}}' ./...`
  反查含 `internal/accessidentity`、`.../adapters/http`、`.../adapters/postgres` 者。

### 判断项

- **缺省序是正序，不是 `-registeredAt`**：两册迁移前都按正序上列（操作者按登记时刻，设备按设备标识），取舍 1 保今天的次序；声明头注写了理由。
- **操作者册决胜键变宽**：迁移前同刻只按主体正序，迁移后按（发行方、主体）正序。只在登记时刻完全相同、且发行方不同时，次序与迁移前不同。
- **授予在操作者名下的次序不随 `sort`**：照旧按登记时刻、同刻按授予标识正序；操作者倒序翻页时授予仍是正序。
- **一句取页的边界**：子查询为判断「后面还有」多取第 limit+1 个操作者，它的授予随它一起被丢弃。总数另起一句，与本页同一组条件，不保证同一快照——决定二要同一时点的是一个操作者的绑定与授予，不是页与总数。
- **操作者册 `q` 只到主体标识与绑定依据**：授予标识、能力面与限定词挂在操作者名下，不是这一册的列，不进 `q`。今天管理台前端的检索还匹配授予标识、能力面及其标签——14 把检索改接 `q` 时这几项会搜不到，要在 14 里定：只搜主体与依据，或回本票加子表 `EXISTS`。
- **作业设备册的 `status` 是导出列**：设备表没有状态列，由停用表左连接导出；可排维与行标识都不碰可为空的列（取舍 6）。这个表达式上没有索引，筛选走 `device_pkey` 的租户前缀，设备按租户以千计。
- **自报 `tenant` 从「不理会」变成 400**：迁移前两口对查询串上的 `tenant` 不理会；迁移后它是集外键，认证过了答 400，认证不过照旧答 401 / 403。
- **读值检查的豁免撤掉**：迁移后两口把 `q`、`after`、`status` 带进库，NUL 或非法 UTF-8 会在库里答 500，所以移出豁免表、由检查答旧形状 400。`/operators` 所在的「操作者接入管理」族是头一次有挂检查的 GET 行，登为旧形状（它的处理器写的坏请求确是 `{"error":{"code"}}`）。`/operator-binding`、`/operator-grants` 仍豁免。
- **页大小是两处常量**：本包的 `operatorCatalogueReadLimit` 与别的上下文 Intake 里的同名常量各是一份，日后改渠道契约要一起改。
- **不是按包裹或事件计量的大册**：操作者与设备按租户以千计，`total` 给精确值。
- **管理台与脚本带的参数**：`apps/admin-web/src/pages/work-devices/api.ts`、`operator-access/api.ts` 都不带查询参数；`scripts/` 不调这两口；`cmd/` 里只有装配用例带 `?tenant=`（已改，见上）与读值检查的探针。不需要登选择器。

### 未验

- 没有跑 `EXPLAIN` 看 `operator_by_tenant_recorded` 是否被缺省序选用、`status` 筛选在千级设备上的代价；只由语句与索引同形支撑。
- 共用件 `cataloguepage` 校验游标里的 Text 值时不拒 NUL，游标摘要也不带密钥：改写一个真游标、把值换成 `\u0000`，NUL 就能进 SQL、在库里答 500（读值检查只看查询串原文，看不到 base64 里面）。凡排序维或行标识有 Text 段的册都一样，不只本票；不在本票地盘，没改也没写用例。
- 管理台不改（归 14）：今天它只取第一页（页大小 200）。
