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

func (store *Waves) LoadWaveAssignment(ctx context.Context, tenant domain.TenantID, wave domain.WaveID) (ports.WaveAssignment, bool, error) {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return ports.WaveAssignment{}, false, fmt.Errorf("load wave assignment: %w", err)
	}
	var assignment ports.WaveAssignment
	var warehouse string
	var assignee *string
	// 与 lockWaves 用同一种行锁：派工与拣货、截单各自改这个波次时排在同一行上。
	err = executor.QueryRow(ctx,
		`SELECT warehouse_code, status, assignee FROM warehouse_fulfillment.wave
		  WHERE tenant_id = $1 AND wave_id = $2
		    FOR NO KEY UPDATE`,
		tenant.String(), wave.String()).Scan(&warehouse, &assignment.Status, &assignee)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.WaveAssignment{}, false, nil
	}
	if err != nil {
		return ports.WaveAssignment{}, false, fmt.Errorf("load wave assignment: %w", err)
	}
	if assignment.Warehouse, err = domain.NewWarehouseCode(warehouse); err != nil {
		return ports.WaveAssignment{}, false, fmt.Errorf("load wave assignment: %w", err)
	}
	assignment.Assignee = valueOrEmpty(assignee)
	return assignment, true, nil
}

func (store *Waves) WaveAssignee(ctx context.Context, tenant domain.TenantID, wave domain.WaveID) (string, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return "", err
	}
	var assignee *string
	err = querier.QueryRow(ctx,
		`SELECT assignee FROM warehouse_fulfillment.wave WHERE tenant_id = $1 AND wave_id = $2`,
		tenant.String(), wave.String()).Scan(&assignee)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("wave assignee: %w", err)
	}
	return valueOrEmpty(assignee), nil
}

func (store *Waves) WaveZoneAssignee(ctx context.Context, tenant domain.TenantID, wave domain.WaveID, zone domain.ZoneCode) (string, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return "", err
	}
	var assignee string
	err = querier.QueryRow(ctx,
		`SELECT assignee FROM warehouse_fulfillment.wave_zone_assignee WHERE tenant_id = $1 AND wave_id = $2 AND zone_code = $3`,
		tenant.String(), wave.String(), zone.String()).Scan(&assignee)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("wave zone assignee: %w", err)
	}
	return assignee, nil
}

func (store *Waves) SaveWaveZoneAssignee(ctx context.Context, tenant domain.TenantID, wave domain.WaveID, zone domain.ZoneCode, assignee, by string, at time.Time) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save wave zone assignee: %w", err)
	}
	if assignee == "" {
		if _, err := executor.Exec(ctx,
			`DELETE FROM warehouse_fulfillment.wave_zone_assignee WHERE tenant_id = $1 AND wave_id = $2 AND zone_code = $3`,
			tenant.String(), wave.String(), zone.String()); err != nil {
			return fmt.Errorf("save wave zone assignee: %w", err)
		}
		return nil
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.wave_zone_assignee (tenant_id, wave_id, zone_code, assignee, assigned_by, assigned_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (tenant_id, wave_id, zone_code) DO UPDATE
		    SET assignee = EXCLUDED.assignee, assigned_by = EXCLUDED.assigned_by, assigned_at = EXCLUDED.assigned_at`,
		tenant.String(), wave.String(), zone.String(), assignee, by, at.UTC()); err != nil {
		return fmt.Errorf("save wave zone assignee: %w", err)
	}
	return nil
}

func (store *Waves) SaveWaveAssignee(ctx context.Context, tenant domain.TenantID, wave domain.WaveID, assignee, by string, at time.Time) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save wave assignee: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.wave
		    SET assignee = NULLIF($3, ''),
		        assigned_by = CASE WHEN $3 = '' THEN NULL ELSE $4 END,
		        assigned_at = CASE WHEN $3 = '' THEN NULL ELSE $5::timestamptz END
		  WHERE tenant_id = $1 AND wave_id = $2`,
		tenant.String(), wave.String(), assignee, by, at.UTC())
	if err != nil {
		return fmt.Errorf("save wave assignee: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("save wave assignee: wave %s is not there", wave.String())
	}
	return nil
}
