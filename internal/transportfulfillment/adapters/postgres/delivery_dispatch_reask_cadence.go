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

// DeliveryDispatchReaskCadences 实现派送重问间隔册（ADR-0230 决定五；迁移 0046）。所有语句显式带租户条件；读回过领域构造门，坏行
// 在这里响，不流进到期重问。
type DeliveryDispatchReaskCadences struct {
	db *bentopg.DB
}

var (
	_ ports.DeliveryDispatchReaskCadenceRegistry = (*DeliveryDispatchReaskCadences)(nil)
	_ ports.DeliveryDispatchReaskCadenceReader   = (*DeliveryDispatchReaskCadences)(nil)
)

func NewDeliveryDispatchReaskCadences(db *bentopg.DB) (*DeliveryDispatchReaskCadences, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &DeliveryDispatchReaskCadences{db: db}, nil
}

func (register *DeliveryDispatchReaskCadences) FindByVersion(
	ctx context.Context,
	tenant domain.TenantID,
	version domain.DeliveryDispatchReaskCadenceVersion,
) (ports.DeliveryDispatchReaskCadenceRecord, bool, error) {
	querier, err := register.db.ReadExecutor(ctx)
	if err != nil {
		return ports.DeliveryDispatchReaskCadenceRecord{}, false, fmt.Errorf("find delivery dispatch reask cadence: %w", err)
	}
	record, err := scanReaskCadence(querier.QueryRow(ctx,
		`SELECT tenant_id, version, interval_minutes, recorded_at
		   FROM transport_fulfillment.delivery_dispatch_reask_cadence
		  WHERE tenant_id = $1 AND version = $2`,
		tenant.String(), version.String(),
	))
	return foundReaskCadence(record, err, "find delivery dispatch reask cadence")
}

// CurrentCadence 取最近登记的那一版，同刻按版本稳定取一个。
func (register *DeliveryDispatchReaskCadences) CurrentCadence(
	ctx context.Context,
	tenant domain.TenantID,
) (ports.DeliveryDispatchReaskCadenceRecord, bool, error) {
	querier, err := register.db.ReadExecutor(ctx)
	if err != nil {
		return ports.DeliveryDispatchReaskCadenceRecord{}, false, fmt.Errorf("current delivery dispatch reask cadence: %w", err)
	}
	record, err := scanReaskCadence(querier.QueryRow(ctx,
		`SELECT tenant_id, version, interval_minutes, recorded_at
		   FROM transport_fulfillment.delivery_dispatch_reask_cadence
		  WHERE tenant_id = $1
		  ORDER BY recorded_at DESC, version DESC
		  LIMIT 1`,
		tenant.String(),
	))
	return foundReaskCadence(record, err, "current delivery dispatch reask cadence")
}

// Save 首登一版；版本已在册答 false。
func (register *DeliveryDispatchReaskCadences) Save(ctx context.Context, record ports.DeliveryDispatchReaskCadenceRecord) (bool, error) {
	executor, err := register.db.RequireExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("save delivery dispatch reask cadence: %w", err)
	}
	cadence := record.Cadence
	tag, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.delivery_dispatch_reask_cadence (tenant_id, version, interval_minutes, recorded_at)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT DO NOTHING`,
		cadence.TenantID().String(), cadence.Version().String(), int(cadence.Interval()/time.Minute), record.RecordedAt.UTC(),
	)
	if err != nil {
		return false, fmt.Errorf("save delivery dispatch reask cadence: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// ListDeliveryDispatchReaskCadences 上列一个租户登过的各版（ADR-0230 决定五）：最近登记的在前，第一行就是生效的那一版。
func (catalogue *ReviewCatalogue) ListDeliveryDispatchReaskCadences(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.DeliveryDispatchReaskCadenceRecord, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("list delivery dispatch reask cadences: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list delivery dispatch reask cadences: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT tenant_id, version, interval_minutes, recorded_at
		   FROM transport_fulfillment.delivery_dispatch_reask_cadence
		  WHERE tenant_id = $1
		  ORDER BY recorded_at DESC, version DESC
		  LIMIT $2`,
		tenant.String(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list delivery dispatch reask cadences: %w", err)
	}
	defer rows.Close()
	list := make([]ports.DeliveryDispatchReaskCadenceRecord, 0, limit)
	for rows.Next() {
		record, err := scanReaskCadence(rows)
		if err != nil {
			return nil, fmt.Errorf("list delivery dispatch reask cadences: %w", err)
		}
		list = append(list, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list delivery dispatch reask cadences: %w", err)
	}
	return list, nil
}

func scanReaskCadence(row pgx.Row) (ports.DeliveryDispatchReaskCadenceRecord, error) {
	var tenantText, versionText string
	var minutes int
	var recordedAt time.Time
	if err := row.Scan(&tenantText, &versionText, &minutes, &recordedAt); err != nil {
		return ports.DeliveryDispatchReaskCadenceRecord{}, err
	}
	tenant, err := domain.NewTenantID(tenantText)
	if err != nil {
		return ports.DeliveryDispatchReaskCadenceRecord{}, err
	}
	version, err := domain.NewDeliveryDispatchReaskCadenceVersion(versionText)
	if err != nil {
		return ports.DeliveryDispatchReaskCadenceRecord{}, err
	}
	cadence, err := domain.NewDeliveryDispatchReaskCadence(tenant, version, time.Duration(minutes)*time.Minute)
	if err != nil {
		return ports.DeliveryDispatchReaskCadenceRecord{}, err
	}
	return ports.DeliveryDispatchReaskCadenceRecord{Cadence: cadence, RecordedAt: recordedAt.UTC()}, nil
}

func foundReaskCadence(
	record ports.DeliveryDispatchReaskCadenceRecord,
	err error,
	verb string,
) (ports.DeliveryDispatchReaskCadenceRecord, bool, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.DeliveryDispatchReaskCadenceRecord{}, false, nil
	}
	if err != nil {
		return ports.DeliveryDispatchReaskCadenceRecord{}, false, fmt.Errorf("%s: %w", verb, err)
	}
	return record, true, nil
}
