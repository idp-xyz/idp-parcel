# 06 node-operations 八册迁到 ADR-0144

Category: enhancement
Status: resolved——2026-11-06 进 main（`098767b1e`），记录见文末「进 main 记录」
Blocked by: 01
地盘：`internal/nodeoperations/{ports,adapters/http,adapters/postgres}` 里下表各册的读端口、端点与读面及其测试；`cmd/parcel-api` 里这些读口的占位；
`migrations/node_operations` 的一条新迁移（索引）。
出处：ADR-0144 决定三至七；spec「横贯各子票的取舍」。

## 本票的册（今天的次序钉 `a1274a6b8`）

| 路由（选择器） | 读端口方法 | 今天的 `ORDER BY` |
|---|---|---|
| `/node-operations-records`（六种） | `ReviewCatalogueRead.ListReceptions` / `ListUnidentifiedItems` / `ListConsolidationUnits` / `ListConsolidationMembers` / `ListIdentityReviewUnits` / `ListPhysicalControlTransitions` | 见 `adapters/postgres/review_catalogue*.go` |
| `/node-operations-delivery-pre-advices` | `DeliveryScopeCatalogueRead.ListDeliveryPreAdvices` | `expected_arrival_at DESC, pre_advice_id` |
| `/node-operations-delivery-handover-notes` | `DeliveryScopeCatalogueRead.ListDeliveryHandoverNotes` | `note.opened_at DESC, note.note_id` |

两条按标识取单条的路由（`…/{preAdviceId}`、`…/{noteId}`）不在内。

## 做法与判据

见 spec「每张迁移子票（02–13）的做法与判据」。本票特有：收寄、集包成员等按实物计量，是 spec「`total` 一律给精确值」点名的那类册；聚合导出的册（如身份复核单元按 `latest` 排）键集落在导出列上。

## 完成记录

作者会话，2026-11-06。分支 `cursor/crpr-06-node`，基 `3a20b2bcc`，已推 origin，未进 main。下文路径省去 `internal/nodeoperations/`。

### 落点

| 提交 | 文件 | 做了什么 |
|---|---|---|
| `3411f4921` | `ports/catalogue_pages.go`、`ports/catalogue_read.go`、`ports/delivery_scope_catalogue_read.go` | 八册的查询声明；`ReviewCatalogueRead` 六个列表方法与 `DeliveryScopeCatalogueRead` 两个列表方法收查询对象、交回 `cataloguepage.Result` |
| | `adapters/postgres/review_catalogue.go`、`review_catalogue_control_transition.go`、`review_catalogue_disposition.go`、`delivery_scope_catalogue.go` | 各册按「租户 + 排序维 + 行标识」取页，同一组条件计 `total`，`q` 不分大小写、`%` 与 `_` 按字面；身份复核单元用公用表表达式把导出列 `latest` 做成取页的列 |
| | `adapters/http/catalogue_page.go`、`query_node_operations_records.go`、`query_identity_review_units.go`、`query_delivery_scope_records.go`、`dispose_identity_conflict.go`、`receive_delivered_unit.go` | 端点先 Intake 后解码，答复带 `page`；查询参数被拒答 400、理由散文进 `detail`；新册名 `identity-review-unit`；按实物的单元行加 `latestRecordedAt` |
| | `migrations/node_operations/0027_catalogue_read_pagination_indexes.sql` | 见下表「索引」一列 |
| | `cmd/parcel-api/unwired_orchestration.go`、`unwired_node_control_transitions.go`、`assemble_identity_disposition.go`、`assemble_delivery_scope_catalogue_test.go`；`cmd/parcel-dispatch/node_control_on_handover_test.go` | 只动本票读口的占位与替身，签名跟读端口走 |
| | 本上下文既有用例 | 接统一结果类型；同刻次序、查询串自报租户与页大小那几格随本票改（见判断项） |
| `024c5a8a4` | `adapters/http/query_node_operations_records_page_test.go`、`query_delivery_scope_records_page_test.go` | 各端点的传输形状 |
| `9aa78c00c` | `adapters/postgres/catalogue_pages_test.go` | 真库翻页契约，八册各一个用例 |
| `a60f744dc` | `adapters/http/query_node_operations_records.go`、`catalogue_intake.go` 与两份用例 | 册名挪到 Intake 之后（判断项「门的次序」） |
| `bb8e4f322` | 本票；`adapters/postgres/delivery_scope_catalogue_plan_test.go` 头注 | 完成记录；头注改指迁移 0027 |
| `e8f309dde` | `adapters/postgres/review_catalogue_disposition.go`、`catalogue_pages_test.go`；`ports/catalogue_pages.go`；`adapters/http/query_delivery_scope_records_page_test.go` | 修订轮：复核单元的作业实物导出成非空列、缺键的组响亮报错；声明头注补导出列前移的两个方向；送站两条列表补未配置用例（见评审记录） |
| `55280d64d` | 本票 | 修订轮：引 spec 改引原句；判断项与评审记录 |
| `bd0dc1b5c` | `adapters/postgres/review_catalogue_disposition.go`、`catalogue_pages_test.go`；`adapters/http/query_node_operations_records_test.go`、`query_delivery_scope_records_page_test.go` | 复核修订：复核单元缺键切页后才判；缺键用例断言报错原句、补坏行只是多取那一行的情形；500 用例补新册与只 `units` 读不动的情形；送站页用例按名字挑读口（见评审记录） |
| 本笔 | 本票 | 复核修订：Status、判断项 11 的代价与评审记录 |

