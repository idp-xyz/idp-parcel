package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// PreAdviceClosures 是入库预报完结与少收的 PostgreSQL 适配器。事务是 READ COMMITTED。
// 只插入。完结在（租户，预报）上至多一行，少收行跟着这一行走。
// 预报锁在应用层，并且先于这一笔插入，不在这里拿。
type PreAdviceClosures struct {
	db *bentopg.DB
}

func NewPreAdviceClosures(db *bentopg.DB) (*PreAdviceClosures, error) {
	if db == nil {
		return nil, fmt.Errorf("warehouse fulfillment postgres: db is nil")
	}
	return &PreAdviceClosures{db: db}, nil
}

var _ ports.PreAdviceClosureStore = (*PreAdviceClosures)(nil)

func (store *PreAdviceClosures) Save(ctx context.Context, closure domain.PreAdviceClosure) (bool, error) {
	const operation = "save pre-advice closure"
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("%s: %w", operation, err)
	}
	inserted, err := insertClosure(ctx, executor, closure)
	if err != nil {
		return false, fmt.Errorf("%s: %w", operation, err)
	}
	return inserted, nil
}

func (store *PreAdviceClosures) ListByPreAdvices(
	ctx context.Context,
	tenant domain.TenantID,
	ids []domain.PreAdviceID,
) ([]domain.PreAdviceClosure, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	keys := make([]string, len(ids))
	for index, id := range ids {
		keys[index] = id.String()
	}
	const operation = "list pre-advice closures"
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	closures, err := loadClosures(ctx, querier, tenant.String(), keys)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	return closures, nil
}

