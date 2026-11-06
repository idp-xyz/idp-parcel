package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
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
		`SELECT tenant_id, version, interval_minutes, basis, recorded_at
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
		`SELECT tenant_id, version, interval_minutes, basis, recorded_at
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
		`INSERT INTO transport_fulfillment.delivery_dispatch_reask_cadence (tenant_id, version, interval_minutes, basis, recorded_at)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT DO NOTHING`,
		cadence.TenantID().String(), cadence.Version().String(), int(cadence.Interval()/time.Minute), cadence.Basis(),
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return false, fmt.Errorf("save delivery dispatch reask cadence: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// deliveryDispatchReaskCadencePages 把 ports.DeliveryDispatchReaskCadencePages 落到列上。q 在版本与依据上做字面包含。
var deliveryDispatchReaskCadencePages = ports.DeliveryDispatchReaskCadencePages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"recordedAt": "recorded_at"},
	Identity: []string{"version"},
	Keyword:  []string{"version", "basis"},
})

// ListDeliveryDispatchReaskCadences 上列一个租户登过的各版（ADR-0230 决定五）：缺省最近登记的在前、同刻按版本倒序，与
// CurrentCadence 同一个次序，不筛不搜时第一页第一行就是生效的那一版。
func (catalogue *ReviewCatalogue) ListDeliveryDispatchReaskCadences(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.DeliveryDispatchReaskCadenceRecord], error) {
	fail := func(err error) (cataloguepage.Result[ports.DeliveryDispatchReaskCadenceRecord], error) {
		return cataloguepage.Result[ports.DeliveryDispatchReaskCadenceRecord]{}, fmt.Errorf("list delivery dispatch reask cadences: %w", err)
	}
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := deliveryDispatchReaskCadencePages.Plan(query, "tenant_id = $1", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}

	const source = "transport_fulfillment.delivery_dispatch_reask_cadence"
	page, err := readCataloguePage(ctx, querier, query, plan, limit, plan.CountSQL(source),
		plan.PageSQL("tenant_id, version, interval_minutes, basis, recorded_at", source),
		scanReaskCadence, deliveryDispatchReaskCadencePosition)
	if err != nil {
		return fail(err)
	}
	return page, nil
}

// deliveryDispatchReaskCadencePosition 取装回的记录：登记时刻只换了时区，版本的构造门原样存值，与列逐位相同。
func deliveryDispatchReaskCadencePosition(record ports.DeliveryDispatchReaskCadenceRecord, _ string) cataloguepage.Position {
	return cataloguepage.Position{
		Value:    cataloguepage.FormatInstant(record.RecordedAt),
		Identity: []string{record.Cadence.Version().String()},
	}
}

func scanReaskCadence(row pgx.Row) (ports.DeliveryDispatchReaskCadenceRecord, error) {
	var tenantText, versionText, basis string
	var minutes int
	var recordedAt time.Time
	if err := row.Scan(&tenantText, &versionText, &minutes, &basis, &recordedAt); err != nil {
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
	cadence, err := domain.NewDeliveryDispatchReaskCadence(tenant, version, time.Duration(minutes)*time.Minute, basis)
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