### 各册声明

| 册（`Spec.Name`） | 路由与选择器 | 读端口方法 | 缺省序（迁移前的 `ORDER BY`） | 可排维 | 筛选维：词表 | `q` 覆盖 | 索引 |
|---|---|---|---|---|---|---|---|
| `node-operations-receptions` | `/node-operations-records?registry=reception` | `ListReceptions` | `-recordedAt`（`recorded_at DESC, source_id`） | `recordedAt` | `kind`：`INTAKE_FORMED`、`PENDING_IDENTIFICATION` | 来源标识、作业实物、节点、交付方 | 新建 `reception_catalogue_by_recorded`（部分索引） |
| `node-operations-unidentified-items` | `registry=unidentified-item` 答复的 `items` | `ListUnidentifiedItems` | `-recordedAt`（`r.recorded_at DESC, r.source_id`） | `recordedAt` | `identityCheck`：`CHECKED`、`UNCHECKED` | 来源标识、作业实物、节点、来源里的外部标识观察 | 新建 `reception_pending_catalogue_by_recorded`（部分索引） |
| `node-operations-identity-review-units` | `registry=identity-review-unit`（新）；`registry=unidentified-item` 答复的 `units` 是它不带条件的第一页 | `ListIdentityReviewUnits` | `-latestRecordedAt`（`latest DESC, unit`） | `latestRecordedAt`（导出列） | 无 | 作业实物 | 不建：导出列上建不了同形索引 |
| `node-operations-consolidation-units` | `registry=consolidation-unit` | `ListConsolidationUnits` | `unitId`（`unit_id`） | `unitId` | `phase`：`OPEN`、`SEALED`、`CLOSED` | 单元标识、载具、最近一次封签 | 不建：主键（租户，单元号）同形 |
| `node-operations-consolidation-members` | `registry=consolidation-member` | `ListConsolidationMembers` | `-recordedAt`（`recorded_at DESC, source_id`） | `recordedAt` | `action`：`ADD_MEMBER`、`REMOVE_MEMBER` | 来源标识、单元、成员 | 新建 `consolidation_fact_membership_by_recorded`（部分索引） |
| `node-operations-physical-control-transitions` | `registry=physical-control` | `ListPhysicalControlTransitions` | `-recordedAt`（`recorded_at DESC, handover_object, handover_scope, handover_version, direction`） | `recordedAt` | `outcome`：`ADVANCED`、`NOT_ADVANCED` | 交接对象、交接范围、交接版本、作业实物、节点 | 0017 的 `physical_control_transition_by_recorded` 同名重建，补齐标识四段 |
| `node-operations-delivery-pre-advices` | `/node-operations-delivery-pre-advices`；选择器 `node`、`documentNumber`、`expectedArrivalFrom`、`expectedArrivalUntil` | `ListDeliveryPreAdvices` | `-expectedArrivalAt`（`expected_arrival_at DESC, pre_advice_id`） | `expectedArrivalAt` | 无 | 预报标识、单据号、客户账户、节点 | 0021 的 `delivery_pre_advice_by_node_expected_arrival`、`delivery_pre_advice_by_expected_arrival` 同名重建 |
| `node-operations-delivery-handover-notes` | `/node-operations-delivery-handover-notes`；选择器 `node`、`documentNumber`、`openedFrom`、`openedUntil` | `ListDeliveryHandoverNotes` | `-openedAt`（`note.opened_at DESC, note.note_id`） | `openedAt` | 无 | 交接单标识、单据号、节点、交付方线索 | 0021 的 `delivery_handover_note_by_node_opening`、`delivery_handover_note_by_opening` 同名重建 |

