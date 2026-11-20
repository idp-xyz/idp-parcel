package postgres

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

func insertStockMovement(ctx context.Context, executor bentopg.Executor, movement domain.StockMovement) (bool, error) {
	var inserted string
	err := executor.QueryRow(ctx,
		`INSERT INTO warehouse_fulfillment.stock_movement (
		    tenant_id, movement_id, owner_account_id, warehouse_code, reason, source_reference, occurred_at, posted_at
		 ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (tenant_id, reason, source_reference) DO NOTHING
		 RETURNING movement_id`,
		movement.Tenant().String(), movement.ID().String(), movement.Owner().String(), movement.Warehouse().String(),
		movement.Reason().String(), movement.Source().String(), movement.OccurredAt(), movement.PostedAt(),
	).Scan(&inserted)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func insertInboundBatch(ctx context.Context, executor bentopg.Executor, batch domain.InboundBatch) error {
	_, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.inbound_batch (
		    tenant_id, inbound_batch_id, owner_account_id, warehouse_code, goods_code,
		    batch_number, expiry_date, inbound_at, formed_by_movement_id
		 ) VALUES ($1, $2, $3, $4, $5, $6, $7::date, $8, $9)`,
		batch.Tenant().String(), batch.ID().String(), batch.Owner().String(), batch.Warehouse().String(), batch.Goods().String(),
		optionalText(batch.BatchNumber().String(), batch.BatchNumber().Given()),
		optionalText(batch.Expiry().String(), batch.Expiry().Given()),
		batch.InboundAt(), batch.MovementID().String(),
	)
	return err
}

func insertStockMovementLine(
	ctx context.Context,
	executor bentopg.Executor,
	movement domain.StockMovement,
	position int,
	line domain.StockMovementLine,
) error {
	delta := line.Quantity()
	if line.Direction() == domain.StockOut {
		delta = -line.Quantity()
	}
	var declared any
	if !line.DeclaredInboundAt().IsZero() {
		declared = line.DeclaredInboundAt()
	}
	_, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.stock_movement_line (
		    tenant_id, movement_id, position, owner_account_id, warehouse_code,
		    location_code, goods_code, unit_name, stock_state, label_alias, inbound_batch_id,
		    batch_number, expiry_date, quantity_delta, declared_inbound_at, formed_inbound_batch
		 ) VALUES (
		    $1, $2, $3, $4, $5,
		    $6, $7, $8, $9, $10, $11,
		    $12, $13::date, $14, $15, $16
		 )`,
		movement.Tenant().String(), movement.ID().String(), position, movement.Owner().String(), movement.Warehouse().String(),
		line.Cell().Location().String(), line.Cell().Goods().String(), line.Cell().Unit().String(), line.Cell().State().String(),
		optionalText(line.Cell().Alias().String(), line.Cell().Alias().String() != ""),
		line.Cell().InboundBatch().String(),
		optionalText(line.Cell().BatchNumber().String(), line.Cell().BatchNumber().Given()),
		optionalText(line.Cell().Expiry().String(), line.Cell().Expiry().Given()),
		delta, declared, line.FormedInboundBatch(),
	)
	return err
}

func optionalText(value string, given bool) any {
	if !given {
		return nil
	}
	return value
}

func loadStockMovement(
	ctx context.Context,
	querier bentopg.Querier,
	tenant domain.TenantID,
	reason domain.StockMovementReason,
	source domain.SourceFactReference,
) (domain.StockMovement, bool, error) {
	var (
		movementID string
		owner      string
		warehouse  string
		occurredAt time.Time
		postedAt   time.Time
	)
	err := querier.QueryRow(ctx,
		`SELECT movement_id, owner_account_id, warehouse_code, occurred_at, posted_at
		   FROM warehouse_fulfillment.stock_movement
		  WHERE tenant_id = $1 AND reason = $2 AND source_reference = $3`,
		tenant.String(), reason.String(), source.String(),
	).Scan(&movementID, &owner, &warehouse, &occurredAt, &postedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.StockMovement{}, false, nil
	}
	if err != nil {
		return domain.StockMovement{}, false, err
	}
	lines, err := loadStockMovementLines(ctx, querier, tenant.String(), movementID)
	if err != nil {
		return domain.StockMovement{}, false, err
	}
	id, err := domain.NewStockMovementID(movementID)
	if err != nil {
		return domain.StockMovement{}, false, err
	}
	ownerID, err := domain.NewOwnerAccountID(owner)
	if err != nil {
		return domain.StockMovement{}, false, err
	}
	warehouseCode, err := domain.NewWarehouseCode(warehouse)
	if err != nil {
		return domain.StockMovement{}, false, err
	}
	movement, err := domain.NewStockMovement(tenant, id, ownerID, warehouseCode, reason, source, occurredAt, postedAt, lines)
	if err != nil {
		return domain.StockMovement{}, false, err
	}
	return movement, true, nil
}

