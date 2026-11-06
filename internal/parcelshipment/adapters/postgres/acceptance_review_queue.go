package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件在 ShipmentRequestViews 上补齐 ports.AcceptanceReviewQueue（票
// admin-skeleton-closure-batch/09，读面形状承接 acceptance-review-read-face/01）：
// 复核队列与委托查阅读的是同一张表、同一套作用域纪律，详情方法本就同一个——另立
// 适配器只会复制 visibleAccounts 与文档解析这一整套。
//
// 队列的过滤在投影列上（迁移 0009）：`task_waiting_on` 由 Insert/Save 与快照同一条
// SQL 写下，谓词 `task_waiting_on = $3 AND state = $4` 与部分索引逐字吻合。数字由
// Go 侧常量传参，SQL 只比较不拥有编号——与 state 列「写入侧 uint8(request.State())、
// 读回侧 domain.ShipmentRequestState(state)」同一条纪律，编号的唯一来源始终是领域包。
// state 上榜不是冗余：等待态只在`已提交`委托上有队列语义，撤回停止的任务留着最后一次
// waitingOn 快照也不该上列。
var _ ports.AcceptanceReviewQueue = (*ShipmentRequestViews)(nil)

// waitingRequests 是委托队列的固定条件：作用域那一段再加等待态与`已提交`。
const waitingRequests = visibleRequests + " AND task_waiting_on = $3 AND state = $4"

// shipmentRequestQueueColumns 把两个委托队列的声明落到列上：q 与委托查阅落在同样的列上。
var shipmentRequestQueueColumns = cataloguepage.Columns{
	Sorts:    map[string]string{"submittedAt": "submitted_at", "shipmentRequestId": "shipment_request_id"},
	Identity: []string{"shipment_request_id"},
	Filters:  map[string]string{"customerAccountId": "customer_account_id"},
	Keyword:  []string{"shipment_request_id", "document_number", "source_request_key"},
}

var acceptanceReviewQueuePages = ports.AcceptanceReviewQueuePages.MustBind(shipmentRequestQueueColumns)

// ListAwaitingManualReview 按作用域取当前停在「等待人工复核」的委托的一页：缺省老的在前（先来先审），同刻按委托标识正序。
// limit 非正是调用方编程错误，判据同 ListVisible。
func (views *ShipmentRequestViews) ListAwaitingManualReview(
	ctx context.Context,
	scope domain.AuthorizedQueryScope,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.AcceptanceReviewQueueRecord], error) {
	fail := func(err error) (cataloguepage.Result[ports.AcceptanceReviewQueueRecord], error) {
		return cataloguepage.Result[ports.AcceptanceReviewQueueRecord]{}, fmt.Errorf("list acceptance review queue: %w", err)
	}
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := acceptanceReviewQueuePages.Plan(query, waitingRequests, []any{
		scope.TenantID().String(), visibleAccounts(scope),
		uint8(domain.ResumeByManualReview), uint8(domain.ShipmentRequestSubmitted),
	}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := views.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, plan.CountSQL(shipmentRequestTable), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	rows, err := querier.Query(ctx, plan.PageSQL(summaryColumns+", snapshot->'acceptanceTask'", shipmentRequestTable), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	var records []ports.AcceptanceReviewQueueRecord
	for rows.Next() {
		var row summaryRow
		var rawTask []byte
		if err := rows.Scan(append(row.targets(), &rawTask)...); err != nil {
			return fail(err)
		}
		summary, err := row.record()
		if err != nil {
			return fail(err)
		}
		var task taskDocument
		if err := json.Unmarshal(rawTask, &task); err != nil {
			return fail(fmt.Errorf("快照不是本适配器写下的形状：%w", err))
		}
		taskView, err := taskViewRecord(task)
		if err != nil {
			return fail(err)
		}
		records = append(records, ports.AcceptanceReviewQueueRecord{
			ShipmentRequestSummaryRecord: summary,
			HasAttempt:                   taskView.HasAttempt,
			LastAttemptReason:            taskView.LastAttemptReason,
			LastAttemptContinuation:      taskView.LastAttemptContinuation,
			LastAttemptedAt:              taskView.LastAttemptedAt,
			ReviewCompleted:              taskView.ReviewCompleted,
			ReviewAuthority:              taskView.ReviewAuthority,
			ReviewReviewer:               taskView.ReviewReviewer,
			ReviewEvidence:               taskView.ReviewEvidence,
			ReviewCompletedAt:            taskView.ReviewCompletedAt,
		})
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	page, err := cataloguepage.Finish(query, records, limit, total,
		func(record ports.AcceptanceReviewQueueRecord, sortField string) cataloguepage.Position {
			return summaryPosition(record.ShipmentRequestSummaryRecord, sortField)
		})
	if err != nil {
		return fail(err)
	}
	return page, nil
}
