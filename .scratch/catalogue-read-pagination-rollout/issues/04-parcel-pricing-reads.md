# 04 parcel-pricing 各册迁到 ADR-0144

Category: enhancement
Status: in-progress——非作者评审（钉 `c39f772f2`）无阻断，四条非阻断已由派单方处理，待进 main（2026-11-06，分支 `cursor/crpr-04-pricing`，基 `d48c5b130`；完成记录、修订记录与评审记录见文末）。迁移编号 parcel-pricing `0015`（进 main 时按 tip 重核）
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

见 spec「每张迁移子票（02–13）的做法与判据」。本票特有：评价按请求计量，是 spec「`total` 一律给精确值」点名的那类大册。

## 完成记录（2026-11-06，分支 `cursor/crpr-04-pricing`，基 `d48c5b130`）

**落点**（路径省去 `internal/parcelpricing/` 前缀）

| 笔 | 文件 | 做了什么 |
|---|---|---|
| `cb75fa7e2` | `ports/catalogue_pages.go`（新）与三份读端口；`adapters/http` 的 `catalogue_page.go`（新）、`catalogue_intake.go`、五个端点、`unconfigured_intake.go`；`adapters/postgres` 四个读面；`cmd/parcel-api/unwired_orchestration.go`、`assemble_pricing_registration_test.go`；各层用例；票面 Status | 五册声明、读端口改签名、Binding 读面、端点解码与 `page`、占位；真库与端点新用例 |
| `0fa0d9de6` | `migrations/parcel_pricing/0015_catalogue_read_pagination_indexes.sql` | 五条取页索引 |
| `46c1c8c87` | `adapters/http` 五个端点、`catalogue_page.go`、`unconfigured_intake.go` 与两份端点用例；`cmd/parcel-api/assemble_pricing_import_test.go` | 改为先 Intake 后解码（spec「端点先 Intake、后解码查询参数」）；那份装配用例还原成基线，净零改动 |
| `e70454938` | `adapters/postgres/catalogue_pages_test.go` | 序列册补「翻页途中插行」；本票新夹具的租户与方案标识改成 `SYN-` 值 |
| `74ad26c40` | `ports/catalogue_pages.go`、`ports/price_card_draft.go`、`adapters/postgres/price_card_draft.go` 与两份用例 | 只动注释：不数别处的词表（「封闭三格」一类改成「闭集」或点名） |
| 修订轮（本笔） | `ports/catalogue_pages.go`；`adapters/http/query_catalogue_pages_test.go`、`query_price_card_drafts_test.go`；票面 | 草稿查阅并入端点表驱动用例；未配置用例扫遍每个坏请求；换条件拿旧游标断言理由原句；注释与票面引 spec 改用原句；覆盖册行标识写明取分组键 |

依赖 01 的 `7b1166838`、`9726b2f35`；照参照实现 `d48c5b130`、`31fcbf4f2` 的形状写。

**各册声明**（声明在 `ports/catalogue_pages.go`，落列在各读面的 `MustBind`）

| 路由 → 读端口方法 | 缺省序 | 可排维 | 筛选维 | q 覆盖 | 行标识 | 索引（`0015`） |
|---|---|---|---|---|---|---|
| `/pricing-evaluations` → `EvaluationCatalogueRead.ListEvaluations` | `-recordedAt` | recordedAt | status（闭集，列存词） | evaluation_id、semantic_digest、plan_content_digest | evaluationId | `evaluation_by_tenant_recorded`（tenant_id, recorded_at DESC, evaluation_id DESC） |
| `/pricing-price-cards` → `PriceCardCatalogueRead.ListPriceCards` | `-registeredAt` | registeredAt、planId | direction（闭集） | plan_id、plan_version、scope | planId + planVersion | `price_card_version_by_tenant_registered`（tenant_id, registered_at DESC, plan_id DESC, plan_version DESC）；planId 序由主键承担 |
| `/pricing-reference-series` → `ReferenceSeriesCatalogueRead.ListReferenceSeries` | `-registeredAt` | registeredAt、seriesId | kind（闭集）、seriesId（无词表，按标识精确取） | series_id、source_identifier、registrant | seriesId + seriesVersion | `reference_series_version_by_tenant_registered`（tenant_id, registered_at DESC, series_id DESC, series_version DESC）；seriesId 的序与筛选由主键承担 |
| `/pricing-reference-series-coverage` → `ReferenceSeriesCoverageRead.ListReferenceSeriesCoverage` | `seriesId` | seriesId | kind | series_id | seriesId + kind | `reference_series_version_by_tenant_series_kind`（tenant_id, series_id, kind） |
| `/pricing-price-card-draft-views` → `PriceCardDraftRead.ListPriceCardDrafts` | `-submittedAt` | submittedAt | status（闭集，列存词） | plan_id、plan_version、source_file_name | planId + planVersion | `price_card_draft_by_tenant_submitted`（tenant_id, submitted_at DESC, plan_id DESC, plan_version DESC） |

