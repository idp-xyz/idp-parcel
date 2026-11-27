package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

type rowQuery interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

// 预报的状态与「这笔到仓算不算这张预报」跟前端 return-flow 的 returnAdviceStatus、receiptMatchesAdvice 同一口径：
// 记了预报标识的只对那一张；没记的按仓加退件单号，货主认出来了还得是同一家。待到仓是一张到仓记录都没有。
// 已处置是入了暂存的每一笔都记了去向；转待认领的不拦已处置。其余是已到仓。

const adviceStatusSQL = `
WITH latest_disp AS (
    SELECT DISTINCT ON (fact_id) fact_id, decision_id
      FROM warehouse_fulfillment.return_disposition
     WHERE tenant_id = $1
     ORDER BY fact_id, decided_at DESC
),
matched AS (
    SELECT a.advice_id, r.fact_id, r.outcome, d.decision_id
      FROM warehouse_fulfillment.return_pre_advice a
      LEFT JOIN warehouse_fulfillment.return_receipt r
        ON r.tenant_id = a.tenant_id
       AND (
            (NULLIF(btrim(r.advice_id), '') IS NOT NULL AND r.advice_id = a.advice_id)
            OR (
                NULLIF(btrim(r.advice_id), '') IS NULL
                AND r.warehouse_code = a.warehouse_code
                AND r.tracking_number = a.tracking_number
                AND (NULLIF(btrim(r.owner_account_id), '') IS NULL OR r.owner_account_id = a.owner_account_id)
            )
       )
      LEFT JOIN latest_disp d ON d.fact_id = r.fact_id
     WHERE a.tenant_id = $1
),
advice_status AS (
    SELECT advice_id,
           CASE
               WHEN count(fact_id) = 0 THEN 'AWAITING_ARRIVAL'
               WHEN count(fact_id) FILTER (WHERE outcome = 'COUNTED') > 0
                AND count(fact_id) FILTER (WHERE outcome = 'COUNTED' AND decision_id IS NULL) = 0
               THEN 'DISPOSED'
               ELSE 'ARRIVED'
           END AS status
      FROM matched
     GROUP BY advice_id
)`

const receiptStatusSQL = `
WITH latest_disp AS (
    SELECT DISTINCT ON (fact_id) fact_id, decision_id, destination, basis, operator_id, origin, decided_at
      FROM warehouse_fulfillment.return_disposition
     WHERE tenant_id = $1
     ORDER BY fact_id, decided_at DESC
),
marked AS (
    SELECT r.fact_id, r.warehouse_code, r.location_code, r.tracking_number, r.owner_account_id,
           r.advice_id, r.arrival_id, r.goods_code, r.unit_name, r.stock_state, r.quantity, r.outcome, r.occurred_at,
           d.decision_id, d.destination, d.basis, d.operator_id, d.origin, d.decided_at,
           CASE
               WHEN r.outcome = 'UNCLAIMED' THEN 'DIVERTED'
               WHEN d.decision_id IS NOT NULL THEN 'DISPOSED'
               WHEN r.outcome = 'COUNTED' THEN 'AWAITING_DISPOSITION'
               ELSE 'OTHER'
           END AS status
      FROM warehouse_fulfillment.return_receipt r
      LEFT JOIN latest_disp d ON d.fact_id = r.fact_id
     WHERE r.tenant_id = $1
)`

// ReadReturns 按页取退货预报或到仓记录，或按一张取。状态在库里算，不把整份明细拉进进程再切。
func (store *WorkspaceReads) ReadReturns(ctx context.Context, tenant domain.TenantID, query ports.ReturnsQuery) (ports.ReturnsPage, error) {
	const op = "returns page"
	if tenant.String() == "" {
		return ports.ReturnsPage{}, fmt.Errorf("%s: 缺租户", op)
	}
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ReturnsPage{}, fmt.Errorf("%s: %w", op, err)
	}
	switch query.View {
	case ports.ReturnsViewAdvices:
		return listReturnAdvices(ctx, querier, tenant.String(), query)
	case ports.ReturnsViewReceipts:
		return listReturnReceipts(ctx, querier, tenant.String(), query)
	case ports.ReturnsViewAdvice:
		return oneReturnAdvice(ctx, querier, tenant.String(), query.Key)
	case ports.ReturnsViewReceipt:
		return oneReturnReceipt(ctx, querier, tenant.String(), query.Key)
	default:
		return ports.ReturnsPage{}, fmt.Errorf("%s: 不认识的视图", op)
	}
}

