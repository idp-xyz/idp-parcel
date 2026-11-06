# 15 首批两册改用 01 的补件、两份页类型并入统一结果类型

Category: enhancement
Status: in-progress——非作者评审（钉 `07463c4d9`）无阻断，非阻断已由派单方处理，待进 main（2026-11-06，分支 `cursor/crpr-15-first-batch`，worktree `idp-parcel-crpr-15`，基 `d6b3f64a5`；同日派单方裁定 `family` 校验挪到 Intake 之后，已按裁定改，`e4a8afb18`；完成记录与评审记录见文末）
Blocked by: 01
地盘：`internal/partycommercial/adapters/postgres/customer_account_catalogue.go`、`internal/networkrouting/adapters/postgres/network_catalog_list.go`，
`internal/partycommercial/ports/catalogue_page.go`、`internal/networkrouting/ports` 里的 `CatalogPage`，以及它们的调用点与测试替身。
出处：ADR-0144 决定七「一处定义」；spec「共用件的补件」。

## 做什么

1. 两册读面里手写的筛选、`q`、键集比较与次序改用 01 的落列声明，删掉两份逐字相同的转义器与取值换算。
2. `ports.CataloguePage`（party-commercial）与 `ports.CatalogPage`（network-routing）并入 01 的统一结果类型。
3. 两册端点改成先 Intake、后解码（spec「端点先 Intake、后解码查询参数」）：渠道未配置时坏参数也答 403。这是本票唯一的行为变化。

## 不做

- 不改两册的声明（可排维、筛选维、缺省序）与答复形状。

## 完成判据

- 两册既有真库用例原样通过；端点用例里「未配置渠道时坏参数先答 400」那几格改成答 403，其余原样通过；`gofmt -l` 空，`go vet` 退 0。

## 完成记录（2026-11-06，作者会话，分支 `cursor/crpr-15-first-batch`，基 `d6b3f64a5`）

**落点**

| 笔 | 文件 | 做了什么 |
|---|---|---|
| `afc6b18c6` | `internal/partycommercial/adapters/postgres/customer_account_catalogue.go` | 筛选、q、键集比较与次序改由 `customerAccountPages`（`ports.CustomerAccountCatalogue.MustBind`）+ `Plan` + `CountSQL` / `PageSQL` + `Finish` 生成；`customerAccountListed` 公用表表达式作取数来源，租户 `$1` 仍落在它的前半句里；删掉手写的落列表、`customerAccountConditions`、`accountLikeLiteral`、`accountTypedValue` 与 `customerAccountSortValue`；游标位置由 `customerAccountPosition` 从扫进行里的登记时刻、生效时刻与账户号取 |
| `afc6b18c6` | `internal/networkrouting/adapters/postgres/network_catalog_list.go` | 七族各绑一份（`catalogFamily.pages`，各取 `ports` 里本族声明的 `MustBind`），`listPage` 走 `Plan` + `CountSQL` / `PageSQL` + `Finish`；删掉 `keyColumn`、`conditions`、`likeLiteral`、`typedValue`、`codeAndVersion` 与本包的 `byEffectiveFrom`；游标位置仍由各族 `sortValues` / `identityOf` 从扫描值取 |
| `afc6b18c6` | `internal/partycommercial/ports/catalogue_page.go`、`internal/networkrouting/ports/catalog_query.go` | `CataloguePage` / `CatalogPage` 改为 `cataloguepage.Result` 的泛型类型别名；头注写明并入统一结果类型、别名只为不拆正在并行迁移的分支 |
| `afc6b18c6` | 本票面 | Status 改 in-progress |
| `39222ea07` | `internal/partycommercial/adapters/http/query_customer_accounts.go`、`internal/networkrouting/adapters/http/query_network_catalog.go` | 先 Intake、后解码；头注里「先于 Intake」的句子改成现状。这一笔 `family` 的在场与封闭集检查仍在 Intake 之前，`e4a8afb18` 按派单方裁定挪到其后 |
| `39222ea07` | `internal/partycommercial/adapters/http/query_customer_accounts_test.go`、`internal/networkrouting/adapters/http/query_network_catalog_test.go` | 「未配置渠道时坏参数先答 400」那几格改成答 403，且与写对的一问逐字节相同（网络目录用 `assertUnconfiguredCatalogueRefusal`）。`TestSelfReportedScopeParametersAreRefusedBeforeTheIntake` 改名 `TestSelfReportedScopeParametersAreRefused`；它与 `TestMalformedQueryParametersAreRefusedWithAReason` 原用「Intake 不得被问到」钉次序，改用放行的 Intake，配置了的渠道上照旧断言 400 带理由散文、读口不被触到 |
| `e4a8afb18` | `internal/networkrouting/adapters/http/query_network_catalog.go`、`internal/networkrouting/adapters/http/query_network_catalog_test.go` | `family` 的在场与封闭集检查挪到 Intake 之后，头注改成现状。`TestNetworkCatalogQueryRejectsAnAbsentOrUnknownFamily` 改用放行的 Intake：配置了的渠道上缺席、空值、未知族名仍答 400 `MALFORMED_REQUEST`，读口不被触到；未配置渠道上这三问与写对的一问逐字节同答 403（并进 `assertUnconfiguredCatalogueRefusal`）。本票改过的用例里租户值改用 `SYN-` 合成值 |

