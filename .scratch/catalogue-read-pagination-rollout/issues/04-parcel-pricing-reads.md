# 04 parcel-pricing 各册迁到 ADR-0144

Category: enhancement
Status: in-progress——作者完成，待非作者评审（2026-11-06，分支 `cursor/crpr-04-pricing`，基 `d48c5b130`；完成记录见文末）。迁移编号 parcel-pricing `0015`（进 main 时按 tip 重核）
Blocked by: 01
地盘：`internal/parcelpricing/{ports,adapters/http,adapters/postgres}` 里下表各册的读端口、端点与读面及其测试；`cmd/parcel-api` 里这些读口的占位；
`migrations/parcel_pricing` 的一条新迁移（索引）。
出处：ADR-0144 决定三至七；spec「横贯各子票的取舍」。

## 本票的册（今天的次序钉 `a1274a6b8`）

| 路由 | 读端口方法 | 今天的 `ORDER BY` |
|---|---|---|
| `/pricing-evaluations` | `EvaluationCatalogueRead.ListEvaluations` | `recorded_at DESC, evaluation_id` |
| `/pricing-price-cards` | `PriceCardCatalogueRead.ListPriceCards` | `registered_at DESC, plan_id, plan_version` |
| `/pricing-reference-series` | `ReferenceSeriesCatalogueRead.ListReferenceSeries` | `registered_at DESC, series_id, series_version` |
| `/pricing-reference-series-coverage` | `ReferenceSeriesCoverageRead.ListReferenceSeriesCoverage` | `series_id, kind` |
| `/pricing-price-card-draft-views` | `PriceCardDraftRead.ListPriceCardDrafts` | `submitted_at DESC, plan_id, plan_version` |

开工先核 `/pricing-pending-series-evaluations`：它的读端口不收 `limit`，判它是「有意不设上限」（写明依据）还是漏网，漏网则一并迁。

## 做法与判据

见 spec「每张迁移子票（02–13）的做法与判据」。本票特有：评价按请求计量，是取舍 4 点名的大册。

## 完成记录（2026-11-06，分支 `cursor/crpr-04-pricing`，基 `d48c5b130`）

**落点**（路径省去 `internal/parcelpricing/` 前缀）

| 笔 | 文件 | 做了什么 |
|---|---|---|
| `cb75fa7e2` | `ports/catalogue_pages.go`（新）与三份读端口；`adapters/http` 的 `catalogue_page.go`（新）、`catalogue_intake.go`、五个端点、`unconfigured_intake.go`；`adapters/postgres` 四个读面；`cmd/parcel-api/unwired_orchestration.go`、`assemble_pricing_registration_test.go`；各层用例；票面 Status | 五册声明、读端口改签名、Binding 读面、端点解码与 `page`、占位；真库与端点新用例 |
| `0fa0d9de6` | `migrations/parcel_pricing/0015_catalogue_read_pagination_indexes.sql` | 五条取页索引 |
| `46c1c8c87` | `adapters/http` 五个端点、`catalogue_page.go`、`unconfigured_intake.go` 与两份端点用例；`cmd/parcel-api/assemble_pricing_import_test.go` | 改为先 Intake 后解码（spec 取舍 8）；那份装配用例还原成基线，净零改动 |
| `e70454938` | `adapters/postgres/catalogue_pages_test.go` | 序列册补「翻页途中插行」；本票新夹具的租户与方案标识改成 `SYN-` 值 |
| `74ad26c40` | `ports/catalogue_pages.go`、`ports/price_card_draft.go`、`adapters/postgres/price_card_draft.go` 与两份用例 | 只动注释：不数别处的词表（「封闭三格」一类改成「闭集」或点名） |

依赖 01 的 `7b1166838`、`9726b2f35`；照参照实现 `d48c5b130`、`31fcbf4f2` 的形状写。

**各册声明**（声明在 `ports/catalogue_pages.go`，落列在各读面的 `MustBind`）

