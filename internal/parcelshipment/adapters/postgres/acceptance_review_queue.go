package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
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

// ListAwaitingManualReview 按作用域取回当前停在「等待人工复核」的委托，老的在前
// （先来先审），同刻按委托标识正序保证分页可重复。limit 非正是调用方编程错误，
// 判据同 ListVisible。
func (views *ShipmentRequestViews) ListAwaitingManualReview(
	ctx context.Context,
	scope domain.AuthorizedQueryScope,
	limit int,
) ([]ports.AcceptanceReviewQueueRecord, error) {
	if limit < 1 {
		return nil, fmt.Errorf("list acceptance review queue: limit must be positive, got %d", limit)
	}
	querier, err := views.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list acceptance review queue: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT customer_account_id, source, source_request_key,
		        shipment_request_id, state, submitted_at,
		        current_submission_version_id, cardinality(declared_parcel_ids),
		        snapshot->'acceptanceTask'
		   FROM parcel_shipment.shipment_request
		  WHERE tenant_id = $1
		    AND customer_account_id = ANY($2)
		    AND task_waiting_on = $3
		    AND state = $4
		  ORDER BY submitted_at ASC, shipment_request_id ASC
		  LIMIT $5`,
		scope.TenantID().String(),
		visibleAccounts(scope),
		uint8(domain.ResumeByManualReview),
		uint8(domain.ShipmentRequestSubmitted),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list acceptance review queue: %w", err)
	}
	defer rows.Close()

	var records []ports.AcceptanceReviewQueueRecord
	for rows.Next() {
		var row summaryRow
		var rawTask []byte
		if err := rows.Scan(
			&row.customerAccountID, &row.source, &row.sourceRequestKey,
			&row.shipmentRequestID, &row.state, &row.submittedAt,
			&row.submissionVersionID, &row.declaredParcelCount,
			&rawTask,
		); err != nil {
			return nil, fmt.Errorf("list acceptance review queue: %w", err)
		}
		summary, err := row.record()
		if err != nil {
			return nil, fmt.Errorf("list acceptance review queue: %w", err)
		}
		var task taskDocument
		if err := json.Unmarshal(rawTask, &task); err != nil {
			return nil, fmt.Errorf("list acceptance review queue: 快照不是本适配器写下的形状：%w", err)
		}
		taskView, err := taskViewRecord(task)
		if err != nil {
			return nil, fmt.Errorf("list acceptance review queue: %w", err)
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
		return nil, fmt.Errorf("list acceptance review queue: %w", err)
	}
	return records, nil
}
