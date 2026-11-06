# 06 node-operations 八册迁到 ADR-0144

Category: enhancement
Status: in-progress——作者完成，待非作者评审（分支 `cursor/crpr-06-node`，基 `3a20b2bcc`；完成记录见文末）。迁移编号 node-operations `0027`（进 main 时按 tip 重核）
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

见 spec「每张迁移子票（02–13）的做法与判据」。本票特有：收寄、集包成员等按实物计量，是取舍 4 点名的大册；聚合导出的册（如身份复核单元按 `latest` 排）键集落在导出列上。

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
| 本笔 | 本票；`adapters/postgres/delivery_scope_catalogue_plan_test.go` 头注 | 完成记录；头注改指迁移 0027 |

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

`registry` 在六册上都登为选择器。决胜键一律与排序维同向。行标识是各登记表主键去掉租户段（控制转移是交接键四段，身份复核单元是分组键作业实物）；排序列与标识列在建表迁移里都是 `NOT NULL`。两条按标识取单条的路由没迁，照票面。

### 完成判据

- ✅ **真库用例**（`SYN-` 数据，带 DSN）：`adapters/postgres/catalogue_pages_test.go` 八册各一个用例——`TestReceptionsPageWhileNewIntakesArrive`、`TestUnidentifiedItemsPageWhileNewPendingItemsArrive`、`TestIdentityReviewUnitsPageOnTheirLatestRecording`、`TestConsolidationUnitsPageByUnitIDWhileNewUnitsOpen`、`TestConsolidationMembersPageWhileNewFactsArrive`、`TestPhysicalControlTransitionsPageWhileNewTransitionsArrive`、`TestDeliveryPreAdvicesPageWhileNewPreAdvicesArrive`、`TestDeliveryHandoverNotesPageWhileNewNotesOpen`。每个都按页大小 2 走完一遍：取完第一页后在游标之前与之后各落一行，走下来恰是原有各行按缺省序各出一次、再加上游标之后那一行，不重出、不漏，游标之前那行这一趟不出现；`total` 随新落的行变；同一排序维反向再走一遍；每个筛选词与 `q` 各收窄出预期的 `total`（含筛选与 `q` 相与、大小写不敏感，`q=%25` 与 `q=_` 答 0）；没有行的租户答空页、`total` 0、无 `next`。身份复核单元另证导出列：已翻过的实物又登一版待识别，导出列前移，它不在后面的页上重出。送站两册另带 `node` 选择器走一遍。
- ✅ **端点用例**：`adapters/http/query_node_operations_records_page_test.go`（六册表驱动）与 `query_delivery_scope_records_page_test.go`。答复带 `page`，`size` 取 Intake 页大小，`next`、`total` 原样带出；空册 200、空数组、`total` 0、`next` 为 null；查询对象原样交给读口。未知键（含查询串自报租户与页大小）、集外排序维、另一册的筛选维、词表外筛选值、空筛选值、重复的排序、解不开的游标各答 400 带 `detail`，读口不被触到。换筛选、换 `q`、换排序、换选择器或拿到另一册去用的旧游标答 400「游标与本次的排序或筛选不符，请从第一页重取」，原条件下照收。
- ✅ **渠道未配置时参数写对写错都答 403**：`TestUnconfiguredRecordsAnswer403WhateverThePageParameters`；册名缺席、为空或认不出时与合规的一问逐字节同答 403：`TestAnUnknownRegistryOnAnUnconfiguredChannelAnswers403`；送站两条列表由既有的 `TestDeliveryScopeReadsAnswerTheEnvelopeGradesFirst` 带集外键证信封各格先答。
- ✅ **迁移无 CR、无 BOM**：按字节查过 `3a20b2bcc` 以来分支上改动的全部文件。

