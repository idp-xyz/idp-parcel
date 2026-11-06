# 02 parcel-shipment 九册迁到 ADR-0144

Category: enhancement
Status: in-progress——修订轮：非作者评审（钉 `3a20b2bcc`）拦下的两条阻断已修，待复核（2026-11-06 通道 1，分支 `cursor/catalogue-pagination-rollout`，基 main `a1274a6b8`；完成记录与评审记录见文末）。迁移编号 parcel-shipment `0036`（进 main 时按 tip 重核）
Blocked by: 01
地盘：`internal/parcelshipment/{ports,adapters/http,adapters/postgres}` 里下表各册的读端口、端点与读面及其测试；`cmd/parcel-api` 里这些读口的占位；
`migrations/parcel_shipment` 的一条新迁移（索引）。
出处：[ADR-0144](../../../docs/adr/0144-catalogue-reads-share-one-cursor-pagination-sort-and-filter-contract.md) 决定三至七；spec「横贯各子票的取舍」。

## 本票的册（今天的次序钉 `a1274a6b8`）

| 路由 | 读端口方法 | 今天的 `ORDER BY` | 形态 |
|---|---|---|---|
| `/shipment-request-views` | `ShipmentRequestViews.ListVisible` | `submitted_at DESC, shipment_request_id DESC` | 委托查阅（按可见客户账户切） |
| `/label-transactions` | `LabelTransactionViews.ListLabelTransactions` | `established_at DESC, label_transaction_id DESC` | 运营查阅 |
| `/channel-selection-decisions` | `ChannelSelectionDecisionRead.ListTiedChannelSelectionDecisions` | `decided_at DESC, decision_id DESC` | 运营查阅，另有按对象的单条分支 |
| `/acceptance-review-queue` | `AcceptanceReviewQueue.ListAwaitingManualReview` | `submitted_at ASC, shipment_request_id ASC` | 待办队列 |
| `/authorized-disposition-queue` | `AuthorizedDispositionQueue.ListAwaitingAuthorizedDisposition` | `submitted_at ASC, shipment_request_id ASC` | 待办队列 |
| `/control-release-refusals` | `ControlReleaseRefusalCatalogue.ListUnhandledControlReleaseRefusals` | `recorded_at, shipment_request_id, submission_version, control_result_id` | 待办队列，带计数与 `truncated` |
| `/cancellation-order-conflicts` | `CancellationOrderConflictCatalogue.ListUnresolvedCancellationOrderConflicts` | `detected_at, parcel_id, boundary_ref, cancellation_id` | 待办队列，带计数与 `truncated` |
| `/post-intake-disposition-requests` | `PostIntakeDispositionRequests.ListUnansweredPostIntakeDispositionRequests` | `decided_at, request_key, parcel_id` | 待办队列，带计数与 `truncated` |
| `/post-intake-disposition-manual-items` | `PostIntakeManualItems.ListPendingPostIntakeManualItems` | `created_at, parcel_id, decision_version, owner, action` | 待办队列，带计数与 `truncated` |

不在内：`CustomerSupplementQueue`、`OperatorRegistrationQueue` 只给后台消费者用、不经 HTTP（spec「范围」）。

## 做法与判据

见 spec「每张迁移子票（02–13）的做法与判据」。本票特有：

- 委托查阅按可见客户账户切，作用域照旧在签名上，键集与筛选落在作用域之内。
- `/channel-selection-decisions` 的按对象单条分支不动；列表分支迁。
- 四个带计数的队列照 spec「已带计数与 `truncated` 的待办队列保留这两格」：计数仍是整个队列的积压数，`truncated` 即 `page.next` 不为 `null`。

## 完成记录（2026-11-06，通道 1，分支 `cursor/catalogue-pagination-rollout`，基 main `a1274a6b8`）

**落点**