func loadStockMovementLines(ctx context.Context, querier bentopg.Querier, tenant, movementID string) ([]domain.StockMovementLine, error) {
	rows, err := querier.Query(ctx,
		`SELECT location_code, goods_code, unit_name, stock_state, label_alias, inbound_batch_id,
		        batch_number, expiry_date::text, quantity_delta, declared_inbound_at, formed_inbound_batch
		   FROM warehouse_fulfillment.stock_movement_line
		  WHERE tenant_id = $1 AND movement_id = $2
		  ORDER BY position`,
		tenant, movementID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var lines []domain.StockMovementLine
	for rows.Next() {
		line, err := scanStockMovementLine(rows)
		if err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

type lineScanner interface {
	Scan(dest ...any) error
}

func scanStockMovementLine(scanner lineScanner) (domain.StockMovementLine, error) {
	var (
		location    string
		goods       string
		unit        string
		stateWord   string
		alias       *string
		batchID     string
		batchNumber *string
		expiry      *string
		delta       int64
		declared    *time.Time
		formed      bool
	)
	if err := scanner.Scan(&location, &goods, &unit, &stateWord, &alias, &batchID, &batchNumber, &expiry, &delta, &declared, &formed); err != nil {
		return domain.StockMovementLine{}, err
	}
	if delta == 0 || delta == math.MinInt64 {
		return domain.StockMovementLine{}, fmt.Errorf("stock movement line quantity %d", delta)
	}
	direction := domain.StockIn
	quantity := delta
	if delta < 0 {
		direction = domain.StockOut
		quantity = -delta
	}
	var declaredAt time.Time
	if declared != nil {
		declaredAt = *declared
	}
	cell, err := stockCellFromColumns(location, goods, unit, stateWord, alias, batchID, batchNumber, expiry)
	if err != nil {
		return domain.StockMovementLine{}, err
	}
	return domain.NewStockMovementLine(direction, quantity, cell, declaredAt, formed), nil
}

func stockCellFromColumns(location, goods, unit, stateWord string, alias *string, batchID string, batchNumber, expiry *string) (domain.StockCell, error) {
	locationCode, err := domain.NewLocationCode(location)
	if err != nil {
		return domain.StockCell{}, err
	}
	goodsCode, err := domain.NewGoodsCode(goods)
	if err != nil {
		return domain.StockCell{}, err
	}
	unitName, err := domain.NewGoodsUnitName(unit)
	if err != nil {
		return domain.StockCell{}, err
	}
	state, err := domain.NewStockState(stateWord)
	if err != nil {
		return domain.StockCell{}, err
	}
	inboundBatch, err := domain.NewInboundBatchID(batchID)
	if err != nil {
		return domain.StockCell{}, err
	}
	aliasCode := domain.GoodsAliasCode{}
	if alias != nil {
		aliasCode, err = domain.NewGoodsAliasCode(*alias)
		if err != nil {
			return domain.StockCell{}, err
		}
	}
	number := domain.BatchNumber{}
	if batchNumber != nil {
		number, err = domain.NewBatchNumber(*batchNumber)
		if err != nil {
			return domain.StockCell{}, err
		}
	}
	date := domain.ExpiryDate{}
	if expiry != nil {
		date, err = domain.NewExpiryDate(*expiry)
		if err != nil {
			return domain.StockCell{}, err
		}
	}
	return domain.NewStockCell(domain.StockCellSpec{
		Location: locationCode, Goods: goodsCode, Unit: unitName, State: state,
		Alias: aliasCode, InboundBatch: inboundBatch, BatchNumber: number, Expiry: date,
	})
}

func loadInboundBatch(ctx context.Context, querier bentopg.Querier, tenant domain.TenantID, id domain.InboundBatchID) (domain.InboundBatch, bool, error) {
	var (
		owner      string
		warehouse  string
		goods      string
		number     *string
		expiry     *string
		inboundAt  time.Time
		movementID string
	)
	err := querier.QueryRow(ctx,
		`SELECT owner_account_id, warehouse_code, goods_code, batch_number, expiry_date::text, inbound_at, formed_by_movement_id
		   FROM warehouse_fulfillment.inbound_batch
		  WHERE tenant_id = $1 AND inbound_batch_id = $2`,
		tenant.String(), id.String(),
	).Scan(&owner, &warehouse, &goods, &number, &expiry, &inboundAt, &movementID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.InboundBatch{}, false, nil
	}
	if err != nil {
		return domain.InboundBatch{}, false, err
	}
	ownerID, err := domain.NewOwnerAccountID(owner)
	if err != nil {
		return domain.InboundBatch{}, false, err
	}
	warehouseCode, err := domain.NewWarehouseCode(warehouse)
	if err != nil {
		return domain.InboundBatch{}, false, err
	}
	goodsCode, err := domain.NewGoodsCode(goods)
	if err != nil {
		return domain.InboundBatch{}, false, err
	}
	formedBy, err := domain.NewStockMovementID(movementID)
	if err != nil {
		return domain.InboundBatch{}, false, err
	}
	lot := domain.BatchNumber{}
	if number != nil {
		lot, err = domain.NewBatchNumber(*number)
		if err != nil {
			return domain.InboundBatch{}, false, err
		}
	}
	date := domain.ExpiryDate{}
	if expiry != nil {
		date, err = domain.NewExpiryDate(*expiry)
		if err != nil {
			return domain.InboundBatch{}, false, err
		}
	}
	batch, err := domain.NewInboundBatch(tenant, id, ownerID, warehouseCode, goodsCode, lot, date, inboundAt, formedBy)
	if err != nil {
		return domain.InboundBatch{}, false, err
	}
	return batch, true, nil
}

func cellBalance(
	ctx context.Context,
	querier bentopg.Querier,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	warehouse domain.WarehouseCode,
	cell domain.StockCell,
) (int64, error) {
	var balance int64
	err := querier.QueryRow(ctx,
		`SELECT COALESCE(SUM(quantity_delta), 0)::bigint
		   FROM warehouse_fulfillment.stock_movement_line
		  WHERE tenant_id = $1 AND owner_account_id = $2 AND warehouse_code = $3
		    AND location_code = $4 AND goods_code = $5 AND unit_name = $6 AND stock_state = $7
		    AND label_alias IS NOT DISTINCT FROM $8
		    AND inbound_batch_id = $9
		    AND batch_number IS NOT DISTINCT FROM $10
		    AND expiry_date IS NOT DISTINCT FROM $11::date`,
		tenant.String(), owner.String(), warehouse.String(),
		cell.Location().String(), cell.Goods().String(), cell.Unit().String(), cell.State().String(),
		optionalText(cell.Alias().String(), cell.Alias().String() != ""),
		cell.InboundBatch().String(),
		optionalText(cell.BatchNumber().String(), cell.BatchNumber().Given()),
		optionalText(cell.Expiry().String(), cell.Expiry().Given()),
	).Scan(&balance)
	return balance, err
}

func listOnHandCells(
	ctx context.Context,
	querier bentopg.Querier,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	warehouse domain.WarehouseCode,
) ([]ports.StockBalanceRow, error) {
	rows, err := querier.Query(ctx,
		`SELECT location_code, goods_code, unit_name, stock_state, label_alias, inbound_batch_id,
		        batch_number, expiry_date::text, SUM(quantity_delta)::bigint
		   FROM warehouse_fulfillment.stock_movement_line
		  WHERE tenant_id = $1 AND owner_account_id = $2 AND warehouse_code = $3
		  GROUP BY location_code, goods_code, unit_name, stock_state, label_alias, inbound_batch_id, batch_number, expiry_date
		 HAVING SUM(quantity_delta) > 0
		  ORDER BY location_code, goods_code, unit_name, stock_state, label_alias NULLS FIRST, inbound_batch_id,
		           batch_number NULLS FIRST, expiry_date NULLS FIRST`,
		tenant.String(), owner.String(), warehouse.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var listed []ports.StockBalanceRow
	for rows.Next() {
		var row ports.StockBalanceRow
		if err := rows.Scan(
			&row.LocationCode, &row.GoodsCode, &row.Unit, &row.State, &row.Alias, &row.InboundBatchID,
			&row.BatchNumber, &row.Expiry, &row.Quantity,
		); err != nil {
			return nil, err
		}
		listed = append(listed, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return listed, nil
}

func goodsOnHand(ctx context.Context, querier bentopg.Querier, tenant domain.TenantID, owner domain.OwnerAccountID, code domain.GoodsCode) (bool, error) {
	var onHand bool
	err := querier.QueryRow(ctx,
		`SELECT EXISTS (
		    SELECT 1
		      FROM warehouse_fulfillment.stock_movement_line
		     WHERE tenant_id = $1 AND owner_account_id = $2 AND goods_code = $3
		     GROUP BY warehouse_code, location_code, unit_name, stock_state, label_alias, inbound_batch_id, batch_number, expiry_date
		    HAVING SUM(quantity_delta) > 0
		 )`,
		tenant.String(), owner.String(), code.String(),
	).Scan(&onHand)
	return onHand, err
}