func insertClosure(ctx context.Context, executor bentopg.Executor, closure domain.PreAdviceClosure) (bool, error) {
	var id string
	err := executor.QueryRow(ctx,
		`INSERT INTO warehouse_fulfillment.inbound_pre_advice_closure (
		    tenant_id, pre_advice_id, operator_id, note, closed_at
		 ) VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT ON CONSTRAINT inbound_pre_advice_closure_pkey DO NOTHING
		 RETURNING pre_advice_id`,
		closure.Tenant().String(), closure.PreAdvice().String(), closure.Operator().String(),
		nullableText(closure.Note().String()), closure.ClosedAt(),
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	discrepancy, hasDiscrepancy := closure.Discrepancy()
	for index, line := range closure.Shortages() {
		if !hasDiscrepancy {
			return false, fmt.Errorf("shortage has no discrepancy id")
		}
		if _, err := executor.Exec(ctx,
			`INSERT INTO warehouse_fulfillment.inbound_pre_advice_shortage (
			    tenant_id, pre_advice_id, line_index, goods_code, carton_number, kind, quantity, discrepancy_id
			 ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			closure.Tenant().String(), closure.PreAdvice().String(), index,
			nullableText(line.Goods().String()), nullableText(line.Carton().String()),
			line.Kind(), line.Quantity(), discrepancy.String(),
		); err != nil {
			return false, err
		}
	}
	return true, nil
}

func loadClosures(ctx context.Context, querier bentopg.Querier, tenant string, ids []string) ([]domain.PreAdviceClosure, error) {
	rows, err := querier.Query(ctx,
		`SELECT pre_advice_id, operator_id, note, closed_at
		   FROM warehouse_fulfillment.inbound_pre_advice_closure
		  WHERE tenant_id = $1 AND pre_advice_id = ANY($2)
		  ORDER BY pre_advice_id`, tenant, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type header struct {
		preAdvice string
		operator  string
		note      *string
		closedAt  time.Time
	}
	var headers []header
	for rows.Next() {
		var row header
		if err := rows.Scan(&row.preAdvice, &row.operator, &row.note, &row.closedAt); err != nil {
			return nil, err
		}
		headers = append(headers, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	lines, discrepancies, err := loadShortageLines(ctx, querier, tenant, ids)
	if err != nil {
		return nil, err
	}
	closures := make([]domain.PreAdviceClosure, 0, len(headers))
	for _, row := range headers {
		preAdvice, err := domain.NewPreAdviceID(row.preAdvice)
		if err != nil {
			return nil, err
		}
		operator, err := domain.NewRegistrant(row.operator)
		if err != nil {
			return nil, err
		}
		var note domain.ClosureNote
		if row.note != nil {
			note, err = domain.NewClosureNote(*row.note)
			if err != nil {
				return nil, err
			}
		}
		tenantID, err := domain.NewTenantID(tenant)
		if err != nil {
			return nil, err
		}
		closure, err := domain.NewPreAdviceClosure(
			tenantID, preAdvice, operator, note, row.closedAt, lines[row.preAdvice], discrepancies[row.preAdvice],
		)
		if err != nil {
			return nil, err
		}
		closures = append(closures, closure)
	}
	return closures, nil
}

func loadShortageLines(
	ctx context.Context,
	querier bentopg.Querier,
	tenant string,
	ids []string,
) (map[string][]domain.PreAdviceShortage, map[string]domain.ReceiptDiscrepancyID, error) {
	rows, err := querier.Query(ctx,
		`SELECT pre_advice_id, line_index, goods_code, carton_number, kind, quantity, discrepancy_id
		   FROM warehouse_fulfillment.inbound_pre_advice_shortage
		  WHERE tenant_id = $1 AND pre_advice_id = ANY($2)
		  ORDER BY pre_advice_id, line_index`, tenant, ids)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	grouped := map[string][]domain.PreAdviceShortage{}
	discrepancies := map[string]domain.ReceiptDiscrepancyID{}
	expect := map[string]int{}
	for rows.Next() {
		var preAdvice, kind, discrepancyID string
		var goodsCode, cartonNumber *string
		var index int
		var quantity int64
		if err := rows.Scan(&preAdvice, &index, &goodsCode, &cartonNumber, &kind, &quantity, &discrepancyID); err != nil {
			return nil, nil, err
		}
		if index != expect[preAdvice] {
			return nil, nil, fmt.Errorf("pre-advice %s shortage index %d is not contiguous", preAdvice, index)
		}
		expect[preAdvice] = index + 1
		if kind != domain.PreAdviceShortageKind() {
			return nil, nil, fmt.Errorf("pre-advice %s shortage kind is not %s", preAdvice, domain.PreAdviceShortageKind())
		}
		discrepancy, err := domain.NewReceiptDiscrepancyID(discrepancyID)
		if err != nil {
			return nil, nil, err
		}
		if known, ok := discrepancies[preAdvice]; ok && known != discrepancy {
			return nil, nil, fmt.Errorf("pre-advice %s shortage lines do not share one discrepancy id", preAdvice)
		}
		discrepancies[preAdvice] = discrepancy
		line, err := shortageFromColumns(goodsCode, cartonNumber, quantity)
		if err != nil {
			return nil, nil, err
		}
		grouped[preAdvice] = append(grouped[preAdvice], line)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return grouped, discrepancies, nil
}

func shortageFromColumns(goodsCode, cartonNumber *string, quantity int64) (domain.PreAdviceShortage, error) {
	switch {
	case goodsCode != nil && cartonNumber == nil:
		goods, err := domain.NewGoodsCode(*goodsCode)
		if err != nil {
			return domain.PreAdviceShortage{}, err
		}
		return domain.NewPreAdviceShortage(goods, quantity)
	case goodsCode == nil && cartonNumber != nil:
		if quantity != 1 {
			return domain.PreAdviceShortage{}, fmt.Errorf("carton shortage quantity")
		}
		carton, err := domain.NewCartonNumber(*cartonNumber)
		if err != nil {
			return domain.PreAdviceShortage{}, err
		}
		return domain.NewCartonPreAdviceShortage(carton)
	default:
		return domain.PreAdviceShortage{}, fmt.Errorf("shortage line subject")
	}
}
