package postgres

import (
	"context"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// ControlReleaseRefusalCatalogue 实现 ports.ControlReleaseRefusalCatalogue：运营查阅按租户列释放未受理册上还没被处理的行（ADR-0293
// 决定一）。「未处理」是册行没有对应的处理留痕，不是原行上的状态列；租户条件进每条语句（ADR-0003）。
type ControlReleaseRefusalCatalogue struct {
	db *bentopg.DB
}

func NewControlReleaseRefusalCatalogue(db *bentopg.DB) (*ControlReleaseRefusalCatalogue, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	return &ControlReleaseRefusalCatalogue{db: db}, nil
}

var _ ports.ControlReleaseRefusalCatalogue = (*ControlReleaseRefusalCatalogue)(nil)

const (
	controlReleaseRefusalSource = "parcel_shipment.control_release_refusal r"
	// unhandledControlReleaseRefusal 是「未处理」：册行没有对应的处理留痕。
	unhandledControlReleaseRefusal = `r.tenant_id = $1
		   AND NOT EXISTS (
		       SELECT 1
		         FROM parcel_shipment.control_release_refusal_handling h
		        WHERE h.tenant_id = r.tenant_id
		          AND h.shipment_request_id = r.shipment_request_id
		          AND h.submission_version = r.submission_version
		          AND h.control_result_id = r.control_result_id)`
)

// controlReleaseRefusalPages 把 ports.ControlReleaseRefusalPages 落到列上。q 在委托号、客户账户、控制结果与委托的单据号（运单号）
// 上做字面包含：单据号不在册行上，按委托点读一列，与清单上示出的那一格是同一个号。
var controlReleaseRefusalPages = ports.ControlReleaseRefusalPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"recordedAt": "r.recorded_at"},
	Identity: []string{"r.shipment_request_id", "r.submission_version", "r.control_result_id"},
	Filters:  map[string]string{"customerAccountId": "r.customer_account_id"},
	Keyword: []string{
		"r.shipment_request_id", "r.customer_account_id", "r.control_result_id",
		`(SELECT s.document_number FROM parcel_shipment.shipment_request s
		   WHERE s.tenant_id = r.tenant_id AND s.shipment_request_id = r.shipment_request_id)`,
	},
})

// scannedControlReleaseRefusal 带着取页读回的键列原值：游标位置从它取，与 ORDER BY 用的是同一份。
type scannedControlReleaseRefusal struct {
	refusal                    ports.ControlReleaseRefusal
	request, version, resultID string
	recordedAt                 time.Time
}

// ListUnhandledControlReleaseRefusals 上列本租户册上未处理的行的一页：缺省最早登记的在前、同刻按（委托、版本、控制结果）收尾。
// Count 是未处理的全部行数，不受页与条件截断。几句不在同一快照里：计数与清单是此刻的大概，处理动作的成败由处理口自己的主键判。
func (catalogue *ControlReleaseRefusalCatalogue) ListUnhandledControlReleaseRefusals(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (ports.UnhandledControlReleaseRefusals, error) {
	fail := func(err error) (ports.UnhandledControlReleaseRefusals, error) {
		return ports.UnhandledControlReleaseRefusals{}, fmt.Errorf("list unhandled control release refusals: %w", err)
	}
	if limit <= 0 {
		return fail(fmt.Errorf("limit %d is not a positive page size", limit))
	}
	plan, err := controlReleaseRefusalPages.Plan(query, unhandledControlReleaseRefusal, []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, plan.CountSQL(controlReleaseRefusalSource), plan.Args...).Scan(&total); err != nil {
		return fail(fmt.Errorf("count: %w", err))
	}
	backlog, err := queueBacklog(ctx, querier, query, total,
		"SELECT count(*) FROM "+controlReleaseRefusalSource+" WHERE "+unhandledControlReleaseRefusal, tenant)
	if err != nil {
		return fail(err)
	}
	rows, err := querier.Query(ctx, plan.PageSQL(
		`r.shipment_request_id, r.submission_version, r.control_result_id, r.customer_account_id,
		 r.occupation_legal_entity, r.occupation_settlement_account, r.occupation_currency,
		 r.reason_ref, r.recorded_at`, controlReleaseRefusalSource), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	var fetched []scannedControlReleaseRefusal
	for rows.Next() {
		var (
			row             scannedControlReleaseRefusal
			account, reason string
			scope           occupationScopeRow
		)
		if err := rows.Scan(&row.request, &row.version, &row.resultID, &account,
			&scope.legalEntity, &scope.account, &scope.currency, &reason, &row.recordedAt); err != nil {
			return fail(err)
		}
		requestID, err := domain.NewShipmentRequestID(row.request)
		if err != nil {
			return fail(err)
		}
		if row.refusal, err = controlReleaseRefusal(tenant, requestID, row.version, row.resultID, account, scope, reason, row.recordedAt); err != nil {
			return fail(err)
		}
		fetched = append(fetched, row)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	rows.Close()
	page, err := cataloguepage.Finish(query, fetched, limit, total,
		func(row scannedControlReleaseRefusal, _ string) cataloguepage.Position {
			return cataloguepage.Position{
				Value:    cataloguepage.FormatInstant(row.recordedAt),
				Identity: []string{row.request, row.version, row.resultID},
			}
		})
	if err != nil {
		return fail(err)
	}

	listed := ports.UnhandledControlReleaseRefusals{
		Refusals:        make([]ports.ControlReleaseRefusal, 0, len(page.Rows)),
		Next:            page.Next,
		Total:           page.Total,
		Count:           backlog,
		DocumentNumbers: map[domain.ShipmentRequestID]domain.ShipmentDocumentNumbering{},
	}
	for _, row := range page.Rows {
		listed.Refusals = append(listed.Refusals, row.refusal)
	}
	if err := catalogue.readDocumentNumbers(ctx, querier, tenant, &listed); err != nil {
		return fail(err)
	}
	return listed, nil
}

// readDocumentNumbers 按清单上出现的委托另读一次单据号两列（ADR-0355 决定十一）。另起一句而不连进清单那一句：册行的过滤
// 与计数共用一段 FROM … WHERE，连表要把它拆开，两句的行集就得各自守一致。读不到的委托不进表，那一行只显委托标识。
func (catalogue *ControlReleaseRefusalCatalogue) readDocumentNumbers(
	ctx context.Context,
	querier bentopg.Querier,
	tenant domain.TenantID,
	listed *ports.UnhandledControlReleaseRefusals,
) error {
	if len(listed.Refusals) == 0 {
		return nil
	}
	requests := make([]string, 0, len(listed.Refusals))
	for _, refusal := range listed.Refusals {
		requests = append(requests, refusal.ShipmentRequestID.String())
	}
	rows, err := querier.Query(ctx,
		`SELECT shipment_request_id, document_number, document_number_absence
		   FROM parcel_shipment.shipment_request
		  WHERE tenant_id = $1
		    AND shipment_request_id = ANY($2)`,
		tenant.String(), requests)
	if err != nil {
		return fmt.Errorf("document numbers: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var requestRaw string
		var columns documentNumberingColumns
		if err := rows.Scan(&requestRaw, &columns.number, &columns.absence); err != nil {
			return fmt.Errorf("document numbers: %w", err)
		}
		requestID, err := domain.NewShipmentRequestID(requestRaw)
		if err != nil {
			return fmt.Errorf("document numbers: %w", err)
		}
		numbering, err := columns.numbering()
		if err != nil {
			return fmt.Errorf("document numbers: %w", err)
		}
		listed.DocumentNumbers[requestID] = numbering
	}
	return rows.Err()
}