排序维与标识各列都是 `NOT NULL`（spec「可排维与行标识的列必须非空」）。这几张表上此前没有以（租户、排序维）开头的索引，`0015` 只建不换。
q 落的都是文本列（价卡的 `scope` 是 `text`），没有落在 JSON 数组上的，不涉 `::text`。

`/pricing-pending-series-evaluations` 判为**有意不设上限**，不迁：读面按 `series_kind` 分组数 `COUNT(DISTINCT evaluation_id)`，种类由
CHECK `evaluation_issue_series_kind_closed` 封成闭集，答复行数以种类数为上界；端点头注早写明「不收 limit：分组键是序列种类的封闭集，
行数有上界」。它答的是计数，不是一册行，没有可翻的页。

**完成判据**（钉修订轮那一笔）

- ✅ 真库用例（`SYN-` 夹具，带 DSN，`adapters/postgres/catalogue_pages_test.go`），本票每册都过：
  - 按小页走完全部页，拼起来恰是全集、次序即缺省序；走完第一页后插入两行——排在游标之前的那行（倒序册比游标新、覆盖册标识比游标小）
    落在已翻过的那头、后面各页不再出现，排在游标之后的那行（倒序册与游标同刻而标识排其后，或更早；覆盖册标识更大）在后面的页出现；
    total 由插入前到插入后各加二、他租不算（`TestEvaluationPagesWalkWithoutRepeatOrGap`、`TestPriceCardPagesWalkWithoutRepeatOrGap`、
    `TestReferenceSeriesPagesFoldReviewsOnEveryPage`、`TestCoveragePagesWalkSeriesInIdentifierOrder`、`TestPriceCardDraftPagesWalkWithoutRepeatOrGap`）。
  - 筛选与 q 改变 total，每一问都断言 total（各册的 `Test*PagesFilterSearchAndSortInTheDatabase`）：维内取或、维间取且，q 不分大小写、
    `%` 按字面（评价另证去首尾空白与 `_`），换序时决胜键同向；空租户答空页、total 为 0、没有 next。
  - 覆盖册另证同一标识两个种类被页界切开不重不漏；序列册另证复核事实在后面几页照样完整、seriesId 精确取不带同前缀的兄弟序列。
- ✅ 端点用例：本票五个端点同在 `adapters/http/query_catalogue_pages_test.go` 的表驱动用例里各过一遍——
  - 合规的一问带 `page`，size 取 Intake 页大小，next、total 原样；读口收到解出的排序、筛选与 q；不带查询串的空页 total 为 0、next 为 null。
  - 集外键（自报租户、自定页大小）、集外排序维、词表外或为空的筛选值、排序给两次、解不开的游标、坏转义各答 400 带 detail，读口不被调用；
    detail 点到那一格。
  - 同条件带游标，读口收到解出的位置；换条件拿旧游标答 400，detail 即决定一那句「游标与本次的排序或筛选不符，请从第一页重取」。
  - 渠道未配置时，不带查询串、合规的一问与上面每一个坏请求都答 403 ACCESS_CHANNEL_NOT_CONFIGURED，答复逐字相同，读口不被调用。
  - 草稿查阅另证没带令牌时不论查询串都答 401（`TestOperatorIntakeTranslatesTheDraftQuery`）。变异自查（未提交）：把草稿端点临时改回
    先解码，`渠道未配置` 与 401 两条用例转红。
