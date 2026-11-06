# 13 access-identity 操作者与工作设备两册迁到 ADR-0144

Category: enhancement
Status: resolved——2026-11-06 进 main（`098767b1e`），记录见文末「进 main 记录」
Blocked by: 01
地盘：`internal/accessidentity` 里 `ListOperators`、`ListDevices` 的端口、postgres 实现与两个端点及其测试；`migrations/access_identity` 的一条新迁移（索引）。
出处：ADR-0144 决定三至七；spec「横贯各子票的取舍」。

## 本票的册（钉 `a1274a6b8`）

| 路由 | 读方法 | 今天 |
|---|---|---|
| `/operators` | `OperatorCatalogue.ListOperators` | 不设上限；交回操作者连同其授予与撤销（ADR-0363 决定二），今天一条查询取全部 |
| `/work-devices` | `DeviceCatalogue.ListDevices` | 不设上限 |

这两册今天的问题与其他各册相反：不截断，而是一次全取；租户上了量（快递员、场站人员与手持设备都以千计）时，一页就是全量。

## 做法与判据

见 spec「每张迁移子票（02–13）的做法与判据」。本票特有：

- 开工先读 ADR-0363 决定二（`GET /operators` 交回操作者连同其授予与撤销）。分页之后，一页里的绑定、授予与撤销仍应是册上同一刻的样子——今天靠一条查询做到；这是读面自己的理由，不是 ADR 的原文，分页写法要保住它。
- 这两口今天没有 `limit`：页大小按操作者渠道契约取（ADR-0144 决定二），写明出处。

## 完成记录

### 落点

| 笔 | 文件 | 做了什么 |
|---|---|---|
| `a3f299094` | `internal/accessidentity/catalogue_pages.go`（新） | 声明两册。本上下文没有 `ports` 子包，端口在包根，声明随端口放 |
| | `internal/accessidentity/operator_catalogue.go`、`device.go` | `OperatorCatalogue.ListOperators`、`DeviceCatalogue.ListDevices` 改收 `limit` 与 `cataloguepage.Query`、答 `cataloguepage.Result[Row]`；租户仍在签名上，`limit` 非正拒；`FindGrant` 不动 |
| | `internal/accessidentity/adapters/postgres/operator_catalogue.go` | 一条查询取本页：操作者表上带 `LIMIT` 的子查询取本页操作者，授予与撤销在同一句里左连接（一页须是册上同一刻的样子，理由在 `accessidentity.OperatorCatalogue` 头注）；总数另起一句、同一组条件；游标位置取扫描回来的列值 |
| | `internal/accessidentity/adapters/postgres/device_registry.go` | 按设备标识取页；状态由停用表导出（`CASE`，词取 `DeviceCatalogueStatus` 的常量）作筛选维；总数一句、本页一句 |
| | `internal/accessidentity/adapters/http/catalogue_page.go`（新）、`query_work_devices.go`、`operator_access_administration.go` | 认证之后按声明解码，坏参数答 400 `MALFORMED_REQUEST` 带 `detail`（本包新增 `writeProblemWithDetail`、`problemDetail.Detail` 带 `omitempty`）；两册答复加 `page`；页大小 `operatorCatalogueReadLimit` |
| | `cmd/parcel-api/read_target_guard.go` | `/work-devices`、`/operators` 移出 `targetCheckExemptions`；`unstorableTargetRefusal` 的旧形状一格加 `faceOperatorAccessAdministration` |
| | `cmd/parcel-api/unstorable_read_targets_test.go`、`assemble_work_device_catalogue_test.go`、`assemble_operator_access_administration_test.go` | 豁免对照用例去掉这两口；真库读值用例加 `/work-devices?q=`；装配用例替身改签名，加「自报 `tenant` 认证后答 400」一格 |
| | `migrations/access_identity/0041_catalogue_read_pagination_indexes.sql`（新） | 操作者册一条与缺省序同形的索引 |
| | 两个适配器包的 `*_test.go` | 旧用例改调用点（`firstPage`/`rowsOf`）；新增真库与端点用例 |
| `8cfaed3cd` | 本票面 | 完成记录与 Status |
| `785f6ec08` | `internal/accessidentity/catalogue_pages.go`、`adapters/postgres/operator_catalogue.go`、`migrations/access_identity/0041_catalogue_read_pagination_indexes.sql` | 自查轮：注释引 spec 原句，不引取舍序号；去掉数别处东西的说法（主键列数、可排维个数） |
| | `adapters/postgres/catalogue_page_test.go`、`adapters/http/catalogue_page_test.go` | 决胜用例新加的发行方换成 `SYN-ISSUER-A` / `SYN-ISSUER-B`（同形，次序不随库的排序规则变）；端点用例游标里的发行方同改；端点用例的 spec 引用改成原句，`pageOf` 头注点名 `size`、`next`、`total` |
| `7887a089c` | 本票面 | 自查轮的完成记录 |
| `f433c4750` | `internal/accessidentity/operator_catalogue.go`、`adapters/postgres/operator_catalogue.go`、`adapters/http/catalogue_page.go` | 评审轮，只改注释：ADR-0363 决定二只引交回什么，「同一刻」写成读面自己的理由；页大小注释只说出处 |
| 本笔 | 本票面 | 评审记录与 Status |