`registry` 在六册上都登为选择器。决胜键一律与排序维同向。行标识是各登记表主键去掉租户段（控制转移是交接键四段，身份复核单元是分组键作业实物）。排序列与标识列在建表迁移里都是 `NOT NULL`，身份复核单元的行标识除外：它是 jsonb 键 `intake ->> 'unit'`，库里的 `reception_intake_presence` 只保证 `intake` 本身非空，键在不在靠收寄写口；读面怎样守住见判断项「复核单元的行标识」。身份复核单元的排序列 `latest` 是 `NOT NULL` 的 `recorded_at` 在各组上取的最大值，照样非空。两条按标识取单条的路由没迁，照票面。

### 完成判据

- ✅ **真库用例**（`SYN-` 数据，带 DSN）：`adapters/postgres/catalogue_pages_test.go` 八册各一个用例——`TestReceptionsPageWhileNewIntakesArrive`、`TestUnidentifiedItemsPageWhileNewPendingItemsArrive`、`TestIdentityReviewUnitsPageOnTheirLatestRecording`、`TestConsolidationUnitsPageByUnitIDWhileNewUnitsOpen`、`TestConsolidationMembersPageWhileNewFactsArrive`、`TestPhysicalControlTransitionsPageWhileNewTransitionsArrive`、`TestDeliveryPreAdvicesPageWhileNewPreAdvicesArrive`、`TestDeliveryHandoverNotesPageWhileNewNotesOpen`。每个都按页大小 2 走完一遍：取完第一页后在游标之前与之后各落一行，走下来恰是原有各行按缺省序各出一次、再加上游标之后那一行，不重出、不漏，游标之前那行这一趟不出现；`total` 随新落的行变；同一排序维反向再走一遍；每个筛选词与 `q` 各收窄出预期的 `total`（含筛选与 `q` 相与、大小写不敏感，`q=%25` 与 `q=_` 答 0）；没有行的租户答空页、`total` 0、无 `next`。身份复核单元另证导出列：已翻过的实物又登一版待识别，导出列前移，它不在后面的页上重出；修订轮另加 `TestIdentityReviewUnitsAreLoudAboutPendingRowsWithoutAUnit`：绕过写口造缺作业实物键的待识别行；与另两件同刻时，两个方向翻页都在含它的那一页报缺作业实物那一句错（断言报错原句），不悄悄走完；它只是多取来判下一页的那一行时，本页照常答、带 `next`，翻到含它的那一页才报错（复核修订补）。送站两册另带 `node` 选择器走一遍。
- ✅ **端点用例**：`adapters/http/query_node_operations_records_page_test.go`（六册表驱动）与 `query_delivery_scope_records_page_test.go`。答复带 `page`，`size` 取 Intake 页大小，`next`、`total` 原样带出；空册 200、空数组、`total` 0、`next` 为 null；查询对象原样交给读口。未知键（含查询串自报租户与页大小）、集外排序维、另一册的筛选维、词表外筛选值、空筛选值、重复的排序、解不开的游标各答 400 带 `detail`，读口不被触到。换筛选、换 `q`、换排序、换选择器或拿到另一册去用的旧游标答 400「游标与本次的排序或筛选不符，请从第一页重取」，原条件下照收。读库失败答 500 `NO_ANSWER_FORMED`、不带 outcome：`TestNodeOperationsRecordsQueryAnswers500WhenTheRegisterFails` 含新册 `identity-review-unit`，并钉 `registry=unidentified-item` 只 `units` 那一册读不动、`items` 完好时也整答 500（复核修订补）。
- ✅ **渠道未配置时参数写对写错都答 403**：`TestUnconfiguredRecordsAnswer403WhateverThePageParameters`；册名缺席、为空或认不出时与合规的一问逐字节同答 403：`TestAnUnknownRegistryOnAnUnconfiguredChannelAnswers403`；送站两条列表：`TestUnconfiguredDeliveryScopeListsAnswerOne403WhateverTheQuery`（修订轮补）对不带参数的一问、带节点排序与 `q` 的合规一问、坏 `sort`、坏 `after`、集外键逐字节同答 403，既有的 `TestDeliveryScopeReadsAnswerTheEnvelopeGradesFirst` 另证信封各格都先于坏参数作答。
- ✅ **迁移无 CR、无 BOM**：按字节查过 `3a20b2bcc` 以来分支上改动的全部文件。