- ✅ 迁移按字节无 CR、无 BOM。
- ✅ 门（作者自验，修订轮那一笔的内容，WSL go1.26.8，`-p 1 -count=1 -v`，全部带 DSN）：相对基线改过的 `.go` 上 `gofmt -l` 空；全仓
  `go build ./...`、`go vet ./...` 退 0。
  - `internal/parcelpricing/...`、`internal/networkrouting/adapters/parcelpricing`、`internal/settlementaccounting/adapters/parcelpricing`、
    `cmd/parcel-api`、`cmd/parcel-dispatch`、`cmd/parcel-pricing-feed`、`cmd/parcel-pricing-register`、`internal/architecture/...`、
    `migrations`、`internal/platform/migrate` 一遍：2548 PASS / 0 SKIP / 0 FAIL。
  - 分包（含子用例）：`adapters/postgres` 113、`adapters/http` 186、`application` 192、`domain` 410、`cmd/parcel-api` 654、
    `cmd/parcel-dispatch` 506、`cmd/parcel-pricing-register` 38、`internal/architecture` 174，其余各包全过，全部 0 SKIP。
  - 计数取证：`go test -count=1 -p 1 -v <包> > log`，再 `grep -c -- '--- PASS' log` 与 `grep -c -- '--- SKIP' log`；分包按
    `ok` 行切段数。
  - 此前几笔的自验（`0fa0d9de6` 全量 2540 PASS、`46c1c8c87` 之后 1025 PASS、`e70454938` 之后 `adapters/postgres` 113 PASS，均 0 SKIP）
    已被这一遍覆盖。

**判断项**

1. **先 Intake 后解码**（spec「端点先 Intake、后解码查询参数」）：起初照 `d48c5b130` 写成先解码，派单补了这条之后在 `46c1c8c87` 改齐。
   修订轮逐个端点再核：五个端点都是方法门 → Intake → 解码，各 Intake 实现（隔离、操作者、未配置）都不读查询串，这一轮无需再对调；
   不迁的 `/pricing-pending-series-evaluations` 不读查询串。草稿查阅随之回到「认证过了才解查询串」：没带令牌的一问不论查询串都答 401。
2. **决胜键改同向**（决定三）：评价、价卡、参考序列、草稿四册迁移前首列倒序、标识正序，现在标识随首列倒序——只有首列完全相同的行之间
   次序变了。覆盖册迁移前 `series_id, kind` 已同向，不变。
3. **大册**：评价按请求计量，是本上下文最大的一册；total 照给精确值，前提是按租户读、租户级规模（越权风险点 3），声明头注写明哪天超出
   回 ADR-0144 改成 null。
4. **管理台与脚本带的参数**：admin-web 调这五个路由都不带查询参数，`scripts/` 不调；答复只多一格 `page`，管理台不改也照旧能用。迁移前
   草稿查阅由 Intake 解的单值 `?status=` 挪进声明作筛选维（词表不变，现在可重复、维内取或），`DecodePriceCardDraftQuery` 退役。
   既有参数传空值的旧行为照旧：基线的 `DecodePriceCardDraftQuery` 对空值 `?status=`、小写状态词、集外键都答 400（它的用例把空值列在
   拒绝格里），迁移后这几样照旧答 400；只有同一键给两次从 400 放宽成维内取或。其余四个路由迁移前不读任何查询参数，没有既有参数要保。
5. **价卡状态不作筛选维**：管理台价卡页的状态（admin-web `priceCardStatus`）是页面对适用期与收尾三格现算的，不是登记册上的列。要
   服务端按它筛，得先在读面导出这一列；连同别的票同类的几处收进 16 号票。
6. **seriesId 筛选维**：为管理台从覆盖摘要「只看这一条序列」留的（今天在客户端收窄）；q 的包含匹配会把同前缀的兄弟序列一并带进来，
   所以另设精确的一维。
7. **严格解查询串**：`decodePageQuery` 照 `url.ParseQuery` 整条拒（坏转义与分号答 400），不用参照包的 `URL.Query()`——后者静默丢掉坏段、
   答出整册。两种口径并存有违 ADR-0144 决定七的「一处定义」，把原始查询串的解析收进 `cataloguepage` 已立 18 号票（本票地盘外，未动）。
8. **两步取页**：序列册先取一页版本、再只为本页取复核计数；覆盖册先取一页（序列、种类）、再只为本页取明细。total 与本页各一条语句
   （同首批），两条之间有写入时 total 可能与本页差一拍。序列册取复核计数、覆盖册取明细也是另一条语句，两句之间新落的复核或版本会进本页
   明细（只多不少）。覆盖册每页按自己那一问的时钟解「在用」、各自回显 asOf，游标不记时刻。
