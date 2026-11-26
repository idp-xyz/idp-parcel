package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// StockReads 从流水现算在库，并读出还冻着的占用。
type StockReads struct {
	db *bentopg.DB
}

func NewStockReads(db *bentopg.DB) (*StockReads, error) {
	if db == nil {
		return nil, fmt.Errorf("warehouse fulfillment postgres: db is nil")
	}
	return &StockReads{db: db}, nil
}

var _ ports.StockReadStore = (*StockReads)(nil)

func (store *StockReads) ListCellBalances(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode) ([]ports.StockCellBalance, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list cell balances: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT line.warehouse_code, line.owner_account_id, line.location_code, line.goods_code, line.unit_name, line.stock_state,
		        line.label_alias, line.inbound_batch_id, line.batch_number, line.expiry_date::text, batch.inbound_at,
		        SUM(line.quantity_delta)::bigint
		   FROM warehouse_fulfillment.stock_movement_line line
		   JOIN warehouse_fulfillment.inbound_batch batch
		     ON batch.tenant_id = line.tenant_id AND batch.inbound_batch_id = line.inbound_batch_id
		  WHERE line.tenant_id = $1 AND line.owner_account_id = $2
		    AND ($3 = '' OR line.warehouse_code = $3)
		  GROUP BY line.warehouse_code, line.owner_account_id, line.location_code, line.goods_code, line.unit_name, line.stock_state,
		           line.label_alias, line.inbound_batch_id, line.batch_number, line.expiry_date, batch.inbound_at
		 HAVING SUM(line.quantity_delta) > 0
		  ORDER BY line.warehouse_code, line.location_code, line.goods_code, line.inbound_batch_id`,
		tenant.String(), owner.String(), warehouse.String())
	if err != nil {
		return nil, fmt.Errorf("list cell balances: %w", err)
	}
	defer rows.Close()
	var listed []ports.StockCellBalance
	for rows.Next() {
		var row ports.StockCellBalance
		if err := rows.Scan(
			&row.Warehouse, &row.Owner, &row.Location, &row.Goods, &row.Unit, &row.State, &row.Alias, &row.InboundBatch,
			&row.BatchNumber, &row.Expiry, &row.InboundAt, &row.Quantity,
		); err != nil {
			return nil, fmt.Errorf("list cell balances: %w", err)
		}
		listed = append(listed, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list cell balances: %w", err)
	}
	return listed, nil
}

func (store *StockReads) ListCartonBalances(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode) ([]ports.StockCartonBalance, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list carton balances: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT line.warehouse_code, line.location_code, line.carton_number, line.inbound_batch_id, batch.inbound_at
		   FROM warehouse_fulfillment.carton_movement_line line
		   JOIN warehouse_fulfillment.carton_inbound_batch batch
		     ON batch.tenant_id = line.tenant_id AND batch.inbound_batch_id = line.inbound_batch_id
		  WHERE line.tenant_id = $1 AND line.owner_account_id = $2
		    AND ($3 = '' OR line.warehouse_code = $3)
		  GROUP BY line.warehouse_code, line.location_code, line.carton_number, line.inbound_batch_id, batch.inbound_at
		 HAVING SUM(line.quantity_delta) > 0
		  ORDER BY line.carton_number`,
		tenant.String(), owner.String(), warehouse.String())
	if err != nil {
		return nil, fmt.Errorf("list carton balances: %w", err)
	}
	defer rows.Close()
	var listed []ports.StockCartonBalance
	for rows.Next() {
		var row ports.StockCartonBalance
		if err := rows.Scan(&row.Warehouse, &row.Location, &row.Carton, &row.InboundBatch, &row.InboundAt); err != nil {
			return nil, fmt.Errorf("list carton balances: %w", err)
		}
		listed = append(listed, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list carton balances: %w", err)
	}
	return listed, nil
}