门（作者自验，WSL，钉 `a60f744dc`；`bb8e4f322` 只改本票与一句测试头注；修订轮的自验见评审记录）：改动的 `.go` 文件 `gofmt -l` 为空；`go build ./...`、`go vet ./...` 退 0；`IDP_PARCEL_POSTGRES_DSN` 设着，`go test -count=1 -p 1 -v <包>`，各包顶层 / 含子测 PASS 数如下，SKIP 与 FAIL 全为 0：

| 包 | PASS |
|---|---|
| `internal/nodeoperations/adapters/postgres`（真库） | 165 / 224 |
| `internal/nodeoperations/adapters/http` | 105 / 386 |
| `internal/nodeoperations/application` | 140 / 360 |
| `internal/nodeoperations/domain` | 78 / 162 |
| `internal/nodeoperations` 其余各包 | accessidentity 10 / 48，documentnumbers 1 / 1，identity 5 / 5，inbox 16 / 29，parcelshipment 12 / 12，partycommercial 3 / 6，transportfulfillment 7 / 7，ports 3 / 3（networkrouting 无测试） |
| `cmd/parcel-api`（含真库） | 446 / 654 |
| `cmd/parcel-dispatch`（含真库） | 319 / 506 |
| `internal/architecture` | 58 / 174 |
| `migrations` | 1 / 1 |
| `internal/platform/migrate`（真库） | 8 / 8 |

### 判断项