| 路由 → 读端口方法 | 缺省序 | 可排维 | 筛选维 | q 覆盖 | 行标识 | 索引（`0015`） |
|---|---|---|---|---|---|---|
| `/pricing-evaluations` → `EvaluationCatalogueRead.ListEvaluations` | `-recordedAt` | recordedAt | status（闭集，列存词） | evaluation_id、semantic_digest、plan_content_digest | evaluationId | `evaluation_by_tenant_recorded`（tenant_id, recorded_at DESC, evaluation_id DESC） |
| `/pricing-price-cards` → `PriceCardCatalogueRead.ListPriceCards` | `-registeredAt` | registeredAt、planId | direction（闭集） | plan_id、plan_version、scope | planId + planVersion | `price_card_version_by_tenant_registered`（tenant_id, registered_at DESC, plan_id DESC, plan_version DESC）；planId 序由主键承担 |
| `/pricing-reference-series` → `ReferenceSeriesCatalogueRead.ListReferenceSeries` | `-registeredAt` | registeredAt、seriesId | kind（闭集）、seriesId（无词表，按标识精确取） | series_id、source_identifier、registrant | seriesId + seriesVersion | `reference_series_version_by_tenant_registered`（tenant_id, registered_at DESC, series_id DESC, series_version DESC）；seriesId 的序与筛选由主键承担 |
| `/pricing-reference-series-coverage` → `ReferenceSeriesCoverageRead.ListReferenceSeriesCoverage` | `seriesId` | seriesId | kind | series_id | seriesId + kind | `reference_series_version_by_tenant_series_kind`（tenant_id, series_id, kind） |
| `/pricing-price-card-draft-views` → `PriceCardDraftRead.ListPriceCardDrafts` | `-submittedAt` | submittedAt | status（闭集，列存词） | plan_id、plan_version、source_file_name | planId + planVersion | `price_card_draft_by_tenant_submitted`（tenant_id, submitted_at DESC, plan_id DESC, plan_version DESC） |

排序维与标识各列都是 `NOT NULL`（取舍 6）。这几张表上此前没有以（租户、排序维）开头的索引，`0015` 只建不换。

`/pricing-pending-series-evaluations` 判为**有意不设上限**，不迁：读面按 `series_kind` 分组数 `COUNT(DISTINCT evaluation_id)`，种类由
CHECK `evaluation_issue_series_kind_closed` 封成闭集，答复行数以种类数为上界；端点头注早写明「不收 limit：分组键是序列种类的封闭集，
行数有上界」。它答的是计数，不是一册行，没有可翻的页。

**完成判据**

- ✅ 真库用例（`SYN-` 数据，带 DSN）：五册各一条「翻页途中插行」——`TestEvaluationPagesWalkWithoutRepeatOrGap`、
  `TestPriceCardPagesWalkWithoutRepeatOrGap`、`TestReferenceSeriesPagesFoldReviewsOnEveryPage`、`TestCoveragePagesWalkSeriesInIdentifierOrder`、
  `TestPriceCardDraftPagesWalkWithoutRepeatOrGap`：排在游标之前的新行不再出现，排在之后的（含与游标同刻、被页界切开的）接着出现，total
  随之变、他租不算。五册各一条 `Test*PagesFilterSearchAndSortInTheDatabase`：维内取或、维间取且、total 随筛选与 q 变，q 不分大小写、`%` 按字面
  （评价另证去首尾空白与 `_`），换序时决胜键同向，空租户答空页且 total 0。覆盖册另证同一标识两个种类被页界切开不重不漏；序列册另证复核
  事实在后面几页照样完整、seriesId 精确取不带同前缀的兄弟序列。
- ✅ 端点用例：四个目录端点表驱动（`query_catalogue_pages_test.go`），草稿查阅另写（`query_price_card_drafts_test.go`）：集外键（自报租户、
  自定页大小）、集外排序维、词表外或为空的筛选值、排序给两次、解不开的游标、换条件拿旧游标、坏转义各答 400 带 detail，读口不被调用；
  渠道未配置时参数写对写错都答 403 ACCESS_CHANNEL_NOT_CONFIGURED、答复逐字相同；答复带 `page`（size 取 Intake 页大小，next、total 原样）。
