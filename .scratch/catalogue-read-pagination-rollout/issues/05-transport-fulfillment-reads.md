# 05 transport-fulfillment 各册迁到 ADR-0144

Category: enhancement
Status: in-progress——作者完成，待非作者评审（分支 `cursor/crpr-05-transport`，基 `d48c5b130`；迁移编号 transport-fulfillment `0058`，进 main 时按 tip 重核）
Blocked by: 01
地盘：`internal/transportfulfillment/{ports,adapters/http,adapters/postgres}` 里下表各册的读端口、端点与读面及其测试；`cmd/parcel-api` 里这些读口的占位；
`migrations/transport_fulfillment` 的一条新迁移（索引）。
出处：ADR-0144 决定三至七；spec「横贯各子票的取舍」。

## 本票的册（今天的次序钉 `a1274a6b8`）

`/transport-fulfillment-records` 按 `kind` 分派的十三种（`ReviewCatalogueRead` 的 `ListTransportSchedules`、`ListScheduleTemplates`、`ListCapacityPools`、`ListTransportHandovers`、
`ListEffectiveDeliveries`、`ListCarrierMasterDocuments`、`ListTransportPreparations`、`ListDeliveryDispatchPendings`、`ListDeliveryDispatchReaskCadences`、
`ListActualFulfillmentUnformed`、`ListBookingRequests`、`ListBookingAnswerBounces`、`ListPendingAttemptResults`），外加：

| 路由 | 读端口方法 | 今天的 `ORDER BY` |
|---|---|---|
| `/transport-fulfillment-external-tracking-facts` | `ExternalTrackingFactReviewRead.ListCurrentExternalTrackingFacts` | `occurred_at, fact_ref, version` |

各种今天的次序见 `adapters/postgres/review_catalogue.go` 等读面；开工先核 `/transport-fulfillment-carrier-first-effective-pickups` 是否按租户上列（读端口不收 `limit`），
按租户上列则一并迁、按对象取链则属单条读取不在内。

## 做法与判据

见 spec「每张迁移子票（02–13）的做法与判据」。本票特有：交接、有效妥投、订舱等按包裹或事件计量，是取舍 4 点名的大册。

## 完成记录（作者会话）

分支 `cursor/crpr-05-transport`，基 `d48c5b130`。开工先核的那一条：`/transport-fulfillment-carrier-first-effective-pickups` 的读端口
`ListByObject` 必给 `?object=`、不收 `limit`，是一件对象的事实链回读，属单条读取，**不迁**。

### 落地

| SHA | 文件 | 做了什么 |
|---|---|---|
| `5d456b8de` | `internal/transportfulfillment/ports/catalogue_pages.go`（新） | 各册查询声明：缺省序、可排维、筛选维、行标识；`registry` 与事实的 `source`、`view` 登为选择器 |
| | `ports/catalogue_read.go`、`ports/external_tracking_fact_review.go` | 读端口收 `cataloguepage.Query`、交回 `cataloguepage.Result`；租户仍在签名上；挡回册原来单列的 `BookingAnswerBounceFilter` 参数并进查询对象 |
| | `adapters/postgres/catalogue_page.go`（新）、`review_catalogue.go`、`delivery_dispatch_pending.go`、`delivery_dispatch_reask_cadence.go`、`actual_fulfillment_unformed.go`、`external_tracking_fact_review.go` | 各册落列声明（`MustBind`）与取页：`total` 与本页同一组条件，`limit` 非正仍拒；班次与周期安排的当前版由 `DISTINCT ON` 改为「没有更高版本」的反连接 |
| | `adapters/http/catalogue_page.go`（新）、`query_transport_fulfillment_records.go`、`query_external_tracking_facts.go`、`register_effective_delivery.go` | 端点先经本册声明解码，坏参数先于 Intake 答 400 `MALFORMED_REQUEST`、理由进 `detail`；答复加 `page`，`size` 取 Intake 页大小 |
| | `adapters/http/catalogue_page_test.go`（新）及原有端点用例 | 端点判据用例；原有替身只改签名 |
| | `adapters/postgres` 原有用例 | 调用点改走第一页（`catalogue_page_helpers_test.go` 的 `firstPage`、`rowsOf`）；交接版本链一条改期望次序（见判断项） |
| | `cmd/parcel-api/unwired_orchestration.go` | 占位只改签名：`unwiredTransportFulfillmentRecords` 的各 List 方法与 `unwiredExternalTrackingFactReview.ListCurrentExternalTrackingFacts` |
| | `cmd/parcel-api`、`cmd/parcel-dispatch`、`cmd/parcel-transport-register` 的测试 | 旧调用点改签名（`tf_catalogue_page_test.go` 的 `tfFirstPage`、`tfRowsOf`），断言不变 |
| | 本票 | Status 记分支与基 |
| `5fe4d45bf` | `adapters/postgres/catalogue_page_test.go`（新） | 各册真库翻页用例 |
| `47e709b4c` | `migrations/transport_fulfillment/0058_catalogue_read_pagination_indexes.sql`（新） | 取页索引 |

