package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件在 ShipmentRequestViews 上补齐 ports.CustomerSupplementQueue（ADR-0106 Consequences，票
// first-tenant-runway/09）。放在查阅适配器而不是仓储上：它是「等客户补件的都有谁」的读面，与
// 复核队列读的是同一张表、同一套作用域纪律，详情方法本就同一个——理由与 acceptance_review_queue
// 逐字相同。
//
// 过滤在投影列上（迁移 0009 立列、0016 建部分索引）：`task_waiting_on` 由 Insert/Save 与快照同一条
// SQL 写下，谓词 `task_waiting_on = $3 AND state = $4` 与部分索引逐字吻合。数字由 Go 侧常量传参，
// SQL 只比较不拥有编号——编号的唯一来源始终是领域包。state 上榜的理由同复核队列：等待态只在
// `已提交`委托上有队列语义，撤回停止的任务留着最后一次 waitingOn 快照也不该上列。
//
// 这一格此前在库里结构上恒空（票 09 第三问）：`ResumeByCustomerSupplement` 曾整笔回滚，等待态随
// 本轮蒸发。ADR-0106 Decision 二把它先 Save 再交回之后，这条谓词才第一次筛得出行。
var _ ports.CustomerSupplementQueue = (*ShipmentRequestViews)(nil)

// ListWaitingOnCustomerSupplement 按作用域取回当前停在`等待受控补充`的委托，老的在前（先停的
// 先催），同刻按委托标识正序保证分页可重复。limit 非正是调用方编程错误，判据同 ListVisible。
func (views *ShipmentRequestViews) ListWaitingOnCustomerSupplement(
	ctx context.Context,
	scope domain.AuthorizedQueryScope,
	limit int,
) ([]ports.CustomerSupplementQueueRecord, error) {
	if limit < 1 {
		return nil, fmt.Errorf("list customer supplement queue: limit must be positive, got %d", limit)
	}
	querier, err := views.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list customer supplement queue: %w", err)
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
		uint8(domain.ResumeByCustomerSupplement),
		uint8(domain.ShipmentRequestSubmitted),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list customer supplement queue: %w", err)
	}
	defer rows.Close()

	var records []ports.CustomerSupplementQueueRecord
	for rows.Next() {
		var row summaryRow
		var rawTask []byte
		if err := rows.Scan(
			&row.customerAccountID, &row.source, &row.sourceRequestKey,
			&row.shipmentRequestID, &row.state, &row.submittedAt,
			&row.submissionVersionID, &row.declaredParcelCount,
			&rawTask,
		); err != nil {
			return nil, fmt.Errorf("list customer supplement queue: %w", err)
		}
		summary, err := row.record()
		if err != nil {
			return nil, fmt.Errorf("list customer supplement queue: %w", err)
		}
		var task taskDocument
		if err := json.Unmarshal(rawTask, &task); err != nil {
			return nil, fmt.Errorf("list customer supplement queue: 快照不是本适配器写下的形状：%w", err)
		}
		taskView, err := taskViewRecord(task)
		if err != nil {
			return nil, fmt.Errorf("list customer supplement queue: %w", err)
		}
		records = append(records, ports.CustomerSupplementQueueRecord{
			ShipmentRequestSummaryRecord: summary,
			HasAttempt:                   taskView.HasAttempt,
			LastAttemptReason:            taskView.LastAttemptReason,
			LastAttemptContinuation:      taskView.LastAttemptContinuation,
			LastAttemptedAt:              taskView.LastAttemptedAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list customer supplement queue: %w", err)
	}
	return records, nil
}
