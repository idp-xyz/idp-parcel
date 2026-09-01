package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// ShipmentRequestViews 实现 ports.ShipmentRequestViews：委托查阅的只读面。
//
// 它与 ShipmentRequests 分立而不是再挂两个方法：仓储走聚合重建，重建门只开到`已提交`
// 与`已接受`（ADR-0061），而查阅必须把`已拒绝`/`已撤回`也如实交代。本读面直读投影列
// 与快照文档，不经 RehydrateShipmentRequest——读的是「登记过什么」，不是「聚合此刻能
// 不能重建」。
//
// 可见性过滤在 SQL 键上（CONTEXT「授权查询作用域」）：租户与客户账户集合进 WHERE，
// 读口因此答不出作用域外的行；否定结果不区分「不存在」与「作用域外」（`统一不可见
// 结果`），两者都是零行命中。
type ShipmentRequestViews struct {
	db *bentopg.DB
}

func NewShipmentRequestViews(db *bentopg.DB) (*ShipmentRequestViews, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	return &ShipmentRequestViews{db: db}, nil
}

var _ ports.ShipmentRequestViews = (*ShipmentRequestViews)(nil)

// ListVisible 按作用域取回最近的委托概要，新的在前；同刻并列时按委托标识倒序，保证
// 分页可重复。limit 非正是调用方编程错误——静默答一页会把「忘了传」变成一个没人决定
// 过的页大小。
func (views *ShipmentRequestViews) ListVisible(
	ctx context.Context,
	scope domain.AuthorizedQueryScope,
	limit int,
) ([]ports.ShipmentRequestSummaryRecord, error) {
	if limit < 1 {
		return nil, fmt.Errorf("list shipment request views: limit must be positive, got %d", limit)
	}
	querier, err := views.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list shipment request views: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT customer_account_id, source, source_request_key,
		        shipment_request_id, state, submitted_at,
		        current_submission_version_id, cardinality(declared_parcel_ids)
		   FROM parcel_shipment.shipment_request
		  WHERE tenant_id = $1
		    AND customer_account_id = ANY($2)
		  ORDER BY submitted_at DESC, shipment_request_id DESC
		  LIMIT $3`,
		scope.TenantID().String(),
		visibleAccounts(scope),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list shipment request views: %w", err)
	}
	defer rows.Close()

	var records []ports.ShipmentRequestSummaryRecord
	for rows.Next() {
		var row summaryRow
		if err := rows.Scan(
			&row.customerAccountID, &row.source, &row.sourceRequestKey,
			&row.shipmentRequestID, &row.state, &row.submittedAt,
			&row.submissionVersionID, &row.declaredParcelCount,
		); err != nil {
			return nil, fmt.Errorf("list shipment request views: %w", err)
		}
		record, err := row.record()
		if err != nil {
			return nil, fmt.Errorf("list shipment request views: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list shipment request views: %w", err)
	}
	return records, nil
}

// FindVisibleByID 按（作用域 + 委托标识）取回查阅详情。作用域过滤与标识同在 WHERE 上，
// 零行即不可见——「不存在」与「属别的作用域」在这条 SQL 里天然同答。
func (views *ShipmentRequestViews) FindVisibleByID(
	ctx context.Context,
	scope domain.AuthorizedQueryScope,
	requestID domain.ShipmentRequestID,
) (ports.ShipmentRequestDetailRecord, bool, error) {
	querier, err := views.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ShipmentRequestDetailRecord{}, false, fmt.Errorf("find shipment request view: %w", err)
	}

	var row summaryRow
	var raw []byte
	err = querier.QueryRow(ctx,
		`SELECT customer_account_id, source, source_request_key,
		        shipment_request_id, state, submitted_at,
		        current_submission_version_id, cardinality(declared_parcel_ids),
		        snapshot
		   FROM parcel_shipment.shipment_request
		  WHERE tenant_id = $1
		    AND customer_account_id = ANY($2)
		    AND shipment_request_id = $3`,
		scope.TenantID().String(),
		visibleAccounts(scope),
		requestID.String(),
	).Scan(
		&row.customerAccountID, &row.source, &row.sourceRequestKey,
		&row.shipmentRequestID, &row.state, &row.submittedAt,
		&row.submissionVersionID, &row.declaredParcelCount,
		&raw,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ShipmentRequestDetailRecord{}, false, nil
	}
	if err != nil {
		return ports.ShipmentRequestDetailRecord{}, false, fmt.Errorf("find shipment request view: %w", err)
	}

	summary, err := row.record()
	if err != nil {
		return ports.ShipmentRequestDetailRecord{}, false, fmt.Errorf("find shipment request view: %w", err)
	}
	var document requestDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return ports.ShipmentRequestDetailRecord{}, false,
			fmt.Errorf("find shipment request view: 快照不是本适配器写下的形状：%w", err)
	}
	detail, err := detailRecord(summary, document)
	if err != nil {
		return ports.ShipmentRequestDetailRecord{}, false, fmt.Errorf("find shipment request view: %w", err)
	}
	return detail, true, nil
}

func visibleAccounts(scope domain.AuthorizedQueryScope) []string {
	accounts := scope.CustomerAccountIDs()
	values := make([]string, len(accounts))
	for index, account := range accounts {
		values[index] = account.String()
	}
	return values
}

// summaryRow 是两条查询共用的列面。逐字段过领域构造函数（record）：库里一行坏数据在
// 这道门上暴露，不会变成一条看起来合法的概要——与仓储读回侧同一条纪律（ADR-0028）。
type summaryRow struct {
	customerAccountID   string
	source              string
	sourceRequestKey    string
	shipmentRequestID   string
	state               uint8
	submittedAt         time.Time
	submissionVersionID string
	declaredParcelCount int
}

func (row summaryRow) record() (ports.ShipmentRequestSummaryRecord, error) {
	customer, err := domain.NewCustomerAccountID(row.customerAccountID)
	if err != nil {
		return ports.ShipmentRequestSummaryRecord{}, err
	}
	source, err := domain.NewSource(row.source)
	if err != nil {
		return ports.ShipmentRequestSummaryRecord{}, err
	}
	requestKey, err := domain.NewSourceRequestKey(row.sourceRequestKey)
	if err != nil {
		return ports.ShipmentRequestSummaryRecord{}, err
	}
	requestID, err := domain.NewShipmentRequestID(row.shipmentRequestID)
	if err != nil {
		return ports.ShipmentRequestSummaryRecord{}, err
	}
	versionID, err := domain.NewSubmissionVersionID(row.submissionVersionID)
	if err != nil {
		return ports.ShipmentRequestSummaryRecord{}, err
	}
	state := domain.ShipmentRequestState(row.state)
	if state.String() == "" {
		return ports.ShipmentRequestSummaryRecord{}, fmt.Errorf("状态列不是本上下文的取值：%d", row.state)
	}
	return ports.ShipmentRequestSummaryRecord{
		CustomerAccountID:   customer,
		Source:              source,
		SourceRequestKey:    requestKey,
		ShipmentRequestID:   requestID,
		State:               state,
		SubmissionVersionID: versionID,
		DeclaredParcelCount: row.declaredParcelCount,
		SubmittedAt:         row.submittedAt,
	}, nil
}

// detailRecord 从快照文档摊出详情。只读文档已有的字段，不重算任何判断；文档里没有的
// （如撤回记录明细）如实缺席，不代拟。
func detailRecord(
	summary ports.ShipmentRequestSummaryRecord,
	document requestDocument,
) (ports.ShipmentRequestDetailRecord, error) {
	batchID, err := domain.NewSubmissionBatchID(document.BatchID)
	if err != nil {
		return ports.ShipmentRequestDetailRecord{}, err
	}

	parcels, err := declaredParcelRecords(document.CurrentVersion)
	if err != nil {
		return ports.ShipmentRequestDetailRecord{}, err
	}

	task, err := taskViewRecord(document.AcceptanceTask)
	if err != nil {
		return ports.ShipmentRequestDetailRecord{}, err
	}

	detail := ports.ShipmentRequestDetailRecord{
		ShipmentRequestSummaryRecord: summary,
		BatchID:                      batchID,
		OccurredAt:                   document.CurrentVersion.Source.OccurredAt,
		ReceivedAt:                   document.CurrentVersion.Source.ReceivedAt,
		DeclaredParcels:              parcels,
		PriorVersionCount:            len(document.PriorVersions),
		Task:                         task,
	}
	if document.Decision != nil {
		decisionID, err := domain.NewAcceptanceDecisionID(document.Decision.DecisionID)
		if err != nil {
			return ports.ShipmentRequestDetailRecord{}, err
		}
		detail.HasDecision = true
		detail.Decision = ports.AcceptanceDecisionViewRecord{
			DecisionID: decisionID,
			Accepted:   document.Decision.Accepted,
			DecidedAt:  document.Decision.DecidedAt,
		}
	}
	return detail, nil
}

func declaredParcelRecords(version versionDocument) ([]ports.DeclaredParcelViewRecord, error) {
	profiles := make(map[string]profileDocument, len(version.Profiles))
	for _, profile := range version.Profiles {
		profiles[profile.Parcel] = profile
	}

	records := make([]ports.DeclaredParcelViewRecord, 0, len(version.DeclaredParcelIDs))
	for _, raw := range version.DeclaredParcelIDs {
		parcel, err := domain.NewDeclaredParcelID(raw)
		if err != nil {
			return nil, err
		}
		record := ports.DeclaredParcelViewRecord{Parcel: parcel}
		if profile, declared := profiles[raw]; declared {
			record.WeightValue = profile.Weight.Value
			record.WeightUnit = profile.Weight.Unit
			if profile.Dimensions != nil {
				record.HasDimensions = true
				record.Length = profile.Dimensions.Length
				record.Width = profile.Dimensions.Width
				record.Height = profile.Dimensions.Height
				record.DimensionsUnit = profile.Dimensions.Unit
			}
		}
		records = append(records, record)
	}
	return records, nil
}

func taskViewRecord(document taskDocument) (ports.AcceptanceTaskViewRecord, error) {
	state := domain.AcceptanceTaskState(document.State)
	if state.String() == "" {
		return ports.AcceptanceTaskViewRecord{}, fmt.Errorf("任务状态不是本上下文的取值：%d", document.State)
	}
	record := ports.AcceptanceTaskViewRecord{State: state}
	if count := len(document.ProcessingAttempts); count > 0 {
		last := document.ProcessingAttempts[count-1]
		record.HasAttempt = true
		record.LastAttemptReason = last.Reason
		record.LastAttemptContinuation = last.Continuation
		record.LastAttemptedAt = last.AttemptedAt
	}
	// 等待态值域在读面也查一道（判据同重建侧）：越界值不校会被读成缺席——「这任务
	// 不等任何人」——而不是被拒成一行坏数据。零值即缺席，照实留零。
	if document.WaitingOn != 0 {
		waiting := domain.ResumePath(document.WaitingOn)
		if waiting.String() == "" {
			return ports.AcceptanceTaskViewRecord{}, fmt.Errorf("任务等待态不是本上下文的取值：%d", document.WaitingOn)
		}
		record.WaitingOn = waiting
	}
	if document.ReviewCompletion != nil {
		record.ReviewCompleted = true
		record.ReviewAuthority = document.ReviewCompletion.Authority
		record.ReviewReviewer = document.ReviewCompletion.Reviewer
		record.ReviewEvidence = document.ReviewCompletion.Evidence
		record.ReviewCompletedAt = document.ReviewCompletion.CompletedAt
	}
	return record, nil
}
