package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// Returns 记下退货预报、到仓与处置。
type Returns struct {
	db *bentopg.DB
}

func NewReturns(db *bentopg.DB) (*Returns, error) {
	if db == nil {
		return nil, errors.New("warehouse fulfillment postgres: returns need a db")
	}
	return &Returns{db: db}, nil
}

var _ ports.ReturnStore = (*Returns)(nil)

func (store *Returns) LoadAdviceByKey(ctx context.Context, tenant domain.TenantID, origin domain.ReturnOrigin, key string) (ports.ReturnAdvice, bool, error) {
	executor, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ReturnAdvice{}, false, fmt.Errorf("load return advice: %w", err)
	}
	var advice ports.ReturnAdvice
	var adviceID, owner, warehouse string
	var basis, operator, number, absence *string
	err = executor.QueryRow(ctx,
		`SELECT advice_id, owner_account_id, warehouse_code, tracking_number, basis, operator_id,
		        document_number, numbering_absence, received_at
		   FROM warehouse_fulfillment.return_pre_advice
		  WHERE tenant_id = $1 AND origin = $2 AND request_key = $3`,
		tenant.String(), origin.String(), key).Scan(
		&adviceID, &owner, &warehouse, &advice.Tracking, &basis, &operator, &number, &absence, &advice.Received)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ports.ReturnAdvice{}, false, nil
		}
		return ports.ReturnAdvice{}, false, fmt.Errorf("load return advice: %w", err)
	}
	advice.ID, err = domain.NewReturnAdviceID(adviceID)
	if err != nil {
		return ports.ReturnAdvice{}, false, err
	}
	advice.Owner, err = domain.NewOwnerAccountID(owner)
	if err != nil {
		return ports.ReturnAdvice{}, false, err
	}
	advice.Warehouse, err = domain.NewWarehouseCode(warehouse)
	if err != nil {
		return ports.ReturnAdvice{}, false, err
	}
	advice.Key = key
	advice.Origin = origin
	advice.Basis = valueOrEmpty(basis)
	advice.Operator = valueOrEmpty(operator)
	advice.Number = valueOrEmpty(number)
	advice.Absence = valueOrEmpty(absence)
	rows, err := executor.Query(ctx,
		`SELECT goods_code, unit_name, quantity
		   FROM warehouse_fulfillment.return_pre_advice_line
		  WHERE tenant_id = $1 AND advice_id = $2
		  ORDER BY line_index`, tenant.String(), advice.ID.String())
	if err != nil {
		return ports.ReturnAdvice{}, false, fmt.Errorf("load return advice lines: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var goods, unit string
		var quantity int64
		if err := rows.Scan(&goods, &unit, &quantity); err != nil {
			return ports.ReturnAdvice{}, false, err
		}
		lineGoods, err := domain.NewGoodsCode(goods)
		if err != nil {
			return ports.ReturnAdvice{}, false, err
		}
		lineUnit, err := domain.NewGoodsUnitName(unit)
		if err != nil {
			return ports.ReturnAdvice{}, false, err
		}
		advice.Lines = append(advice.Lines, ports.ReturnLine{Goods: lineGoods, Unit: lineUnit, Quantity: quantity})
	}
	return advice, true, rows.Err()
}

func (store *Returns) SaveAdvice(ctx context.Context, tenant domain.TenantID, advice ports.ReturnAdvice) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save return advice: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.return_pre_advice (
		    tenant_id, advice_id, owner_account_id, warehouse_code, origin, request_key, tracking_number,
		    basis, operator_id, document_number, numbering_absence, received_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		tenant.String(), advice.ID.String(), advice.Owner.String(), advice.Warehouse.String(), advice.Origin.String(),
		advice.Key, advice.Tracking, nullableText(advice.Basis), nullableText(advice.Operator),
		nullableText(advice.Number), nullableText(advice.Absence), advice.Received); err != nil {
		return fmt.Errorf("save return advice: %w", err)
	}
	for index, line := range advice.Lines {
		if _, err := executor.Exec(ctx,
			`INSERT INTO warehouse_fulfillment.return_pre_advice_line (
			    tenant_id, advice_id, line_index, goods_code, unit_name, quantity)
			 VALUES ($1,$2,$3,$4,$5,$6)`,
			tenant.String(), advice.ID.String(), index, line.Goods.String(), line.Unit.String(), line.Quantity); err != nil {
			return fmt.Errorf("save return advice line: %w", err)
		}
	}
	return nil
}