1. **选择器键是 `registry`**：派单写的是 `kind`，端点与管理台实际按 `registry` 分派，照实登 `registry`。`kind` 在本票里是收寄册的筛选维（收寄的两种结果），不是选择器。
2. **门的次序**：迁移前是方法 → 册名 → Intake，`TestAnUnknownRegistryIsRefusedBeforeTheChannelCheck` 钉着「认不出的册名先于渠道未配置答 400」。spec「端点先 Intake、后解码查询参数」要渠道未配置时对一切请求同答 403、答复不随请求内容变，册名也是查询参数，于是挪到 Intake 之后（`a60f744dc`）：旧用例换成未配置的渠道上缺席、空与认不出的册名都与合规的一问逐字节同答 403；配置了的渠道上认不出的册名仍答 400，`detail` 只列本端点认的册名。同包 `catalogue_intake.go` 的门次序头注随之改写。
3. **新增 `registry=identity-review-unit`**：`ListIdentityReviewUnits` 原只在 `registry=unidentified-item` 的答复里交回全部 `units`，无从翻页；现自成一册（outcome `IDENTITY_REVIEW_UNITS_LISTED`），`registry=unidentified-item` 的 `units` 改为这一册不带条件的第一页，答复里的 `page` 只属于 `items`，`items` 的排序、筛选、`q` 与游标不套到 `units` 上。单元行加 `latestRecordedAt`（这一册的排序维）。管理台读的 `units` 各格没变；但单元多于一页时 `unidentified-item` 答复里只有第一页，要看全得改走新册名，归 14。
4. **同刻收尾改倒序**：集运单元（单元号正序）不变；其余各册迁移前同刻按标识正序，现按 ADR-0144 决定三与排序维同向；只在排序列相等时次序不同。既有用例里依赖同刻正序的断言随之改。
5. **大册与精确 `total`**：收寄、待识别实物、成员关系按实物计量，控制转移按交接事件计量，是 ADR-0144「越权风险点」里「精确 `total`」那条最先撞上的一类；照决定五给精确值，声明头注写明超出租户级规模时回该记录改给 null。
6. **聚合册的导出列**：身份复核单元按导出列 `latestRecordedAt` 取页，计数与取页都要在租户的全部待识别行上分组，导出列上建不了同形索引，待识别行积压多时这一读的代价请评审看一眼。导出列会前移，两个方向各有一种后果（声明头注写了，修订轮补）：倒序时，一件还没翻到的实物在翻页期间又登一版待识别，挪到已翻过的那头，这一趟可能漏；已翻过的前移不重出（真库用例钉的是这一半）。正序时反过来：已翻过的那件跳到游标之后，这一趟可能再出现一次；还没翻到的只往后挪，不漏。
7. **0027 同名重建**：0017、0021 建的那几条索引前缀与新序相同而决胜键反向或缺标识段，DROP 后同名 CREATE，不让前缀相同的两条索引在写路径上各付一次。迁移执行器只收能在事务内跑的迁移，不能 `CONCURRENTLY`：重建期间这几张表的写被挡，表大时上线窗口要算进去。
8. **管理台与脚本带的参数**：管理台 `NodeOperationsReviewPage` 经 `records-api.ts` 只带 `registry`（`reception`、`unidentified-item`、`consolidation-unit`）；送站两区只带 `node`、`documentNumber` 与区间两端（`expectedArrivalFrom`/`expectedArrivalUntil`、`openedFrom`/`openedUntil`），都已登为选择器，空值照收。`scripts/` 不调这三条路由。成功的答复只多 `page` 与按实物单元行上的 `latestRecordedAt`，管理台不改照旧能用。
9. **查询串自报租户与页大小从被忽略改为答 400**：`operator_read_intake_test.go` 里那一格原断言被忽略、照答，现改为答 400 且读口不被触到（决定四：集外的键）。
10. **派生状态没放作筛选维**：预报撤回与否（撤回册上有没有那一行）、交接单关闭与范围成立（关闭册上有没有那一行、声称的账户是否全部确认）、身份复核单元此刻冲突与否与处置过没有（各链的当前有效版本）都是派生的，本票没放作筛选维，理由在 `ports/catalogue_pages.go` 各册声明的头注里。派单方已把它们收进 16 号票（`.scratch/catalogue-read-pagination-rollout/issues/16-derived-state-filter-dimensions.md`，在分支 `cursor/catalogue-pagination-rollout` 上），本票不改那张票。
11. **复核单元的行标识**：作业实物是 jsonb 键，库里不保证在，非空靠收寄写口（行模型 `intakeRow` 的 `unit` 不省略）。写口若放进缺键的行，`bb8e4f322` 的读面上那一组的作业实物是 NULL：倒序时它排在同刻各件之前，取到它的那一页扫描报错；正序时它排在同刻各件之后，游标若停在同刻的某一件上，键集比较对它得 NULL，那一组就可能从后面各页消失，`total` 却还数着它——spec「可排维与行标识的列必须非空」要防的就是这个。修订轮照 spec 给的办法在读面先导出成非空列：`COALESCE(intake ->> 'unit', '')`，缺键的行归到空串那一组，它落进交回的那一页时报错，两个方向都不再悄悄跳过。这一判在 `cataloguepage.Finish` 切页之后、只查本页的行（复核修订挪的）：多取来判下一页的那一行是它时，不含它的本页照常答；游标位置仍取自 `ORDER BY` 两列的扫描值。没让空串那一组当一件实物照常列出：坏行响亮、不演成看起来正常的一件，同 `TestABrokenDispositionDocumentIsLoud` 对坏处置文档的取向。也没在库里加约束：那是改收寄写口的表约束，超出本票的索引迁移，加约束还要在锁下扫全表验旧行。这个取向有代价（复核修订补）：`registry=unidentified-item` 的 `units` 就是本册的第一页，`serveUnidentifiedItems` 读它出错时整答 500；缺省序是最近登记倒序，新落的缺键行排在最前，于是整条待识别队列连同完好的 `items` 一起答不出，直到登得比它新的实物把它挤出首页（`registry=identity-review-unit` 的第一页同此）。迁移前（`3a20b2bcc`）的读面按 `(r.intake ->> 'unit') = units.unit` JOIN 链头，NULL 那一组连不上任何一行，被悄悄丢掉、照答 200。另：本包 `writeProblem` 不带 `err`、不记日志，报错里带的登记时刻没人看得到；这是全包既有做法，不在本票改。