func (store *StockReads) ListActiveFreezes(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode) ([]ports.StockFreezeRow, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list freezes: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT warehouse_code, target, location_code, goods_code, unit_name, stock_state, carton_number,
		        inbound_batch_id, quantity, reason, operator_id, frozen_at
		   FROM warehouse_fulfillment.stock_freeze
		  WHERE tenant_id = $1 AND owner_account_id = $2 AND status = 'FROZEN'
		    AND ($3 = '' OR warehouse_code = $3)
		  ORDER BY frozen_at, freeze_id`,
		tenant.String(), owner.String(), warehouse.String())
	if err != nil {
		return nil, fmt.Errorf("list freezes: %w", err)
	}
	defer rows.Close()
	var listed []ports.StockFreezeRow
	for rows.Next() {
		var row ports.StockFreezeRow
		var location, goods, unit, state, carton, batch *string
		if err := rows.Scan(
			&row.Warehouse, &row.Target, &location, &goods, &unit, &state, &carton, &batch,
			&row.Quantity, &row.Reason, &row.Operator, &row.FrozenAt,
		); err != nil {
			return nil, fmt.Errorf("list freezes: %w", err)
		}
		row.Location = valueOrEmpty(location)
		row.Goods = valueOrEmpty(goods)
		row.Unit = valueOrEmpty(unit)
		row.State = valueOrEmpty(state)
		row.Carton = valueOrEmpty(carton)
		row.InboundBatch = valueOrEmpty(batch)
		listed = append(listed, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list freezes: %w", err)
	}
	return listed, nil
}

// q 那一格是共用件要求的落列：它不接受一册没有检索列。库存读口不收 q（见 ports.StockMovementPages），这一列用不到。
var stockMovementListBinding = ports.StockMovementPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"postedAt": "posted_at"},
	Identity: []string{"movement_id"},
	Keyword:  []string{"reason"},
})

func (store *StockReads) ListMovements(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode, limit int, query cataloguepage.Query) (cataloguepage.Result[ports.StockMovementRow], error) {
	fail := func(err error) (cataloguepage.Result[ports.StockMovementRow], error) {
		return cataloguepage.Result[ports.StockMovementRow]{}, fmt.Errorf("list movements: %w", err)
	}
	const source = "warehouse_fulfillment.stock_movement"
	plan, err := stockMovementListBinding.Plan(query,
		"tenant_id = $1 AND owner_account_id = $2 AND ($3 = '' OR warehouse_code = $3)",
		[]any{tenant.String(), owner.String(), warehouse.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, plan.CountSQL(source), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	headers, err := querier.Query(ctx,
		plan.PageSQL("movement_id, warehouse_code, reason, source_reference, occurred_at, posted_at", source), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	var fetched []ports.StockMovementRow
	for headers.Next() {
		var row ports.StockMovementRow
		if err := headers.Scan(&row.ID, &row.Warehouse, &row.Reason, &row.Source, &row.Occurred, &row.Posted); err != nil {
			headers.Close()
			return fail(err)
		}
		fetched = append(fetched, row)
	}
	headers.Close()
	if err := headers.Err(); err != nil {
		return fail(err)
	}
	// 位置取扫回来的 posted_at 与 movement_id，就是 ORDER BY 用的那两列。
	page, err := cataloguepage.Finish(query, fetched, limit, total, func(row ports.StockMovementRow, _ string) cataloguepage.Position {
		return cataloguepage.Position{Value: cataloguepage.FormatInstant(row.Posted), Identity: []string{row.ID}}
	})
	if err != nil {
		return fail(err)
	}
	if len(page.Rows) == 0 {
		return page, nil
	}
	// 行只取本页的：多取来判有没有下一页的那一笔已经切掉了。
	ids := make([]string, 0, len(page.Rows))
	for _, row := range page.Rows {
		ids = append(ids, row.ID)
	}
	lines, err := querier.Query(ctx,
		`SELECT movement_id, location_code, goods_code, unit_name, stock_state, quantity_delta
		   FROM warehouse_fulfillment.stock_movement_line
		  WHERE tenant_id = $1 AND movement_id = ANY($2)
		  ORDER BY movement_id, position`,
		tenant.String(), ids)
	if err != nil {
		return fail(err)
	}
	defer lines.Close()
	byID := map[string][]ports.StockMovementLineRow{}
	for lines.Next() {
		var id string
		var line ports.StockMovementLineRow
		if err := lines.Scan(&id, &line.Location, &line.Goods, &line.Unit, &line.State, &line.Delta); err != nil {
			return fail(err)
		}
		byID[id] = append(byID[id], line)
	}
	if err := lines.Err(); err != nil {
		return fail(err)
	}
	for index := range page.Rows {
		page.Rows[index].Lines = byID[page.Rows[index].ID]
	}
	return page, nil
}

func (store *StockReads) CartonBill(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode, carton domain.CartonNumber) ([]domain.CartonContentLine, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("carton bill: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT line.goods_code, line.quantity
		   FROM warehouse_fulfillment.inbound_pre_advice_carton carton
		   JOIN warehouse_fulfillment.inbound_pre_advice advice
		     ON advice.tenant_id = carton.tenant_id AND advice.pre_advice_id = carton.pre_advice_id
		   JOIN warehouse_fulfillment.inbound_pre_advice_carton_line line
		     ON line.tenant_id = carton.tenant_id AND line.pre_advice_id = carton.pre_advice_id AND line.carton_index = carton.carton_index
		  WHERE carton.tenant_id = $1 AND advice.owner_account_id = $2 AND advice.warehouse_code = $3 AND carton.carton_number = $4
		  ORDER BY line.line_index`,
		tenant.String(), owner.String(), warehouse.String(), carton.String())
	if err != nil {
		return nil, fmt.Errorf("carton bill: %w", err)
	}
	defer rows.Close()
	var bill []domain.CartonContentLine
	for rows.Next() {
		var goods string
		var quantity int64
		if err := rows.Scan(&goods, &quantity); err != nil {
			return nil, fmt.Errorf("carton bill: %w", err)
		}
		code, err := domain.NewGoodsCode(goods)
		if err != nil {
			return nil, fmt.Errorf("carton bill: %w", err)
		}
		line, err := domain.NewCartonContentLine(code, quantity)
		if err != nil {
			return nil, fmt.Errorf("carton bill: %w", err)
		}
		bill = append(bill, line)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("carton bill: %w", err)
	}
	return bill, nil
}
