package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// CapacityPools 实现 ports.CapacityPoolStore。Save 建池；Replace 只换预占子表与
// 记录时刻——班次、单位、容量在 INSERT 里冻结（与委托 Replace 只写转换列同一纪律）。
type CapacityPools struct {
	db *bentopg.DB
}

func NewCapacityPools(db *bentopg.DB) (*CapacityPools, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &CapacityPools{db: db}, nil
}

// FindByKey 按（租户+池）取回容量池及其预占。否定结果只回 false。读回经重建门
// 复验三量守恒，不重放 Reserve/Release/Consume。
func (repository *CapacityPools) FindByKey(
	ctx context.Context,
	key ports.CapacityPoolKey,
) (ports.CapacityPoolRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.CapacityPoolRecord{}, false, fmt.Errorf("find capacity pool: %w", err)
	}

	var scheduleID, unit, digest string
	var capacity int64
	var recordedAt time.Time
	err = querier.QueryRow(ctx,
		`SELECT schedule_id, unit_ref, capacity, content_digest, recorded_at
		   FROM transport_fulfillment.capacity_pool
		  WHERE tenant_id = $1
		    AND pool_id = $2`,
		key.TenantID.String(),
		key.Pool.String(),
	).Scan(&scheduleID, &unit, &capacity, &digest, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.CapacityPoolRecord{}, false, nil
	}
	if err != nil {
		return ports.CapacityPoolRecord{}, false, fmt.Errorf("find capacity pool: %w", err)
	}

	scheduleRef, err := domain.NewScheduleReference(scheduleID)
	if err != nil {
		return ports.CapacityPoolRecord{}, false, fmt.Errorf("find capacity pool: %w", err)
	}
	unitRef, err := domain.NewQuantityUnitReference(unit)
	if err != nil {
		return ports.CapacityPoolRecord{}, false, fmt.Errorf("find capacity pool: %w", err)
	}
	snapshots, err := repository.loadReservations(ctx, querier, key)
	if err != nil {
		return ports.CapacityPoolRecord{}, false, fmt.Errorf("find capacity pool: %w", err)
	}
	pool, err := domain.RehydrateCapacityPool(domain.CapacityPoolSpec{
		TenantID: key.TenantID,
		Pool:     key.Pool,
		Schedule: scheduleRef,
		Unit:     unitRef,
		Capacity: capacity,
	}, snapshots)
	if err != nil {
		return ports.CapacityPoolRecord{}, false, fmt.Errorf("find capacity pool: %w", err)
	}
	return ports.CapacityPoolRecord{
		Key:           key,
		ContentDigest: digest,
		Pool:          pool,
		RecordedAt:    recordedAt.UTC(),
	}, true, nil
}

// Save 建一份容量池。同标识已有记录时答`已建立`，不覆盖先到者。
func (repository *CapacityPools) Save(
	ctx context.Context,
	record ports.CapacityPoolRecord,
) (ports.PoolSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.PoolSaveOutcomeInvalid, fmt.Errorf("save capacity pool: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.capacity_pool
			(tenant_id, pool_id, schedule_id, unit_ref, capacity, content_digest, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Pool.String(),
		record.Pool.Schedule().String(),
		record.Pool.Unit().String(),
		record.Pool.Capacity(),
		record.ContentDigest,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.PoolSaveOutcomeInvalid, fmt.Errorf("save capacity pool: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.PoolAlreadyEstablished, nil
	}
	if err := insertReservations(ctx, executor, record); err != nil {
		return ports.PoolSaveOutcomeInvalid, fmt.Errorf("save capacity pool: %w", err)
	}
	return ports.PoolSaved, nil
}

// Replace 只落预占子表与记录时刻。UPDATE 不含班次/单位/容量。0 行（没有可转换的池）
// 答 false。
func (repository *CapacityPools) Replace(
	ctx context.Context,
	record ports.CapacityPoolRecord,
) (bool, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("replace capacity pool: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`UPDATE transport_fulfillment.capacity_pool
		    SET content_digest = $3, recorded_at = $4
		  WHERE tenant_id = $1
		    AND pool_id = $2`,
		record.Key.TenantID.String(),
		record.Key.Pool.String(),
		record.ContentDigest,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return false, fmt.Errorf("replace capacity pool: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}

	if _, err := executor.Exec(ctx,
		`DELETE FROM transport_fulfillment.capacity_reservation
		  WHERE tenant_id = $1
		    AND pool_id = $2`,
		record.Key.TenantID.String(),
		record.Key.Pool.String(),
	); err != nil {
		return false, fmt.Errorf("replace capacity pool: %w", err)
	}
	if err := insertReservations(ctx, executor, record); err != nil {
		return false, fmt.Errorf("replace capacity pool: %w", err)
	}
	return true, nil
}

func (repository *CapacityPools) loadReservations(
	ctx context.Context,
	querier bentopg.Querier,
	key ports.CapacityPoolKey,
) ([]domain.CapacityReservationSnapshot, error) {
	rows, err := querier.Query(ctx,
		`SELECT reservation_id, quantity, valid_until, released, consumed
		   FROM transport_fulfillment.capacity_reservation
		  WHERE tenant_id = $1 AND pool_id = $2
		  ORDER BY reservation_id`,
		key.TenantID.String(),
		key.Pool.String(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snapshots []domain.CapacityReservationSnapshot
	for rows.Next() {
		var reservationID string
		var quantity, released, consumed int64
		var validUntil time.Time
		if err := rows.Scan(&reservationID, &quantity, &validUntil, &released, &consumed); err != nil {
			return nil, err
		}
		reference, err := domain.NewCapacityReservationReference(reservationID)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, domain.CapacityReservationSnapshot{
			Reference:  reference,
			Quantity:   quantity,
			Released:   released,
			Consumed:   consumed,
			ValidUntil: validUntil,
		})
	}
	return snapshots, rows.Err()
}

func insertReservations(
	ctx context.Context,
	executor bentopg.Executor,
	record ports.CapacityPoolRecord,
) error {
	for _, snapshot := range record.Pool.ReservationSnapshots() {
		if _, err := executor.Exec(ctx,
			`INSERT INTO transport_fulfillment.capacity_reservation
				(tenant_id, pool_id, reservation_id, quantity, valid_until, released, consumed)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			record.Key.TenantID.String(),
			record.Key.Pool.String(),
			snapshot.Reference.String(),
			snapshot.Quantity,
			snapshot.ValidUntil.UTC(),
			snapshot.Released,
			snapshot.Consumed,
		); err != nil {
			return err
		}
	}
	return nil
}