### 未验

- 管理台没在浏览器里过：本票不改管理台，答复只多格。
- 索引只有送站两册经 `delivery_scope_catalogue_plan_test.go` 在灌量的库上用 EXPLAIN 证明被用上；收寄、待识别、成员关系、控制转移四条只按形状对过缺省序，身份复核单元的分组读走哪条索引没看计划。
- 全量 `scripts/test-full.sh` 没跑。
- 迁移编号 0027 进 main 时按 tip 重核。

## 评审记录

**评审 ← 非作者评审 · 钉 `bb8e4f322`**（经通道 1 转来）

| 评审意见 | 处理 |
|---|---|
| 阻断：「做法与判据」那段按序号引 spec 的取舍，是跨文件的序数（AGENTS.md「改文档」只豁免已接受 ADR 的决定号） | 改引原句：「是 spec「`total` 一律给精确值」点名的那类册」。`grep -nE '取舍 ?[0-9一二三四五六七八九]'` 对本票与分支自 `3a20b2bcc` 以来改过的全部文件零命中 |
| 非阻断：派生状态没放筛选维（预报撤回、交接单关闭与范围、复核单元冲突与处置）只写在声明头注里，没进判断项 | 判断项 10，写明已收进 16 号票 |
| 非阻断：`IdentityReviewUnitPages` 的排序列会前移，判断项 6 写了，声明头注没写 | 头注补上，两个方向都写（本册的可排维没限方向，正序也放出了）：倒序时挤到已翻过的那头、这一趟可能漏；正序时跳到游标之后、可能再出现一次。判断项 6 一并补正序那一半 |
| 非阻断：「排序列与标识列在建表迁移里都是 NOT NULL」不实——复核单元的标识是 jsonb 键，非空靠写口 | 按实改写；读面把作业实物导出成非空列，缺键的那一组取到即报错（判断项 11）。`TestIdentityReviewUnitsAreLoudAboutPendingRowsWithoutAUnit` 放回 `bb8e4f322` 的读面上跑是 FAIL：正序走完 2 页、见到 4 件、`total` 5；修订后两个方向都报错。没在库里加约束，理由在判断项 11 |
| 非阻断：送站两册的未配置用例只有 `TestDeliveryScopeReadsAnswerTheEnvelopeGradesFirst` 一问 | 补 `TestUnconfiguredDeliveryScopeListsAnswerOne403WhateverTheQuery`：不带参数的一问、带节点排序与 `q` 的合规一问、坏 `sort`、坏 `after`、集外键逐字节同答 403，读口不被触到 |

修订轮自验（钉 `e8f309dde`，WSL）：改过的 `.go` 跑过 `gofmt -w`；`go build ./...` 与 `go vet ./internal/nodeoperations/... ./cmd/...` 退 0；`IDP_PARCEL_POSTGRES_DSN` 设着，`go test -count=1 -p 1 -v`，顶层 / 含子测 PASS 数如下，SKIP 与 FAIL 全为 0：

| 包 | PASS |
|---|---|
| `internal/nodeoperations/adapters/http` | 106 / 389 |
| `internal/nodeoperations/adapters/postgres`（真库） | 166 / 225 |
| `internal/nodeoperations/ports` | 3 / 3 |
| `cmd/parcel-api`（含真库；读面 SQL 动过） | 446 / 654 |

