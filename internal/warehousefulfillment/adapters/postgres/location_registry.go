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

// Locations 实现 ports.LocationRegistry：库位修订表（0005 迁移）的只增登记与读回。撞键不覆盖：同内容重放，异内容冲突（ADR-0031）；
// 比的是完整内容，不比登记责任方与登记时刻。
//
// 并发提交同一修订号的两笔靠主键守，机制同 Warehouses：先插入的那一笔落册，后插入的那一笔在行锁上等到先一笔提交，再由
// ON CONFLICT DO NOTHING 得零行，随后读回已落册的那一笔按内容答已在册或冲突。事务是 READ COMMITTED，等锁之后的读看得见刚提交的那一行。
type Locations struct {
	db *bentopg.DB
}

func NewLocations(db *bentopg.DB) (*Locations, error) {
	if db == nil {
		return nil, fmt.Errorf("warehouse fulfillment postgres: db is nil")
	}
	return &Locations{db: db}, nil
}

var (
	_ ports.LocationRegistry = (*Locations)(nil)
	// 库位登记用例核「所属库区已登记且属于这个仓」与「库位类型已登记、是否停用」读的是库区登记册与库位类型登记册本身，不另有读路：
	// 两册各自的这一个读方法就是库位要的窄读口。
	_ ports.ZoneLookup         = (*Zones)(nil)
	_ ports.LocationTypeLookup = (*LocationTypes)(nil)
)

func (repository *Locations) SaveLocationRevision(
	ctx context.Context,
	revision domain.LocationRevision,
) (ports.RegistrySaveOutcome, error) {
	const operation = "save location revision"
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.RegistrySaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}

	content := revision.Content()
	// 没带库位类型、没给优先级就写 NULL：「没有」与「有」是不同的内容，库里不取任何默认值。
	var locationType *string
	if code, has := content.LocationType(); has {
		text := code.String()
		locationType = &text
	}
	var priority *int32
	if content.Priority().Given() {
		value := content.Priority().Value()
		priority = &value
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.location_revision
			(tenant_id, warehouse_code, location_code, revision, zone_code, location_type_code, priority, deactivated, registrant, registered_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 ON CONFLICT (tenant_id, warehouse_code, location_code, revision) DO NOTHING`,
		revision.Tenant().String(), revision.Warehouse().String(), revision.Code().String(), revision.Revision(),
		content.ZoneCode().String(), locationType, priority, content.Deactivated(),
		revision.Registrant().String(), revision.RegisteredAt(),
	)
	if err != nil {
		return ports.RegistrySaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() > 0 {
		return ports.RegistryRevisionSaved, nil
	}

	existing, found, err := loadLocationRevision(
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

func (repository *Locations) LoadLatestLocation(
	ctx context.Context,
	tenant domain.TenantID,
	warehouse domain.WarehouseCode,
	code domain.LocationCode,
) (domain.LocationRevision, bool, error) {
	const operation = "load latest location"
	if tenant.String() == "" || warehouse.String() == "" || code.String() == "" {
		return domain.LocationRevision{}, false, fmt.Errorf("%s: tenant, warehouse code and location code are required", operation)
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.LocationRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	stored, err := scanStoredLocation(querier.QueryRow(ctx,
		`SELECT `+locationColumns+`
		   FROM warehouse_fulfillment.location_revision
		  WHERE tenant_id = $1 AND warehouse_code = $2 AND location_code = $3
		  ORDER BY revision DESC
		  LIMIT 1`,
		tenant.String(), warehouse.String(), code.String()))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.LocationRevision{}, false, nil
	}
	if err != nil {
		return domain.LocationRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	revision, err := locationRevisionOfStored(stored)
	if err != nil {
		return domain.LocationRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return revision, true, nil
}

func (repository *Locations) LoadLocationRevision(
	ctx context.Context,
	tenant domain.TenantID,
	warehouse domain.WarehouseCode,
	code domain.LocationCode,
	revision int,
) (domain.LocationRevision, bool, error) {
	const operation = "load location revision"
	if tenant.String() == "" || warehouse.String() == "" || code.String() == "" {
		return domain.LocationRevision{}, false, fmt.Errorf("%s: tenant, warehouse code and location code are required", operation)
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.LocationRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	loaded, found, err := loadLocationRevision(ctx, querier, tenant, warehouse, code, revision)
	if err != nil {
		return domain.LocationRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return loaded, found, nil
}

// loadLocationRevision 按键读回一笔修订；保存撞键后的读回与点读共用这一句，免得长出第二套读回转写。
func loadLocationRevision(
	ctx context.Context,
	querier bentopg.Querier,
	tenant domain.TenantID,
	warehouse domain.WarehouseCode,
	code domain.LocationCode,
	revision int,
) (domain.LocationRevision, bool, error) {
	stored, err := scanStoredLocation(querier.QueryRow(ctx,
		`SELECT `+locationColumns+`
		   FROM warehouse_fulfillment.location_revision
		  WHERE tenant_id = $1 AND warehouse_code = $2 AND location_code = $3 AND revision = $4`,
		tenant.String(), warehouse.String(), code.String(), revision))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.LocationRevision{}, false, nil
	}
	if err != nil {
		return domain.LocationRevision{}, false, err
	}
	loaded, err := locationRevisionOfStored(stored)
	if err != nil {
		return domain.LocationRevision{}, false, err
	}
	return loaded, true, nil
}