func listReturnAdvices(ctx context.Context, querier rowQuery, tenant string, query ports.ReturnsQuery) (ports.ReturnsPage, error) {
	like := likeContains(query.Keyword)
	counts, err := adviceCounts(ctx, querier, tenant, query.Owner, query.Warehouse, like)
	if err != nil {
		return ports.ReturnsPage{}, err
	}
	total := counts["AWAITING_ARRIVAL"] + counts["ARRIVED"] + counts["DISPOSED"]
	if query.Status != "" {
		total = counts[query.Status]
	}
	page := ports.ReturnsPage{View: query.View, Page: query.Page, PageSize: query.PageSize, Total: total, Counts: counts, PreAdvices: []ports.ReturnPreAdviceRow{}, Receipts: []ports.ReturnReceiptRow{}}
	rows, err := querier.Query(ctx, adviceStatusSQL+`
SELECT a.advice_id, a.document_number, a.numbering_absence, a.owner_account_id, a.warehouse_code,
       a.origin, a.tracking_number, a.basis, a.operator_id, a.received_at, s.status
  FROM warehouse_fulfillment.return_pre_advice a
  JOIN advice_status s ON s.advice_id = a.advice_id
 WHERE a.tenant_id = $1
   AND ($2 = '' OR a.owner_account_id = $2)
   AND ($3 = '' OR a.warehouse_code = $3)
   AND ($4 = '' OR s.status = $4)
   AND ($5 = '' OR a.advice_id ILIKE $5 ESCAPE E'\\'
        OR COALESCE(a.document_number, '') ILIKE $5 ESCAPE E'\\'
        OR a.tracking_number ILIKE $5 ESCAPE E'\\'
        OR a.owner_account_id ILIKE $5 ESCAPE E'\\'
        OR a.warehouse_code ILIKE $5 ESCAPE E'\\')
 ORDER BY CASE WHEN $6 = 'oldest' THEN a.received_at END ASC,
          CASE WHEN $6 <> 'oldest' THEN a.received_at END DESC,
          a.advice_id
 LIMIT $7 OFFSET $8`, tenant, query.Owner, query.Warehouse, query.Status, like, query.Sort, query.PageSize, (query.Page-1)*query.PageSize)
	if err != nil {
		return ports.ReturnsPage{}, fmt.Errorf("returns page: advices: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		row, err := scanAdvice(rows)
		if err != nil {
			return ports.ReturnsPage{}, err
		}
		page.PreAdvices = append(page.PreAdvices, row)
		ids = append(ids, row.AdviceID)
	}
	if err := rows.Err(); err != nil {
		return ports.ReturnsPage{}, err
	}
	if err := attachAdviceLines(ctx, querier, tenant, page.PreAdvices, ids); err != nil {
		return ports.ReturnsPage{}, err
	}
	return page, nil
}

func adviceCounts(ctx context.Context, querier rowQuery, tenant, owner, warehouse, like string) (map[string]int, error) {
	counts := map[string]int{"AWAITING_ARRIVAL": 0, "ARRIVED": 0, "DISPOSED": 0}
	rows, err := querier.Query(ctx, adviceStatusSQL+`
SELECT s.status, count(*)
  FROM warehouse_fulfillment.return_pre_advice a
  JOIN advice_status s ON s.advice_id = a.advice_id
 WHERE a.tenant_id = $1
   AND ($2 = '' OR a.owner_account_id = $2)
   AND ($3 = '' OR a.warehouse_code = $3)
   AND ($4 = '' OR a.advice_id ILIKE $4 ESCAPE E'\\'
        OR COALESCE(a.document_number, '') ILIKE $4 ESCAPE E'\\'
        OR a.tracking_number ILIKE $4 ESCAPE E'\\'
        OR a.owner_account_id ILIKE $4 ESCAPE E'\\'
        OR a.warehouse_code ILIKE $4 ESCAPE E'\\')
 GROUP BY s.status`, tenant, owner, warehouse, like)
	if err != nil {
		return nil, fmt.Errorf("returns page: advice counts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		counts[status] = count
	}
	return counts, rows.Err()
}

func listReturnReceipts(ctx context.Context, querier rowQuery, tenant string, query ports.ReturnsQuery) (ports.ReturnsPage, error) {
	like := likeContains(query.Keyword)
	counts, err := receiptCounts(ctx, querier, tenant, query.Owner, query.Warehouse, like)
	if err != nil {
		return ports.ReturnsPage{}, err
	}
	total := 0
	for _, count := range counts {
		total += count
	}
	if query.Status != "" {
		total = counts[query.Status]
	}
	page := ports.ReturnsPage{View: query.View, Page: query.Page, PageSize: query.PageSize, Total: total, Counts: counts, PreAdvices: []ports.ReturnPreAdviceRow{}, Receipts: []ports.ReturnReceiptRow{}}
	rows, err := querier.Query(ctx, receiptStatusSQL+`
SELECT fact_id, warehouse_code, location_code, tracking_number, owner_account_id, advice_id, arrival_id,
       goods_code, unit_name, stock_state, quantity, outcome, occurred_at,
       decision_id, destination, basis, operator_id, origin, decided_at
  FROM marked
 WHERE ($2 = '' OR owner_account_id = $2)
   AND ($3 = '' OR warehouse_code = $3)
   AND ($4 = '' OR status = $4)
   AND ($5 = '' OR fact_id ILIKE $5 ESCAPE E'\\'
        OR tracking_number ILIKE $5 ESCAPE E'\\'
        OR COALESCE(owner_account_id, '') ILIKE $5 ESCAPE E'\\'
        OR goods_code ILIKE $5 ESCAPE E'\\'
        OR warehouse_code ILIKE $5 ESCAPE E'\\')
 ORDER BY CASE WHEN status = 'AWAITING_DISPOSITION' THEN 0 ELSE 1 END,
          CASE WHEN $6 = 'oldest' THEN occurred_at END ASC,
          CASE WHEN $6 <> 'oldest' THEN occurred_at END DESC,
          fact_id
 LIMIT $7 OFFSET $8`, tenant, query.Owner, query.Warehouse, query.Status, like, query.Sort, query.PageSize, (query.Page-1)*query.PageSize)
	if err != nil {
		return ports.ReturnsPage{}, fmt.Errorf("returns page: receipts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		row, err := scanReceipt(rows)
		if err != nil {
			return ports.ReturnsPage{}, err
		}
		page.Receipts = append(page.Receipts, row)
	}
	return page, rows.Err()
}

func receiptCounts(ctx context.Context, querier rowQuery, tenant, owner, warehouse, like string) (map[string]int, error) {
	counts := map[string]int{"AWAITING_DISPOSITION": 0, "DISPOSED": 0, "DIVERTED": 0}
	rows, err := querier.Query(ctx, receiptStatusSQL+`
SELECT status, count(*)
  FROM marked
 WHERE ($2 = '' OR owner_account_id = $2)
   AND ($3 = '' OR warehouse_code = $3)
   AND ($4 = '' OR fact_id ILIKE $4 ESCAPE E'\\'
        OR tracking_number ILIKE $4 ESCAPE E'\\'
        OR COALESCE(owner_account_id, '') ILIKE $4 ESCAPE E'\\'
        OR goods_code ILIKE $4 ESCAPE E'\\'
        OR warehouse_code ILIKE $4 ESCAPE E'\\')
 GROUP BY status`, tenant, owner, warehouse, like)
	if err != nil {
		return nil, fmt.Errorf("returns page: receipt counts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		counts[status] += count
	}
	return counts, rows.Err()
}

func oneReturnAdvice(ctx context.Context, querier rowQuery, tenant, key string) (ports.ReturnsPage, error) {
	page := ports.ReturnsPage{View: ports.ReturnsViewAdvice, PreAdvices: []ports.ReturnPreAdviceRow{}, Receipts: []ports.ReturnReceiptRow{}}
	rows, err := querier.Query(ctx, `
SELECT advice_id, document_number, numbering_absence, owner_account_id, warehouse_code,
       origin, tracking_number, basis, operator_id, received_at, ''
  FROM warehouse_fulfillment.return_pre_advice
 WHERE tenant_id = $1 AND (document_number = $2 OR advice_id = $2)
 ORDER BY CASE WHEN document_number = $2 THEN 0 ELSE 1 END, advice_id
 LIMIT 1`, tenant, key)
	if err != nil {
		return ports.ReturnsPage{}, fmt.Errorf("returns page: one advice: %w", err)
	}
	if rows.Next() {
		row, err := scanAdvice(rows)
		rows.Close()
		if err != nil {
			return ports.ReturnsPage{}, err
		}
		page.PreAdvices = []ports.ReturnPreAdviceRow{row}
	} else {
		rows.Close()
		return page, rows.Err()
	}
	if err := attachAdviceLines(ctx, querier, tenant, page.PreAdvices, []string{page.PreAdvices[0].AdviceID}); err != nil {
		return ports.ReturnsPage{}, err
	}
	advice := page.PreAdvices[0]
	receipts, err := querier.Query(ctx, `
SELECT r.fact_id, r.warehouse_code, r.location_code, r.tracking_number, r.owner_account_id,
       r.advice_id, r.arrival_id, r.goods_code, r.unit_name, r.stock_state, r.quantity, r.outcome, r.occurred_at,
       d.decision_id, d.destination, d.basis, d.operator_id, d.origin, d.decided_at
  FROM warehouse_fulfillment.return_receipt r
  LEFT JOIN LATERAL (
      SELECT decision_id, destination, basis, operator_id, origin, decided_at
        FROM warehouse_fulfillment.return_disposition
       WHERE tenant_id = r.tenant_id AND fact_id = r.fact_id
       ORDER BY decided_at DESC LIMIT 1
  ) d ON true
 WHERE r.tenant_id = $1
   AND (
        (NULLIF(btrim(r.advice_id), '') IS NOT NULL AND r.advice_id = $2)
        OR (
            NULLIF(btrim(r.advice_id), '') IS NULL
            AND r.warehouse_code = $3
            AND r.tracking_number = $4
            AND (NULLIF(btrim(r.owner_account_id), '') IS NULL OR r.owner_account_id = $5)
        )
   )
 ORDER BY r.occurred_at DESC, r.fact_id`, tenant, advice.AdviceID, advice.WarehouseCode, advice.TrackingNumber, advice.OwnerAccount)
	if err != nil {
		return ports.ReturnsPage{}, err
	}
	defer receipts.Close()
	for receipts.Next() {
		row, err := scanReceipt(receipts)
		if err != nil {
			return ports.ReturnsPage{}, err
		}
		page.Receipts = append(page.Receipts, row)
	}
	return page, receipts.Err()
}

func oneReturnReceipt(ctx context.Context, querier rowQuery, tenant, key string) (ports.ReturnsPage, error) {
	page := ports.ReturnsPage{View: ports.ReturnsViewReceipt, PreAdvices: []ports.ReturnPreAdviceRow{}, Receipts: []ports.ReturnReceiptRow{}}
	rows, err := querier.Query(ctx, `
SELECT r.fact_id, r.warehouse_code, r.location_code, r.tracking_number, r.owner_account_id,
       r.advice_id, r.arrival_id, r.goods_code, r.unit_name, r.stock_state, r.quantity, r.outcome, r.occurred_at,
       d.decision_id, d.destination, d.basis, d.operator_id, d.origin, d.decided_at
  FROM warehouse_fulfillment.return_receipt r
  LEFT JOIN LATERAL (
      SELECT decision_id, destination, basis, operator_id, origin, decided_at
        FROM warehouse_fulfillment.return_disposition
       WHERE tenant_id = r.tenant_id AND fact_id = r.fact_id
       ORDER BY decided_at DESC LIMIT 1
  ) d ON true
 WHERE r.tenant_id = $1 AND r.fact_id = $2`, tenant, key)
	if err != nil {
		return ports.ReturnsPage{}, err
	}
	if !rows.Next() {
		rows.Close()
		return page, rows.Err()
	}
	receipt, err := scanReceipt(rows)
	rows.Close()
	if err != nil {
		return ports.ReturnsPage{}, err
	}
	siblings, err := querier.Query(ctx, `
SELECT r.fact_id, r.warehouse_code, r.location_code, r.tracking_number, r.owner_account_id,
       r.advice_id, r.arrival_id, r.goods_code, r.unit_name, r.stock_state, r.quantity, r.outcome, r.occurred_at,
       d.decision_id, d.destination, d.basis, d.operator_id, d.origin, d.decided_at
  FROM warehouse_fulfillment.return_receipt r
  LEFT JOIN LATERAL (
      SELECT decision_id, destination, basis, operator_id, origin, decided_at
        FROM warehouse_fulfillment.return_disposition
       WHERE tenant_id = r.tenant_id AND fact_id = r.fact_id
       ORDER BY decided_at DESC LIMIT 1
  ) d ON true
 WHERE r.tenant_id = $1 AND r.warehouse_code = $2 AND r.tracking_number = $3
 ORDER BY r.occurred_at DESC, r.fact_id`, tenant, receipt.WarehouseCode, receipt.TrackingNumber)
	if err != nil {
		return ports.ReturnsPage{}, err
	}
	defer siblings.Close()
	for siblings.Next() {
		row, err := scanReceipt(siblings)
		if err != nil {
			return ports.ReturnsPage{}, err
		}
		page.Receipts = append(page.Receipts, row)
	}
	if err := siblings.Err(); err != nil {
		return ports.ReturnsPage{}, err
	}
	advices, err := querier.Query(ctx, `
SELECT advice_id, document_number, numbering_absence, owner_account_id, warehouse_code,
       origin, tracking_number, basis, operator_id, received_at, ''
  FROM warehouse_fulfillment.return_pre_advice
 WHERE tenant_id = $1
   AND (
        ($2 <> '' AND advice_id = $2)
        OR (
            $2 = ''
            AND warehouse_code = $3
            AND tracking_number = $4
            AND ($5 = '' OR owner_account_id = $5)
        )
   )`, tenant, receipt.AdviceID, receipt.WarehouseCode, receipt.TrackingNumber, receipt.OwnerAccount)
	if err != nil {
		return ports.ReturnsPage{}, err
	}
	defer advices.Close()
	var ids []string
	for advices.Next() {
		row, err := scanAdvice(advices)
		if err != nil {
			return ports.ReturnsPage{}, err
		}
		page.PreAdvices = append(page.PreAdvices, row)
		ids = append(ids, row.AdviceID)
	}
	if err := advices.Err(); err != nil {
		return ports.ReturnsPage{}, err
	}
	if err := attachAdviceLines(ctx, querier, tenant, page.PreAdvices, ids); err != nil {
		return ports.ReturnsPage{}, err
	}
	return page, nil
}

type adviceScanner interface {
	Scan(dest ...any) error
}

func scanAdvice(rows adviceScanner) (ports.ReturnPreAdviceRow, error) {
	var (
		row                   ports.ReturnPreAdviceRow
		num, abs, basis, opID *string
	)
	if err := rows.Scan(&row.AdviceID, &num, &abs, &row.OwnerAccount, &row.WarehouseCode, &row.Origin, &row.TrackingNumber, &basis, &opID, &row.ReceivedAt, &row.Status); err != nil {
		return ports.ReturnPreAdviceRow{}, fmt.Errorf("returns page: advice: %w", err)
	}
	row.DocumentNumber = valueOrEmpty(num)
	row.NumberingAbsence = valueOrEmpty(abs)
	row.Basis = valueOrEmpty(basis)
	row.Operator = valueOrEmpty(opID)
	return row, nil
}

func scanReceipt(rows adviceScanner) (ports.ReturnReceiptRow, error) {
	var (
		row                              ports.ReturnReceiptRow
		owner, advice, arrival           *string
		decID, dest, basis, opID, origin *string
		decided                          *time.Time
	)
	if err := rows.Scan(
		&row.FactID, &row.WarehouseCode, &row.LocationCode, &row.TrackingNumber, &owner, &advice, &arrival,
		&row.GoodsCode, &row.Unit, &row.StockState, &row.Quantity, &row.Outcome, &row.OccurredAt,
		&decID, &dest, &basis, &opID, &origin, decided,
	); err != nil {
		return ports.ReturnReceiptRow{}, fmt.Errorf("returns page: receipt: %w", err)
	}
	row.OwnerAccount = valueOrEmpty(owner)
	row.AdviceID = valueOrEmpty(advice)
	row.ArrivalID = valueOrEmpty(arrival)
	if decID != nil && *decID != "" {
		disp := &ports.ReturnDispositionRow{
			DecisionID:  *decID,
			Destination: valueOrEmpty(dest),
			Basis:       valueOrEmpty(basis),
			Operator:    valueOrEmpty(opID),
			Origin:      valueOrEmpty(origin),
		}
		if decided != nil {
			disp.DecidedAt = *decided
		}
		row.Disposition = disp
	}
	return row, nil
}

func attachAdviceLines(ctx context.Context, querier rowQuery, tenant string, advices []ports.ReturnPreAdviceRow, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	rows, err := querier.Query(ctx, `
SELECT advice_id, goods_code, unit_name, quantity
  FROM warehouse_fulfillment.return_pre_advice_line
 WHERE tenant_id = $1 AND advice_id = ANY($2)
 ORDER BY advice_id, line_index`, tenant, ids)
	if err != nil {
		return fmt.Errorf("returns page: lines: %w", err)
	}
	defer rows.Close()
	byID := map[string]*ports.ReturnPreAdviceRow{}
	for i := range advices {
		advices[i].Lines = []ports.ReturnPreAdviceLineRow{}
		byID[advices[i].AdviceID] = &advices[i]
	}
	for rows.Next() {
		var adviceID string
		var line ports.ReturnPreAdviceLineRow
		if err := rows.Scan(&adviceID, &line.GoodsCode, &line.Unit, &line.Quantity); err != nil {
			return err
		}
		if advice := byID[adviceID]; advice != nil {
			advice.Lines = append(advice.Lines, line)
		}
	}
	return rows.Err()
}

func likeContains(keyword string) string {
	if keyword == "" {
		return ""
	}
	var b strings.Builder
	b.WriteByte('%')
	for _, r := range keyword {
		if r == '%' || r == '_' || r == '\\' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('%')
	return b.String()
}
