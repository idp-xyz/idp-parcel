package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// LocationCatalogue 实现库位册的读口：各库位当前修订的分页列表（ADR-0077、ADR-0144）与一个库位的修订史。读的都是已落册的
// 修订，不读在途的写；每条语句显式带租户条件（作用域是身份的一部分，不是过滤器）。
type LocationCatalogue struct {
	db *bentopg.DB
}

func NewLocationCatalogue(db *bentopg.DB) (*LocationCatalogue, error) {
	if db == nil {
		return nil, fmt.Errorf("warehouse fulfillment postgres: db is nil")
	}
	return &LocationCatalogue{db: db}, nil
}

var (
	_ ports.LocationCatalogueRead = (*LocationCatalogue)(nil)
	_ ports.LocationHistoryRead   = (*LocationCatalogue)(nil)
)

// locationListed 是总数与取页共用的前半句：每个库位（仓编码 + 库位编码）取它修订号最大的那一笔。停用标记另导出一列文本
// 'true' / 'false'，供是否停用这个筛选维按词表比（布尔维的落列约定见 cataloguepage.BooleanVocabulary）。
const locationListed = `WITH latest AS (
        SELECT DISTINCT ON (warehouse_code, location_code) *
          FROM warehouse_fulfillment.location_revision
         WHERE tenant_id = $1
         ORDER BY warehouse_code, location_code, revision DESC
     ), listed AS (
        SELECT latest.*, latest.deactivated::text AS deactivated_word
          FROM latest
     )`

// locationListBinding：排序维 warehouseCode 落仓编码列，行标识是（仓编码，库位编码）两段，所以缺省序是先仓后库位；筛选维 zoneCode
// 落所属库区列（当前修订的所属库区，登记后不改，所以与历史修订同值）；q 覆盖仓编码、库位编码与所属库区——登记表单与下游挑库位时认的
// 就是它们。库位类型编码是可空列、另有它自己的册可查，不进 q。
var locationListBinding = ports.LocationPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"warehouseCode": "warehouse_code"},
	Identity: []string{"warehouse_code", "location_code"},
	Filters: map[string]string{
		"warehouseCode": "warehouse_code",
		"zoneCode":      "zone_code",
		"deactivated":   "deactivated_word",
	},
	Keyword: []string{"warehouse_code", "location_code", "zone_code"},
})

// ListLocations 取一页各库位的当前修订。总数与本页是两条语句，ReadExecutor 不保证同一快照：两句之间新登的一笔会让 total 与本页
// 差一行，下一次取页即对上。
func (catalogue *LocationCatalogue) ListLocations(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.LocationRow], error) {
	const operation = "list locations"
	fail := func(err error) (cataloguepage.Result[ports.LocationRow], error) {
		return cataloguepage.Result[ports.LocationRow]{}, fmt.Errorf("%s: %w", operation, err)
	}
	if tenant.String() == "" {
		return fail(fmt.Errorf("tenant is required"))
	}
	// 把「忘了传页大小」挡在读口上：静默答一页会把缺参变成一个没人决定过的页大小（ADR-0077 决定五）。
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := locationListBinding.Plan(query, "", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, locationListed+" "+plan.CountSQL("listed"), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	fetched, err := collectLocationRows(ctx, querier,
		locationListed+" "+plan.PageSQL(locationColumns, "listed"), plan.PageArgs, limit+1)
	if err != nil {
		return fail(err)
	}
	// 位置的值取自扫描到的行，不取领域对象里的副本：键集比较要的是 ORDER BY 那几列原样的值。
	page, err := cataloguepage.Finish(query, fetched, limit, total,
		func(row ports.LocationRow, _ string) cataloguepage.Position {
			return cataloguepage.Position{Value: row.WarehouseCode, Identity: []string{row.WarehouseCode, row.LocationCode}}
		})
	if err != nil {
		return fail(err)
	}
	return withListedFacets(ctx, querier, locationListBinding, locationListed, query, []any{tenant.String()}, page)
}

// ListLocationRevisions 取一个库位册上的全部修订，按修订号升序。不在册与别的租户的同编码库位同形：空切片、无错。
func (catalogue *LocationCatalogue) ListLocationRevisions(
	ctx context.Context,
	tenant domain.TenantID,
	warehouse domain.WarehouseCode,
	code domain.LocationCode,
) ([]ports.LocationRow, error) {
	const operation = "list location revisions"
	if tenant.String() == "" || warehouse.String() == "" || code.String() == "" {
		return nil, fmt.Errorf("%s: tenant, warehouse code and location code are required", operation)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	rows, err := collectLocationRows(ctx, querier,
		`SELECT `+locationColumns+`
		   FROM warehouse_fulfillment.location_revision
		  WHERE tenant_id = $1 AND warehouse_code = $2 AND location_code = $3
		  ORDER BY revision ASC`,
		[]any{tenant.String(), warehouse.String(), code.String()}, 4)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	return rows, nil
}

func collectLocationRows(
	ctx context.Context,
	querier bentopg.Querier,
	statement string,
	args []any,
	capacity int,
) ([]ports.LocationRow, error) {
	rows, err := querier.Query(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	collected := make([]ports.LocationRow, 0, capacity)
	for rows.Next() {
		stored, err := scanStoredLocation(rows)
		if err != nil {
			return nil, err
		}
		collected = append(collected, stored.row())
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return collected, nil
}