| 笔 | 做了什么 |
|---|---|
| `d48c5b130` | 参照实现：面单交易一册（声明、读端口、Binding 读面、端点、占位、用例），迁移 `0036` 九册索引一次写齐 |
| `31fcbf4f2` | 其余八册；九个端点改为先 Intake 后解码；`cmd/parcel-api`、`cmd/parcel-dispatch` 的占位、替身与调用点；新用例 |
| `d6b3f64a5` | 九个端点的表驱动端点用例 |
| `c3c226b00` | 评审修订：七册真库用例重写、人工项翻页用例独立成文；渠道择优两条分支改为先 Intake；面单交易的 `q` 改落在逐个包裹号上；空 `?parcel=` 照旧落到清单；注释里引 spec 的序数改成原句；夹具一律 `SYN-` |
| 补 `q`（本笔） | 面单交易的 `q` 补上渠道签发的包裹级标识（管理台叫渠道面单号）：14a 接面单交易页时发现页上按它检索，而本册 `q` 漏了这一列，前端只能撤掉这项检索 |

依赖 01 的两笔：`7b1166838`（落列、取页片段、`Finish`、统一结果类型）、`9726b2f35`（`Words` / `Code`）。

**各册声明**（声明在 `ports/catalogue_pages.go`，落列在各读面头部）

| 册 | 缺省序 | 可排维 | 筛选维 | q 覆盖 | 索引（`0036`） |
|---|---|---|---|---|---|
| 面单交易 | `-establishedAt` | establishedAt、transactionId | transactionResult（代码列） | 交易号、快照渠道账户、覆盖包裹、渠道面单号（两个数组都只拼元素，不含 JSON 标点） | 换 `label_transaction_by_tenant` 补标识段 |
| 委托查阅 | `-submittedAt` | submittedAt、shipmentRequestId | state（代码列）、customerAccountId | 委托号、单据号、来源请求键 | 新 `shipment_request_by_tenant_submitted` |
| 复核队列 / 授权处置队列 | `submittedAt` | 同上 | customerAccountId | 同上 | 两条部分索引补标识段 |
| 渠道择优并列冲突 | `-decidedAt` | decidedAt、decisionId | —（view / scope / mapping 为选择器） | 决定号、商业范围、映射 | 新 `channel_selection_decision_by_conclusion` |
| 释放未受理册 | `recordedAt` | recordedAt | customerAccountId | 委托号、账户、控制结果、单据号（子查询） | 换 `control_release_refusal_tenant_recorded` 补标识段 |
| 取消先后冲突册 | `detectedAt` | detectedAt | boundaryKind | 包裹、边界那一代、取消 | 新 `cancellation_order_conflict_by_tenant_detected` |
| 等回应的处置请求 | `recordedAt` | recordedAt | — | 请求键、包裹、取消 | 换 `parcel_cancellation_pending_by_tenant` 补标识段 |
| 待人工处理项 | `decidedAt`（落在 `i.created_at`） | decidedAt | owner、action | 包裹 | 换 `post_intake_disposition_manual_item_by_tenant` 补标识段 |

**完成判据**（钉「补 `q`」那一笔；复核范围是 `3a20b2bcc` 之后各笔）

- ✅ 真库用例（`SYN-` 夹具，带 DSN）：
  - 面单交易（`label_transaction_views_page_test.go`）：翻页期间新建立一笔，不重出、不漏行，total 由 5 到 6；transactionResult 维内为或、q 命中覆盖包裹与渠道账户、`%` 按字面、查引号与逗号零命中、覆盖两件的交易按一笔计、空册；q 命中渠道面单号，还没有渠道答复（`parcelResults` 整键缺席）的交易照常参与检索不报错。
  - 委托查阅、复核队列、授权处置队列、渠道择优并列冲突、释放未受理册、取消先后冲突册、等回应的处置请求七册（`catalogue_pages_test.go`）每册都过：按小页走完全部页，拼起来恰是全集、次序即缺省序；走完第一页后插入一行——降序册的新行落在游标之前，后面各页不变、total 加一，升序队列的新行出现在末页；一条筛选或 q 改变 total；空租户 total 为 0；带计数的队列积压数不随筛选与 q 变。渠道择优另证选择器进游标摘要（不收窄时的游标换上收窄选择器解不开）。
  - 待人工处理项（`cmd/parcel-dispatch/post_intake_manual_items_page_test.go`；人工项只能由拦截决定落下，夹具要走整条收寄后处置链）：按小页最早的在前；翻页中途再形成一份决定，新项出现在末页，total 与积压数一起由 4 到 6；按所有者收窄改 total、不改积压数；空租户 total 为 0。
