# 01 `internal/platform` 共用件：游标编解码、查询参数校验、答复 `page` 拼装

Category: enhancement
Status: in-progress——2026-09-24 通道 2 在分支 `mcp2-crp01` 上做完（基 `51d2548a`，完成记录见文末），待非作者评审与推送方重放；分支已推 origin（凭据到位后补推，见完成记录末段）。此前：通道 2 认领（派单 `task-44d76717` ← 通道 3），隔离 worktree 分支 `mcp2-crp01`，基于认领笔
Blocked by: 无
地盘：新包 `internal/platform/<名自定，如 cataloguepage>/` 与其测试。不碰任何上下文包。
出处：[ADR-0144](../../../docs/adr/0144-catalogue-reads-share-one-cursor-pagination-sort-and-filter-contract.md) 决定一、三、四、五、七。

## 做什么

1. **各册的声明形状**：一册把自己的可排维（含缺省序）、筛选维（维名、是否封闭词表及其词表）交给共用件；`q` 覆盖哪几列是 SQL 侧的事，不进声明。
2. **解码**：从查询串解出查询对象（游标、排序、筛选、`q`），按声明校验；集外键、集外排序维、词表外的筛选值、解不开或摘要不符的游标、超长的 `q`
   一律报成一个可映射到 `MALFORMED_REQUEST` 的错误，并带一句给操作者看的理由散文（形同各上下文 `problemDetail.Detail` 的用法）。
3. **游标**：编成不透明串；内容与摘要口径照 ADR-0144 决定一。
4. **答复 `page` 拼装**：照决定五。

## 不做

- 不写任何一册的 SQL，不改任何读端口签名（归 02、03）。
- 不开页大小参数（决定二）。

## 完成判据

- 包内测试钉住：游标往返；排序或筛选或 `q` 变了之后拿旧游标即拒；集外键即拒；集外排序维即拒；词表外筛选值即拒；`q` 去首尾空白、空即缺席、超长即拒；
  同一维重复给的解成多值。
- `go test ./internal/architecture/ -count=1` 过：共用件不导入任何上下文包。
- 完成记录写清包名与导出面，供 02 / 03 照用。

## 完成记录（2026-09-24，通道 2，分支 `mcp2-crp01`，基 `51d2548a`）

**落点**

| 笔 | 文件（`internal/platform/cataloguepage/`） | 做了什么 |
|---|---|---|
| `5537db0c` | `catalogue.go`、`catalogue_test.go` | 第 1 条：各册声明形状，`NewCatalogue` 在装配时拒不合规声明 |
| `c048746a` | `query.go`、`query_test.go` | 第 2 条：查询参数解码（排序、筛选、`q`、选择器、集外键、单值键给多次） |
| `91d932d6` | `cursor.go`、`cursor_test.go`、`query.go` | 第 2、3 条：游标编解码，`Decode` 接 `after` |
| `f23f16e6` | `page.go`、`page_test.go` | 第 4 条：答复 `page` 拼装 |
| 本笔 | 票面 | 完成记录 |

**包名与导出面**（供 02 / 03 照用）

包 `go.idp.xyz/idp-parcel/internal/platform/cataloguepage`，只导入标准库。

- 声明：`Spec{Name, Sorts []SortDimension, DefaultSort Sort, Identity []ValueKind, Filters []FilterDimension, Selectors []string}`；
  `SortDimension{Name, Kind}`；`FilterDimension{Name, Vocabulary}`（`Vocabulary` 为空即不设词表）；`Sort{Field, Descending}`，`Sort.String()` 给
  `sort` 参数写法（倒序前缀 `-`）；`ValueKind` 取 `Text` / `Integer` / `Instant`；`NewCatalogue(Spec) (*Catalogue, error)`、`MustCatalogue(Spec) *Catalogue`
  （包级声明用）；`MaxKeywordRunes`（`q` 的上限）。
- 解码：`(*Catalogue).Decode(url.Values) (Query, error)`，拒绝一律是 `*MalformedQuery{Reason}`——映射 `MALFORMED_REQUEST`，`Reason` 原样交给
  `writeProblemWithDetail` 的 detail。`Query{Sort, Filters map[string][]string, Keyword string, After *Position}`，`Filters` 的值已去重升序，`After` 为 nil 即第一页。
- 游标：`Position{Value string, Identity []string}`；`(Query).CursorAfter(Position) (string, error)` 由读面对本页末行调用，得到 `page.next`；
  `FormatInteger` / `ParseInteger`、`FormatInstant` / `ParseInstant` 是位置各格的写法与读法（`Text` 原样）。
- 答复：`Page{Size, Next *string, Total *int64}`（JSON `size` / `next` / `total`）；`NewPage(size int, next string, total int64) Page`，`next` 为空串即末页。

