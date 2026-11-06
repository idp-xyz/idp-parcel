# 02 parcel-shipment 九册迁到 ADR-0144

Category: enhancement
Status: in-progress——作者完成，待非作者评审（2026-11-06 通道 1，分支 `cursor/catalogue-pagination-rollout`，基 main `a1274a6b8`；完成记录见文末）。迁移编号 parcel-shipment `0036`（进 main 时按 tip 重核）
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
- 四个带计数的队列照取舍 5：计数仍是整个队列的积压数，`truncated` 即 `page.next` 不为 `null`。

## 完成记录（2026-11-06，通道 1，分支 `cursor/catalogue-pagination-rollout`，基 main `a1274a6b8`）

**落点**

| 笔 | 做了什么 |
|---|---|
| `d48c5b130` | 参照实现：面单交易一册（声明、读端口、Binding 读面、端点、占位、用例），迁移 `0036` 九册索引一次写齐 |
| `31fcbf4f2` | 其余八册；九个端点改为先 Intake 后解码（spec 取舍 8）；`cmd/parcel-api`、`cmd/parcel-dispatch` 的占位、替身与调用点；新用例 |

依赖 01 的两笔：`7b1166838`（落列、取页片段、`Finish`、统一结果类型）、`9726b2f35`（`Words` / `Code`）。

**各册声明**（声明在 `ports/catalogue_pages.go`，落列在各读面头部）

| 册 | 缺省序 | 可排维 | 筛选维 | q 覆盖 | 索引（`0036`） |
|---|---|---|---|---|---|
| 面单交易 | `-establishedAt` | establishedAt、transactionId | transactionResult（代码列） | 交易号、快照渠道账户、覆盖包裹 | 换 `label_transaction_by_tenant` 补标识段 |
| 委托查阅 | `-submittedAt` | submittedAt、shipmentRequestId | state（代码列）、customerAccountId | 委托号、单据号、来源请求键 | 新 `shipment_request_by_tenant_submitted` |
| 复核队列 / 授权处置队列 | `submittedAt` | 同上 | customerAccountId | 同上 | 两条部分索引补标识段 |
| 渠道择优并列冲突 | `-decidedAt` | decidedAt、decisionId | —（view / scope / mapping 为选择器） | 决定号、商业范围、映射 | 新 `channel_selection_decision_by_conclusion` |
| 释放未受理册 | `recordedAt` | recordedAt | customerAccountId | 委托号、账户、控制结果、单据号（子查询） | 换 `control_release_refusal_tenant_recorded` 补标识段 |
| 取消先后冲突册 | `detectedAt` | detectedAt | boundaryKind | 包裹、边界那一代、取消 | 新 `cancellation_order_conflict_by_tenant_detected` |
| 等回应的处置请求 | `recordedAt` | recordedAt | — | 请求键、包裹、取消 | 换 `parcel_cancellation_pending_by_tenant` 补标识段 |
| 待人工处理项 | `decidedAt`（落在 `i.created_at`） | decidedAt | owner、action | 包裹 | 换 `post_intake_disposition_manual_item_by_tenant` 补标识段 |

**完成判据**

- ✅ 真库用例（`SYN-` / 既有夹具数据，带 DSN，钉 `31fcbf4f2`）：面单交易「翻页期间新建立一笔不重出不漏行、total 5 → 6」与「transactionResult 维内为或、q 命中覆盖包裹与渠道账户、`%` 按字面、空册」两条；其余七册在 `catalogue_pages_test.go` 各按小页走完全部页（拼起来恰是全集、次序即缺省序）、筛选或 q 改变 total、队列积压数不变、空册；渠道择优另证选择器进游标摘要（不收窄时的游标换上收窄选择器解不开）；人工项在 `cmd/parcel-dispatch` 的真库用例里补翻页与按所有者收窄。「翻页期间插入新行」只在面单交易一册上做：键集机制是共用件的，各册证的是落列，按小页走完全集已能抓出落错的列。
- ✅ 端点用例：面单交易的集外键、集外排序维、词表外筛选值、坏游标各答 400 带 detail，读口不被调用；九个端点各自在 `catalogue_pages_endpoints_test.go` 的表驱动用例里过一遍「合规的一问带 page、size 取 Intake 页大小；集外参数答 400 带理由散文；渠道未配置时同一个坏请求答 403」，另证按运单号、按包裹两条有界查找不解码、不带 page。`unconfigured_intake_test` 的查询串变体照旧答 403。
- ✅ 迁移无 CR、无 BOM（`migrations` 行尾守卫过）。
- 门（作者自验，钉 `31fcbf4f2`，WSL go1.26.8）：`gofmt -l` 空；全仓 `go build`、`go vet` 退 0；带 DSN `-p 1 -count=1`：`internal/parcelshipment/adapters/postgres` 382 PASS / 0 SKIP，`cmd/parcel-api` 654 PASS，`cmd/parcel-dispatch` 506 PASS，`internal/architecture/...` 174 PASS，`migrations`、`internal/platform/migrate` 过；`internal/parcelshipment/...` 其余包与 `internal/platform/cataloguepage` 无 DSN 全过。计数取证：`grep -c -- '--- PASS'` 与 `grep -c -- '--- SKIP'` 打在各包 `-v` 输出上。

**判断项**

1. **先 Intake 后解码**（spec 取舍 8）：起初照首批两册写成先解码，被 `TestUnconfiguredIntakeAnswersEveryRequestIdentically` 的查询串变体拦下（ADR-0055 决定三）。改成先 Intake 之后，未配置渠道的答复不随请求内容变，没有凭据的调用方也探不出查询面；首批两册的次序归 15 改齐。
2. **决胜键方向**：渠道择优迁移前是 `decided_at DESC, decision_id DESC`，同向，无变化；取消先后冲突册迁移前漏了边界种类一段，补上后次序在同刻同包裹时才稳定。
3. **大册**：面单交易、委托查阅按交易与委托计量，是 spec 取舍 4 点名的大册，total 照给精确值；面单交易的 page 与 total 数的是交易，不是摊开的「交易 × 包裹」行（声明头注）。
4. **管理台带的参数**：`shipmentRequestId`、`documentNumber`、`shipmentRequestIds`、`decisionId`、`parcel` 都走有界或单条分支，在解码之前分派走；`view` / `scope` / `mapping` 登为选择器。答复只多一格 `page`，管理台不改也照旧能用。
5. **自报参数从「被忽略」变「答 400」**：`cmd/parcel-api` 的放行拒绝装配用例里，查询串自报租户与页大小那一格原断言被忽略照答，改为答 400 且不带任何行（ADR-0144 决定四、越权风险点 2）。

**未验**

- 管理台未接（归 14）；全量 `scripts/test-full.sh` 由推送方在重放后的 tip 上跑。