门（作者自验，WSL，钉 `a60f744dc`，本笔只改本票与一句测试头注）：改动的 `.go` 文件 `gofmt -l` 为空；`go build ./...`、`go vet ./...` 退 0；`IDP_PARCEL_POSTGRES_DSN` 设着，`go test -count=1 -p 1 -v <包>`，各包顶层 / 含子测 PASS 数如下，SKIP 与 FAIL 全为 0：

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
2. **门的次序**：迁移前是方法 → 册名 → Intake，`TestAnUnknownRegistryIsRefusedBeforeTheChannelCheck` 钉着「认不出的册名先于渠道未配置答 400」。spec 取舍「端点先 Intake、后解码查询参数」要渠道未配置时对一切请求同答 403、答复不随请求内容变，册名也是查询参数，于是挪到 Intake 之后（`a60f744dc`）：旧用例换成未配置的渠道上缺席、空与认不出的册名都与合规的一问逐字节同答 403；配置了的渠道上认不出的册名仍答 400，`detail` 只列本端点认的册名。同包 `catalogue_intake.go` 的门次序头注随之改写。
3. **新增 `registry=identity-review-unit`**：`ListIdentityReviewUnits` 原只在 `registry=unidentified-item` 的答复里交回全部 `units`，无从翻页；现自成一册（outcome `IDENTITY_REVIEW_UNITS_LISTED`），`registry=unidentified-item` 的 `units` 改为这一册不带条件的第一页，答复里的 `page` 只属于 `items`，`items` 的排序、筛选、`q` 与游标不套到 `units` 上。单元行加 `latestRecordedAt`（这一册的排序维）。管理台读的 `units` 各格没变；但单元多于一页时 `unidentified-item` 答复里只有第一页，要看全得改走新册名，归 14。
4. **同刻收尾改倒序**：集运单元（单元号正序）不变；其余各册迁移前同刻按标识正序，现按 ADR-0144 决定三与排序维同向；只在排序列相等时次序不同。既有用例里依赖同刻正序的断言随之改。
5. **大册与精确 `total`**：收寄、待识别实物、成员关系按实物计量，控制转移按交接事件计量，是 ADR-0144「越权风险点」里「精确 `total`」那条最先撞上的一类；照决定五给精确值，声明头注写明超出租户级规模时回该记录改给 null。
6. **聚合册的导出列**：身份复核单元按导出列 `latestRecordedAt` 取页，计数与取页都要在租户的全部待识别行上分组，导出列上建不了同形索引，待识别行积压多时这一读的代价请评审看一眼。导出列会前移：一件还没翻到的实物在翻页期间又登一版待识别，它挪到已翻过的那头，这一趟见不到；已翻过的实物前移不会重出（真库用例钉的是这一半）。
7. **0027 同名重建**：0017、0021 建的那几条索引前缀与新序相同而决胜键反向或缺标识段，DROP 后同名 CREATE，不让前缀相同的两条索引在写路径上各付一次。迁移执行器只收能在事务内跑的迁移，不能 `CONCURRENTLY`：重建期间这几张表的写被挡，表大时上线窗口要算进去。
8. **管理台与脚本带的参数**：管理台 `NodeOperationsReviewPage` 经 `records-api.ts` 只带 `registry`（`reception`、`unidentified-item`、`consolidation-unit`）；送站两区只带 `node`、`documentNumber` 与区间两端（`expectedArrivalFrom`/`expectedArrivalUntil`、`openedFrom`/`openedUntil`），都已登为选择器，空值照收。`scripts/` 不调这三条路由。成功的答复只多 `page` 与按实物单元行上的 `latestRecordedAt`，管理台不改照旧能用。
9. **查询串自报租户与页大小从被忽略改为答 400**：`operator_read_intake_test.go` 里那一格原断言被忽略、照答，现改为答 400 且读口不被触到（决定四：集外的键）。

### 未验

- 管理台没在浏览器里过：本票不改管理台，答复只多格。
- 索引只有送站两册经 `delivery_scope_catalogue_plan_test.go` 在灌量的库上用 EXPLAIN 证明被用上；收寄、待识别、成员关系、控制转移四条只按形状对过缺省序，身份复核单元的分组读走哪条索引没看计划。
- 全量 `scripts/test-full.sh` 没跑。
- 迁移编号 0027 进 main 时按 tip 重核。
