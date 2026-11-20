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

// UnclaimedArrivals 是待认领到货的 PostgreSQL 适配器。
// 写入必须和它所指向的收货事实在同一笔事务里：事实先落，到货再指向它。没有环境事务时拒绝。
type UnclaimedArrivals struct {
	db *bentopg.DB
}

func NewUnclaimedArrivals(db *bentopg.DB) (*UnclaimedArrivals, error) {
	if db == nil {
		return nil, fmt.Errorf("warehouse fulfillment postgres: db is nil")
	}
	return &UnclaimedArrivals{db: db}, nil
}

var _ ports.UnclaimedArrivalStore = (*UnclaimedArrivals)(nil)

func (store *UnclaimedArrivals) Save(ctx context.Context, arrival domain.UnclaimedArrival) (bool, error) {
	const operation = "save unclaimed arrival"
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("%s: %w", operation, err)
	}
	var id string
	err = executor.QueryRow(ctx,
		`INSERT INTO warehouse_fulfillment.unclaimed_arrival (
		    tenant_id, arrival_id, fact_id, pre_advice_id, owner_account_id
		 ) VALUES ($1,$2,$3,$4,$5)
		 ON CONFLICT ON CONSTRAINT unclaimed_arrival_fact_key DO NOTHING
		 RETURNING arrival_id`,
		arrival.Tenant().String(), arrival.ID().String(), arrival.Receipt().FactID().String(),
		arrival.PreAdvice().String(), arrival.Owner().String(),
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("%s: %w", operation, err)
	}
	return true, nil
}

func (store *UnclaimedArrivals) PreAdviceIDsWithUnclaimedArrival(
	ctx context.Context,
	tenant domain.TenantID,
	ids []domain.PreAdviceID,
) ([]domain.PreAdviceID, error) {
	const operation = "list pre-advices with unclaimed arrival"
	if len(ids) == 0 {
		return nil, nil
	}
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	keys := make([]string, len(ids))
	for index, id := range ids {
		keys[index] = id.String()
	}
	rows, err := querier.Query(ctx,
		`SELECT DISTINCT pre_advice_id
		   FROM warehouse_fulfillment.unclaimed_arrival
		  WHERE tenant_id = $1 AND pre_advice_id = ANY($2)`,
		tenant.String(), keys)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	defer rows.Close()
	var found []domain.PreAdviceID
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("%s: %w", operation, err)
		}
		id, err := domain.NewPreAdviceID(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", operation, err)
		}
		found = append(found, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	return found, nil
}