### 逐册声明

缺省序都取迁移前 `ORDER BY` 的首列与方向；行标识取主键去掉租户，与排序同向决胜。各册为什么是这个缺省序写在 `catalogue_pages.go` 各声明头注。

| 册（`registry`）· 读端口方法 | 缺省序 | 可排维 | 筛选维 | `q` 列 | 索引 |
|---|---|---|---|---|---|
| `transport-schedule` · `ListTransportSchedules` | `-departsAt` | `departsAt`、`scheduleId` | `origin` | 班次、连接、起讫节点、周期安排代码 | 新 `transport_schedule_by_tenant_departure` |
| `transport-schedule-template` · `ListScheduleTemplates` | `template` | `template`、`recordedAt` | — | 代码、连接、起讫节点 | 复用主键 |
| `capacity-pool` · `ListCapacityPools` | `poolId` | `poolId` | `dimension` | 池、班次 | 复用主键 |
| `transport-handover` · `ListTransportHandovers` | `-judgedAt` | `judgedAt`、`object` | `verdict` | 对象、范围、交出方、接收方 | 新 `transport_handover_by_tenant_judged` |
| `effective-delivery` · `ListEffectiveDeliveries` | `-occurredAt` | `occurredAt`、`object` | `gateReading` | 对象、尝试、地点、收件人、证明 | 新 `effective_delivery_current_by_tenant_occurred`（`WHERE is_current`） |
| `carrier-master-document` · `ListCarrierMasterDocuments` | `-recordedAt` | `recordedAt`、`document` | `standing` | 总单、签发方、范围、委托、订舱 | 新 `carrier_master_document_by_tenant_recorded` |
| `transport-preparation` · `ListTransportPreparations` | `-judgedAt` | `judgedAt`、`object` | `outcome` | 段、对象、连接、起讫节点、选中班次 | `transport_preparation_judged` 换成补齐标识段的同名索引 |
| `delivery-dispatch-pending` · `ListDeliveryDispatchPendings` | `lastAskedAt` | `lastAskedAt`、`firstAnsweredAt` | — | 段、对象、入场依据 | 复用 `delivery_dispatch_pending_open_by_tenant` |
| `delivery-dispatch-reask-cadence` · `ListDeliveryDispatchReaskCadences` | `-recordedAt` | `recordedAt` | — | 版本、依据 | 复用 `delivery_dispatch_reask_cadence_current` |
| `actual-fulfillment-unformed` · `ListActualFulfillmentUnformed` | `endedAt` | `endedAt`、`commission` | `reason` | 段、对象、委托、协议对象 | 复用 `actual_fulfillment_unformed_open_by_tenant` |
| `booking-request` · `ListBookingRequests` | `-requestedAt` | `requestedAt`、`booking` | — | 订舱、委托、班次、单据号 | 新 `booking_request_by_tenant_requested` |
| `booking-answer-bounce` · `ListBookingAnswerBounces` | `-lastBouncedAt` | `lastBouncedAt`、`count` | `booking`、`source`（无词表）、`basis` | 答复方、订舱、答复 | 新 `booking_answer_bounce_by_tenant_last_bounced` |
| `pending-attempt-result` · `ListPendingAttemptResults` | `occurredAt` | `occurredAt` | `side` | 尝试、对象、任务、来源身份 | 复用 `pickup_attempt_result_pending` 与 `delivery_attempt_result_pending` |
| 外部轨迹事实 · `ListCurrentExternalTrackingFacts`（选择器 `source`、`view`） | `occurredAt` | `occurredAt`、`receivedAt` | `effectiveBasis` | 事实、对象、来源事件、状态 | 新 `external_carrier_tracking_fact_by_source_occurred`（源进键） |