`cmd/parcel-api` 未改：两口占位写的是 `ports.CatalogPage[...]` / `ports.CataloguePage[...]`，经别名照旧编过。

**声明**：未改。`ports.CustomerAccountCatalogue` 与网络目录七族声明的缺省序、可排维、筛选维照旧，答复形状照旧。`Columns` 里的列与删掉的手写落列表逐列相同（排序列；行标识即主键去掉租户；筛选列；q 覆盖列仍以两个读面文件的头注为准），对照看 `afc6b18c6` 的 diff。排序与筛选的列都没变，没有索引迁移。

**完成判据**

- ✅ 两册既有真库用例原样通过。两个 postgres 包的测试文件未动：`git diff --stat d6b3f64a5 e4a8afb18 -- internal/partycommercial/adapters/postgres internal/networkrouting/adapters/postgres` 只列出两个读面文件。带 DSN、`-p 1 -count=1`，钉 `e4a8afb18`：`internal/partycommercial/adapters/postgres` 431 PASS / 0 SKIP，`TestCustomerAccountCatalogueShowsAllThreeLifecycleCellsAndTheDanglingParty`、`TestCustomerAccountCataloguePagesByCursorWithoutRepeatsOrGapsWhileAccountsArrive`、`TestCustomerAccountCatalogueTotalFollowsFiltersAndKeyword` 在内；`internal/networkrouting/adapters/postgres` 148 PASS / 0 SKIP，`network_catalog_list_test.go` 与 `network_catalog_page_test.go` 的用例全在内（七族各自翻页不重不漏、时刻维降序同刻按标识决胜、筛选与 q 收窄行与 total 的各子用例）。`39222ea07` 上另用 `-v` 取证一次，计数相同。
- ✅ 端点用例：客户账户「未配置渠道 + 坏参数」一格改为 403，且与好参数那一问逐字节相同。网络目录缺 `family`、空值、族名不在七族内，自报 tenant / limit，以及集外排序维、词表外筛选值、集外筛选维、别族游标、换条件后的旧游标、超长 q 各问，在未配置渠道上都与写对的一问逐字节同答 403，配置了的渠道上照旧答 400。其余原样通过：`TestUnconfiguredCatalogueIntakeRefusesAllFamiliesIdentically`、`TestCatalogueIntakeFailuresKeepTheirGrades` 等未动。反证：把两个端点换回 `d6b3f64a5` 的先解码写法，`TestCustomerAccountsEndpointRefusesQueriesOutsideTheDeclaration`、`TestSelfReportedScopeParametersAreRefused`、`TestMalformedQueryParametersAreRefusedWithAReason` 都在 400 对 403 那一格上红；把网络目录端点换回 `39222ea07` 的写法（`family` 检查在 Intake 之前），`TestNetworkCatalogQueryRejectsAnAbsentOrUnknownFamily` 在未配置那一格上红；换回即绿。
- ✅ 改动的 Go 文件 `gofmt -l` 空；全仓 `go build ./...`、`go vet ./...` 退 0。
- 门（作者自验，钉 `e4a8afb18`，WSL go1.26.8）：被改的包（两个 `ports`、两个 postgres 读面包、两个 http 端点包）加它们带测试的反向依赖，再加 `./internal/platform/cataloguepage/...`、`./internal/architecture/...`、`./migrations/`、`./internal/platform/migrate/`，带 DSN 一次 `go test -count=1 -p 1 -json`：41 个包全 ok，6183 PASS / 0 SKIP / 0 FAIL。其中 `cmd/parcel-api` 654 PASS、`cmd/parcel-dispatch` 506、`cmd/parcel-commercial` 166、`cmd/parcel-frontline-import` 107、`cmd/parcel-settlement-register` 48、`cmd/parcel-network-register` 33、`internal/architecture` 174、`internal/partycommercial/adapters/http` 303、`internal/networkrouting/adapters/http` 90、`internal/platform/cataloguepage` 130、`internal/platform/migrate` 8、`migrations` 1。反向依赖在 `e4a8afb18` 上用 `go list -test -f '{{.ImportPath}} {{join .Deps " "}}' ./...` 按被改包过滤求得；计数取证：`-json` 事件按包数 `Action` 为 pass / skip / fail 且带 `Test` 的行。

