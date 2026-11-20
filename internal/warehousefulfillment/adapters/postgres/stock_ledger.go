package postgres

import (
	"context"
	"errors"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// StockLedger 是库存账的 PostgreSQL 适配器。余额不存，由流水行现算。事务是 READ COMMITTED。
type StockLedger struct {
	db *bentopg.DB
}

func NewStockLedger(db *bentopg.DB) (*StockLedger, error) {
	if db == nil {
		return nil, fmt.Errorf("warehouse fulfillment postgres: db is nil")
	}
	return &StockLedger{db: db}, nil
}

var (
	_ ports.StockLedger      = (*StockLedger)(nil)
	_ ports.StockOnHandRead  = (*StockLedger)(nil)
	_ ports.GoodsStockLookup = (*StockLedger)(nil)
)

// errStockQuantityOverflow 是合并或余额相加超出 int64。这是技术失败，不是余额不足。
var errStockQuantityOverflow = errors.New("stock ledger: quantity overflow")

// SerializePosting 先取货主商品锁的共享一侧，再取记数锁。商品登记只取商品锁的独占一侧，
// 记数总是先商品锁后记数锁，两边不会成环。没有环境事务时拒绝：锁在语句结束就放掉，等于没锁。
func (ledger *StockLedger) SerializePosting(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	warehouse domain.WarehouseCode,
) error {
	const operation = "serialize stock posting"
	executor, err := ledger.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if err := lockPosting(ctx, executor, tenant, owner, warehouse); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

func (ledger *StockLedger) LoadMovementBySource(
	ctx context.Context,
	tenant domain.TenantID,
	reason domain.StockMovementReason,
	source domain.SourceFactReference,
) (domain.StockMovement, bool, error) {
	const operation = "load stock movement"
	querier, err := ledger.db.ReadExecutor(ctx)
	if err != nil {
		return domain.StockMovement{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	movement, found, err := loadStockMovement(ctx, querier, tenant, reason, source)
	if err != nil {
		return domain.StockMovement{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return movement, found, nil
}

func (ledger *StockLedger) LoadInboundBatch(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.InboundBatchID,
) (domain.InboundBatch, bool, error) {
	const operation = "load inbound batch"
	querier, err := ledger.db.ReadExecutor(ctx)
	if err != nil {
		return domain.InboundBatch{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	batch, found, err := loadInboundBatch(ctx, querier, tenant, id)
	if err != nil {
		return domain.InboundBatch{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return batch, found, nil
}

func (ledger *StockLedger) CellBalance(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	warehouse domain.WarehouseCode,
	cell domain.StockCell,
) (int64, error) {
	const operation = "cell balance"
	if cell.InboundBatch().String() == "" {
		return 0, fmt.Errorf("%s: inbound batch is required", operation)
	}
	querier, err := ledger.db.ReadExecutor(ctx)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", operation, err)
	}
	balance, err := cellBalance(ctx, querier, tenant, owner, warehouse, cell)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", operation, err)
	}
	return balance, nil
}

func (ledger *StockLedger) ListOnHandCells(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	warehouse domain.WarehouseCode,
) ([]ports.StockBalanceRow, error) {
	const operation = "list on-hand cells"
	querier, err := ledger.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	rows, err := listOnHandCells(ctx, querier, tenant, owner, warehouse)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	return rows, nil
}

// AppendMovement 在锁里再判判重与余额，然后按外键方向写入：流水头、入库批次、行。
// 不是 Appended 的结果不写任何行：调用方的事务还会继续，可能提交。
// 判重用 ON CONFLICT DO NOTHING，不用捕获唯一冲突：唯一冲突会把当前事务打成不可用。
func (ledger *StockLedger) AppendMovement(
	ctx context.Context,
	movement domain.StockMovement,
	formed []domain.InboundBatch,
) (ports.StockAppendOutcome, error) {
	const operation = "append stock movement"
	executor, err := ledger.db.RequireExecutor(ctx)
	if err != nil {
		return ports.StockAppendOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	if err := lockPosting(ctx, executor, movement.Tenant(), movement.Owner(), movement.Warehouse()); err != nil {
		return ports.StockAppendOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	if err := formedBatchesMatch(movement, formed); err != nil {
		return ports.StockAppendOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	_, found, err := loadStockMovement(ctx, executor, movement.Tenant(), movement.Reason(), movement.Source())
	if err != nil {
		return ports.StockAppendOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	if found {
		return ports.StockAlreadyPosted, nil
	}
	short, err := postingWouldGoNegative(ctx, executor, movement)
	if err != nil {
		return ports.StockAppendOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	if short {
		return ports.StockInsufficientBalance, nil
	}
	inserted, err := insertStockMovement(ctx, executor, movement)
	if err != nil {
		return ports.StockAppendOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	if !inserted {
		return ports.StockAlreadyPosted, nil
	}
	for _, batch := range formed {
		if err := insertInboundBatch(ctx, executor, batch); err != nil {
			return ports.StockAppendOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
		}
	}
	for position, line := range movement.Lines() {
		if err := insertStockMovementLine(ctx, executor, movement, position, line); err != nil {
			return ports.StockAppendOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
		}
	}
	return ports.StockAppended, nil
}

// HasOnHandStock 先取货主商品锁的独占一侧，再答这个商品在任何仓有没有余额为正的格。
// 答案要撑到调用方这一笔修订落册：正在记数的事务拿着共享一侧，独占要等它们提交，提交之后这里才读；
// 此后新来的记数要等这一笔结束，再照新修订判（UC-WF-002「一致性、幂等与并发」：修订先落，之后的收货照新修订判；
// 收货先入账，修订答不可改的格）。没有环境事务时拒绝。
func (ledger *StockLedger) HasOnHandStock(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	code domain.GoodsCode,
) (bool, error) {
	const operation = "has on-hand stock"
	executor, err := ledger.db.RequireExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("%s: %w", operation, err)
	}
	if err := lockGoodsOwnerExclusive(ctx, executor, tenant.String(), owner.String()); err != nil {
		return false, fmt.Errorf("%s: %w", operation, err)
	}
	onHand, err := goodsOnHand(ctx, executor, tenant, owner, code)
	if err != nil {
		return false, fmt.Errorf("%s: %w", operation, err)
	}
	return onHand, nil
}

func lockPosting(
	ctx context.Context,
	executor bentopg.Executor,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	warehouse domain.WarehouseCode,
) error {
	if err := lockGoodsOwnerShared(ctx, executor, tenant.String(), owner.String()); err != nil {
		return err
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('warehouse_fulfillment.stock.posting'), hashtext($1 || chr(31) || $2 || chr(31) || $3))`,
		tenant.String(), owner.String(), warehouse.String(),
	); err != nil {
		return fmt.Errorf("lock stock posting: %w", err)
	}
	return nil
}

func formedBatchesMatch(movement domain.StockMovement, formed []domain.InboundBatch) error {
	for _, batch := range formed {
		if batch.Tenant() != movement.Tenant() || batch.Owner() != movement.Owner() || batch.Warehouse() != movement.Warehouse() || batch.MovementID() != movement.ID() {
			return fmt.Errorf("formed inbound batch %s does not belong to this movement", batch.ID())
		}
	}
	return nil
}

func postingWouldGoNegative(ctx context.Context, querier bentopg.Executor, movement domain.StockMovement) (bool, error) {
	nets := map[domain.StockCell]int64{}
	var order []domain.StockCell
	for _, line := range movement.Lines() {
		cell := line.Cell()
		if cell.InboundBatch().String() == "" {
			return false, fmt.Errorf("inbound batch is required")
		}
		if _, seen := nets[cell]; !seen {
			order = append(order, cell)
		}
		delta := line.Quantity()
		if line.Direction() == domain.StockOut {
			delta = -line.Quantity()
		}
		next, ok := domain.AddQuantity(nets[cell], delta)
		if !ok {
			return false, errStockQuantityOverflow
		}
		nets[cell] = next
	}
	for _, cell := range order {
		if nets[cell] >= 0 {
			continue
		}
		balance, err := cellBalance(ctx, querier, movement.Tenant(), movement.Owner(), movement.Warehouse(), cell)
		if err != nil {
			return false, err
		}
		total, ok := domain.AddQuantity(balance, nets[cell])
		if !ok {
			return false, errStockQuantityOverflow
		}
		if total < 0 {
			return true, nil
		}
	}
	return false, nil
}