### 声明

页大小 200：`identityhttp` 的 `operatorCatalogueReadLimit`。出处是 ADR-0144 决定二（页大小由 Intake 按渠道契约定，调用方不给）；本包没有操作者查阅 Intake，常量定在端点包，注释只写出处。迁移前各上下文的操作者查阅 Intake 就各持一份 200，见评审记录。

| 册（路由） | 缺省序 | 可排维 | 行标识 | 筛选维 | `q` 列 | 索引 |
|---|---|---|---|---|---|---|
| `operators`（`/operators`） | `recordedAt` 正序 | `recordedAt` | （发行方、主体），即 `operator_pkey` | 无 | 主体标识、绑定依据 | 新建 `operator_by_tenant_recorded (tenant_id, recorded_at, issuer, subject)`（迁移 `0041`） |
| `work-devices`（`/work-devices`） | `deviceId` 正序 | `deviceId` | 设备标识 | `status`：`REGISTERED` / `DEACTIVATED` | 设备标识、作业范围引用 | 沿用 `device_pkey (tenant_id, device_id)`，不另建 |

### 完成判据

- ✅ 翻页期间插入新行不重出、不漏行，空册答空页 `total` 为 0：`TestOperatorCataloguePagesByCursorWithoutRepeatsOrGapsWhileOperatorsRegister`——翻完第一页后在已翻那头落 `P00`、未翻那头落 `P06`，第二页接着 `P03`、`P04`，末页 `P05`、`P06`，`total` 5 → 7；本页每个操作者的授予各在各的名下，`P03` 那笔带撤销。`TestDeviceCataloguePagesByCursorWithoutRepeatsOrGapsWhileDevicesRegister` 同一做法，新行经登记口登。
- ✅ 交回本页操作者连同授予与撤销（ADR-0363 决定二），三者出自同一句（落点表；为什么非一句不可是读面自己的理由，在 `accessidentity.OperatorCatalogue` 头注）；授予的种类、作业范围、终点与撤销随行回来仍由 `TestListOperatorsListsTheTenantInRecordedOrder` 钉着。
- ✅ `total` 随筛选与 `q` 变、`%` 与 `_` 按字面：操作者册 `TestOperatorCatalogueTotalFollowsTheKeyword`（主体与绑定依据各自命中、不分大小写，带 `q` 跨页）；作业设备册在 `TestDeviceCataloguePagesByCursorWithoutRepeatsOrGapsWhileDevicesRegister` 里逐条件一行一页走完——`status` 单值、双值、筛选下倒序，`q` 命中设备标识、命中作业范围引用，筛选与 `q` 并用——每页 `total` 都等于该条件下的行数。操作者册没有筛选维，「随筛选变」一格不适用。
- ✅ 决胜键同向、跨页续得上：`TestOperatorCatalogueBreaksTiesOnIssuerAndSubjectInTheSortDirection`（同刻的一组操作者跨了 `SYN-ISSUER-A`、`SYN-ISSUER-B` 两个发行方，一行一页，正倒序各走完全册，先按发行方决胜）。作业设备册的行标识就是排序维，没有同值。
- ✅ 端点：未知键（`tenant`、`limit`）、集外排序维（设备册 `-recordedAt`、操作者册 `subject`）、操作者册未声明的 `status`、设备册词表外的 `status=RETIRED`、解不开的游标、换 `q` 拿旧游标、拿另一册的游标，各答 400 带 `detail`，读口不被调用；渠道未配置时参数写对写错同答 403、答复体逐字相同；两册的 `page` 带齐 `size`、`next`、`total`，`size` 为 200，解出的查询原样交给读口：`TestCataloguesRefuseQueriesOutsideTheDeclaration`、`TestCataloguesCarryThePageAndPassTheDecodedQuery`。生产装配上：`TestWorkDeviceCatalogueAnswersFromTheOperatorChannel`、`TestOperatorAccessAdministrationAnswersFromTheOperatorChannel` 的 `self-reported tenant is outside the query set`。
- ✅ 迁移按字节无 CR、无 BOM：`0041` 与票面已量。
- ✅ 自验（自查轮在 `785f6ec08` 上重跑，计数与上一轮相同：本轮只改注释与夹具取值，没有增减用例）：`go build ./...`、`go vet ./...` 全仓过。`go test -count=1 -p 1 -v <包>`（带 `IDP_PARCEL_POSTGRES_DSN`）逐包数 `grep -c -- '--- PASS'` 与 `'--- SKIP'`（PASS/SKIP，均 FAIL 0）：
  `cmd/parcel-api` 656/0、`cmd/parcel-access-register` 15/0、`internal/architecture` 174/0、`internal/accessidentity` 307/0、`.../adapters/http` 83/0、
  `.../adapters/postgres` 65/0、`.../adapters/oidc` 81/0、`.../adapters/pilotgovernance` 2/0；各上下文的 `adapters/accessidentity`：customscompliance 18/0、
  networkrouting 9/0、nodeoperations 48/0、parcelpricing 15/0、parcelshipment 26/0、partycommercial 8/0、settlementaccounting 4/0、transportfulfillment 21/0、
  visibilityexception 23/0；`internal/platform/migrate` 8/0、`migrations` 1/0。`.../adapters/registrationjson`、collectionremittance 与 pilotgovernance 的
  `adapters/accessidentity` 没有测试文件。反向依赖由 `go list -f '{{.ImportPath}} {{join .Deps " "}} {{join .TestImports " "}} {{join .XTestImports " "}}' ./...`
  反查含 `internal/accessidentity`、`.../adapters/http`、`.../adapters/postgres` 者。