**判断项**

1. **`family` 在 Intake 之后校验。** 现状：Intake 先行，之后才读 `family`；配置了的渠道上缺席或不在七族之内答 400 `MALFORMED_REQUEST`，渠道未配置时这几问与写对的一问逐字节同答 403。理由：分派可以先于 Intake（只看参数在不在），校验（缺失、取值不在封闭集内答 400）不行——ADR-0055 决定三要未配置渠道的答复不随请求内容变，七族名公开也不构成例外；票面「其余原样」让位于 spec「端点先 Intake、后解码查询参数」。这是派单方（通道 1）的裁定，与 02 非作者评审对渠道择优 `view` 选择器判的阻断同形（02 在 `c3c226b00` 改齐）。本端点七族共用一个 Intake，网络目录的各种 Intake 都不读查询串，所以连分派也不必先于它，整段挪到 Intake 之后。作者原先把检查留在 Intake 之前（`39222ea07`），`e4a8afb18` 按裁定改。
2. **别名保留。** `ports.CataloguePage` / `ports.CatalogPage` 留作别名，调用点不改写。按 `e4a8afb18` 试合：本分支分别合入 `origin/cursor/crpr-07-network`（`4e3c13494`）、`origin/cursor/crpr-11-party`（`75a859b40`）以及两条一起，均无冲突，`go build ./...` 与 `go vet ./internal/networkrouting/... ./internal/partycommercial/... ./cmd/...` 都过；那两条分支都没碰本票改的文件。评审核过：在途各分支的新增代码没有一处用旧名，留别名的真实理由只是调用点里有几处在共享的 `cmd/parcel-api/unwired_orchestration.go`，紧挨着同批别的票改过的占位方法，现在改写会让分批进 main 撞出合并冲突。头注已改成这个理由并写明删除条件：本批各票都进 main 之后，由推送方一笔改写调用点、删掉别名。
3. **生成的 SQL 只差写法。** 网络目录的固定条件带括号 `(tenant_id = $1)`，客户账户的 WHERE 不再以 `TRUE AND` 开头；筛选、q（同一份转义）、键集比较与次序的语义与迁移前一致，证据是上面原样通过的真库用例。
4. **管理台与脚本带的参数。** 管理台对 `/network-catalog` 带 `family`（选择器）、声明内的条件与 `after`，对 `/commercial-customer-accounts` 带声明内的条件（如 `q`）与 `after`；`scripts/demo-seeds/demo-run/up.sh` 带凭据探 `?family=node`。都在声明之内，配置了的渠道上合规请求的答法不变。变了的答法有两处：未配置渠道上坏参数（含缺 `family`、族名写错）由 400 改答 403；配置了的渠道上，Intake 自己的失败（`writeCatalogueIntakeProblem` 的 401、403、503）现在排在坏参数的 400 之前。
5. **地盘外一句注释随本票部分过时。** `cmd/parcel-api/endpoints_test.go` 里 `businessEndpointProbe` 的头注说漏掉 family / registry / kind 测出的 400 会盖过未配置 Intake 的 403；family 那一项自 `e4a8afb18` 起不再成立（未配置时同答 403），registry / kind 各口归各自的迁移票。不在本票地盘，没改；各票的选择器校验都挪到 Intake 之后，这句才整句过时，由推送方在本批都进 main 之后一并改。

