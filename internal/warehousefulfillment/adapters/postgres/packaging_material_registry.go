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

// PackagingMaterials 实现 ports.PackagingMaterialRegistry：包材修订表（0004 迁移）的只增登记与读回。撞键不覆盖：同内容重放，异内容冲突
// （ADR-0031）；比的是完整内容（数值按数值比，单位按码逐字比），不比登记责任方与登记时刻。
//
// 并发提交同一修订号的两笔靠主键守，机制同 Warehouses：先插入的那一笔落册，后插入的那一笔在行锁上等到先一笔提交，再由
// ON CONFLICT DO NOTHING 得零行，随后读回已落册的那一笔按内容答已在册或冲突。事务是 READ COMMITTED，等锁之后的读看得见刚提交的那一行。
type PackagingMaterials struct {
	db *bentopg.DB
}

func NewPackagingMaterials(db *bentopg.DB) (*PackagingMaterials, error) {
	if db == nil {
		return nil, fmt.Errorf("warehouse fulfillment postgres: db is nil")
	}
	return &PackagingMaterials{db: db}, nil
}

var _ ports.PackagingMaterialRegistry = (*PackagingMaterials)(nil)

// SavePackagingMaterialRevision 落一笔包材修订。四个数值以规范写法的文本交给 numeric 列（Go 的字符串经 pgx 以文本格式送出，库按精确十进制
// 收），不经浮点。
func (repository *PackagingMaterials) SavePackagingMaterialRevision(
	ctx context.Context,
	revision domain.PackagingMaterialRevision,
) (ports.RegistrySaveOutcome, error) {
	const operation = "save packaging material revision"
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.RegistrySaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}

	content := revision.Content()
	tag, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.packaging_material_revision
			(tenant_id, packaging_material_code, revision, packaging_material_name,
			 length_value, width_value, height_value, dimensions_unit,
			 weight_value, weight_unit, deactivated, registrant, registered_at)
		 VALUES ($1, $2, $3, $4, $5::numeric, $6::numeric, $7::numeric, $8, $9::numeric, $10, $11, $12, $13)
		 ON CONFLICT (tenant_id, packaging_material_code, revision) DO NOTHING`,
		revision.Tenant().String(), revision.Code().String(), revision.Revision(), content.Name().String(),
		content.Length().String(), content.Width().String(), content.Height().String(), content.DimensionsUnit().String(),
		content.Weight().String(), content.WeightUnit().String(), content.Deactivated(),
		revision.Registrant().String(), revision.RegisteredAt(),
	)
	if err != nil {
		return ports.RegistrySaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() > 0 {
		return ports.RegistryRevisionSaved, nil
	}

	existing, found, err := loadPackagingMaterialRevision(ctx, executor, revision.Tenant(), revision.Code(), revision.Revision())
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

func (repository *PackagingMaterials) LoadLatestPackagingMaterial(
	ctx context.Context,
	tenant domain.TenantID,
	code domain.PackagingMaterialCode,
) (domain.PackagingMaterialRevision, bool, error) {
	const operation = "load latest packaging material"
	if tenant.String() == "" || code.String() == "" {
		return domain.PackagingMaterialRevision{}, false, fmt.Errorf("%s: tenant and packaging material code are required", operation)
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.PackagingMaterialRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	stored, err := scanStoredPackagingMaterial(querier.QueryRow(ctx,
		`SELECT `+packagingMaterialColumns+`
		   FROM warehouse_fulfillment.packaging_material_revision
		  WHERE tenant_id = $1 AND packaging_material_code = $2
		  ORDER BY revision DESC
		  LIMIT 1`,
		tenant.String(), code.String()))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PackagingMaterialRevision{}, false, nil
	}
	if err != nil {
		return domain.PackagingMaterialRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	revision, err := packagingMaterialRevisionOfStored(stored)
	if err != nil {
		return domain.PackagingMaterialRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return revision, true, nil
}

func (repository *PackagingMaterials) LoadPackagingMaterialRevision(
	ctx context.Context,
	tenant domain.TenantID,
	code domain.PackagingMaterialCode,
	revision int,
) (domain.PackagingMaterialRevision, bool, error) {
	const operation = "load packaging material revision"
	if tenant.String() == "" || code.String() == "" {
		return domain.PackagingMaterialRevision{}, false, fmt.Errorf("%s: tenant and packaging material code are required", operation)
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.PackagingMaterialRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	loaded, found, err := loadPackagingMaterialRevision(ctx, querier, tenant, code, revision)
	if err != nil {
		return domain.PackagingMaterialRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return loaded, found, nil
}

// loadPackagingMaterialRevision 按键读回一笔修订；保存撞键后的读回与点读共用这一句，免得长出第二套读回转写。
func loadPackagingMaterialRevision(
	ctx context.Context,
	querier bentopg.Querier,
	tenant domain.TenantID,
	code domain.PackagingMaterialCode,
	revision int,
) (domain.PackagingMaterialRevision, bool, error) {
	stored, err := scanStoredPackagingMaterial(querier.QueryRow(ctx,
		`SELECT `+packagingMaterialColumns+`
		   FROM warehouse_fulfillment.packaging_material_revision
		  WHERE tenant_id = $1 AND packaging_material_code = $2 AND revision = $3`,
		tenant.String(), code.String(), revision))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PackagingMaterialRevision{}, false, nil
	}
	if err != nil {
		return domain.PackagingMaterialRevision{}, false, err
	}
	loaded, err := packagingMaterialRevisionOfStored(stored)
	if err != nil {
		return domain.PackagingMaterialRevision{}, false, err
	}
	return loaded, true, nil
}
