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
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// ShipmentRequestViews 实现 ports.ShipmentRequestViews：委托查阅的只读面。
//
// 它与 ShipmentRequests 分立而不是再挂两个方法：仓储走聚合重建，一行过不了重建门的坏快照读不回，
// 而查阅必须把每一行如实交代。本读面直读投影列与快照文档，不经 RehydrateShipmentRequest——
// 读的是「登记过什么」，不是「聚合此刻能不能重建」。
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

// shipmentRequestViewPages 把 ports.ShipmentRequestViewPages 落到列上。state 落在存代码的 state 列；q 在委托号、单据号（运单号）
// 与来源请求键上做字面包含——单据号可空，没有号的委托在 q 上只靠另两列命中。
var shipmentRequestViewPages = ports.ShipmentRequestViewPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"submittedAt": "submitted_at", "shipmentRequestId": "shipment_request_id"},
	Identity: []string{"shipment_request_id"},
	Filters:  map[string]string{"state": "state", "customerAccountId": "customer_account_id"},
	Codes:    map[string]func(string) (int64, error){"state": cataloguepage.Code[domain.ShipmentRequestState]},
	Keyword:  []string{"shipment_request_id", "document_number", "source_request_key"},
})

// visibleRequests 是作用域那一段固定条件：租户与可见客户账户集合进 WHERE，读面答不出作用域外的行（`统一不可见结果`）。
const visibleRequests = "tenant_id = $1 AND customer_account_id = ANY($2)"

// ListVisible 按作用域取委托概要的一页：缺省提交时刻倒序、同刻按委托标识倒序。limit 非正是调用方编程错误——静默答一页
// 会把「忘了传」变成一个没人决定过的页大小。
func (views *ShipmentRequestViews) ListVisible(
	ctx context.Context,
	scope domain.AuthorizedQueryScope,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.ShipmentRequestSummaryRecord], error) {
	fail := func(err error) (cataloguepage.Result[ports.ShipmentRequestSummaryRecord], error) {
		return cataloguepage.Result[ports.ShipmentRequestSummaryRecord]{}, fmt.Errorf("list shipment request views: %w", err)
	}
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := shipmentRequestViewPages.Plan(query, visibleRequests,
		[]any{scope.TenantID().String(), visibleAccounts(scope)}, limit)
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
	rows, err := querier.Query(ctx, plan.PageSQL(summaryColumns, shipmentRequestTable), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	records, err := collectSummaries(rows)
	if err != nil {
		return fail(err)
	}
	page, err := cataloguepage.Finish(query, records, limit, total, summaryPosition)
	if err != nil {
		return fail(err)
	}
	return page, nil
}

const shipmentRequestTable = "parcel_shipment.shipment_request"

// summaryPosition 给出一份概要在排序维上的位置。提交时刻与委托号都是概要直接从列上读回的值（collectSummaries），与 ORDER BY
// 用的是同一份。
func summaryPosition(summary ports.ShipmentRequestSummaryRecord, sortField string) cataloguepage.Position {
	value := summary.ShipmentRequestID.String()
	if sortField == "submittedAt" {
		value = cataloguepage.FormatInstant(summary.SubmittedAt)
	}
	return cataloguepage.Position{Value: value, Identity: []string{summary.ShipmentRequestID.String()}}
}

// ListVisibleByDocumentNumber 按（作用域 + 单据号）取回概要，逐字比对、不折算大小写或空白（号由编号规则渲染，折算就是另一个号）。
// 作用域过滤同在 WHERE 上，作用域外的同号委托与没有同一个答案。
func (views *ShipmentRequestViews) ListVisibleByDocumentNumber(
	ctx context.Context,
	scope domain.AuthorizedQueryScope,
	number domain.ShipmentDocumentNumber,
) ([]ports.ShipmentRequestSummaryRecord, error) {
	querier, err := views.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list shipment request views by document number: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT `+summaryColumns+`
		   FROM parcel_shipment.shipment_request
		  WHERE tenant_id = $1
		    AND customer_account_id = ANY($2)
		    AND document_number = $3
		  ORDER BY submitted_at DESC, shipment_request_id DESC`,
		scope.TenantID().String(),
		visibleAccounts(scope),
		number.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("list shipment request views by document number: %w", err)
	}
	records, err := collectSummaries(rows)
	if err != nil {
		return nil, fmt.Errorf("list shipment request views by document number: %w", err)
	}
	return records, nil
}

// ListVisibleByIDs 按（作用域 + 一组委托标识）取回概要。作用域过滤与标识集合同在 WHERE 上：作用域外的与不存在的都是零行，
// 读口答不出缺的那一份是哪一种。重复的标识只命中一行。一组多大由接入面按渠道契约裁决；空组不发查询。
func (views *ShipmentRequestViews) ListVisibleByIDs(
	ctx context.Context,
	scope domain.AuthorizedQueryScope,
	requestIDs []domain.ShipmentRequestID,
) ([]ports.ShipmentRequestSummaryRecord, error) {
	if len(requestIDs) == 0 {
		return nil, nil
	}
	ids := make([]string, len(requestIDs))
	for index, requestID := range requestIDs {
		ids[index] = requestID.String()
	}
	querier, err := views.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list shipment request views by ids: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT `+summaryColumns+`
		   FROM parcel_shipment.shipment_request
		  WHERE tenant_id = $1
		    AND customer_account_id = ANY($2)
		    AND shipment_request_id = ANY($3)
		  ORDER BY submitted_at DESC, shipment_request_id DESC`,
		scope.TenantID().String(),
		visibleAccounts(scope),
		ids,
	)
	if err != nil {
		return nil, fmt.Errorf("list shipment request views by ids: %w", err)
	}
	records, err := collectSummaries(rows)
	if err != nil {
		return nil, fmt.Errorf("list shipment request views by ids: %w", err)
	}
	return records, nil
}