9. **问题体**：本包没有 `writeProblemWithDetail`，`problemDetail` 加了 `detail`（omitempty），只有查询参数被拒那一格带它。
10. **自报参数**：`isolated_read_intake_test` 里 `/pricing-price-cards?tenant=` 那一格测的是 Intake 本身、不经端点，Intake 照旧无视自报，原样
    通过；经端点时 `tenant` 是集外键、答 400（端点用例证）。改动过的旧用例沿用既有的 `tenant-a` 一类租户字面量，没有新增；端点表驱动
    用例的 Intake 替身沿用既有的 `catalogueQuery`（`tenant-1`），本票新写的夹具值一律 `SYN-`。
11. **覆盖册的行标识取分组键**（偏离 spec「行标识取登记表主键去掉租户那几段」）：覆盖册一行是一条序列各版的聚合，版本表主键去掉租户是
    （序列标识、序列版本），定的是版本、定不了聚合行；行标识取聚合的分组键（序列标识、种类）——版本表没有约束把一条标识钉在一个种类上，
    同一标识两个种类各是一行（声明头注）。其余四册的行标识即登记表主键去掉租户：评价表主键只有 `evaluation_id`，价卡、序列、草稿是
    （方案或序列标识、版本）。
12. **没有自带 Next / Total 的计数队列**：五个读端口都答 `cataloguepage.Result`，本票没有带积压计数或 `truncated` 的队列；
    `/pricing-pending-series-evaluations` 答的分组计数不迁（见上）。

**未验**

- 索引没有用 `EXPLAIN` 核实被取页语句选中（合成数据量小，计划器不一定走索引）。
- 管理台没在浏览器里过（本票未改管理台，归 14）；全量 `scripts/test-full.sh` 由推送方在重放后的 tip 上跑。
- 迁移号 `0015` 只在基 `d48c5b130` 与 `origin/main`（`09722f195`）上核过未占。

## 修订记录

**派单方修订轮（通道 1）**：照派单说明「02 的非作者评审拦下过的」逐条自查，参照 02 修订后的 `c3c226b00`；不 rebase，留在基 `d48c5b130`。

| 自查项 | 自查结果 | 处理 |
|---|---|---|
| 每个端点先 Intake，渠道未配置对一切请求同答 403 | 五个端点在 `46c1c8c87` 已是方法门 → Intake → 解码，Intake 实现不读查询串 | 无需对调；未配置用例改为扫遍不带查询串、合规的一问与每一个坏请求，五个端点都过 |
| 每册真库：翻页途中插入新行、空册 total 为 0、筛选或 q 改变 total 并断言它 | 五册都有（序列册的插行在 `e70454938` 补上），筛选与 q 的每一问都断言 total | 无需补；完成判据按实际改写 |
| 端点表驱动：合规一问带 page 且 size 取 Intake 页大小、集外参数答 400 带 detail、未配置时合规与不合规都答 403；换条件拿旧游标答 400 | 草稿查阅不在表里、另写一份；旧游标只断言 400，没核理由原句 | 草稿查阅并入同一张表、删去另写的两条重复用例；旧游标断言 detail 即决定一原句 |
| 注释与票面不引 spec 的取舍序号、不数别处的东西 | 声明头注引了一处序号；票面四处（含立票时正文里那句「点名的大册」） | 都改成引规则原句；数别处的计数已在 `74ad26c40` 清掉 |
| 新夹具一律 `SYN-`；JSON 数组上的 q 不用 `::text`；既有参数传空值的旧行为保住 | 新夹具已是 `SYN-`；q 都落文本列；草稿 `?status=` 空值迁移前就答 400 | 无需改；判断项 4、10 写明 |
| 计数队列自带 Next / Total、行标识不取主键去租户的写进判断项 | 没有计数队列；覆盖册行标识取聚合的分组键 | 判断项 11、12；覆盖册声明头注同写 |

## 评审记录

**评审 ← 子代理 · 钉 `c39f772f2`**：两轴无阻断。

| 评审意见 | 处理 |
|---|---|
| 非阻断：`ports/catalogue_pages.go` 文件头注与 `PriceCardDraftPages` 头注各有一句变更说明 | 删去这两句，规则本身留下 |
| 非阻断：查询串解析分成 `url.ParseQuery` 与 `URL.Query()` 两种口径，有违「一处定义」 | 本票不退回；判断项 7 改指 18 号票（收进 `cataloguepage`） |
| 非阻断：判断项 8 没写明细可能比本页新一拍 | 判断项 8 补上（只多不少） |
| 非阻断：判断项 5 把价卡状态指给 14，现行 spec 已归 16 | 判断项 5 改指 16 号票 |

修订由派单方在本分支上做（作者会话已收工），只动两段头注与本票面。