- ✅ 自验（评审轮，在 `f433c4750` 上；只改注释，没有增减用例）：改过的 `.go` 跑过 `gofmt -w`，`gofmt -l` 为空；`go build ./...` 全仓过；
  `go vet ./internal/accessidentity/...` 过。`internal/accessidentity` 各包带 `IDP_PARCEL_POSTGRES_DSN` 以 `go test -count=1 -p 1 -v <包>` 重跑（PASS/SKIP，均 FAIL 0）：
  `internal/accessidentity` 307/0、`.../adapters/http` 83/0、`.../adapters/postgres` 65/0、`.../adapters/oidc` 81/0、`.../adapters/pilotgovernance` 2/0，
  与自查轮相同；`.../adapters/registrationjson` 没有测试文件。

### 判断项

- **缺省序是正序，不是 `-registeredAt`**：两册迁移前都按正序上列（操作者按登记时刻，设备按设备标识），照 spec「缺省序保今天的次序」保住；声明头注写了理由。
- **操作者册决胜键变宽**：迁移前同刻只按主体正序，迁移后按（发行方、主体）正序。只在登记时刻完全相同、且发行方不同时，次序与迁移前不同。
- **授予在操作者名下的次序不随 `sort`**：照旧按登记时刻、同刻按授予标识正序；操作者倒序翻页时授予仍是正序。
- **一句取页的边界**：子查询为判断「后面还有」多取第 limit+1 个操作者，它的授予随它一起被丢弃。总数另起一句，与本页同一组条件，不保证同一快照——要同一刻的是一页里的绑定、授予与撤销（读面自己的理由，在 `accessidentity.OperatorCatalogue` 头注；ADR-0363 决定二只定交回什么），不是页与总数。
- **操作者册 `q` 只到主体标识与绑定依据**：授予标识、能力面与限定词挂在操作者名下，不是这一册的列，不进 `q`。今天管理台前端的检索还匹配授予标识、能力面及其标签——14 把检索改接 `q` 时这几项会搜不到，要在 14 里定：只搜主体与依据，或回本票加子表 `EXISTS`。
- **作业设备册的 `status` 是导出列**：设备表没有状态列，由停用表左连接导出；可排维与行标识都不碰可为空的列（spec「可排维与行标识的列必须非空」）。这个表达式上没有索引，筛选走 `device_pkey` 的租户前缀，设备按租户以千计。
- **自报 `tenant` 从「不理会」变成 400**：迁移前两口对查询串上的 `tenant` 不理会；迁移后它是集外键，认证过了答 400，认证不过照旧答 401 / 403。
- **读值检查的豁免撤掉**：`targetCheckExemptions` 头注的入表条件对这两口都不再成立——迁移后两口把 `q` 与游标 `after` 里的值带进库（`/work-devices` 还有 `status`），坏请求分格里也有了 `MALFORMED_REQUEST`（`writePageQueryProblem`），所以移出豁免表、由检查答旧形状 400。`/operators` 所在的「操作者接入管理」族是头一次有挂检查的 GET 行，登为旧形状（它的处理器写的坏请求确是 `{"error":{"code"}}`）。`/operator-binding`、`/operator-grants` 仍豁免。
- **页大小不止一份常量**：迁移前各上下文的操作者查阅 Intake 就各持一份 200；本包没有那样的 Intake，另定 `operatorCatalogueReadLimit`，注释只写出处（ADR-0144 决定二）。本票不动 Intake，收归一处属另一件事（见评审记录）。
- **不是按包裹或事件计量的大册**：操作者与设备按租户以千计，`total` 给精确值。
- **管理台与脚本带的参数**：`apps/admin-web/src/pages/work-devices/api.ts`、`operator-access/api.ts` 都不带查询参数；`scripts/` 不调这两口；`cmd/` 里只有装配用例带 `?tenant=`（已改，见落点表）与读值检查的探针。不需要登选择器。

