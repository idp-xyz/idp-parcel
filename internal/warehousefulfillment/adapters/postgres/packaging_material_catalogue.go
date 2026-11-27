package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// PackagingMaterialCatalogue 实现包材册的读口：各包材当前修订的分页列表（ADR-0077、ADR-0144）与一个包材的修订史。读的都是已落册的修订，
// 不读在途的写；每条语句显式带租户条件（作用域是身份的一部分，不是过滤器）。
type PackagingMaterialCatalogue struct {
	db *bentopg.DB
}

func NewPackagingMaterialCatalogue(db *bentopg.DB) (*PackagingMaterialCatalogue, error) {
	if db == nil {
		return nil, fmt.Errorf("warehouse fulfillment postgres: db is nil")
	}
	return &PackagingMaterialCatalogue{db: db}, nil
}

var (
	_ ports.PackagingMaterialCatalogueRead = (*PackagingMaterialCatalogue)(nil)
	_ ports.PackagingMaterialHistoryRead   = (*PackagingMaterialCatalogue)(nil)
)

// packagingMaterialListed 是总数与取页共用的前半句：每个包材取它修订号最大的那一笔。停用标记另导出一列文本 'true' / 'false'，供是否停用
// 这个筛选维按词表比（布尔维的落列约定见 cataloguepage.BooleanVocabulary）。
const packagingMaterialListed = `WITH latest AS (
        SELECT DISTINCT ON (packaging_material_code) *
          FROM warehouse_fulfillment.packaging_material_revision
         WHERE tenant_id = $1
         ORDER BY packaging_material_code, revision DESC
     ), listed AS (
        SELECT latest.*, latest.deactivated::text AS deactivated_word
          FROM latest
     )`

// packagingMaterialListBinding：排序维 packagingMaterialCode 落包材编码列，行标识就是它；q 覆盖编码与名称——打包选包材时认的就是它们。
var packagingMaterialListBinding = ports.PackagingMaterialPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"packagingMaterialCode": "packaging_material_code"},
	Identity: []string{"packaging_material_code"},
	Filters:  map[string]string{"deactivated": "deactivated_word"},
	Keyword:  []string{"packaging_material_code", "packaging_material_name"},
})

// ListPackagingMaterials 取一页各包材的当前修订。总数与本页是两条语句，ReadExecutor 不保证同一快照：两句之间新登的一笔会让 total 与本页差一行，
// 下一次取页即对上。
func (catalogue *PackagingMaterialCatalogue) ListPackagingMaterials(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.PackagingMaterialRow], error) {
	const operation = "list packaging materials"
	fail := func(err error) (cataloguepage.Result[ports.PackagingMaterialRow], error) {
		return cataloguepage.Result[ports.PackagingMaterialRow]{}, fmt.Errorf("%s: %w", operation, err)
	}
	if tenant.String() == "" {
		return fail(fmt.Errorf("tenant is required"))
	}
	// 把「忘了传页大小」挡在读口上：静默答一页会把缺参变成一个没人决定过的页大小（ADR-0077 决定五）。
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := packagingMaterialListBinding.Plan(query, "", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, packagingMaterialListed+" "+plan.CountSQL("listed"), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	fetched, err := collectPackagingMaterialRows(ctx, querier,
		packagingMaterialListed+" "+plan.PageSQL(packagingMaterialColumns, "listed"), plan.PageArgs, limit+1)
	if err != nil {
		return fail(err)
	}
	// 位置的值取自扫描到的行，不取领域对象里的副本：键集比较要的是 ORDER BY 那几列原样的值。
	page, err := cataloguepage.Finish(query, fetched, limit, total,
		func(row ports.PackagingMaterialRow, _ string) cataloguepage.Position {
			return cataloguepage.Position{Value: row.PackagingMaterialCode, Identity: []string{row.PackagingMaterialCode}}
		})
	if err != nil {
		return fail(err)
	}
	return withListedFacets(ctx, querier, packagingMaterialListBinding, packagingMaterialListed, query, []any{tenant.String()}, page)
}

// ListPackagingMaterialRevisions 取一个包材册上的全部修订，按修订号升序。不在册与别的租户的同编码包材同形：空切片、无错。
func (catalogue *PackagingMaterialCatalogue) ListPackagingMaterialRevisions(
	ctx context.Context,
	tenant domain.TenantID,
	code domain.PackagingMaterialCode,
) ([]ports.PackagingMaterialRow, error) {
	const operation = "list packaging material revisions"
	if tenant.String() == "" || code.String() == "" {
		return nil, fmt.Errorf("%s: tenant and packaging material code are required", operation)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	rows, err := collectPackagingMaterialRows(ctx, querier,
		`SELECT `+packagingMaterialColumns+`
		   FROM warehouse_fulfillment.packaging_material_revision
		  WHERE tenant_id = $1 AND packaging_material_code = $2
		  ORDER BY revision ASC`,
		[]any{tenant.String(), code.String()}, 4)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	return rows, nil
}

func collectPackagingMaterialRows(
	ctx context.Context,
	querier bentopg.Querier,
	statement string,
	args []any,
	capacity int,
) ([]ports.PackagingMaterialRow, error) {
	rows, err := querier.Query(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	collected := make([]ports.PackagingMaterialRow, 0, capacity)
	for rows.Next() {
		stored, err := scanStoredPackagingMaterial(rows)
		if err != nil {
			return nil, err
		}
		row, err := stored.row()
		if err != nil {
			return nil, err
		}
		collected = append(collected, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return collected, nil
}
