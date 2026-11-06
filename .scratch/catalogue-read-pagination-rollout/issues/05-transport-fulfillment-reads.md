# 05 transport-fulfillment 各册迁到 ADR-0144

Category: enhancement
Status: in-progress——非作者评审（钉 `1d76b5990`）无阻断，三条非阻断已处理，待进 main（分支 `cursor/crpr-05-transport`，基 `d48c5b130`；迁移编号 transport-fulfillment `0058`，进 main 时按 tip 重核；评审记录见文末）
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

见 spec「每张迁移子票（02–13）的做法与判据」。本票特有：交接、有效妥投、订舱等按包裹或事件计量，是 spec「`total` 一律给精确值」那条点名的大册。

## 完成记录（作者会话）

分支 `cursor/crpr-05-transport`，基 `d48c5b130`。开工先核的那一条：`/transport-fulfillment-carrier-first-effective-pickups` 的读端口
`ListByObject` 必给 `?object=`、不收 `limit`，是一件对象的事实链回读，属单条读取，**不迁**。

修订轮（派单方 · 通道 1）：spec 补了「端点先 Intake、后解码查询参数」，派单说明补了「02 的非作者评审拦下过的」各条。本轮照派单留在原基上、不 rebase：
两个端点对调门次序，未配置用例补坏选择器与坏参数的变体，照评审各条自查补齐，落在 `343aa9fbb`。下面的判据与自验都是 `343aa9fbb` 上的实测。

### 落地

| SHA | 文件 | 做了什么 |
|---|---|---|
| `5d456b8de` | `internal/transportfulfillment/ports/catalogue_pages.go`（新） | 各册查询声明：缺省序、可排维、筛选维、行标识；`registry` 与事实的 `source`、`view` 登为选择器 |
| | `ports/catalogue_read.go`、`ports/external_tracking_fact_review.go` | 读端口收 `cataloguepage.Query`、交回 `cataloguepage.Result`；租户仍在签名上；挡回册原来单列的 `BookingAnswerBounceFilter` 参数并进查询对象 |
| | `adapters/postgres/catalogue_page.go`（新）、`review_catalogue.go`、`delivery_dispatch_pending.go`、`delivery_dispatch_reask_cadence.go`、`actual_fulfillment_unformed.go`、`external_tracking_fact_review.go` | 各册落列声明（`MustBind`）与取页：`total` 与本页同一组条件，`limit` 非正仍拒；班次与周期安排的当前版由 `DISTINCT ON` 改为「没有更高版本」的反连接 |
| | `adapters/http/catalogue_page.go`（新）、`query_transport_fulfillment_records.go`、`query_external_tracking_facts.go`、`register_effective_delivery.go` | 端点按本册声明解码，坏参数答 400 `MALFORMED_REQUEST`、理由进 `detail`；答复加 `page`，`size` 取 Intake 页大小。这一笔的解码在 Intake 之前，`343aa9fbb` 对调 |
| | `adapters/http/catalogue_page_test.go`（新）及原有端点用例 | 端点判据用例；原有替身只改签名 |
| | `adapters/postgres` 原有用例 | 调用点改走第一页（`catalogue_page_helpers_test.go` 的 `firstPage`、`rowsOf`）；交接版本链一条改期望次序（见判断项） |
| | `cmd/parcel-api/unwired_orchestration.go` | 占位只改签名：`unwiredTransportFulfillmentRecords` 的各 List 方法与 `unwiredExternalTrackingFactReview.ListCurrentExternalTrackingFacts` |
| | `cmd/parcel-api`、`cmd/parcel-dispatch`、`cmd/parcel-transport-register` 的测试 | 旧调用点改签名（`tf_catalogue_page_test.go` 的 `tfFirstPage`、`tfRowsOf`），断言不变 |
| | 本票 | Status 记分支与基 |
| `5fe4d45bf` | `adapters/postgres/catalogue_page_test.go`（新） | 各册真库翻页用例 |
| `47e709b4c` | `migrations/transport_fulfillment/0058_catalogue_read_pagination_indexes.sql`（新） | 取页索引 |
| `53986201f` | 本票 | 完成记录 |
| `343aa9fbb` | `adapters/http/query_transport_fulfillment_records.go`、`query_external_tracking_facts.go`、`catalogue_page.go` | 修订轮：门次序改为方法 → 准入 → 选择器 → 声明解码，渠道未配置时一切请求同答 403 |
| | `adapters/http/catalogue_page_test.go`（重写）、`query_transport_fulfillment_records_test.go`、`query_booking_records_test.go`、`query_external_tracking_facts_test.go`、`operator_read_intake_test.go` | 端点表驱动用例；未配置用例补坏册名、坏筛选与各册坏参数的变体；钉「先于准入答 400」的旧用例改为配置了答 400、未配置答 403，其中两条改名（见判断项）；新加用例的替身值改 `SYN-` |
| | `adapters/postgres/catalogue_page_test.go` | 翻页途中插入一行后断言逐页的精确序列；交接、有效交付、外部轨迹事实、待确认复核改用本文件的 `SYN-` 构造器 |
| | `ports/catalogue_pages.go`、`adapters/postgres/delivery_dispatch_pending.go`、`0058` | 只改注释：spec 取舍序号换成规则原名，去掉数别处东西的计数；`0058` 的 SQL 不变 |
| `1d76b5990` | 本票 | 修订轮完成记录 |
| `0f9a88d93` | `ports/catalogue_pages.go`、`adapters/http/query_transport_fulfillment_records.go`、`adapters/postgres/review_catalogue.go` | 评审收尾：挡回册订舱、答复方两维的维名由 `ports` 导出常量，声明、端点判据与落列都引它；`BookingRequestPages` 头注补订舱册不是大册的理由 |
| | `adapters/http/query_booking_records_test.go`、`catalogue_page_test.go` | 挡回册筛选用例断言 `detail` 是判据自己的理由；`assertPageQueryRefused` 交回 `detail` |
| | `0058` | 头注如实写待确认复核清单的两条部分索引各自同形到哪一段；SQL 不变 |