### 自查轮

派单方通道 1 按派单说明「02 的非作者评审拦下过的」逐条核。有缺已补：注释与票面引 spec 的取舍序号、数别处的东西；决胜用例新加的发行方不是 `SYN-`（`785f6ec08`、`7887a089c`）。其余无缺：

- 每册真库用例都有翻页途中插入、空册 `total` 为 0、筛选或 `q` 改变 `total`（操作者册没有筛选维，走 `q`）。
- 端点表驱动：合规一问带 `page`、`size` 取操作者渠道页大小 200；集外参数答 400 带 `detail`；未配置时合规与不合规的一问同答 403；换 `q` 拿旧游标答 400。
- `q` 落的列都是 `text`（主体标识、绑定依据、设备标识、作业范围引用），没有 JSON 数组，也没有 `::text`。
- 既有参数传空值：迁移前两口不读查询参数，没有空值行为可保；自报 `tenant` 改答 400 已在判断项。
- 偏离：操作者册行标识就是 `operator_pkey`，那条主键本就不含租户，与 spec「行标识取登记表主键去掉租户那几段」一致；作业设备册取 `device_pkey` 去掉租户。两册可排维都只放缺省那一维，没有多放。
- `cmd/parcel-api/read_target_guard.go` 的改动照派单方说明留给评审，不回退；游标 NUL 缺口（见未验）归 01。

### 未验

- 没有跑 `EXPLAIN` 看 `operator_by_tenant_recorded` 是否被缺省序选用、`status` 筛选在千级设备上的代价；只由语句与索引同形支撑。
- 游标里的 NUL（改写一个真游标、把 Text 值换成 `\u0000`；读值检查只看查询串原文，看不到 base64 里面）已由 01 补：分支 `cursor/catalogue-pagination-rollout` 的 `672247e38` 让共用件 `cataloguepage` 对 `q`、筛选值与游标文本段拒库存不下的文本。本分支基于 `3a20b2bcc`，不含那一笔；本票没有另写用例。
- 管理台不改（归 14）：今天它只取第一页（页大小 200）。