**未验**

- 全量 `scripts/test-full.sh` 由推送方在重放后的 tip 上跑；管理台未在浏览器里走（本票不改答复形状）。

## 评审记录

**评审 ← 子代理 · 钉 `07463c4d9`**：两轴无阻断。SQL 新旧对拍 162 例只差写法、参数逐个相等，两册既有真库用例原样通过。

| 评审意见 | 处理 |
|---|---|
| 非阻断：两个别名的头注说并行分支还写着旧名，证据不支持，也没写何时删 | 头注改成真实理由（避开共享占位文件里的合并冲突）与删除条件（本批都进 main 后由推送方删）；判断项 2 同写 |
| 非阻断：两个端点头注「对一切请求同答 403」比 ADR-0055 决定三的「一切请求内容」宽 | 改成「答复不随查询参数变，一律 403」 |
| 非阻断：判断项 1「网络目录的三种 Intake」是跨文件计数 | 改为「各种 Intake」 |
| 非阻断：判断项 4 写窄了——配置了的渠道上 Intake 的失败现在排在坏参数的 400 之前 | 判断项 4 补上 |
| 非阻断：`businessEndpointProbe` 头注的 family 一项过时，没有归属 | 判断项 5 写明由推送方在本批都进 main 之后一并改 |

修订由派单方在本分支上做，只动注释与本票面。

## 评审记录

**评审 ← 子代理 · 钉 `07463c4d9`**：两轴无阻断。评审用 `-overlay` 对拍新旧 SQL 162 例，只差 `(tenant_id = $1)` 的括号与去掉的 `TRUE AND`，参数逐个相等。

| 评审意见 | 处理 |
|---|---|
| 非阻断：别名头注说并行分支还写着旧名、删了合进来编不过，证据不支持；也没写何时删 | 头注改成真实理由（共享占位文件里改写调用点会撞冲突）与删除条件；判断项 2 同改 |
| 非阻断：两个端点头注写「对一切请求同答 403」，比 ADR-0055 决定三的「请求内容」宽（方法门的 405、读值检查的 400 都在它前面） | 改成「答复不随查询参数变」 |
| 非阻断：判断项 1「网络目录的三种 Intake」是跨文件计数 | 改为不数 |
| 非阻断：判断项 4 写窄了，配置了的渠道上 Intake 自己的失败现在也排在坏参数的 400 之前 | 判断项 4 补上 |
| 非阻断：`businessEndpointProbe` 头注的 family 一项已过时、没有归属 | 判断项 5 写明由推送方在本批都进 main 之后一并改 |

修订由派单方在本分支上做（作者会话的交回未送达），只动注释与本票面。