**复核 ← 非作者评审修订轮复核 · 钉 `55280d64d`**（经通道 1 转来）

结论：上一轮的阻断已解，上一轮的非阻断都已到位，没有新阻断；「缺键那一组响亮报错」这个取舍本身合适，分格到 500 `NO_ANSWER_FORMED` 也对。另提的非阻断与处理：

| 评审意见 | 处理 |
|---|---|
| 非阻断：缺键检查写在扫描循环里、早于 `cataloguepage.Finish`，多取来判下一页的那一行是坏行时，不含它的本页也报错（评审的探针：A、B、X、C 按登记时刻倒序，X 缺键，页大小 2 的第一页只该显示 A、B，却报错） | 挪到 `Finish` 之后，只查本页的行，照 07 的 `scannedInitialRoute` 那样切页后再判；扫描时作业实物原样留空串，游标位置仍取自 `ORDER BY` 两列的扫描值。判断项 11 随之改写 |
| 非阻断：`TestIdentityReviewUnitsAreLoudAboutPendingRowsWithoutAUnit` 遇到任何错误都收手，换回旧读面时倒序那支靠 `cannot scan NULL` 照样通过；读库失败答 500 的用例名单缺新册 `identity-review-unit`，`unidentified-item` 没有只 `units` 读不动的情形 | 缺键用例断言报错含 `without an intake unit`，补「缺键那一组只是多取的那一行」情形：第一页照答 A、B、带 `next`、`total` 4，第二页报那一句错。用例放回 `55280d64d` 的读面上是 FAIL（第一页答不出），放回 `bb8e4f322` 的读面上也是 FAIL（报的是 `cannot scan NULL`，不是那一句）。`TestNodeOperationsRecordsQueryAnswers500WhenTheRegisterFails` 名单补 `identity-review-unit`，另加子测：`items` 照答、只 `units` 那一册读不动时整答 500 `NO_ANSWER_FORMED`、不带 outcome，替身记着 `units` 那一册确被问到 |
| 非阻断：判断项 11 没写这个取舍的代价 | 补上：`units` 读不动时 `unidentified-item` 整答 500，新落的缺键行在缺省序里排最前，整条待识别队列连同完好的 `items` 一起答不出，直到登得比它新的实物把它挤出首页；迁移前的 JOIN 悄悄丢掉这一组、照答 200。另记本包 `writeProblem` 不带 `err`、不记日志，报错里的登记时刻没人看得到（全包既有做法，不在本票改） |
| 非阻断（Standards）：`TestUnconfiguredDeliveryScopeListsAnswerOne403WhateverTheQuery` 按下标取 `scopeReads`，切片定义在另一个文件 | 同文件加 `scopeReadNamed`，按 `name` 挑；本票在同一文件里写的 `TestDeliveryScopeListsCarryThePageAndPassTheDecodedQuery`、`TestDeliveryScopeListsRefuseQueriesOutsideTheDeclaration` 一并改。`query_delivery_scope_records_test.go` 里基上既有的按下标取法不是本票写的，没动 |

复核修订自验（钉 `bd0dc1b5c`，WSL）：改过的 `.go` 跑过 `gofmt -w`；`go build ./...` 与 `go vet ./internal/nodeoperations/... ./cmd/...` 退 0；`IDP_PARCEL_POSTGRES_DSN` 设着，`go test -count=1 -p 1 -v`，顶层 / 含子测 PASS 数如下，SKIP 与 FAIL 全为 0：

| 包 | PASS |
|---|---|
| `internal/nodeoperations/adapters/http` | 106 / 391 |
| `internal/nodeoperations/adapters/postgres`（真库） | 166 / 225 |
| `internal/nodeoperations/ports` | 3 / 3 |
| `cmd/parcel-api`（含真库） | 446 / 654 |

## 进 main 记录（2026-11-06，通道 1 推送）

**出处**：分支 `cursor/crpr-06-node`，tip `5dbfe532c`。**评审**：首轮钉 `bb8e4f322` 拦一条阻断（票面引 spec 取舍序号），修订轮复核钉 `55280d64d` 两轴无阻断。

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
