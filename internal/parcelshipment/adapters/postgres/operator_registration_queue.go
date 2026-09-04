package postgres

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件在 ShipmentRequests 上补齐 ports.OperatorRegistrationQueue（ADR-0094 Decision 五，票
// first-tenant-runway/07）。放在仓储而不是 ShipmentRequestViews 上：它交回的是重驱要用的来源
// 身份与成员清单，那是写侧的键，不是查阅概要；而且以租户为键，不走 visibleAccounts 那套作用域
// 过滤——理由在端口注释里，此处不复述。
//
// 过滤在投影列上（迁移 0009 立列、0011 放宽到第四格、0013 建部分索引）：`task_waiting_on` 由
// Insert/Save 与快照同一条 SQL 写下，谓词 `task_waiting_on = $2 AND state = $3` 与部分索引逐字
// 吻合。数字由 Go 侧常量传参，SQL 只比较不拥有编号——编号的唯一来源始终是领域包。state 上榜
// 的理由与复核队列同：等待态只在`已提交`委托上有队列语义，撤回停止的任务留着最后一次 waitingOn
// 快照也不该上列。
var _ ports.OperatorRegistrationQueue = (*ShipmentRequests)(nil)

// ListWaitingOnOperatorRegistration 按租户取回当前停在`等待运营登记`的委托，老的在前，同刻按
// 委托标识正序。逐字段过领域构造函数：库里一行坏数据在这道门上暴露，不会变成一条看起来合法的
// 重驱命令（ADR-0028）。limit 非正是调用方编程错误，判据同 ListVisible。
func (repository *ShipmentRequests) ListWaitingOnOperatorRegistration(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.OperatorRegistrationQueueRecord, error) {
	if limit < 1 {
		return nil, fmt.Errorf("list operator registration queue: limit must be positive, got %d", limit)
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list operator registration queue: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT customer_account_id, source, source_request_key,
		        shipment_request_id, current_submission_version_id,
		        declared_parcel_ids, submitted_at
		   FROM parcel_shipment.shipment_request
		  WHERE tenant_id = $1
		    AND task_waiting_on = $2
		    AND state = $3
		  ORDER BY submitted_at ASC, shipment_request_id ASC
		  LIMIT $4`,
		tenant.String(),
		uint8(domain.ResumeByOperatorRegistration),
		uint8(domain.ShipmentRequestSubmitted),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list operator registration queue: %w", err)
	}
	defer rows.Close()

	var records []ports.OperatorRegistrationQueueRecord
	for rows.Next() {
		var row operatorRegistrationQueueRow
		if err := rows.Scan(
			&row.customerAccountID, &row.source, &row.sourceRequestKey,
			&row.shipmentRequestID, &row.submissionVersionID,
			&row.declaredParcelIDs, &row.submittedAt,
		); err != nil {
			return nil, fmt.Errorf("list operator registration queue: %w", err)
		}
		record, err := row.record(tenant)
		if err != nil {
			return nil, fmt.Errorf("list operator registration queue: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list operator registration queue: %w", err)
	}
	return records, nil
}

// operatorRegistrationQueueRow 是队列查询的列面。与 summaryRow 分开：它多带成员清单、少带状态与
// 成员计数，两者要的列不同，合成一个会让其中一边扫描自己用不到的列。
type operatorRegistrationQueueRow struct {
	customerAccountID   string
	source              string
	sourceRequestKey    string
	shipmentRequestID   string
	submissionVersionID string
	declaredParcelIDs   []string
	submittedAt         time.Time
}

func (row operatorRegistrationQueueRow) record(tenant domain.TenantID) (ports.OperatorRegistrationQueueRecord, error) {
	customer, err := domain.NewCustomerAccountID(row.customerAccountID)
	if err != nil {
		return ports.OperatorRegistrationQueueRecord{}, err
	}
	source, err := domain.NewSource(row.source)
	if err != nil {
		return ports.OperatorRegistrationQueueRecord{}, err
	}
	requestKey, err := domain.NewSourceRequestKey(row.sourceRequestKey)
	if err != nil {
		return ports.OperatorRegistrationQueueRecord{}, err
	}
	identity, err := domain.NewSourceIdentity(tenant, customer, source, requestKey)
	if err != nil {
		return ports.OperatorRegistrationQueueRecord{}, err
	}
	requestID, err := domain.NewShipmentRequestID(row.shipmentRequestID)
	if err != nil {
		return ports.OperatorRegistrationQueueRecord{}, err
	}
	versionID, err := domain.NewSubmissionVersionID(row.submissionVersionID)
	if err != nil {
		return ports.OperatorRegistrationQueueRecord{}, err
	}
	parcels := make([]domain.DeclaredParcelID, 0, len(row.declaredParcelIDs))
	for _, raw := range row.declaredParcelIDs {
		parcel, err := domain.NewDeclaredParcelID(raw)
		if err != nil {
			return ports.OperatorRegistrationQueueRecord{}, err
		}
		parcels = append(parcels, parcel)
	}
	return ports.OperatorRegistrationQueueRecord{
		Identity:          identity,
		ShipmentRequestID: requestID,
		SubmissionVersion: versionID,
		DeclaredParcelIDs: parcels,
		SubmittedAt:       row.submittedAt,
	}, nil
}