- ✅ 端点用例：
  - 九个端点在 `catalogue_pages_endpoints_test.go` 的表驱动用例里各过一遍：合规的一问带 page、size 取 Intake 页大小；集外参数答 400 带理由散文；渠道未配置时合规与不合规的一问都答 403。
  - 面单交易另证集外键、集外排序维、词表外筛选值、坏游标、换条件拿旧游标各答 400 带 detail，读口不被调用。四种坏参数怎么分辨在共用件 `Decode` 里，由 `cataloguepage` 的用例逐种证；其余八个端点证的是经本册声明解码、错答 400。
  - 渠道择优：渠道未配置时，缺 `view`、`view=all`、只给 `scope`、集外参数与空 `decisionId` 都答 403。
  - 按运单号、按包裹两条有界查找不解码、不带 page；`?parcel=` 带空值照旧落到清单。`unconfigured_intake_test` 的查询串变体照旧答 403。
- ✅ 迁移无 CR、无 BOM（`migrations` 行尾守卫过）。
- 门（作者自验，WSL go1.26.8）：`gofmt -l` 空；全仓 `go build`、`go vet` 退 0；带 DSN `-p 1 -count=1`：`internal/parcelshipment/adapters/postgres` 386 PASS / 0 SKIP，`internal/parcelshipment/adapters/http` 175 PASS，`cmd/parcel-api` 654 PASS，`cmd/parcel-dispatch` 507 PASS，`internal/architecture/...` 174 PASS，`migrations`、`internal/platform/migrate` 过；`internal/parcelshipment/...` 其余包与 `internal/platform/cataloguepage`（130 PASS）全过。计数取证：`grep -c -- '--- PASS'` 与 `grep -c -- '--- SKIP'` 打在各包 `-v` 输出上。

**判断项**