### 逐册声明

缺省序都取迁移前 `ORDER BY` 的首列与方向；行标识与排序同向作决胜键，取登记表主键去掉租户那几段，待确认复核清单例外（见判断项「待确认复核清单的行标识与索引」）。
各册为什么是这个缺省序写在 `catalogue_pages.go` 各声明头注。

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
| `pending-attempt-result` · `ListPendingAttemptResults` | `occurredAt` | `occurredAt` | `side` | 尝试、对象、任务、来源身份 | 不新建；两侧部分索引 `pickup_attempt_result_pending`、`delivery_attempt_result_pending` 只部分同形（见判断项） |
| 外部轨迹事实 · `ListCurrentExternalTrackingFacts`（选择器 `source`、`view`） | `occurredAt` | `occurredAt`、`receivedAt` | `effectiveBasis` | 事实、对象、来源事件、状态 | 新 `external_carrier_tracking_fact_by_source_occurred`（源进键） |

### 判据（实测于 `343aa9fbb`）

- ✅ **真库：每一册都做足翻页途中插入、空册与条件改 `total`**——`TestEveryTransportFulfillmentCataloguePagesWithoutRepeatsOrGapsAndCountsUnderItsConditions`
  每册一个子测试，跑同一套断言：空册答空页、`total` 0、没有下一页；落五行后第一页两行、`total` 5；这时再落第六行，接着翻的各页 `total` 6，逐页的键序列与期望
  整段相等——缺省序倒序的册里新行排在最前、落在已翻过的一侧，后面各页见不到它；正序的册（周期安排、容量池、派送待形成、实际履约未形成、待确认复核、
  外部轨迹事实）新行出现在末页；两种都要原有五行各见恰好一次。再按一个非缺省可排维翻完六行。条件改 `total` 的断言逐册都有，比的是 `total` 与行数：`q` 不分
  大小写、`%` 按字面；有筛选维的册另有筛选改 `total` 的断言，维内为或、维与 `q` 为与在实际履约未形成等册上验过。行经各册写侧适配器或编排真实落库；
  标识与 `q` 所查各列的夹具值全为 `SYN-`。命令：
  `IDP_PARCEL_POSTGRES_DSN=… go test -count=1 -p 1 -v -run TestEveryTransportFulfillmentCatalogue ./internal/transportfulfillment/adapters/postgres/`，
  `--- PASS` 15（母测加各册子测）、`--- SKIP` 0。
