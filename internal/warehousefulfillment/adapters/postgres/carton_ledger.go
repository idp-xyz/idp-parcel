package postgres

import (
	"context"
	"errors"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// CartonLedger 是整箱账的 PostgreSQL 适配器。余额不存，由流水行现算。事务是 READ COMMITTED。
// 已入账的行没有更新语句，也没有删除语句。
type CartonLedger struct {
	db *bentopg.DB
}

func NewCartonLedger(db *bentopg.DB) (*CartonLedger, error) {
	if db == nil {
		return nil, fmt.Errorf("warehouse fulfillment postgres: db is nil")
	}
	return &CartonLedger{db: db}, nil
}

var (
	_ ports.CartonLedger       = (*CartonLedger)(nil)
	_ ports.CartonOnHandLookup = (*CartonLedger)(nil)
)

// errCartonAlreadyOnHand 是写入时发现要入账的箱号余额已经为正。
// 调用方应当在同一把锁里先判过。走到这里是锁内的判断和写入不一致，整笔回滚。
var errCartonAlreadyOnHand = errors.New("carton ledger: carton number is already on hand")

// Serialize 锁这个货主在这个仓的整箱账。
// 键写明是整箱，不拿商品库存的货主锁：商品修订先拿货主锁再读商品库存，收货先拿预报锁再锁账。
// 整箱若再去拿货主锁，收货与商品修订两边的锁序会交叉成环。没有环境事务时拒绝。
func (ledger *CartonLedger) Serialize(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	warehouse domain.WarehouseCode,
) error {
	const operation = "serialize carton posting"
	executor, err := ledger.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if err := lockCartonPosting(ctx, executor, tenant, owner, warehouse); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

func (ledger *CartonLedger) OnHandCartons(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	warehouse domain.WarehouseCode,
) ([]domain.CartonNumber, error) {
	const operation = "list on-hand cartons"
	querier, err := ledger.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	numbers, err := onHandCartonNumbers(ctx, querier, tenant, owner, warehouse)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	return numbers, nil
}

// CartonOnHand 问这一箱此刻在这个货主这个仓的余额是不是正数。读不动走 error，不折成不在库。
func (ledger *CartonLedger) CartonOnHand(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	warehouse domain.WarehouseCode,
	carton domain.CartonNumber,
) (bool, error) {
	const operation = "check carton on hand"
	querier, err := ledger.db.ReadExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("%s: %w", operation, err)
	}
	var onHand bool
	err = querier.QueryRow(ctx,
		`SELECT COALESCE(SUM(quantity_delta), 0) > 0
		   FROM warehouse_fulfillment.carton_movement_line
		  WHERE tenant_id = $1 AND owner_account_id = $2 AND warehouse_code = $3 AND carton_number = $4`,
		tenant.String(), owner.String(), warehouse.String(), carton.String(),
	).Scan(&onHand)
	if err != nil {
		return false, fmt.Errorf("%s: %w", operation, err)
	}
	return onHand, nil
}

// Append 在锁里再判箱号是否已在库，然后按外键方向写入：流水头、入库批次、行。
// 任一箱已经在库就不写任何行，交回错误，调用方回滚。
func (ledger *CartonLedger) Append(
	ctx context.Context,
	movement domain.CartonMovement,
	formed []domain.CartonInboundBatch,
) error {
	const operation = "append carton movement"
	executor, err := ledger.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if err := lockCartonPosting(ctx, executor, movement.Tenant(), movement.Owner(), movement.Warehouse()); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if err := cartonBatchesMatch(movement, formed); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	onHand, err := onHandCartonNumbers(ctx, executor, movement.Tenant(), movement.Owner(), movement.Warehouse())
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	held := map[string]struct{}{}
	for _, number := range onHand {
		held[number.String()] = struct{}{}
	}
	for _, line := range movement.Lines() {
		if _, exists := held[line.Carton().String()]; exists {
			return fmt.Errorf("%s: %w", operation, errCartonAlreadyOnHand)
		}
	}
	inserted, err := insertCartonMovement(ctx, executor, movement)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if !inserted {
		return fmt.Errorf("%s: source already posted", operation)
	}
	for _, batch := range formed {
		if err := insertCartonInboundBatch(ctx, executor, batch); err != nil {
			return fmt.Errorf("%s: %w", operation, err)
		}
	}
	for position, line := range movement.Lines() {
		if err := insertCartonMovementLine(ctx, executor, movement, position, line); err != nil {
			return fmt.Errorf("%s: %w", operation, err)
		}
	}
	return nil
}

// Locate 回答这一箱此刻在哪。同一箱在两个库位余额都为正，是册不自洽，交 error。
func (ledger *CartonLedger) Locate(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	warehouse domain.WarehouseCode,
	carton domain.CartonNumber,
) (ports.CartonPlace, error) {
	const operation = "locate carton"
	querier, err := ledger.db.ReadExecutor(ctx)
	if err != nil {
		return ports.CartonPlace{}, fmt.Errorf("%s: %w", operation, err)
	}
	return locateCarton(ctx, querier, tenant, owner, warehouse, carton)
}

// AppendRelocation 记一笔整箱移动。箱必须已经在库，移出的库位必须是它此刻所在的库位，不新起入库批次。
func (ledger *CartonLedger) AppendRelocation(ctx context.Context, movement domain.CartonMovement) error {
	const operation = "append carton relocation"
	executor, err := ledger.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if movement.Reason() != domain.CartonMovementInternalMove {
		return fmt.Errorf("%s: reason %s", operation, movement.Reason())
	}
	if err := lockCartonPosting(ctx, executor, movement.Tenant(), movement.Owner(), movement.Warehouse()); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	seen := map[string]int64{}
	for _, line := range movement.Lines() {
		seen[line.Carton().String()] += line.Delta()
		if line.Delta() != -1 {
			continue
		}
		place, err := locateCarton(ctx, executor, movement.Tenant(), movement.Owner(), movement.Warehouse(), line.Carton())
		if err != nil {
			return fmt.Errorf("%s: %w", operation, err)
		}
		if !place.Found || place.Location != line.Location() || place.Batch != line.Batch() {
			return fmt.Errorf("%s: carton %s is not on hand at the source", operation, line.Carton())
		}
	}
	for carton, net := range seen {
		if net != 0 {
			return fmt.Errorf("%s: carton %s does not balance", operation, carton)
		}
	}
	inserted, err := insertCartonMovement(ctx, executor, movement)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if !inserted {
		return fmt.Errorf("%s: source already posted", operation)
	}
	for position, line := range movement.Lines() {
		if err := insertCartonMovementLine(ctx, executor, movement, position, line); err != nil {
			return fmt.Errorf("%s: %w", operation, err)
		}
	}
	return nil
}

func locateCarton(
	ctx context.Context,
	querier bentopg.Querier,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	warehouse domain.WarehouseCode,
	carton domain.CartonNumber,
) (ports.CartonPlace, error) {
	rows, err := querier.Query(ctx,
		`SELECT location_code, inbound_batch_id
		   FROM warehouse_fulfillment.carton_movement_line
		  WHERE tenant_id = $1 AND owner_account_id = $2 AND warehouse_code = $3 AND carton_number = $4
		  GROUP BY location_code, inbound_batch_id
		 HAVING sum(quantity_delta) > 0`,
		tenant.String(), owner.String(), warehouse.String(), carton.String(),
	)
	if err != nil {
		return ports.CartonPlace{}, err
	}
	defer rows.Close()
	var place ports.CartonPlace
	count := 0
	for rows.Next() {
		var location, batch string
		if err := rows.Scan(&location, &batch); err != nil {
			return ports.CartonPlace{}, err
		}
		parsedLocation, err := domain.NewLocationCode(location)
		if err != nil {
			return ports.CartonPlace{}, err
		}
		parsedBatch, err := domain.NewCartonInboundBatchID(batch)
		if err != nil {
			return ports.CartonPlace{}, err
		}
		place = ports.CartonPlace{Location: parsedLocation, Batch: parsedBatch, Found: true}
		count++
	}
	if err := rows.Err(); err != nil {
		return ports.CartonPlace{}, err
	}
	if count > 1 {
		return ports.CartonPlace{}, fmt.Errorf("carton %s is on hand in more than one place", carton)
	}
	return place, nil
}

func lockCartonPosting(
	ctx context.Context,
	executor bentopg.Executor,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	warehouse domain.WarehouseCode,
) error {
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('warehouse_fulfillment.carton.posting'), hashtext($1 || chr(31) || $2 || chr(31) || $3))`,
		tenant.String(), owner.String(), warehouse.String(),
	); err != nil {
		return fmt.Errorf("lock carton posting: %w", err)
	}
	return nil
}

func cartonBatchesMatch(movement domain.CartonMovement, formed []domain.CartonInboundBatch) error {
	byID := map[string]domain.CartonNumber{}
	for _, batch := range formed {
		if batch.Tenant() != movement.Tenant() || batch.Owner() != movement.Owner() || batch.Warehouse() != movement.Warehouse() || batch.MovementID() != movement.ID() {
			return fmt.Errorf("formed carton inbound batch %s does not belong to this movement", batch.ID())
		}
		if _, seen := byID[batch.ID().String()]; seen {
			return fmt.Errorf("carton inbound batch %s is repeated", batch.ID())
		}
		byID[batch.ID().String()] = batch.Carton()
	}
	for _, line := range movement.Lines() {
		carton, found := byID[line.Batch().String()]
		if !found || carton != line.Carton() {
			return fmt.Errorf("carton movement line %s has no matching inbound batch", line.Carton())
		}
		delete(byID, line.Batch().String())
	}
	if len(byID) != 0 {
		return fmt.Errorf("carton inbound batch is not used by a line")
	}
	return nil
}
