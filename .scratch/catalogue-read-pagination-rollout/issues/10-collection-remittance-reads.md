# 10 collection-remittance 一册迁到 ADR-0144

Category: enhancement
Status: in-progress——修订轮：非作者评审（钉 `42622b45c`）拦下的阻断已修，待复核（分支 `cursor/crpr-10-collection`，基 `3a20b2bcc`；预留的 collection-remittance `0005` 未用，见完成记录；评审记录见文末）
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

## 完成记录

### 落点

| 笔 | 文件 | 做了什么 |
|---|---|---|
| `1b24a543e` | `internal/collectionremittance/ports/catalogue_pages.go`（新） | 声明 `CodSubledgerPages` |
| | `internal/collectionremittance/ports/catalogue_read.go` | `CodSubledgerCatalogueRead.ListCodSubledgers` 改收 `cataloguepage.Query`、答 `cataloguepage.Result[CodSubledgerCatalogueRow]`；`limit` 非正仍拒 |
| | `internal/collectionremittance/adapters/postgres/cod_subledger_catalogue.go` | 落列 `codSubledgerPages`；本页先在开立面上取出（带 `LIMIT` 的子查询 `FROM (<取页>) AS s`），余额侧连接与批次子查询只随本页行算；`total` 另起一句、同一组条件；游标位置取扫描回来的四维键与 `opened_at` |
| | `internal/collectionremittance/adapters/http/query_cod_subledgers.go`、`catalogue_page.go`（新） | 端点先 Intake、后按声明解码；坏参数答 400 `MALFORMED_REQUEST` 带 `detail`（本包新增 `writeProblemWithDetail`、`problemDetail.Detail` 带 `omitempty`）；答复加 `page` |
| | `cmd/parcel-api/unwired_orchestration.go` | 只改 `unwiredCodSubledgers.ListCodSubledgers` 的签名 |
| | `adapters/http` 与 `adapters/postgres` 的 `*_test.go` | 旧用例改调用点（`firstPage`/`rowsOf`）；新增真库与端点用例；`?tenant=TENANT-9&limit=9999` 那一问改断言 400 不带行，再补一问不带参数照答本租户 |
| `42622b45c` | 本票面 | 完成记录与 Status |
| 评审修订（本笔） | `internal/collectionremittance/ports/catalogue_pages.go` | 文件头注与 `CodSubledgerPages` 头注引 spec 的取舍改引规则原名；删 `openedAt` 可排维，头注写明为什么只放缺省序这一维 |
| | `internal/collectionremittance/adapters/postgres/cod_subledger_catalogue.go` | 落列删 `openedAt`；`codSubledgerPosition` 删按开立时刻取值的分支 |
| | `.../adapters/postgres/cod_subledger_catalogue_page_test.go` | 删「开立时刻倒序」「开立时刻正序」子用例与只为它们写的播种助手 `seedSubledgerOpenedAt` |
| | `.../adapters/http/query_cod_subledgers_page_test.go` | 带 `sort` 的合规一问（含渠道未配置时的那一问）由 `-openedAt` 改为 `-customer` |
| | 本票面 | Status；声明、判据、判断项与门的计数按实际改写；加评审记录 |

### 声明

| 册 | 缺省序 | 可排维 | 筛选维 | 行标识 | `q` 列 | 索引 |
|---|---|---|---|---|---|---|
| `collection-subledgers` | `customer`（正序） | 只 `customer` | 无 | 客户、责任法人、币种、代收渠道（主键去租户） | 四维键与保管依据 `custody_basis_ref` | 不新建：主键 `subledger_pkey (tenant_id, customer_ref, legal_entity_ref, currency, channel_ref)` 即与缺省序同形（spec「每迁一册补一条与缺省序同形的索引」：已有同形的写明是哪一条、不重建），预留的 `0005` 未用 |

可排维与行标识落的列均 `NOT NULL`（迁移 0001 开立面），照 spec「可排维与行标识的列必须非空」不必在读面里另导非空列。

### 完成判据

