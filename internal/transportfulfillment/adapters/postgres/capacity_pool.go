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
// 记录时刻——班次、维度、单位、容量在 INSERT 里冻结（与委托 Replace 只写转换列同一纪律）。
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
//
// 读池一律锁住池那一行（FOR UPDATE）：在写事务里锁到事务结束，不在事务里随这一条语句放。池按值换存：两笔预占各自
// 读到同一个旧池、各算各的余量、先后换存，后到的那一笔会把先到的预占整份删掉——硬上限与先到者的预占一起丢。锁住读，
// 第二笔就等第一笔落定再读，读到的是含第一笔的池（ADR-0203 决定四）。不按「在不在事务里」分两种读：那要先问一次写
// 执行器，这一口就被当成了无事务即拒的写口，而它在事务外照样要读得回。
func (repository *CapacityPools) FindByKey(
	ctx context.Context,
	key ports.CapacityPoolKey,
) (ports.CapacityPoolRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.CapacityPoolRecord{}, false, fmt.Errorf("find capacity pool: %w", err)
	}

	var scheduleID, dimension, unit, digest string
	var capacity int64
	var recordedAt time.Time
	err = querier.QueryRow(ctx,
		`SELECT schedule_id, dimension, unit_ref, capacity, content_digest, recorded_at
		   FROM transport_fulfillment.capacity_pool
		  WHERE tenant_id = $1
		    AND pool_id = $2
		    FOR UPDATE`,
		key.TenantID.String(),
		key.Pool.String(),
	).Scan(&scheduleID, &dimension, &unit, &capacity, &digest, &recordedAt)
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
	dimensionWord, err := domain.ParseCapacityDimension(dimension)
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
		TenantID:  key.TenantID,
		Schedule:  scheduleRef,
		Dimension: dimensionWord,
		Unit:      unitRef,
		Capacity:  capacity,
	}, snapshots)
	if err != nil {
		return ports.CapacityPoolRecord{}, false, fmt.Errorf("find capacity pool: %w", err)
	}
	// 库上 CHECK 已钉住池身份由班次与维度派生；这里再核一遍，是因为读回的池要交给按池身份转换的编排——身份对不上
	// 的一行宁可答读不懂，也不以另一个身份的池作答。
	if pool.Pool() != key.Pool {
		return ports.CapacityPoolRecord{}, false, fmt.Errorf("find capacity pool: stored identity %q is not derived from its schedule and dimension", key.Pool)
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
			(tenant_id, pool_id, schedule_id, dimension, unit_ref, capacity, content_digest, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Pool.String(),
		record.Pool.Schedule().String(),
		record.Pool.Dimension().String(),
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

// Replace 只落预占子表（连同释放明细，外键随预占删）与记录时刻。UPDATE 不含班次/维度/单位/容量。0 行（没有可转换
// 的池）答 false。
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
	releases, err := repository.loadReleases(ctx, querier, key)
	if err != nil {
		return nil, err
	}
	rows, err := querier.Query(ctx,
		`SELECT reservation_id, quantity, valid_until, released, consumed, source_kind, source_ref, reserved_at
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
		var reservationID, sourceKind, sourceRef string
		var quantity, released, consumed int64
		var validUntil, reservedAt time.Time
		if err := rows.Scan(&reservationID, &quantity, &validUntil, &released, &consumed, &sourceKind, &sourceRef, &reservedAt); err != nil {
			return nil, err
		}
		reference, err := domain.NewCapacityReservationReference(reservationID)
		if err != nil {
			return nil, err
		}
		source, err := demandSourceFrom(sourceKind, sourceRef)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, domain.CapacityReservationSnapshot{
			Reference:  reference,
			Quantity:   quantity,
			Released:   released,
			Consumed:   consumed,
			ValidUntil: validUntil,
			Source:     source,
			ReservedAt: reservedAt,
			Releases:   releases[reservationID],
		})
	}
	return snapshots, rows.Err()
}

// loadReleases 按所属预占分组读回本池全部显式释放，组内按释放时点、再按释放引用排，只为读回稳定。次序不带语义：领域里
// 按提交先后追加，倒填的释放让两者不同；归还按各次自己的时点算，重建门核的是各次时点、引用不重与和，都不看次序。
func (repository *CapacityPools) loadReleases(
	ctx context.Context,
	querier bentopg.Querier,
	key ports.CapacityPoolKey,
) (map[string][]domain.CapacityReleaseSnapshot, error) {
	rows, err := querier.Query(ctx,
		`SELECT reservation_id, release_id, quantity, source_kind, source_ref, released_at
		   FROM transport_fulfillment.capacity_release
		  WHERE tenant_id = $1 AND pool_id = $2
		  ORDER BY reservation_id, released_at, release_id`,
		key.TenantID.String(),
		key.Pool.String(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	releases := map[string][]domain.CapacityReleaseSnapshot{}
	for rows.Next() {
		var reservationID, releaseID, sourceKind, sourceRef string
		var quantity int64
		var releasedAt time.Time
		if err := rows.Scan(&reservationID, &releaseID, &quantity, &sourceKind, &sourceRef, &releasedAt); err != nil {
			return nil, err
		}
		reference, err := domain.NewCapacityReleaseReference(releaseID)
		if err != nil {
			return nil, err
		}
		source, err := demandSourceFrom(sourceKind, sourceRef)
		if err != nil {
			return nil, err
		}
		releases[reservationID] = append(releases[reservationID], domain.CapacityReleaseSnapshot{
			Reference:  reference,
			Quantity:   quantity,
			Source:     source,
			ReleasedAt: releasedAt,
		})
	}
	return releases, rows.Err()
}

func demandSourceFrom(kind, reference string) (domain.CapacityDemandSource, error) {
	sourceKind, err := domain.ParseCapacityDemandSourceKind(kind)
	if err != nil {
		return domain.CapacityDemandSource{}, err
	}
	sourceRef, err := domain.NewCapacityDemandSourceReference(reference)
	if err != nil {
		return domain.CapacityDemandSource{}, err
	}
	return domain.NewCapacityDemandSource(sourceKind, sourceRef)
}

func insertReservations(
	ctx context.Context,
	executor bentopg.Executor,
	record ports.CapacityPoolRecord,
) error {
	for _, snapshot := range record.Pool.ReservationSnapshots() {
		if _, err := executor.Exec(ctx,
			`INSERT INTO transport_fulfillment.capacity_reservation
				(tenant_id, pool_id, reservation_id, quantity, valid_until, released, consumed,
				 source_kind, source_ref, reserved_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			record.Key.TenantID.String(),
			record.Key.Pool.String(),
			snapshot.Reference.String(),
			snapshot.Quantity,
			snapshot.ValidUntil.UTC(),
			snapshot.Released,
			snapshot.Consumed,
			snapshot.Source.Kind().String(),
			snapshot.Source.Reference().String(),
			snapshot.ReservedAt.UTC(),
		); err != nil {
			return err
		}
		for _, release := range snapshot.Releases {
			if _, err := executor.Exec(ctx,
				`INSERT INTO transport_fulfillment.capacity_release
					(tenant_id, pool_id, release_id, reservation_id, quantity, source_kind, source_ref, released_at)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
				record.Key.TenantID.String(),
				record.Key.Pool.String(),
				release.Reference.String(),
				snapshot.Reference.String(),
				release.Quantity,
				release.Source.Kind().String(),
				release.Source.Reference().String(),
				release.ReleasedAt.UTC(),
			); err != nil {
				return err
			}
		}
	}
	return nil
}