- ✅ **端点表驱动：每一册合规一问带 `page`、集外参数答 400 带 `detail`、未配置时好坏两问都答 403**——`TestEveryMigratedCatalogueCarriesThePageAndDecodesAfterTheIntake`
  每册一个子测试（`registry` 分派的各册与外部轨迹事实）：合规请求答 200，`page.size` 等于替身 Intake 给的页大小（7）；同一请求加一个声明之外的参数
  （`registry` 各册是集外排序维，事实是词表外的有效时间依据）答 400 `MALFORMED_REQUEST`、`detail` 非空；渠道未配置时好、坏两问都答 403
  `ACCESS_CHANNEL_NOT_CONFIGURED`，两份答复体逐字相同，读口不被触到。
- ✅ **端点：未知键、集外排序维、词表外筛选值、换条件拿旧游标各答 400 带 `detail`**——`TestRecordsCatalogueRefusesQueriesOutsideTheDeclaration`（集外键、集外排序维、
  排序维写两次、解不开的游标、各册筛选维的词表外取值、订舱册的状态与待形成册的筛选不在声明里）与 `TestExternalTrackingFactsRefuseQueriesOutsideTheDeclaration`
  （集外键、集外排序维、词表外有效时间依据、解不开的游标、自报租户与页大小）在配置了的渠道上答 400 带 `detail`、读口不被触到，同一问在未配置的渠道上答 403；
  自报的 `tenant`、`limit` 答 400（`TestRecordsCatalogueRefusesSelfReportedTenantAndLimit`）。换条件拿旧游标：`TestRecordsCataloguePassesTheDeclaredConditionsToTheReader`
  收窄 `verdict` 再翻答 400，`TestExternalTrackingFactsAnswerAPageCellAndPassTheDeclaredConditions` 换源、换视图再翻答 400，`TestEveryRecordsRegistryIsItsOwnCatalogue`
  拿一册的游标翻另一册答 400。答复带 `page`、末页 `next` 为 `null`（`TestEveryRecordsRegistryAnswersAPageCell`）。这一条与「端点表驱动」那一条共用一个命令：
  `go test -count=1 -v -run 'TestEveryMigratedCatalogue|TestEveryRecordsRegistry|TestRecordsCatalogue|TestExternalTrackingFacts(Answer|Refuse)' ./internal/transportfulfillment/adapters/http/`，
  `--- PASS` 71、`--- SKIP` 0。
- ✅ **准入先于一切查询参数的读取、校验与解码**（spec「端点先 Intake、后解码查询参数」）——两个端点的门次序都是方法 → 准入 → 选择器 → 声明解码，迁移前就有的
  选择器判定（`registry` 选册、挡回册 `booking` 与 `source` 的取值判据、事实页的 `source` 与 `view`）也在准入之后。`TestUnconfiguredIntakeRefusesEveryTransportFulfillmentRecordsRequest`
  在未配置的渠道上发缺册名、空册名、未知册名、挡回册立不起来的筛选，以及每一册的合规一问与集外键、坏排序、坏游标、自报 `tenant`、自报 `limit` 变体，全部答 403、
  读口不被触到；`TestTheChannelIsCheckedBeforeTheRegistry` 未知册名在未配置时答 403、配置了答 400；`TestABookingAnswerBounceFilterThatCannotStandIsRefused`
  （挡回册空、空白、带 NUL、超界的筛选）与 `TestFactQueryRefusesMissingSourceOrUnknownViewBeforeReadingAnything`（缺源、空白源、缺视图、集外视图、大小写混写的视图）
  都是配置了答 400、未配置答 403，读口都不被触到。命令：
  `go test -count=1 -v -run 'TestTheChannelIsCheckedBeforeTheRegistry|TestUnconfiguredIntakeRefusesEveryTransportFulfillmentRecordsRequest|TestABookingAnswerBounceFilterThatCannotStandIsRefused|TestFactQueryRefusesMissingSourceOrUnknownView' ./internal/transportfulfillment/adapters/http/`，
  `--- PASS` 100、`--- SKIP` 0。
