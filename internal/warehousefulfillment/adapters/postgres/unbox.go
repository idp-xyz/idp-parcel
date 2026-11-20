package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

func (store *Instructions) HoldUnbox(ctx context.Context, tenant domain.TenantID, factID domain.InternalJobFactID) error {
	return store.hold(ctx, "hold unbox", "warehouse_fulfillment.unbox_fact", tenant.String(), factID.String())
}

func (store *Instructions) LoadUnbox(ctx context.Context, tenant domain.TenantID, factID domain.InternalJobFactID) (ports.UnboxFact, bool, error) {
	const operation = "load unbox"
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.UnboxFact{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	fact, found, err := scanUnbox(querier.QueryRow(ctx,
		`SELECT tenant_id, fact_id, owner_account_id, warehouse_code, carton_number, device_id, operator_id,
		        occurred_at, device_sequence, received_at, movement_id
		   FROM warehouse_fulfillment.unbox_fact
		  WHERE tenant_id = $1 AND fact_id = $2`, tenant.String(), factID.String()))
	if err != nil {
		return ports.UnboxFact{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	if !found {
		return ports.UnboxFact{}, false, nil
	}
	lines, err := loadUnboxLines(ctx, querier, tenant, factID)
	if err != nil {
		return ports.UnboxFact{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	differences, err := loadUnboxDifferences(ctx, querier, tenant, factID)
	if err != nil {
		return ports.UnboxFact{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	fact.Lines = lines
	fact.Differences = differences
	return fact, true, nil
}

func (store *Instructions) SaveUnbox(ctx context.Context, fact ports.UnboxFact) error {
	const operation = "save unbox"
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.unbox_fact (
		    tenant_id, fact_id, owner_account_id, warehouse_code, carton_number, device_id, operator_id,
		    occurred_at, device_sequence, received_at, movement_id
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		fact.Tenant.String(), fact.FactID.String(), fact.Owner.String(), fact.Warehouse.String(), fact.Carton.String(),
		fact.Device.String(), fact.Operator.String(), fact.OccurredAt, fact.DeviceSequence, fact.ReceivedAt,
		nullableText(fact.MovementID.String()),
	); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	for position, line := range fact.Lines {
		if _, err := executor.Exec(ctx,
			`INSERT INTO warehouse_fulfillment.unbox_line (
			    tenant_id, fact_id, position, observed_text, goods_code, unit_name, stock_state, label_alias,
			    batch_number, expiry_date, location_code, quantity
			 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
			fact.Tenant.String(), fact.FactID.String(), position, nullableText(line.Observed), nullableText(line.Goods.String()),
			nullableText(line.Unit.String()), nullableText(line.State.String()), nullableText(line.Alias.String()),
			nullableText(line.Batch.String()), nullableDate(line.Expiry), nullableText(line.Location.String()), line.Quantity,
		); err != nil {
			return fmt.Errorf("%s: %w", operation, err)
		}
	}
	for position, difference := range fact.Differences {
		if _, err := executor.Exec(ctx,
			`INSERT INTO warehouse_fulfillment.unbox_difference (
			    tenant_id, fact_id, position, kind, quantity, goods_code, observed_text
			 ) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			fact.Tenant.String(), fact.FactID.String(), position, difference.Kind.String(), difference.Quantity,
			nullableText(difference.Goods.String()), nullableText(difference.Observed),
		); err != nil {
			return fmt.Errorf("%s: %w", operation, err)
		}
	}
	return nil
}

func (store *Instructions) CartonBill(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode, carton domain.CartonNumber) ([]domain.CartonContentLine, error) {
	const operation = "load carton bill"
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
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
		tenant.String(), owner.String(), warehouse.String(), carton.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	defer rows.Close()
	var lines []domain.CartonContentLine
	for rows.Next() {
		var goods string
		var quantity int64
		if err := rows.Scan(&goods, &quantity); err != nil {
			return nil, fmt.Errorf("%s: %w", operation, err)
		}
		code, err := domain.NewGoodsCode(goods)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", operation, err)
		}
		line, err := domain.NewCartonContentLine(code, quantity)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", operation, err)
		}
		lines = append(lines, line)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	return lines, nil
}

func scanUnbox(row pgx.Row) (ports.UnboxFact, bool, error) {
	var (
		rawTenant, rawFact, rawOwner, rawWarehouse, rawCarton, rawDevice, rawOperator string
		occurred, received                                                            time.Time
		sequence                                                                      int64
		movement                                                                      *string
	)
	err := row.Scan(&rawTenant, &rawFact, &rawOwner, &rawWarehouse, &rawCarton, &rawDevice, &rawOperator, &occurred, &sequence, &received, &movement)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.UnboxFact{}, false, nil
	}
	if err != nil {
		return ports.UnboxFact{}, false, err
	}
	tenant, err := domain.NewTenantID(rawTenant)
	if err != nil {
		return ports.UnboxFact{}, false, err
	}
	fact, err := domain.NewInternalJobFactID(rawFact)
	if err != nil {
		return ports.UnboxFact{}, false, err
	}
	owner, err := domain.NewOwnerAccountID(rawOwner)
	if err != nil {
		return ports.UnboxFact{}, false, err
	}
	warehouse, err := domain.NewWarehouseCode(rawWarehouse)
	if err != nil {
		return ports.UnboxFact{}, false, err
	}
	carton, err := domain.NewCartonNumber(rawCarton)
	if err != nil {
		return ports.UnboxFact{}, false, err
	}
	device, err := domain.NewReceiptDeviceID(rawDevice)
	if err != nil {
		return ports.UnboxFact{}, false, err
	}
	operator, err := domain.NewReceiptOperatorID(rawOperator)
	if err != nil {
		return ports.UnboxFact{}, false, err
	}
	var movementID domain.StockMovementID
	if movement != nil {
		movementID, err = domain.NewStockMovementID(*movement)
		if err != nil {
			return ports.UnboxFact{}, false, err
		}
	}
	return ports.UnboxFact{
		Tenant: tenant, FactID: fact, Owner: owner, Warehouse: warehouse, Carton: carton, Device: device, Operator: operator,
		OccurredAt: occurred, DeviceSequence: sequence, ReceivedAt: received, MovementID: movementID,
	}, true, nil
}

func loadUnboxLines(ctx context.Context, querier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, tenant domain.TenantID, factID domain.InternalJobFactID) ([]ports.UnboxLine, error) {
	rows, err := querier.Query(ctx,
		`SELECT observed_text, goods_code, unit_name, stock_state, label_alias, batch_number, expiry_date::text, location_code, quantity
		   FROM warehouse_fulfillment.unbox_line
		  WHERE tenant_id = $1 AND fact_id = $2
		  ORDER BY position`, tenant.String(), factID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var lines []ports.UnboxLine
	for rows.Next() {
		var observed, goods, unit, state, alias, batch, expiry, location *string
		var quantity int64
		if err := rows.Scan(&observed, &goods, &unit, &state, &alias, &batch, &expiry, &location, &quantity); err != nil {
			return nil, err
		}
		parsedExpiry, err := mustExpiryText(expiry)
		if err != nil {
			return nil, err
		}
		var parsedState domain.StockState
		if state != nil {
			parsedState, err = domain.NewStockState(*state)
			if err != nil {
				return nil, err
			}
		}
		lines = append(lines, ports.UnboxLine{
			Observed: valueOrEmpty(observed), Goods: mustGoods(goods), Unit: mustUnit(unit), State: parsedState,
			Alias: mustAlias(alias), Batch: mustBatch(batch), Expiry: parsedExpiry, Location: mustLocation(location), Quantity: quantity,
		})
	}
	return lines, rows.Err()
}

func loadUnboxDifferences(ctx context.Context, querier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, tenant domain.TenantID, factID domain.InternalJobFactID) ([]ports.UnboxDifference, error) {
	rows, err := querier.Query(ctx,
		`SELECT kind, quantity, goods_code, observed_text
		   FROM warehouse_fulfillment.unbox_difference
		  WHERE tenant_id = $1 AND fact_id = $2
		  ORDER BY position`, tenant.String(), factID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var differences []ports.UnboxDifference
	for rows.Next() {
		var kind string
		var quantity int64
		var goods, observed *string
		if err := rows.Scan(&kind, &quantity, &goods, &observed); err != nil {
			return nil, err
		}
		parsed, err := parseUnboxKind(kind)
		if err != nil {
			return nil, err
		}
		differences = append(differences, ports.UnboxDifference{Kind: parsed, Quantity: quantity, Goods: mustGoods(goods), Observed: valueOrEmpty(observed)})
	}
	return differences, rows.Err()
}

func parseUnboxKind(word string) (domain.UnboxDifferenceKind, error) {
	for _, kind := range []domain.UnboxDifferenceKind{
		domain.UnboxDifferenceShort, domain.UnboxDifferenceOverage, domain.UnboxDifferenceDamage, domain.UnboxDifferenceWrong,
	} {
		if kind.String() == word {
			return kind, nil
		}
	}
	return domain.UnboxDifferenceKindInvalid, fmt.Errorf("unknown unbox difference %q", word)
}