func (store *Returns) FindAdviceByTracking(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, tracking string) (domain.OwnerAccountID, domain.ReturnAdviceID, bool, error) {
	executor, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return domain.OwnerAccountID{}, domain.ReturnAdviceID{}, false, fmt.Errorf("find return advice: %w", err)
	}
	var owner, advice string
	err = executor.QueryRow(ctx,
		`SELECT owner_account_id, advice_id
		   FROM warehouse_fulfillment.return_pre_advice
		  WHERE tenant_id = $1 AND warehouse_code = $2 AND tracking_number = $3`,
		tenant.String(), warehouse.String(), tracking).Scan(&owner, &advice)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.OwnerAccountID{}, domain.ReturnAdviceID{}, false, nil
		}
		return domain.OwnerAccountID{}, domain.ReturnAdviceID{}, false, fmt.Errorf("find return advice: %w", err)
	}
	ownerID, err := domain.NewOwnerAccountID(owner)
	if err != nil {
		return domain.OwnerAccountID{}, domain.ReturnAdviceID{}, false, err
	}
	adviceID, err := domain.NewReturnAdviceID(advice)
	if err != nil {
		return domain.OwnerAccountID{}, domain.ReturnAdviceID{}, false, err
	}
	return ownerID, adviceID, true, nil
}

func (store *Returns) FindOwnerByExternalNumber(ctx context.Context, tenant domain.TenantID, tracking string) (domain.OwnerAccountID, bool, error) {
	executor, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return domain.OwnerAccountID{}, false, fmt.Errorf("find packed owner: %w", err)
	}
	var owner string
	err = executor.QueryRow(ctx,
		`SELECT request.owner_account_id
		   FROM warehouse_fulfillment.outbound_owner_label label
		   JOIN warehouse_fulfillment.outbound_request request
		     ON request.tenant_id = label.tenant_id AND request.request_id = label.request_id
		  WHERE label.tenant_id = $1 AND label.external_number = $2`,
		tenant.String(), tracking).Scan(&owner)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.OwnerAccountID{}, false, nil
		}
		return domain.OwnerAccountID{}, false, fmt.Errorf("find packed owner: %w", err)
	}
	ownerID, err := domain.NewOwnerAccountID(owner)
	if err != nil {
		return domain.OwnerAccountID{}, false, err
	}
	return ownerID, true, nil
}

func (store *Returns) LoadReceipt(ctx context.Context, tenant domain.TenantID, fact domain.ReturnFactID) (ports.ReturnReceipt, bool, error) {
	executor, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ReturnReceipt{}, false, fmt.Errorf("load return receipt: %w", err)
	}
	var receipt ports.ReturnReceipt
	var warehouse, location, goods, unit, state, owner, advice, arrival *string
	err = executor.QueryRow(ctx,
		`SELECT warehouse_code, location_code, tracking_number, owner_account_id, advice_id, arrival_id,
		        goods_code, unit_name, stock_state, quantity, outcome, occurred_at
		   FROM warehouse_fulfillment.return_receipt
		  WHERE tenant_id = $1 AND fact_id = $2`,
		tenant.String(), fact.String()).Scan(
		&warehouse, &location, &receipt.Tracking, &owner, &advice, &arrival, &goods, &unit, &state,
		&receipt.Quantity, &receipt.Outcome, &receipt.Occurred)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ports.ReturnReceipt{}, false, nil
		}
		return ports.ReturnReceipt{}, false, fmt.Errorf("load return receipt: %w", err)
	}
	receipt.Fact = fact
	receipt.Warehouse, err = domain.NewWarehouseCode(valueOrEmpty(warehouse))
	if err != nil {
		return ports.ReturnReceipt{}, false, err
	}
	receipt.Location, err = domain.NewLocationCode(valueOrEmpty(location))
	if err != nil {
		return ports.ReturnReceipt{}, false, err
	}
	receipt.Goods, err = domain.NewGoodsCode(valueOrEmpty(goods))
	if err != nil {
		return ports.ReturnReceipt{}, false, err
	}
	receipt.Unit, err = domain.NewGoodsUnitName(valueOrEmpty(unit))
	if err != nil {
		return ports.ReturnReceipt{}, false, err
	}
	receipt.State, err = domain.NewStockState(valueOrEmpty(state))
	if err != nil {
		return ports.ReturnReceipt{}, false, err
	}
	receipt.Owner = valueOrEmpty(owner)
	receipt.Advice = valueOrEmpty(advice)
	receipt.Arrival = valueOrEmpty(arrival)
	return receipt, true, nil
}

