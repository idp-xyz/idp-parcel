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

// LocationTypes 实现 ports.LocationTypeRegistry：库位类型修订表（0003 迁移）的只增登记与读回。撞键不覆盖：同内容重放，异内容冲突
// （ADR-0031）；比的是完整内容，不比登记责任方与登记时刻。
//
// 并发提交同一修订号的两笔靠主键守，机制同 Warehouses：先插入的那一笔落册，后插入的那一笔在行锁上等到先一笔提交，再由
// ON CONFLICT DO NOTHING 得零行，随后读回已落册的那一笔按内容答已在册或冲突。事务是 READ COMMITTED，等锁之后的读看得见刚提交的那一行。
type LocationTypes struct {
	db *bentopg.DB
}

func NewLocationTypes(db *bentopg.DB) (*LocationTypes, error) {
	if db == nil {
		return nil, fmt.Errorf("warehouse fulfillment postgres: db is nil")
	}
	return &LocationTypes{db: db}, nil
}

var _ ports.LocationTypeRegistry = (*LocationTypes)(nil)

func (repository *LocationTypes) SaveLocationTypeRevision(
	ctx context.Context,
	revision domain.LocationTypeRevision,
) (ports.RegistrySaveOutcome, error) {
	const operation = "save location type revision"
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.RegistrySaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}

	content := revision.Content()
	tag, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.location_type_revision
			(tenant_id, location_type_code, revision, location_type_name, deactivated, registrant, registered_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (tenant_id, location_type_code, revision) DO NOTHING`,
		revision.Tenant().String(), revision.Code().String(), revision.Revision(),
		content.Name().String(), content.Deactivated(),
		revision.Registrant().String(), revision.RegisteredAt(),
	)
	if err != nil {
		return ports.RegistrySaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() > 0 {
		return ports.RegistryRevisionSaved, nil
	}

	existing, found, err := loadLocationTypeRevision(ctx, executor, revision.Tenant(), revision.Code(), revision.Revision())
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

func (repository *LocationTypes) LoadLatestLocationType(
	ctx context.Context,
	tenant domain.TenantID,
	code domain.LocationTypeCode,
) (domain.LocationTypeRevision, bool, error) {
	const operation = "load latest location type"
	if tenant.String() == "" || code.String() == "" {
		return domain.LocationTypeRevision{}, false, fmt.Errorf("%s: tenant and location type code are required", operation)
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.LocationTypeRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	stored, err := scanStoredLocationType(querier.QueryRow(ctx,
		`SELECT `+locationTypeColumns+`
		   FROM warehouse_fulfillment.location_type_revision
		  WHERE tenant_id = $1 AND location_type_code = $2
		  ORDER BY revision DESC
		  LIMIT 1`,
		tenant.String(), code.String()))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.LocationTypeRevision{}, false, nil
	}
	if err != nil {
		return domain.LocationTypeRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	revision, err := locationTypeRevisionOfStored(stored)
	if err != nil {
		return domain.LocationTypeRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return revision, true, nil
}

func (repository *LocationTypes) LoadLocationTypeRevision(
	ctx context.Context,
	tenant domain.TenantID,
	code domain.LocationTypeCode,
	revision int,
) (domain.LocationTypeRevision, bool, error) {
	const operation = "load location type revision"
	if tenant.String() == "" || code.String() == "" {
		return domain.LocationTypeRevision{}, false, fmt.Errorf("%s: tenant and location type code are required", operation)
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.LocationTypeRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	loaded, found, err := loadLocationTypeRevision(ctx, querier, tenant, code, revision)
	if err != nil {
		return domain.LocationTypeRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return loaded, found, nil
}

// loadLocationTypeRevision 按键读回一笔修订；保存撞键后的读回与点读共用这一句，免得长出第二套读回转写。
func loadLocationTypeRevision(
	ctx context.Context,
	querier bentopg.Querier,
	tenant domain.TenantID,
	code domain.LocationTypeCode,
	revision int,
) (domain.LocationTypeRevision, bool, error) {
	stored, err := scanStoredLocationType(querier.QueryRow(ctx,
		`SELECT `+locationTypeColumns+`
		   FROM warehouse_fulfillment.location_type_revision
		  WHERE tenant_id = $1 AND location_type_code = $2 AND revision = $3`,
		tenant.String(), code.String(), revision))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.LocationTypeRevision{}, false, nil
	}
	if err != nil {
		return domain.LocationTypeRevision{}, false, err
	}
	loaded, err := locationTypeRevisionOfStored(stored)
	if err != nil {
		return domain.LocationTypeRevision{}, false, err
	}
	return loaded, true, nil
}
