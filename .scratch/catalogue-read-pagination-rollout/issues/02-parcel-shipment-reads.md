# 02 parcel-shipment 九册迁到 ADR-0144

Category: enhancement
Status: ready-for-agent——迁移编号预留 parcel-shipment `0036`（进 main 时按 tip 重核）
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
