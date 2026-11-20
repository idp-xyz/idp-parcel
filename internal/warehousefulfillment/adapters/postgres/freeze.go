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

// Freezes 是库存冻结的 PostgreSQL 适配器。冻结不写库存流水。
type Freezes struct {
	db *bentopg.DB
}

func NewFreezes(db *bentopg.DB) (*Freezes, error) {
	if db == nil {
		return nil, fmt.Errorf("warehouse fulfillment postgres: db is nil")
	}
	return &Freezes{db: db}, nil
}

var _ ports.FreezeStore = (*Freezes)(nil)

func (store *Freezes) HoldDecision(ctx context.Context, tenant domain.TenantID, key domain.RequestKey) error {
	return store.hold(ctx, "hold freeze decision", "warehouse_fulfillment.stock_freeze.decision", tenant.String(), key.String())
}

func (store *Freezes) HoldFreeze(ctx context.Context, tenant domain.TenantID, id domain.FreezeID) error {
	return store.hold(ctx, "hold freeze", "warehouse_fulfillment.stock_freeze", tenant.String(), id.String())
}

func (store *Freezes) hold(ctx context.Context, operation, namespace, tenant, id string) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext($1), hashtext($2 || chr(31) || $3))`,
		namespace, tenant, id,
	); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

func (store *Freezes) LoadByDecision(ctx context.Context, tenant domain.TenantID, key domain.RequestKey) (domain.Freeze, bool, error) {
	return store.loadWhere(ctx, "load freeze by decision", `tenant_id = $1 AND decision_id = $2`, tenant.String(), key.String())
}

func (store *Freezes) Load(ctx context.Context, tenant domain.TenantID, id domain.FreezeID) (domain.Freeze, bool, error) {
	return store.loadWhere(ctx, "load freeze", `tenant_id = $1 AND freeze_id = $2`, tenant.String(), id.String())
}

func (store *Freezes) loadWhere(ctx context.Context, operation, where string, args ...any) (domain.Freeze, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return domain.Freeze{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	row := querier.QueryRow(ctx, `SELECT `+freezeColumns+` FROM warehouse_fulfillment.stock_freeze WHERE `+where, args...)
	freeze, err := scanFreeze(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Freeze{}, false, nil
	}
	if err != nil {
		return domain.Freeze{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return freeze, true, nil
}

const freezeColumns = `tenant_id, freeze_id, decision_id, owner_account_id, warehouse_code, target, location_code, goods_code,
	unit_name, stock_state, label_alias, inbound_batch_id, batch_number, expiry_date::text, carton_number, quantity, reason,
	operator_id, frozen_at, status, unfreeze_operator_id, unfrozen_at, unfreeze_note`

func (store *Freezes) Save(ctx context.Context, freeze domain.Freeze) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save freeze: %w", err)
	}
	var location, goods, unit, state, alias, batchID, batchNumber, expiry, carton any
	if freeze.Target() == domain.FreezeGoods {
		cell := freeze.Cell()
		location = cell.Location().String()
		goods = cell.Goods().String()
		unit = cell.Unit().String()
		state = cell.State().String()
		alias = optionalText(cell.Alias().String(), cell.Alias().String() != "")
		batchID = cell.InboundBatch().String()
		batchNumber = optionalText(cell.BatchNumber().String(), cell.BatchNumber().Given())
		expiry = optionalText(cell.Expiry().String(), cell.Expiry().Given())
	} else {
		carton = freeze.Carton().String()
	}
	_, err = executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.stock_freeze (
		    tenant_id, freeze_id, decision_id, owner_account_id, warehouse_code, target, location_code, goods_code,
		    unit_name, stock_state, label_alias, inbound_batch_id, batch_number, expiry_date, carton_number, quantity,
		    reason, operator_id, frozen_at, status
		 ) VALUES (
		    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14::date, $15, $16, $17, $18, $19, 'FROZEN'
		 )`,
		freeze.Tenant().String(), freeze.ID().String(), freeze.DecisionID().String(), freeze.Owner().String(), freeze.Warehouse().String(),
		freeze.Target().String(), location, goods, unit, state, alias, batchID, batchNumber, expiry, carton, freeze.Quantity(),
		freeze.Reason(), freeze.Operator().String(), freeze.FrozenAt(),
	)
	if err != nil {
		return fmt.Errorf("save freeze: %w", err)
	}
	return nil
}