## 评审记录

非作者评审钉 `7887a089c`，无阻断。非阻断意见与处理：

| 评审意见 | 处理 |
|---|---|
| ADR-0363 归属不实：`accessidentity.OperatorCatalogue` 的 doc、postgres 侧 `ListOperators` 头注与票面判断项，把「同一条查询、绑定与授予不取自两个时点」归给 ADR-0363 决定二；决定二原文只说 `GET /operators` 交回操作者及其授予与撤销、外加词表与发行方，没有时点或一条查询的字样 | 已改（`f433c4750` 与本笔）：ADR-0363 决定二只引交回什么；「一页里的绑定、授予与撤销须是册上同一刻的样子」写成读面自己的理由，放在 `OperatorCatalogue` 头注——分几次读，两次之间落下的登记或撤销只被其中一次看见，页上就会出现册上哪一刻都没有过的组合。postgres 侧头注与票面的落点表、判据、判断项改指它。票首「本票的册」与「做法与判据」是派单原文，也把一条查询归给决定二，本笔没动；派单方随后在下一笔改正 |
| 撤豁免的理由「会在库里答 500」与未验里「游标 NUL 缺口归 01」已过时：01 在分支 `cursor/catalogue-pagination-rollout` 的 `672247e38` 让共用件对 `q`、筛选值与游标文本段拒库存不下的文本 | 已改（本笔）：撤豁免改以 `targetCheckExemptions` 头注的入表条件不再成立为据——两口把查询值带进库，坏请求分格里也有了 `MALFORMED_REQUEST`；未验那一条改为已由 01 补 |
| 页大小常量是无人守的副本 | 迁移前各上下文的操作者查阅 Intake 就各持一份 200；本票不动 Intake，收归一处属另一件事。`operatorCatalogueReadLimit` 的注释去掉对别的包常量的同值断言，只说出处：ADR-0144 决定二，页大小归渠道契约（`f433c4750`）。「做法与判据」派单原文要的「与各上下文操作者 Intake 同值」照本条不再写进注释；派单原文那句也已由派单方在下一笔删去 |

## 进 main 记录（2026-11-06，通道 1 推送）

**出处**：分支 `cursor/crpr-13-access`，tip `69f2e39af`。**评审**：钉 `7887a089c` 两轴无阻断。

**main 上的 SHA**：`098767b1e`（重放到当时的 `origin/main` `0874218c9` 之上：各分支逐笔 cherry-pick（带 `-x`，出处在提交说明里），推送方补的几笔见下）。

| 步骤 | 结果 |
|---|---|
| 重放 | 01/02 分支与 03–13、15、14a 各分支共 106 笔，无冲突 |
| 推送方补的几笔 | `de1fb75d8` AT-TF-333 改指新用例名；`66a689cbc` `cmd/parcel-api` 三处注释改成现状；`29af46c24` 删掉 15 留的两个别名；`6aff3149a` 管理台路线计划契约用例认 `page`；`446252dc4` 重生成机制清点；`098767b1e` 01 与 05 叠合处的用例 |
| 生成物 | `go run ./tools/adr-index -check`、`go run ./tools/at-coverage-counts -check` 在 tip 上退 0；机制清点在 `446252dc4` 重生成，tip 上再生成无差异 |
| 全量真库测试 | `scripts/test-full.sh` 对 `098767b1e` 退 0（175 个包 ok、17 个包无测试文件）。前一跑对 `446252dc4` 红一条：01 的 NUL 补件与 05 的挡回册判据用例叠合，`098767b1e` 修后重跑全绿 |
| 种子灌一次性库 | 升级路径：`0874218c9` 的代码灌库 A，本批代码对库 A 迁移退 0（施加 11 条新迁移）；本批代码灌空库 B 退 0；两库用后删 |
| 管理台三道门 | tsc 退 0、run-tests 1236 条全过、vite build 退 0 |
| 推确切 SHA | `git push origin 098767b1e:main`，`origin/main` 等于 `098767b1e` |
