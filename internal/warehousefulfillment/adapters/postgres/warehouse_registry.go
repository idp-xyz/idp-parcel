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

// Warehouses 实现 ports.WarehouseRegistry：仓修订表（0001 迁移）的只增登记与读回。撞键不覆盖：同内容重放，异内容冲突
// （ADR-0031）；比的是完整内容，不比登记责任方与登记时刻。
//
// 并发提交同一修订号的两笔靠主键守：先插入的那一笔落册，后插入的那一笔在行锁上等到先一笔提交，再由 ON CONFLICT DO NOTHING
// 得零行，随后读回已落册的那一笔按内容答已在册或冲突。事务是 READ COMMITTED，等锁之后的读看得见刚提交的那一行。
type Warehouses struct {
	db *bentopg.DB
}

func NewWarehouses(db *bentopg.DB) (*Warehouses, error) {
	if db == nil {
		return nil, fmt.Errorf("warehouse fulfillment postgres: db is nil")
	}
	return &Warehouses{db: db}, nil
}

var (
	_ ports.WarehouseRegistry = (*Warehouses)(nil)
	// 不指名仓的册核「本租户有没有登记过仓」读的也是仓登记册本身，不另有一条读路。
	_ ports.TenantWarehouseLookup = (*Warehouses)(nil)
)

// HasAnyWarehouse 回答本租户在仓册上有没有任何一笔修订。别的租户登记的仓不算；停用的仓也算（只追加的册里登记过就留着）。
func (repository *Warehouses) HasAnyWarehouse(ctx context.Context, tenant domain.TenantID) (bool, error) {
	const operation = "has any warehouse"
	if tenant.String() == "" {
		return false, fmt.Errorf("%s: tenant is required", operation)
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("%s: %w", operation, err)
	}
	var registered bool
	if err := querier.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM warehouse_fulfillment.warehouse_revision WHERE tenant_id = $1)`,
		tenant.String()).Scan(&registered); err != nil {
		return false, fmt.Errorf("%s: %w", operation, err)
	}
	return registered, nil
}

func (repository *Warehouses) SaveWarehouseRevision(
	ctx context.Context,
	revision domain.WarehouseRevision,
) (ports.WarehouseSaveOutcome, error) {
	const operation = "save warehouse revision"
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.WarehouseSaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}

	content := revision.Content()
	var zone, node *string
	if content.BusinessTimeZone().Registered() {
		value := content.BusinessTimeZone().String()
		zone = &value
	}
	if content.ColocatedNode().Named() {
		value := content.ColocatedNode().String()
		node = &value
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.warehouse_revision
			(tenant_id, warehouse_code, revision, warehouse_name, kind, country_code, address_text,
			 business_time_zone, colocated_node, deactivated, registrant, registered_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 ON CONFLICT (tenant_id, warehouse_code, revision) DO NOTHING`,
		revision.Tenant().String(), revision.Code().String(), revision.Revision(),
		content.Name().String(), content.Kind().String(),
		content.Address().Country().String(), content.Address().Text().String(),
		zone, node, content.Deactivated(),
		revision.Registrant().String(), revision.RegisteredAt(),
	)
	if err != nil {
		return ports.WarehouseSaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() > 0 {
		return ports.WarehouseRevisionSaved, nil
	}

	existing, found, err := loadWarehouseRevision(ctx, executor, revision.Tenant(), revision.Code(), revision.Revision())
	if err != nil {
		return ports.WarehouseSaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	if !found {
		return ports.WarehouseSaveOutcomeInvalid, fmt.Errorf("%s: 撞键后读不回既有行", operation)
	}
	if existing.SameContentAs(revision) {
		return ports.WarehouseRevisionAlreadyRegistered, nil
	}
	return ports.WarehouseRevisionContentConflict, nil
}

func (repository *Warehouses) LoadLatestWarehouse(
	ctx context.Context,
	tenant domain.TenantID,
	code domain.WarehouseCode,
) (domain.WarehouseRevision, bool, error) {
	const operation = "load latest warehouse"
	if tenant.String() == "" || code.String() == "" {
		return domain.WarehouseRevision{}, false, fmt.Errorf("%s: tenant and warehouse code are required", operation)
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.WarehouseRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	stored, err := scanStoredWarehouse(querier.QueryRow(ctx,
		`SELECT `+warehouseColumns+`
		   FROM warehouse_fulfillment.warehouse_revision
		  WHERE tenant_id = $1 AND warehouse_code = $2
		  ORDER BY revision DESC
		  LIMIT 1`,
		tenant.String(), code.String()))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.WarehouseRevision{}, false, nil
	}
	if err != nil {
		return domain.WarehouseRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	revision, err := revisionOfStored(stored)
	if err != nil {
		return domain.WarehouseRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return revision, true, nil
}

func (repository *Warehouses) LoadWarehouseRevision(
	ctx context.Context,
	tenant domain.TenantID,
	code domain.WarehouseCode,
	revision int,
) (domain.WarehouseRevision, bool, error) {
	const operation = "load warehouse revision"
	if tenant.String() == "" || code.String() == "" {
		return domain.WarehouseRevision{}, false, fmt.Errorf("%s: tenant and warehouse code are required", operation)
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.WarehouseRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	loaded, found, err := loadWarehouseRevision(ctx, querier, tenant, code, revision)
	if err != nil {
		return domain.WarehouseRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return loaded, found, nil
}

// loadWarehouseRevision 按键读回一笔修订；保存撞键后的读回与点读共用这一句，免得长出第二套读回转写。
func loadWarehouseRevision(
	ctx context.Context,
	querier bentopg.Querier,
	tenant domain.TenantID,
	code domain.WarehouseCode,
	revision int,
) (domain.WarehouseRevision, bool, error) {
	stored, err := scanStoredWarehouse(querier.QueryRow(ctx,
		`SELECT `+warehouseColumns+`
		   FROM warehouse_fulfillment.warehouse_revision
		  WHERE tenant_id = $1 AND warehouse_code = $2 AND revision = $3`,
		tenant.String(), code.String(), revision))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.WarehouseRevision{}, false, nil
	}
	if err != nil {
		return domain.WarehouseRevision{}, false, err
	}
	loaded, err := revisionOfStored(stored)
	if err != nil {
		return domain.WarehouseRevision{}, false, err
	}
	return loaded, true, nil
}