### 判据

- ✅ **真库用例**：`TestEveryTransportFulfillmentCataloguePagesWithoutRepeatsOrGapsAndCountsUnderItsConditions` 每册一个子测试，同一套表驱动用例——空册答空页、
  `total` 为 0；第一页两行、`total` 5，再添一行后接着翻，`total` 6、已翻的不重出、原有的不漏；换一个可排维翻完六行；`total` 随筛选（维内为或、维与 `q` 为与）
  与 `q`（不分大小写、`%` 按字面）变。行经各册写侧适配器或编排真实落库，夹具全为 `SYN-`。计数：
  `IDP_PARCEL_POSTGRES_DSN=… go test -count=1 -p 1 -v -run TestEveryTransportFulfillmentCatalogue ./internal/transportfulfillment/adapters/postgres/ | grep -c -- '--- PASS'`
  得 15（母测加各册子测），`--- SKIP` 0（实测于 `47e709b4c`）。
- ✅ **端点用例**：未知键、集外排序维、词表外筛选值、换条件拿旧游标各答 400 `MALFORMED_REQUEST`、`detail` 非空、不进读口——
  `TestRecordsCatalogueRefusesQueriesOutsideTheDeclaration` 对授予齐备与未配置两种 Intake 各跑一遍，另有一条对照证合法参数在未配置的渠道上答 403，
  即坏参数先于 Intake；`TestExternalTrackingFactsRefuseQueriesOutsideTheDeclaration` 同。拿一册的游标去翻另一册答 400（`TestEveryRecordsRegistryIsItsOwnCatalogue`）。
  答复带 `page`，`size` 取 Intake 页大小、末页 `next` 为 `null`（`TestEveryRecordsRegistryAnswersAPageCell`、`TestExternalTrackingFactsAnswerAPageCellAndPassTheDeclaredConditions`）。计数：
  `go test -count=1 -v -run 'TestEveryRecordsRegistry|TestRecordsCatalogue|TestExternalTrackingFacts(Answer|Refuse)' ./internal/transportfulfillment/adapters/http/ | grep -c -- '--- PASS'`
  得 56，`--- SKIP` 0（实测于 `47e709b4c`）。
- ✅ **迁移无 CR、无 BOM**：`0058` 与本票文件逐个查过，无 `\r`，首三字节不是 `efbbbf`。
- ✅ 做什么 2「`limit` 非正仍拒」：各读面先拒 `limit < 1`，未改。做什么 4 的次序：两个端点都是选择器 → 声明解码 → Intake。
- ✅ 取舍 6：各可排维所在列在建表语句里都是 `NOT NULL`；标识列是主键段；待确认清单派送一侧的来源身份是字面空串。
- ✅ 取舍 7：见上表索引一列，复用的已有索引同在 `0058` 头注点名。