- ✅ **迁移按字节无 CR、无 BOM**：`0058` 与本票逐个查过，无 `\r`，首三字节不是 `efbbbf`。
- ✅ spec「`limit` 非正仍拒」：各读面先拒 `limit < 1`，未改。
- ✅ spec「可排维与行标识的列必须非空」：各可排维所在列在建表语句里都是 `NOT NULL`；标识列是主键段；待确认复核清单派送一侧的来源身份是字面空串。
- ◑ spec「每迁一册补一条与缺省序同形的索引」：除待确认复核清单外，各册都有同形索引，新建或复用见上表索引一列，复用的在 `0058` 头注点名；
  待确认复核清单没有同形索引，本票也不补，理由见判断项「待确认复核清单的行标识与索引」。

自验（作者口径，实测于 `343aa9fbb`，真库包带 DSN、`-p 1`）：`go build ./...`、`go vet ./...` 全仓通过；改动包与反向依赖（`go list` 连测试导入算出）共 22 包全数通过，
`--- PASS` 4273、`--- SKIP` 0、`--- FAIL` 0——`transportfulfillment/adapters/postgres` 586、`adapters/http` 666、`application` 1326、`cmd/parcel-api` 654、
`cmd/parcel-dispatch` 506、`cmd/parcel-transport-register` 5、`visibilityexception/adapters/transportfulfillment` 213、`parcelshipment/adapters/transportfulfillment` 100，
其余各包同过、无 SKIP（`ports` 无测试文件）；`./internal/architecture/...` 174，`./migrations/` 1，`./internal/platform/migrate/` 8。

### 判断项

1. **选择器叫 `registry`**：票面写「按 `kind` 分派」，实际查询键与管理台用的都是 `registry`，照实登。
2. **决胜键方向变了的册**：spec「行标识取登记表主键去掉租户那几段」要求标识与排序同向。缺省序倒序、而迁移前标识升序的——班次、交接、有效交付、总单、运输准备、
   订舱、挡回——迁移后标识随排序倒序，只在首列完全相同时次序与迁移前不同。`TestReviewCatalogueListsHandoverVersionChain` 里两版裁决时刻相同，期望改为后一版在前。
   重问间隔迁移前已是双倒序；升序各册不变。
3. **待确认复核清单的行标识与索引**：行标识不是登记表主键去掉租户——揽收与派送两侧合成一册，揽收一侧的登记键是（来源身份，对象），派送一侧是（尝试，对象）、
   来源身份恒为空串，单取哪一侧的键另一侧都会撞，故取（一侧，尝试，对象，来源身份）。揽收一侧同一尝试同一对象可由不同来源报来，迁移前那几行之间没有定序，
   现在由来源身份定。其余各册的行标识都是登记表主键去掉租户那几段。索引也只部分同形：派送一侧的部分索引与本册在那一支的次序相同，揽收一侧的只有（租户，
   发生时刻）这段前缀同形（尝试取自联上的揽收尝试表）；两支并起来之后整册还要再排一遍，哪一条单表索引都给不出整册的序，本票不补。细节在 `0058` 头注。
4. **大册**（spec「`total` 一律给精确值」）：交接、有效交付、运输准备按包裹计量，外部轨迹事实按事件计量；`total` 照缺省给精确值，超出租户级规模时回 ADR-0144 改 `null`。
   订舱册不在其列：一次订舱一行，申请量记在 `quantity` 一格，订多少都只占一行，不按包裹计量；立票时「本票特有」把订舱算进大册，按这个计量不成立
   （`BookingRequestPages` 头注同写）。