func (store *Returns) SaveReceipt(ctx context.Context, tenant domain.TenantID, receipt ports.ReturnReceipt) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save return receipt: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.return_receipt (
		    tenant_id, fact_id, warehouse_code, location_code, tracking_number, owner_account_id, advice_id,
		    arrival_id, goods_code, unit_name, stock_state, quantity, outcome, occurred_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		tenant.String(), receipt.Fact.String(), receipt.Warehouse.String(), receipt.Location.String(), receipt.Tracking,
		nullableText(receipt.Owner), nullableText(receipt.Advice), nullableText(receipt.Arrival),
		receipt.Goods.String(), receipt.Unit.String(), receipt.State.String(), receipt.Quantity, receipt.Outcome, receipt.Occurred); err != nil {
		return fmt.Errorf("save return receipt: %w", err)
	}
	return nil
}

func (store *Returns) LoadDisposition(ctx context.Context, tenant domain.TenantID, decision domain.ReturnDecisionID) (ports.ReturnDisposition, bool, error) {
	executor, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ReturnDisposition{}, false, fmt.Errorf("load return disposition: %w", err)
	}
	var disposition ports.ReturnDisposition
	var fact, destination, origin string
	var basis, operator *string
	err = executor.QueryRow(ctx,
		`SELECT fact_id, destination, basis, operator_id, origin, decided_at
		   FROM warehouse_fulfillment.return_disposition
		  WHERE tenant_id = $1 AND decision_id = $2`,
		tenant.String(), decision.String()).Scan(&fact, &destination, &basis, &operator, &origin, &disposition.Decided)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ports.ReturnDisposition{}, false, nil
		}
		return ports.ReturnDisposition{}, false, fmt.Errorf("load return disposition: %w", err)
	}
	disposition.Decision = decision
	disposition.Fact, err = domain.NewReturnFactID(fact)
	if err != nil {
		return ports.ReturnDisposition{}, false, err
	}
	disposition.Destination, err = domain.NewReturnDestination(destination)
	if err != nil {
		return ports.ReturnDisposition{}, false, err
	}
	disposition.Origin, err = domain.NewReturnOrigin(origin)
	if err != nil {
		return ports.ReturnDisposition{}, false, err
	}
	disposition.Basis = valueOrEmpty(basis)
	disposition.Operator = valueOrEmpty(operator)
	return disposition, true, nil
}

func (store *Returns) SaveDisposition(ctx context.Context, tenant domain.TenantID, disposition ports.ReturnDisposition) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save return disposition: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.return_disposition (
		    tenant_id, decision_id, fact_id, destination, basis, operator_id, origin, decided_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		tenant.String(), disposition.Decision.String(), disposition.Fact.String(), disposition.Destination.String(),
		nullableText(disposition.Basis), nullableText(disposition.Operator), disposition.Origin.String(), disposition.Decided); err != nil {
		return fmt.Errorf("save return disposition: %w", err)
	}
	return nil
}