func (store *Freezes) MarkUnfrozen(ctx context.Context, freeze domain.Freeze) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("unfreeze: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.stock_freeze
		    SET status = 'UNFROZEN', unfreeze_operator_id = $3, unfrozen_at = $4, unfreeze_note = $5
		  WHERE tenant_id = $1 AND freeze_id = $2 AND status = 'FROZEN'`,
		freeze.Tenant().String(), freeze.ID().String(), freeze.UnfrozenBy().String(), freeze.UnfrozenAt(),
		nullableText(freeze.Note()),
	)
	if err != nil {
		return fmt.Errorf("unfreeze: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("unfreeze: freeze %s was not frozen", freeze.ID().String())
	}
	return nil
}

func (store *Freezes) CellFreeze(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode, cell domain.StockCell) (int64, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return 0, fmt.Errorf("cell freeze: %w", err)
	}
	var quantity int64
	err = querier.QueryRow(ctx,
		`SELECT COALESCE(SUM(quantity), 0)::bigint
		   FROM warehouse_fulfillment.stock_freeze
		  WHERE tenant_id = $1 AND owner_account_id = $2 AND warehouse_code = $3 AND status = 'FROZEN' AND target = 'GOODS'
		    AND location_code = $4 AND goods_code = $5 AND unit_name = $6 AND stock_state = $7
		    AND label_alias IS NOT DISTINCT FROM $8 AND inbound_batch_id = $9
		    AND batch_number IS NOT DISTINCT FROM $10 AND expiry_date IS NOT DISTINCT FROM $11::date`,
		tenant.String(), owner.String(), warehouse.String(),
		cell.Location().String(), cell.Goods().String(), cell.Unit().String(), cell.State().String(),
		optionalText(cell.Alias().String(), cell.Alias().String() != ""), cell.InboundBatch().String(),
		optionalText(cell.BatchNumber().String(), cell.BatchNumber().Given()),
		optionalText(cell.Expiry().String(), cell.Expiry().Given()),
	).Scan(&quantity)
	if err != nil {
		return 0, fmt.Errorf("cell freeze: %w", err)
	}
	return quantity, nil
}

func (store *Freezes) SummaryFreeze(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode, goods domain.GoodsCode, unit domain.GoodsUnitName, state domain.StockState) (int64, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return 0, fmt.Errorf("summary freeze: %w", err)
	}
	var quantity int64
	err = querier.QueryRow(ctx,
		`SELECT COALESCE(SUM(quantity), 0)::bigint
		   FROM warehouse_fulfillment.stock_freeze
		  WHERE tenant_id = $1 AND owner_account_id = $2 AND warehouse_code = $3 AND status = 'FROZEN' AND target = 'GOODS'
		    AND goods_code = $4 AND unit_name = $5 AND stock_state = $6`,
		tenant.String(), owner.String(), warehouse.String(), goods.String(), unit.String(), state.String(),
	).Scan(&quantity)
	if err != nil {
		return 0, fmt.Errorf("summary freeze: %w", err)
	}
	return quantity, nil
}

func (store *Freezes) CartonFrozen(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode, carton domain.CartonNumber) (bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("carton freeze: %w", err)
	}
	var frozen bool
	err = querier.QueryRow(ctx,
		`SELECT EXISTS (
		    SELECT 1 FROM warehouse_fulfillment.stock_freeze
		     WHERE tenant_id = $1 AND owner_account_id = $2 AND warehouse_code = $3 AND status = 'FROZEN'
		       AND target = 'CARTON' AND carton_number = $4
		 )`,
		tenant.String(), owner.String(), warehouse.String(), carton.String(),
	).Scan(&frozen)
	if err != nil {
		return false, fmt.Errorf("carton freeze: %w", err)
	}
	return frozen, nil
}

func scanFreeze(row pgx.Row) (domain.Freeze, error) {
	var tenant, id, decision, owner, warehouse, target, reason, operator, status string
	var location, goods, unit, state, alias, batchID, batchNumber, expiry, carton, unfreezeOperator, note *string
	var quantity int64
	var frozenAt time.Time
	var unfrozenAt *time.Time
	err := row.Scan(
		&tenant, &id, &decision, &owner, &warehouse, &target, &location, &goods, &unit, &state, &alias, &batchID,
		&batchNumber, &expiry, &carton, &quantity, &reason, &operator, &frozenAt, &status, &unfreezeOperator, &unfrozenAt, &note,
	)
	if err != nil {
		return domain.Freeze{}, err
	}
	spec := domain.FreezeSpec{
		Tenant: mustTenant(tenant), ID: mustFreezeID(id), DecisionID: mustRequest(&decision), Owner: mustOwnerID(owner),
		Warehouse: mustWarehouse(warehouse), Quantity: quantity, Reason: reason, Operator: mustRegistrant(&operator), FrozenAt: frozenAt,
	}
	if target == "CARTON" {
		spec.Target = domain.FreezeCarton
		spec.Carton = mustCarton(carton)
	} else {
		spec.Target = domain.FreezeGoods
		cell, err := domain.NewStockCell(domain.StockCellSpec{
			Location: mustLocation(location), Goods: mustGoods(goods), Unit: mustUnit(unit), State: mustState(state),
			Alias: mustAlias(alias), InboundBatch: mustInbound(batchID), BatchNumber: mustBatch(batchNumber), Expiry: mustExpiry(expiry),
		})
		if err != nil {
			return domain.Freeze{}, err
		}
		spec.Cell = cell
	}
	freeze, err := domain.NewFreeze(spec)
	if err != nil {
		return domain.Freeze{}, err
	}
	if status == "UNFROZEN" {
		when := frozenAt
		if unfrozenAt != nil {
			when = *unfrozenAt
		}
		return freeze.Unfreeze(mustRegistrant(unfreezeOperator), when, valueOrEmpty(note))
	}
	return freeze, nil
}

func mustFreezeID(value string) domain.FreezeID {
	parsed, _ := domain.NewFreezeID(value)
	return parsed
}

// FreezeHolds 把还冻着的数量交给库内入账。预占与作业锁定还没有账，这两格仍是零。
type FreezeHolds struct {
	freezes *Freezes
}

func NewFreezeHolds(freezes *Freezes) FreezeHolds {
	return FreezeHolds{freezes: freezes}
}

func (holds FreezeHolds) CellHold(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode, cell domain.StockCell) (int64, error) {
	return holds.freezes.CellFreeze(ctx, tenant, owner, warehouse, cell)
}

// LedgerHolds 把冻结、还在盘点中的作业锁定和未分配的预占加在一起。
type LedgerHolds struct {
	Freezes      FreezeHolds
	Counts       *Counts
	Reservations interface {
		UnassignedReservation(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode, domain.GoodsCode, domain.StockState, domain.GoodsAliasCode, domain.BatchNumber, domain.ExpiryDate) (int64, error)
	}
}

func (holds LedgerHolds) CellHold(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode, cell domain.StockCell) (int64, error) {
	frozen, err := holds.Freezes.CellHold(ctx, tenant, owner, warehouse, cell)
	if err != nil {
		return 0, err
	}
	locked, err := holds.Counts.CellLock(ctx, tenant, owner, warehouse, cell)
	if err != nil {
		return 0, err
	}
	return frozen + locked, nil
}

func (holds LedgerHolds) UnassignedReservation(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode, goods domain.GoodsCode, state domain.StockState, alias domain.GoodsAliasCode, batch domain.BatchNumber, expiry domain.ExpiryDate) (int64, error) {
	if holds.Reservations == nil {
		return 0, nil
	}
	return holds.Reservations.UnassignedReservation(ctx, tenant, owner, warehouse, goods, state, alias, batch, expiry)
}

func (holds LedgerHolds) CartonHeld(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode, carton domain.CartonNumber) (bool, error) {
	return holds.Freezes.CartonHeld(ctx, tenant, owner, warehouse, carton)
}

var _ ports.StockHolds = LedgerHolds{}

func (holds FreezeHolds) UnassignedReservation(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode, domain.GoodsCode, domain.StockState, domain.GoodsAliasCode, domain.BatchNumber, domain.ExpiryDate) (int64, error) {
	return 0, nil
}

func (holds FreezeHolds) CartonHeld(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode, carton domain.CartonNumber) (bool, error) {
	return holds.freezes.CartonFrozen(ctx, tenant, owner, warehouse, carton)
}

var _ ports.StockHolds = FreezeHolds{}