1. **先 Intake 后解码**（spec「端点先 Intake、后解码查询参数」）：起初照首批两册写成先解码，被 `TestUnconfiguredIntakeAnswersEveryRequestIdentically` 的查询串变体拦下（ADR-0055 决定三）。改成先 Intake 之后，未配置渠道的答复不随请求内容变，没有凭据的调用方也探不出查询面；首批两册的次序归 15 改齐。
2. **渠道择优的单条分支也对调了**（偏离本票「按对象单条分支不动」）：列表分支的选择器解析、单条分支的标识解析，迁移前都排在 Intake 之前，未配置时坏选择器或空标识答 400。评审拦下列表分支后两处一起对调——端点头注写着「未配置 Intake 对两条分支同答 403」，只调一处，单条分支仍与头注和 ADR-0055 决定三不符。单条分支的查找与答复形状没动。
3. **决胜键方向**：渠道择优迁移前是 `decided_at DESC, decision_id DESC`，同向，无变化；取消先后冲突册迁移前漏了边界种类一段，补上后次序在同刻同包裹时才稳定。
4. **大册**：面单交易、委托查阅按交易与委托计量，是 spec「`total` 一律给精确值」点名的那类册，total 照给精确值；面单交易的 page 与 total 数的是交易，不是摊开的「交易 × 包裹」行（声明头注）。
5. **管理台带的参数**：`shipmentRequestId`、`documentNumber`、`shipmentRequestIds`、`decisionId`、`parcel` 都走有界或单条分支，在解码之前分派走；`view` / `scope` / `mapping` 登为选择器。答复只多一格 `page`，管理台不改也照旧能用。`?parcel=` 带空值迁移前落到清单，端点先删掉这个空键再解码，旧调用照旧拿到清单。
6. **自报参数从「被忽略」变「答 400」**：`cmd/parcel-api` 的放行拒绝装配用例里，查询串自报租户与页大小那一格原断言被忽略照答，改为答 400 且不带任何行（ADR-0144 决定四、越权风险点 2）。
7. **委托三册的行标识取 `shipment_request_id`**（偏离 spec「行标识取登记表主键去掉租户那几段」）：`shipment_request` 的主键是完整来源身份（租户、客户账户、来源、来源请求键），去掉租户还剩三段；`shipment_request_id` 由 `shipment_request_id_unique`（租户 + 委托标识）保证租户内唯一，一段就能定行，又正是今天 `ORDER BY` 的决胜列（spec「缺省序保今天的次序」），游标里的标识也与答复的 `shipmentRequestId` 一致。换成三段只会让同刻的次序变样、游标变长，不多防住什么。
8. **四个带计数的端口各自带 `Next` / `Total`**（偏离 spec「读端口改为收查询对象、答 01 的统一结果类型」）：这四个结构的行字段（`Refusals`、`Conflicts`、`Requests`、`Items`）与 `Count` 被端点、两个 `cmd` 的装配与用例按名引用，释放未受理册还多带一张 `DocumentNumbers`。换成 `cataloguepage.Result` 要么丢掉这些格、要么再包一层，行为不变。两格与 `Result` 同名同义，由同一个 `Finish` 填。

**未验**

- 管理台未接（归 14）；全量 `scripts/test-full.sh` 由推送方在重放后的 tip 上跑。

## 评审记录

**评审 ← 子代理 · 钉 `3a20b2bcc`**（01 与 02 一并评）

| 评审意见 | 处理 |
|---|---|
| 阻断：真库判据未满足——除面单交易外八册没在翻页中途插入新行；空册 total 为 0 只三册断言；委托查阅的筛选子项丢了 total；授权处置队列没有筛选或 q 用例；完成记录的 ✅ 与实际不符 | 七册在 `catalogue_pages_test.go` 重写，每册都插入新行、断言空租户 total 与筛选 total；人工项另写 `post_intake_manual_items_page_test.go`；完成判据按实际改写 |
| 阻断：`serveTiedChannelSelectionDecisions` 先解析选择器再 Intake，未配置时 `?view=all` 答 400 | 对调；单条分支一并对调（判断项 2）；未配置用例补上坏选择器、集外参数与空标识 |
| 非阻断：注释引「spec 取舍 1/5」，序数会漂 | 改成引原句 |
| 非阻断：`shipmentRequestQueueColumns` 注里数另一个文件的列数 | 改成说落在同样的列上，不数 |
| 非阻断：新夹具值不带 `SYN-` | 改为 `SYN-` |
| 非阻断：面单交易 q 落在 `coveredParcels::text` 上，会命中 JSON 标点 | 改落在逐个包裹号以 `chr(31)` 连成的串上；真库用例证查 `"`、`,` 零命中，按第二件包裹号查到那笔交易且只算一笔 |
| 非阻断：端点用例只覆盖面单交易，没测换条件拿旧游标 | `d6b3f64a5` 已补九个端点；本笔补旧游标用例 |
| 非阻断：委托三册用 `shipment_request_id` 作行标识，未记判断 | 判断项 7 |
| 非阻断：四个计数端口自己带 `Next` / `Total`，没用 `Result` | 判断项 8 |
| 非阻断：`?parcel=` 传空值由列清单变成答 400 | 端点先删空键再解码；端点用例钉住 |