5. **订舱册不放状态筛选维**：状态词由读口从当前那一份答复派生，库里没有列；按它筛只能先读后筛，`total` 就不是真数。这与 spec「可排维、筛选维从小放」里
   「有状态（或同类封闭词表）的册放出那一维作筛选维」有张力；要补得先把状态落成列或在 SQL 里导出，归 16 号票（`16-derived-state-filter-dimensions.md`）。
6. **管理台、脚本与 cmd 发的参数**：管理台发 `registry`、挡回册的 `booking` 与 `source`、事实页的 `source` 与 `view`；`scripts/demo-seeds` 只发 `registry`；
   `cmd/parcel-api` 的探针发 `registry` 与 `source`+`view`（`object` 只发给不迁的首次有效收寄）。`registry`、`source`、`view` 登为选择器；挡回册的 `booking`、
   `source` 登为无词表的筛选维，取值立不立得起来仍由端点照迁移前的判据判（空白、NUL、超长答 400），判据与声明引 `ports` 导出的同一对维名常量。
   这些判定现在都在准入之后。
7. **自报的 `tenant`、`limit` 现在答 400**：迁移前端点收下不用，现在是未知键（`TestRecordsCatalogueRefusesSelfReportedTenantAndLimit`）。管理台、脚本与 cmd 都不发。
   未配置的渠道上同答 403。
8. **门次序对调改了未配置渠道上的旧答法**：迁移前两个端点先判选择器再准入——缺或未知的 `registry`、挡回册立不起来的 `booking` 与 `source`、事实页缺 `source`
   或坏 `view`，在未配置的渠道上答 400；现在一律答 403（ADR-0055 决定三：未配置的答复不随请求变）。配置了的渠道上照旧答 400 `MALFORMED_REQUEST`。
   钉旧次序的两条用例改名：`TestAnUnknownRegistryIsRefusedBeforeTheChannelCheck` 改为 `TestTheChannelIsCheckedBeforeTheRegistry`，
   `TestABookingAnswerBounceFilterThatCannotStandIsRefusedBeforeTheChannel` 改为 `TestABookingAnswerBounceFilterThatCannotStandIsRefused`。
   `.scratch/at-coverage-inventory.md` 的 `AT-TF-333` 一行仍写后者的旧名，不在本票地盘，由推送方进 main 时改；`demo-chain-gaps/167` 里的旧名是已收口票的历史，不改。
   node-operations 与 settlement-accounting 各有一条同名的 `TestAnUnknownRegistryIsRefusedBeforeTheChannelCheck`，06、08 照 spec 对调门次序时会碰到同一处。
9. **空值的旧答法保住**：`?registry=`、挡回册的 `?booking=` 与 `?source=`、事实页的 `?source=` 与 `?view=` 迁移前都答 400，配置了的渠道上照旧答 400；
   没有哪个参数的空值从前落到列表支、迁移后变成集外键。
10. **`q` 不落在 JSON 列上**：各册 `q` 所查的列都是标量文本列，没有 JSON 数组，不涉及 `::text`。
11. **没有自带 Next/Total 的计数队列**：本票各册迁移前都没有计数与 `truncated`，spec「已带计数与 `truncated` 的待办队列保留这两格」不适用；各册读端口都交回统一结果类型。
12. **后台路径未动**：`DeliveryDispatchPendingQueue` 的 `ListPending`、`CountPending` 与 `DeliveryDispatchAskLedger.ListAsks` 的签名与 SQL 不变；派送待形成册的
    HTTP 读面另起取页函数，与后台只共用「待形成」谓词常量 `deliveryDispatchPendingOpen`。
13. **当前版改反连接**：班次与周期安排原用 `DISTINCT ON` 取每个的最高版，它要求外层先按分组列排，与按册排序取页相冲。
14. **端点用例补册名**：端点用例的 registry 封闭集补上了迁移前漏列的 `actual-fulfillment-unformed`。
15. **夹具**：修订轮新写的真库夹具（交接、有效交付、外部轨迹事实、待确认复核）用本文件自带的 `SYN-` 构造器，标识与 `q` 所查各列全为 `SYN-`；借用的既有构造器
    只在内容摘要、计量单位这类不进标识也不进 `q` 的格上留着自己的写法。端点用例新加的替身值同为 `SYN-`；旧用例的夹具不回改。