// collectSummaries 把一条概要查询的结果逐行过 summaryRow.record 收齐，并关掉结果集。
func collectSummaries(rows pgx.Rows) ([]ports.ShipmentRequestSummaryRecord, error) {
	defer rows.Close()
	var records []ports.ShipmentRequestSummaryRecord
	for rows.Next() {
		var row summaryRow
		if err := rows.Scan(row.targets()...); err != nil {
			return nil, err
		}
		record, err := row.record()
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
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
		`SELECT `+summaryColumns+`,
		        snapshot
		   FROM parcel_shipment.shipment_request
		  WHERE tenant_id = $1
		    AND customer_account_id = ANY($2)
		    AND shipment_request_id = $3`,
		scope.TenantID().String(),
		visibleAccounts(scope),
		requestID.String(),
	).Scan(append(row.targets(), &raw)...)
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

// summaryColumns 是委托概要的列面，查阅列表、单份与各队列共用；扫描目标由 summaryRow.targets 按同一顺序给出，两处一起改。
const summaryColumns = `customer_account_id, source, source_request_key,
		        shipment_request_id, state, submitted_at,
		        current_submission_version_id, cardinality(declared_parcel_ids),
		        document_number, document_number_absence`

// summaryRow 是各条查询共用的列面。逐字段过领域构造函数（record）：库里一行坏数据在
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
	documentNumbering   documentNumberingColumns
}

// targets 是 summaryColumns 的扫描目标，顺序与它逐列对应。
func (row *summaryRow) targets() []any {
	return []any{
		&row.customerAccountID, &row.source, &row.sourceRequestKey,
		&row.shipmentRequestID, &row.state, &row.submittedAt,
		&row.submissionVersionID, &row.declaredParcelCount,
		&row.documentNumbering.number, &row.documentNumbering.absence,
	}
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
	numbering, err := row.documentNumbering.numbering()
	if err != nil {
		return ports.ShipmentRequestSummaryRecord{}, err
	}
	return ports.ShipmentRequestSummaryRecord{
		DocumentNumbering:   numbering,
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