- ✅ 翻页期间插入新行不重出、不漏行：`TestCodSubledgersPageByCursorWithoutRepeatsOrGapsWhileLedgersOpen`——第一页后新开立一本排在已翻那头的账，第二页从 `SYN-CUST-03` 接上，`total` 5 → 6，末页 `next` 为空；第二页行的余额、记账笔数与批次照实。
- ✅ 决胜键同向、跨页续得上：`TestCodSubledgersBreakTiesOnTheFourDimensionKey`——同一客户四本账跨页，缺省序（`customer` 正序）与 `-customer` 按两行一页走完全册，各恰是全集、次序即四维键同向。
- ✅ `total` 随 `q` 变、`%` 与 `_` 按字面、空册答空页 `total` 为 0：`TestCodSubledgersTotalFollowsTheKeyword`。本册没有筛选维，「随筛选变」一格不适用。
- ✅ 端点：未知键（`tenant`）、未声明的筛选键（`currency`）、集外排序维、解不开的游标、换 `q` 拿旧游标各答 400 带 `detail`；渠道未配置时各变体同答 403、答复体逐字相同；`page` 的 `size`、`next`、`total` 都在场，`size` 取 Intake 页大小：`TestCodSubledgersRefuseQueriesOutsideTheDeclaration`、`TestCodSubledgersCarryThePageAndPassTheDecodedQuery`。词表外筛选值一格不适用（无筛选维）。
- ✅ 迁移按字节无 CR、无 BOM：本票无迁移；票面已量，无 CR、无 BOM。
- ✅ 自验（评审修订，钉本笔）：改过的 `.go` 已 `gofmt -w`、`gofmt -l` 为空；`go build ./...`、`go vet ./...` 全仓退 0。`go test -count=1 -p 1 -v <包>`（带 `IDP_PARCEL_POSTGRES_DSN`）
  逐包数 `grep -c -- '--- PASS'` 与 `'--- SKIP'`（PASS/SKIP，均 FAIL 0）：`cmd/parcel-api` 654/0、`cmd/parcel-collection-register` 18/0、`internal/architecture` 174/0、
  `internal/collectionremittance/adapters/http` 13/0、`.../adapters/postgres` 38/0（作者轮 40，删去的是开立时刻正倒序那两个子用例）、`.../adapters/settlementaccounting` 2/0、
  `.../application` 45/0、`.../domain` 44/0；`.../adapters/accessidentity` 与 `.../ports` 无测试文件。反向依赖由 `go list` 反查 `.Deps` 与测试的 `.TestImports`、`.XTestImports`
  含 `internal/collectionremittance/` 者，本上下文之外只有 `cmd/parcel-api` 与 `cmd/parcel-collection-register`。本轮没碰迁移，`internal/platform/migrate` 8/0、`migrations` 1/0
  是作者轮钉 `42622b45c` 的数。

### 判断项

- **缺省序不是 `-registeredAt`**：迁移前按四维键升序上列，同一客户的几本账挨在一起；缺省 `customer` 正序保这个次序（spec「缺省序保今天的次序」），声明头注已写。决胜键方向与迁移前一致（全升序），没有改向。
- **可排维只放 `customer` 一维**：作者轮另放过 `openedAt`（开立时刻），依据只是管理台分户账页的技术详情里显示开立时间；列表上没有这一列、也不按它排，这一维也没有索引。spec「可排维、筛选维从小放」只为操作者确有换序需要的维度开口，放出去再收回，调用方就开始吃 400，评审修订时删去，声明头注写明。
- **不放筛选维**：分户账没有状态，四维都是租户实例值、没有封闭词表；找账走 `q`。各位置余额是派生值，不作可排维、筛选维——否则要为翻不到的行也扫记账。
- **不是按包裹或事件计量的大册**：一行是一本分户账，不是一笔记账；`total` 给精确值。
- **管理台与脚本带的参数**：`apps/admin-web/src/pages/collection/api.ts` 调 `/collection-subledgers` 不带参数；`scripts/` 与 `cmd/` 下没有带参数的调用方。无需登记选择器。
- **读目标检查**：`/collection-subledgers` 不在 `cmd/parcel-api` 读目标检查的豁免表里，`q` 里的 NUL 与坏 UTF-8 在进端点前已答 400，本票没动它。

### 未验

- 未在真实规模上看计划：本页子查询带 `LIMIT`、规划器不拍平，余额只为本页行算，这一句只由语句形状与用例结果支撑，没有跑 `EXPLAIN`。
- 管理台不改（归 14）：与迁移前一样只取第一页（Intake 页大小 200）、客户端搜索；接上 `next` 与 `q` 之前，超过一页的租户看不到后面的账。

## 评审记录

**评审 ← 非作者评审 · 钉 `42622b45c`**

| 评审意见 | 处理 |
|---|---|
| 阻断（Standards）：`ports/catalogue_pages.go` 的文件头注与 `CodSubledgerPages` 头注、票面完成记录的声明一节与判断项，都按序号引 spec 的取舍；这是跨文件序数，spec 的取舍还在增删，编号会漂 | 代码注释与票面一律改引规则原名：spec「缺省序保今天的次序」「可排维、筛选维从小放」「可排维与行标识的列必须非空」「每迁一册补一条与缺省序同形的索引」。顺带查过本票改过的注释与票面，没有数别处东西的句子；`rg -n '取舍 ?[0-9]' internal/collectionremittance .scratch/catalogue-read-pagination-rollout/issues/10-*.md` 零命中 |
| 非阻断（Spec）：另放的 `openedAt` 可排维依据只是 `CodLedgerPage.tsx` 技术详情里显示开立时间；列表里没有这一列也没有排序，这一维也没有索引；放出去再收回，调用方会开始吃 400 | 删去：声明里的那一维、落列里的 `s.opened_at`、`codSubledgerPosition` 按开立时刻取值的分支，「开立时刻倒序」「开立时刻正序」子用例与只为它们写的 `seedSubledgerOpenedAt`；端点用例里带 `sort` 的合规一问改用 `-customer`。删前查过 `apps/admin-web`、`scripts`、`cmd`，没有带 `sort=openedAt` 的调用方。判断项「可排维只放 `customer` 一维」 |
| 非阻断（Spec，交 14）：管理台 COD 台账页今天的客户端搜索会命中客户显示名，`codSubledgerPages.Keyword` 只覆盖标识列 | 不归本票，已由派单方登到 14 号票 |