16. **`0058` 修订轮与评审收尾都只改头注**：SQL 不变。迁移校验和按文件内容在运行时算、只存在应用过它的库里；`0058` 还没进 main，只在本分支的测试库里应用过。
17. **提交没有逐册切**：`registry` 分派的各册共用一个读端口接口 `ReviewCatalogueRead` 与一个分派端点，逐册切会让中间提交里端点半迁（有的册拒未知键、有的不拒），
    故按「代码与端点用例 → 真库用例 → 索引迁移」分笔提交；修订轮与评审收尾的改动各成一笔。

### 没验证的

- 管理台没跑（归 14）：页面还不读 `page`、只显示第一页；页大小与迁移前的上限同取自 Intake，可见行数应不变，但没在浏览器里看。
- 索引只按形状对齐缺省序，没在有量的数据上看 `EXPLAIN`。
- `scripts/demo-seeds/demo-run/chain.sh` 没跑：它取运输准备册第一页找本件的几段，缺省序新近在前，按理落在第一页。
- 迁移编号 `0058` 进 main 时按 tip 重核。

## 评审记录

**非作者评审 · 钉 `1d76b5990`**（两轴无阻断）

| 评审意见 | 处理 |
|---|---|
| 非阻断：`bookingAnswerBounceFilterProblem` 用字面量 `booking`、`source` 取筛选值，没有对上声明 `BookingAnswerBouncePages`；声明里这一维改了名，判据就静默落空，用例只断言 400，测不出来 | `0f9a88d93`：`ports` 导出 `BookingAnswerBounceBookingFilter`、`BookingAnswerBounceSourceFilter`，声明的 `Filters`、端点判据与 postgres 落列都引这两个常量；`TestABookingAnswerBounceFilterThatCannotStandIsRefused` 改为断言 `detail` 含判据自己的理由（空串归声明解码拒，不钉共用件的措辞），为此 `assertPageQueryRefused` 交回 `detail`。变异核对：只把声明里订舱一维改成别的字面量、判据照引常量，「空白订舱」「订舱带 NUL」「订舱超界」变红，「空订舱」与答复方各格照过 |
| 非阻断：`0058` 头注把 `pickup_attempt_result_pending` 列在「已有索引恰与缺省序同形」之下，实际只有（租户，发生时刻）前缀同形 | `0f9a88d93`：待确认复核清单移出那一列，如实写两侧部分索引各自同形到哪一段、整册为何没有同形索引，SQL 不动；本票逐册表的索引一格与判据同步改，判据记 ◑，理由归判断项「待确认复核清单的行标识与索引」 |
| 非阻断：订舱册不算大册缺一句理由——「本票特有」点了订舱，判断项「大册」与 `BookingRequestPages` 头注都没点名 | `0f9a88d93` 的 `BookingRequestPages` 头注与本票判断项「大册」补：一次订舱一行、申请量记在 `quantity`，不按包裹计量 |
| 已知项：订舱册的状态筛选维 | 归 16 号票，见判断项「订舱册不放状态筛选维」 |
| 已知项：`.scratch/at-coverage-inventory.md` 的 `AT-TF-333` 仍写旧用例名 | 不在本票地盘，由推送方进 main 时改，见判断项「门次序对调改了未配置渠道上的旧答法」 |

处理后自验（实测于 `0f9a88d93`）：改过的 `.go` 已 `gofmt -w`；`go build ./...`、`go vet ./internal/transportfulfillment/... ./cmd/...` 通过；带 DSN、`-p 1`：
`transportfulfillment/adapters/http` `--- PASS` 666、`adapters/postgres` 586、`cmd/parcel-api` 654，`--- SKIP` 与 `--- FAIL` 均为 0；`transportfulfillment/ports` 无测试文件。
`0058` 与本票无 CR、无 BOM。
