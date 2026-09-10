package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件在 ShipmentRequestViews 上补齐 ports.AuthorizedDispositionQueue（ADR-0132 决定二，票
// sa-preacceptance-policy-view/04）。放在查阅适配器而不是仓储上，理由与 customer_supplement_queue
// 逐字相同：它是「等授权处置的都有谁」的读面，与另几个队列读的是同一张表、同一套作用域纪律。
//
// 过滤在投影列上（迁移 0009 立列、0021 建部分索引）：谓词 `task_waiting_on = $3 AND state = $4` 与部分索引
// 逐字吻合，数字由 Go 侧常量传参，编号的唯一来源始终是领域包。
//
// 受限项从判断表取（同包 loadFinancialControl，与形成决定那一步读的是同一批行）：每行一次点读。队列
// 行数受 limit 约束，且停在等处置的委托本就少——那是策略正文登了`进入授权处置`的合同才走得到的一格。
var _ ports.AuthorizedDispositionQueue = (*ShipmentRequestViews)(nil)

// ListAwaitingAuthorizedDisposition 按作用域取回当前停在`等待授权处置`的委托，老的在前（先停的先处置），
// 同刻按委托标识正序保证分页可重复。limit 非正是调用方编程错误，判据同 ListVisible。
func (views *ShipmentRequestViews) ListAwaitingAuthorizedDisposition(
	ctx context.Context,
	scope domain.AuthorizedQueryScope,
	limit int,
) ([]ports.AuthorizedDispositionQueueRecord, error) {
	if limit < 1 {
		return nil, fmt.Errorf("list authorized disposition queue: limit must be positive, got %d", limit)
	}
	querier, err := views.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list authorized disposition queue: %w", err)
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
		uint8(domain.ResumeByAuthorizedDisposition),
		uint8(domain.ShipmentRequestSubmitted),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list authorized disposition queue: %w", err)
	}
	defer rows.Close()

	var records []ports.AuthorizedDispositionQueueRecord
	for rows.Next() {
		var row summaryRow
		var rawTask []byte
		if err := rows.Scan(
			&row.customerAccountID, &row.source, &row.sourceRequestKey,
			&row.shipmentRequestID, &row.state, &row.submittedAt,
			&row.submissionVersionID, &row.declaredParcelCount,
			&rawTask,
		); err != nil {
			return nil, fmt.Errorf("list authorized disposition queue: %w", err)
		}
		summary, err := row.record()
		if err != nil {
			return nil, fmt.Errorf("list authorized disposition queue: %w", err)
		}
		var task taskDocument
		if err := json.Unmarshal(rawTask, &task); err != nil {
			return nil, fmt.Errorf("list authorized disposition queue: 快照不是本适配器写下的形状：%w", err)
		}
		taskView, err := taskViewRecord(task)
		if err != nil {
			return nil, fmt.Errorf("list authorized disposition queue: %w", err)
		}
		records = append(records, ports.AuthorizedDispositionQueueRecord{
			ShipmentRequestSummaryRecord: summary,
			HasAttempt:                   taskView.HasAttempt,
			LastAttemptReason:            taskView.LastAttemptReason,
			LastAttemptContinuation:      taskView.LastAttemptContinuation,
			LastAttemptedAt:              taskView.LastAttemptedAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list authorized disposition queue: %w", err)
	}
	// 游标关了再逐行点读控制结果：同一个执行器上不套第二个游标。
	for index := range records {
		control, err := loadFinancialControl(ctx, querier,
			scope.TenantID(), records[index].ShipmentRequestID, records[index].SubmissionVersionID)
		if err != nil {
			return nil, fmt.Errorf("list authorized disposition queue: %w", err)
		}
		records[index].ControlResultID = control.ResultID()
		records[index].RestrictedItems = restrictedItemsOf(control)
	}
	return records, nil
}

// restrictedItemsOf 只取受限项——成立项没有去向可看。处置与责任引用照采用引用透出，没采用的项两格为零值：
// 一份停在等处置的委托不该有那样的项（进这一格的前提是全部受限项都采用了`进入授权处置`），读到即如实透出，
// 不补造。
func restrictedItemsOf(control domain.FinancialControlResult) []ports.RestrictedControlItemRecord {
	var restricted []ports.RestrictedControlItemRecord
	for _, item := range control.Items() {
		if item.Satisfied() {
			continue
		}
		record := ports.RestrictedControlItemRecord{
			Kind:  item.Kind(),
			Order: item.Order(),
			Basis: item.Basis(),
		}
		if adopted, present := item.AdoptedDisposition(); present {
			record.FailureDisposition = adopted.FailureDisposition()
			record.Responsibility = adopted.Responsibility()
		}
		restricted = append(restricted, record)
	}
	return restricted
}