自验（带 DSN、`-p 1`，实测于 `47e709b4c`）：`go build ./...`、`go vet ./...` 全仓通过；改动包与反向依赖（`go list` 连测试导入算出）全数通过、无 SKIP——
`transportfulfillment/adapters/postgres` 586、`adapters/http` 580、`application` 1326、`cmd/parcel-api` 654、`cmd/parcel-dispatch` 506、
`cmd/parcel-transport-register` 5，其余各反向依赖包同过；`./internal/architecture/...` 174，`./migrations/` 1，`./internal/platform/migrate/` 8。

### 判断项

1. **选择器叫 `registry`**：票面写「按 `kind` 分派」，实际查询键与管理台用的都是 `registry`，照实登。
2. **决胜键方向变了的册（取舍 2）**：缺省序倒序、而迁移前标识升序的——班次、交接、有效交付、总单、运输准备、订舱、挡回——迁移后标识随排序倒序，只在首列完全
   相同时次序与迁移前不同。`TestReviewCatalogueListsHandoverVersionChain` 里两版裁决时刻相同，期望改为后一版在前。重问间隔迁移前已是双倒序；升序各册不变。
   待确认清单的标识末段补了来源身份：揽收一侧同一尝试同一对象可由不同来源报来，迁移前那几行之间没有定序。
3. **大册（取舍 4）**：交接、有效交付、运输准备按包裹计量，外部轨迹事实按事件计量；`total` 照缺省给精确值，超出租户级规模时回 ADR-0144 改 `null`。
4. **订舱册不放状态筛选维**：状态词由读口从当前那一份答复派生，库里没有列；按它筛只能先读后筛，`total` 就不是真数。这与取舍 3「有状态的册放出那一维」
   有张力；要补得先把状态落成列或在 SQL 里导出，属另票。
5. **管理台、脚本与 cmd 发的参数**：管理台发 `registry`、挡回册的 `booking` 与 `source`、事实页的 `source` 与 `view`；`scripts/demo-seeds` 只发 `registry`；
   `cmd/parcel-api` 的探针发 `registry` 与 `source`+`view`（`object` 只发给不迁的首次有效收寄）。`registry`、`source`、`view` 登为选择器；挡回册的 `booking`、
   `source` 登为无词表的筛选维，取值立不立得起来仍由端点照迁移前的判据判（空白、NUL、超长答 400）。
6. **自报的 `tenant`、`limit` 现在答 400**：迁移前端点收下不用，现在是未知键（`TestRecordsCatalogueRefusesSelfReportedTenantAndLimit`）。管理台、脚本与 cmd 都不发。
   未知或缺席的 `registry` 照迁移前在解码之前答 400、不带理由。
7. **取舍 5 不适用**：本票各册迁移前都没有计数与 `truncated`。
8. **后台路径未动**：`DeliveryDispatchPendingQueue` 的 `ListPending`、`CountPending` 与 `DeliveryDispatchAskLedger.ListAsks` 的签名与 SQL 不变；派送待形成册的
   HTTP 读面另起取页函数，与后台只共用「待形成」谓词常量 `deliveryDispatchPendingOpen`。
9. **当前版改反连接**：班次与周期安排原用 `DISTINCT ON` 取每个的最高版，它要求外层先按分组列排，与按册排序取页相冲。
10. **端点用例补册名**：端点用例的 registry 封闭集补上了迁移前漏列的 `actual-fulfillment-unformed`。
11. **提交没有逐册切**：十三种共用一个读端口接口 `ReviewCatalogueRead` 与一个分派端点，逐册切会让中间提交里端点半迁（有的册拒未知键、有的不拒），
    故按「代码与端点用例 → 真库用例 → 索引迁移」三笔提交。

### 没验证的

- 管理台没跑（归 14）：页面还不读 `page`、只显示第一页；页大小与迁移前的上限同取自 Intake，可见行数应不变，但没在浏览器里看。
- 索引只按形状对齐缺省序，没在有量的数据上看 `EXPLAIN`。
- `scripts/demo-seeds/demo-run/chain.sh` 没跑：它取运输准备册第一页找本件的几段，缺省序新近在前，按理落在第一页。
- 迁移编号 `0058` 进 main 时按 tip 重核。
