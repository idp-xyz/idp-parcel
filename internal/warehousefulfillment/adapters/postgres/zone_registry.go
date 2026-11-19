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

// Zones 实现 ports.ZoneRegistry：库区修订表（0002 迁移）的只增登记与读回。撞键不覆盖：同内容重放，异内容冲突（ADR-0031）；
// 比的是完整内容，不比登记责任方与登记时刻。
//
// 并发提交同一修订号的两笔靠主键守，机制同 Warehouses：先插入的那一笔落册，后插入的那一笔在行锁上等到先一笔提交，再由
// ON CONFLICT DO NOTHING 得零行，随后读回已落册的那一笔按内容答已在册或冲突。事务是 READ COMMITTED，等锁之后的读看得见刚提交的那一行。
type Zones struct {
	db *bentopg.DB
}

func NewZones(db *bentopg.DB) (*Zones, error) {
	if db == nil {
		return nil, fmt.Errorf("warehouse fulfillment postgres: db is nil")
	}
	return &Zones{db: db}, nil
}

var (
	_ ports.ZoneRegistry = (*Zones)(nil)
	// 库区登记用例核「仓已登记」读的是仓登记册本身，不另有一条读路：仓册的这一个读方法就是 ports.WarehouseLookup。
	_ ports.WarehouseLookup = (*Warehouses)(nil)
)

func (repository *Zones) SaveZoneRevision(
	ctx context.Context,
	revision domain.ZoneRevision,
) (ports.RegistrySaveOutcome, error) {
	const operation = "save zone revision"
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.RegistrySaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}

	content := revision.Content()
	tag, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.zone_revision
			(tenant_id, warehouse_code, zone_code, revision, zone_name, zone_usage, deactivated, registrant, registered_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT (tenant_id, warehouse_code, zone_code, revision) DO NOTHING`,
		revision.Tenant().String(), revision.Warehouse().String(), revision.Code().String(), revision.Revision(),
		content.Name().String(), content.Usage().String(), content.Deactivated(),
		revision.Registrant().String(), revision.RegisteredAt(),
	)
	if err != nil {
		return ports.RegistrySaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() > 0 {
		return ports.RegistryRevisionSaved, nil
	}

	existing, found, err := loadZoneRevision(
		ctx, executor, revision.Tenant(), revision.Warehouse(), revision.Code(), revision.Revision())
	if err != nil {
		return ports.RegistrySaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	if !found {
		return ports.RegistrySaveOutcomeInvalid, fmt.Errorf("%s: 撞键后读不回既有行", operation)
	}
	if existing.SameContentAs(revision) {
		return ports.RegistryRevisionAlreadyRegistered, nil
	}
	return ports.RegistryRevisionContentConflict, nil
}

func (repository *Zones) LoadLatestZone(
	ctx context.Context,
	tenant domain.TenantID,
	warehouse domain.WarehouseCode,
	code domain.ZoneCode,
) (domain.ZoneRevision, bool, error) {
	const operation = "load latest zone"
	if tenant.String() == "" || warehouse.String() == "" || code.String() == "" {
		return domain.ZoneRevision{}, false, fmt.Errorf("%s: tenant, warehouse code and zone code are required", operation)
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.ZoneRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	stored, err := scanStoredZone(querier.QueryRow(ctx,
		`SELECT `+zoneColumns+`
		   FROM warehouse_fulfillment.zone_revision
		  WHERE tenant_id = $1 AND warehouse_code = $2 AND zone_code = $3
		  ORDER BY revision DESC
		  LIMIT 1`,
		tenant.String(), warehouse.String(), code.String()))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ZoneRevision{}, false, nil
	}
	if err != nil {
		return domain.ZoneRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	revision, err := zoneRevisionOfStored(stored)
	if err != nil {
		return domain.ZoneRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return revision, true, nil
}

func (repository *Zones) LoadZoneRevision(
	ctx context.Context,
	tenant domain.TenantID,
	warehouse domain.WarehouseCode,
	code domain.ZoneCode,
	revision int,
) (domain.ZoneRevision, bool, error) {
	const operation = "load zone revision"
	if tenant.String() == "" || warehouse.String() == "" || code.String() == "" {
		return domain.ZoneRevision{}, false, fmt.Errorf("%s: tenant, warehouse code and zone code are required", operation)
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.ZoneRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	loaded, found, err := loadZoneRevision(ctx, querier, tenant, warehouse, code, revision)
	if err != nil {
		return domain.ZoneRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return loaded, found, nil
}

// loadZoneRevision 按键读回一笔修订；保存撞键后的读回与点读共用这一句，免得长出第二套读回转写。
func loadZoneRevision(
	ctx context.Context,
	querier bentopg.Querier,
	tenant domain.TenantID,
	warehouse domain.WarehouseCode,
	code domain.ZoneCode,
	revision int,
) (domain.ZoneRevision, bool, error) {
	stored, err := scanStoredZone(querier.QueryRow(ctx,
		`SELECT `+zoneColumns+`
		   FROM warehouse_fulfillment.zone_revision
		  WHERE tenant_id = $1 AND warehouse_code = $2 AND zone_code = $3 AND revision = $4`,
		tenant.String(), warehouse.String(), code.String(), revision))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ZoneRevision{}, false, nil
	}
	if err != nil {
		return domain.ZoneRevision{}, false, err
	}
	loaded, err := zoneRevisionOfStored(stored)
	if err != nil {
		return domain.ZoneRevision{}, false, err
	}
	return loaded, true, nil
}