接法示意（不是约束）：读面写 `var <册> = cataloguepage.MustCatalogue(...)`；端点 `query, err := <册>.Decode(request.URL.Query())`，`errors.As` 取到
`*MalformedQuery` 即 400 带 detail；读端口收 `query` 与 limit，SQL 按 `query.Sort`、`query.Filters`、`query.Keyword`、`query.After` 落成 keyset 条件，多取
一行判有无下一页，有则 `query.CursorAfter(本页末行位置)`；端点把 `cataloguepage.NewPage(limit, next, total)` 放进答复的 `page`。

**完成判据**（钉 `f23f16e6`，WSL，go1.26.8）

- ✅ 包内测试钉住：游标往返 `TestACursorRoundTripsThePositionOfTheLastRow`、`TestACursorRoundTripsUnderEachValueKind`；排序或筛选或 `q` 变了之后拿旧游标即拒
  `TestAnOldCursorIsRefusedOnceTheConditionsChange`；集外键即拒 `TestAKeyOutsideTheDeclaredSetIsRefused`；集外排序维即拒
  `TestASortOutsideTheDeclaredDimensionsIsRefused`；词表外筛选值即拒 `TestAFilterValueOutsideTheVocabularyIsRefused`；`q` 去首尾空白、空即缺席、超长即拒
  `TestTheKeywordIsTrimmedAndEmptyMeansAbsent`、`TestTheKeywordLengthLimitCountsCharacters`；同一维重复给解成多值 `TestARepeatedDimensionDecodesToSeveralValues`。
  钉 `f23f16e6` 实测：29 个顶层用例，含子用例 92 PASS / 0 FAIL / 0 SKIP，`-race` 过。
- ✅ `go test ./internal/architecture/ -count=1` 过，`TestSharedPlatformDoesNotDependOnBusinessContexts` 为 PASS。
- ✅ 完成记录写清包名与导出面（上节）。
- 另：gofmt 按入库字节逐笔判、钉 `f23f16e6` 对包内全部文件再判，均无输出；`go build ./...`、`go vet ./...` 全仓退 0。变异核对（证用例能红，已还原）：
  拿掉解码时的位置校验，恰好红值与标识篡改那几例；从摘要里分别拿掉 `q`、选择器、册名，各红对应用例。

**判断项**（供评审与 02 / 03）

1. **值形状进了声明。** 票面点的是可排维、缺省序与筛选维；而游标装着末行的排序维值与标识，决定一只要求它不透明、不防篡改——不声明形状就校验不了
   伪造的值，那个值会带进 SQL 答 500 而不是 400。所以可排维带 `Kind`、行标识逐段带 `Kind`，且只认规整写法（同一个值只有一种写法，往返才稳）。
2. **行标识是多段的。** 网络目录各版本册的决胜键是「代码 + 版本号」，服务日历是「对象种类 + 代码 + 版本号」（读于 `51d2548a`：`network_catalog_list.go`
   的 `ListNodeVersions`、`ListServiceCalendarVersions` 各自的 `ORDER BY`）；单个标识串装不下，不该逼各册自造拼接编码。
3. **摘要比决定一的原文多覆盖了册名与选择器取值。** 决定一的摘要口径是「本次排序与筛选（含 `q`）」；另一册或另一个 `family` 的游标拿来用，翻出来的
   同样是另一份列表的中段，正是决定一要拦的事。评审若认为这越出了决定一，删 `digest` 里对应的一行即可，用例会随之指出。
4. **`Selectors` 是为票 03 的 `?family=` 开的。** 决定四要选择器保持原义、不兼作筛选维，而未知键即拒——不声明它就会被当成集外键拒掉。
5. **游标解不开与摘要不符用同一句散文**：决定一点名的「游标与本次的排序或筛选不符，请从第一页重取」，两种情形写在同一句里，不另造措辞。
6. **`q` 上限定为 100 个字符，按 rune 计。** 决定四只要求上限在共用件里定一次，数值由本票定；按字节计会让中文检索词只剩三分之一的余量。
7. **空值一律拒**（封闭词表维、不设词表的维与 `after=` 都是）；`q` 为空按决定四视为缺席，是唯一的例外。页大小不开参数，`limit` / `pageSize` 按集外键拒（决定二）。
8. **未做**：不写任何一册的 SQL、不改读端口签名（归 02、03）；也没给读面写「多取一行判下一页」的辅助——那是 SQL 侧的写法，02、03 各写一遍后重复了再抬。

**分支推送**：完工时本宿主没有 GitHub 推送凭据（经 WSL 中继网络已通，`git push --dry-run` 停在认证）；用户随后完成 gh 设备码授权，
2026-09-24 15:3x 补推，`git ls-remote origin refs/heads/mcp2-crp01` 答 `33fd352d`（上一笔）。代码笔的 tip 是 `f23f16e6`，其后只有票面。
