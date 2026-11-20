package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// Receipts 是收货事实的 PostgreSQL 适配器。事务是 READ COMMITTED。
// 事实身份的锁只串行同一条事实。预报锁在应用层，并且先于库存的货主锁与记数锁，不在这里拿。
type Receipts struct {
	db *bentopg.DB
}

func NewReceipts(db *bentopg.DB) (*Receipts, error) {
	if db == nil {
		return nil, fmt.Errorf("warehouse fulfillment postgres: db is nil")
	}
	return &Receipts{db: db}, nil
}

var (
	_ ports.ReceiptStore = (*Receipts)(nil)
	_ ports.ReceiptRead  = (*Receipts)(nil)
)

func (store *Receipts) HoldFact(ctx context.Context, tenant domain.TenantID, factID domain.ReceiptFactID) error {
	const operation = "hold receipt fact"
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(
		    hashtext('warehouse_fulfillment.receipt_fact'),
		    hashtext($1 || chr(31) || $2))`,
		tenant.String(), factID.String()); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

func (store *Receipts) Load(
	ctx context.Context,
	tenant domain.TenantID,
	factID domain.ReceiptFactID,
) (domain.RecordedReceipt, bool, error) {
	receipts, err := store.load(ctx, "load receipt",
		`tenant_id = $1 AND fact_id = $2`, tenant.String(), factID.String())
	if err != nil {
		return domain.RecordedReceipt{}, false, err
	}
	switch len(receipts) {
	case 0:
		return domain.RecordedReceipt{}, false, nil
	case 1:
		return receipts[0], true, nil
	default:
		return domain.RecordedReceipt{}, false, fmt.Errorf("load receipt: fact %s returned more than one row", factID)
	}
}

func (store *Receipts) ListByPreAdvices(
	ctx context.Context,
	tenant domain.TenantID,
	ids []domain.PreAdviceID,
) ([]domain.RecordedReceipt, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	keys := make([]string, len(ids))
	for index, id := range ids {
		keys[index] = id.String()
	}
	return store.load(ctx, "list receipts",
		`tenant_id = $1 AND pre_advice_id = ANY($2)`, tenant.String(), keys)
}

func (store *Receipts) Save(ctx context.Context, receipt domain.RecordedReceipt) (bool, error) {
	const operation = "save receipt"
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("%s: %w", operation, err)
	}
	inserted, err := insertReceipt(ctx, executor, receipt)
	if err != nil {
		return false, fmt.Errorf("%s: %w", operation, err)
	}
	if !inserted {
		return false, nil
	}
	return true, nil
}

func (store *Receipts) load(ctx context.Context, operation, where string, args ...any) ([]domain.RecordedReceipt, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	receipts, err := loadReceipts(ctx, querier, where, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	return receipts, nil
}