- ✅ 迁移按字节无 CR、无 BOM。
- ✅ 门（WSL，`-p 1 -count=1 -v`，全部带 DSN）：`gofmt -l` 空；全仓 `go build ./...`、`go vet ./...` 退 0。
  - 全量一遍（内容即 `0fa0d9de6`）：`internal/parcelpricing/...`、`internal/networkrouting/adapters/parcelpricing`、
    `internal/settlementaccounting/adapters/parcelpricing`、`cmd/parcel-api`、`cmd/parcel-dispatch`、`cmd/parcel-pricing-feed`、
    `cmd/parcel-pricing-register`、`internal/architecture/...`、`migrations`、`internal/platform/migrate`：2540 PASS / 0 SKIP / 0 FAIL。
    顶层用例：`adapters/postgres` 108、`adapters/http` 121、`application` 125、`domain` 345、`cmd/parcel-api` 446、`cmd/parcel-dispatch` 319、
    `cmd/parcel-pricing-register` 18、`internal/architecture` 58，其余各包全过。
  - `46c1c8c87` 之后重跑 `adapters/http` 及其反向依赖（`cmd/parcel-api`、`adapters/accessidentity`）与 `internal/architecture/...`：
    1025 PASS / 0 SKIP / 0 FAIL；`e70454938` 之后重跑 `adapters/postgres`：113 PASS / 0 SKIP / 0 FAIL。
  - 计数取证：`go test -count=1 -p 1 -v <包> > log`，再 `grep -c -- '--- PASS' log` 与 `grep -c -- '--- SKIP' log`。

**判断项**

1. **先 Intake 后解码**：起初照 `d48c5b130` 写成先解码，派单与 spec 补了取舍 8 之后在 `46c1c8c87` 改齐。草稿查阅随之回到「认证过了才解
   查询串」：没带令牌的一问不论查询串都答 401。
2. **决胜键改同向**（决定三）：评价、价卡、参考序列、草稿四册迁移前首列倒序、标识正序，现在标识随首列倒序——只有首列完全相同的行之间
   次序变了。覆盖册迁移前 `series_id, kind` 已同向，不变。
3. **大册**：评价按请求计量，是本上下文最大的一册；total 照给精确值，前提是按租户读、租户级规模（越权风险点 3），声明头注写明哪天超出
   回 ADR-0144 改成 null。
4. **管理台与脚本带的参数**：admin-web 调这五个路由都不带查询参数，`scripts/` 不调；答复只多一格 `page`，管理台不改也照旧能用。迁移前
   草稿查阅由 Intake 解的单值 `?status=` 挪进声明作筛选维（词表不变，现在可重复、维内取或），`DecodePriceCardDraftQuery` 退役。
5. **价卡状态不作筛选维**：管理台价卡页的状态（admin-web `priceCardStatus`）是页面对适用期与收尾三格现算的，不是登记册上的列。14 若要
   服务端按它筛，得先在读面导出这一列。
6. **seriesId 筛选维**：为管理台从覆盖摘要「只看这一条序列」留的（今天在客户端收窄）；q 的包含匹配会把同前缀的兄弟序列一并带进来，
   所以另设精确的一维。
7. **严格解查询串**：`decodePageQuery` 照 `url.ParseQuery` 整条拒（坏转义与分号答 400），不用参照包的 `URL.Query()`——后者静默丢掉坏段、
   答出整册。若要各上下文一致，宜挪进 `cataloguepage`（本票地盘外，未动）。
8. **两步取页**：序列册先取一页版本、再只为本页取复核计数；覆盖册先取一页（序列、种类）、再只为本页取明细。total 与本页各一条语句
   （同首批），两条之间有写入时 total 可能与本页差一拍。覆盖册每页按自己那一问的时钟解「在用」、各自回显 asOf，游标不记时刻。
9. **问题体**：本包没有 `writeProblemWithDetail`，`problemDetail` 加了 `detail`（omitempty），只有查询参数被拒那一格带它。
10. **自报参数**：`isolated_read_intake_test` 里 `/pricing-price-cards?tenant=` 那一格测的是 Intake 本身、不经端点，Intake 照旧无视自报，原样
    通过；经端点时 `tenant` 是集外键、答 400（端点用例证）。改动过的旧用例沿用既有的 `tenant-a` 一类租户字面量，没有新增。

**未验**

- 索引没有用 `EXPLAIN` 核实被取页语句选中（合成数据量小，计划器不一定走索引）。
- 管理台没在浏览器里过（本票未改管理台，归 14）；全量 `scripts/test-full.sh` 由推送方在重放后的 tip 上跑。
- 迁移号 `0015` 只在基 `d48c5b130` 与 `origin/main`（`09722f195`）上核过未占。
