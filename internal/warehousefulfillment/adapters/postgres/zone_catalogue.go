package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// ZoneCatalogue 实现库区册的读口：各库区当前修订的分页列表（ADR-0077、ADR-0144）与一个库区的修订史。读的都是已落册的
// 修订，不读在途的写；每条语句显式带租户条件（作用域是身份的一部分，不是过滤器）。
type ZoneCatalogue struct {
	db *bentopg.DB
}

func NewZoneCatalogue(db *bentopg.DB) (*ZoneCatalogue, error) {
	if db == nil {
		return nil, fmt.Errorf("warehouse fulfillment postgres: db is nil")
	}
	return &ZoneCatalogue{db: db}, nil
}

var (
	_ ports.ZoneCatalogueRead = (*ZoneCatalogue)(nil)
	_ ports.ZoneHistoryRead   = (*ZoneCatalogue)(nil)
)

// zoneListed 是总数与取页共用的前半句：每个库区（仓编码 + 库区编码）取它修订号最大的那一笔。停用标记另导出一列文本
// 'true' / 'false'，供是否停用这个筛选维按词表比（布尔维的落列约定见 cataloguepage.BooleanVocabulary）。
const zoneListed = `WITH latest AS (
        SELECT DISTINCT ON (warehouse_code, zone_code) *
          FROM warehouse_fulfillment.zone_revision
         WHERE tenant_id = $1
         ORDER BY warehouse_code, zone_code, revision DESC
     ), listed AS (
        SELECT latest.*, latest.deactivated::text AS deactivated_word
          FROM latest
     )`

// zoneListBinding：排序维 warehouseCode 落仓编码列，行标识是（仓编码，库区编码）两段，所以缺省序是先仓后库区；q 覆盖仓编码、
// 库区编码与名称——登记表单与下游挑库区时认的就是它们。
var zoneListBinding = ports.ZonePages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"warehouseCode": "warehouse_code"},
	Identity: []string{"warehouse_code", "zone_code"},
	Filters:  map[string]string{"warehouseCode": "warehouse_code", "deactivated": "deactivated_word"},
	Keyword:  []string{"warehouse_code", "zone_code", "zone_name"},
})

// ListZones 取一页各库区的当前修订。总数与本页是两条语句，ReadExecutor 不保证同一快照：两句之间新登的一笔会让 total 与本页
// 差一行，下一次取页即对上。
func (catalogue *ZoneCatalogue) ListZones(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.ZoneRow], error) {
	const operation = "list zones"
	fail := func(err error) (cataloguepage.Result[ports.ZoneRow], error) {
		return cataloguepage.Result[ports.ZoneRow]{}, fmt.Errorf("%s: %w", operation, err)
	}
	if tenant.String() == "" {
		return fail(fmt.Errorf("tenant is required"))
	}
	// 把「忘了传页大小」挡在读口上：静默答一页会把缺参变成一个没人决定过的页大小（ADR-0077 决定五）。
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := zoneListBinding.Plan(query, "", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, zoneListed+" "+plan.CountSQL("listed"), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	fetched, err := collectZoneRows(ctx, querier,
		zoneListed+" "+plan.PageSQL(zoneColumns, "listed"), plan.PageArgs, limit+1)
	if err != nil {
		return fail(err)
	}
	// 位置的值取自扫描到的行，不取领域对象里的副本：键集比较要的是 ORDER BY 那几列原样的值。
	page, err := cataloguepage.Finish(query, fetched, limit, total,
		func(row ports.ZoneRow, _ string) cataloguepage.Position {
			return cataloguepage.Position{Value: row.WarehouseCode, Identity: []string{row.WarehouseCode, row.ZoneCode}}
		})
	if err != nil {
		return fail(err)
	}
	return withListedFacets(ctx, querier, zoneListBinding, zoneListed, query, []any{tenant.String()}, page)
}

// ListZoneRevisions 取一个库区册上的全部修订，按修订号升序。不在册与别的租户的同编码库区同形：空切片、无错。
func (catalogue *ZoneCatalogue) ListZoneRevisions(
	ctx context.Context,
	tenant domain.TenantID,
	warehouse domain.WarehouseCode,
	code domain.ZoneCode,
) ([]ports.ZoneRow, error) {
	const operation = "list zone revisions"
	if tenant.String() == "" || warehouse.String() == "" || code.String() == "" {
		return nil, fmt.Errorf("%s: tenant, warehouse code and zone code are required", operation)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	rows, err := collectZoneRows(ctx, querier,
		`SELECT `+zoneColumns+`
		   FROM warehouse_fulfillment.zone_revision
		  WHERE tenant_id = $1 AND warehouse_code = $2 AND zone_code = $3
		  ORDER BY revision ASC`,
		[]any{tenant.String(), warehouse.String(), code.String()}, 4)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	return rows, nil
}

func collectZoneRows(
	ctx context.Context,
	querier bentopg.Querier,
	statement string,
	args []any,
	capacity int,
) ([]ports.ZoneRow, error) {
	rows, err := querier.Query(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	collected := make([]ports.ZoneRow, 0, capacity)
	for rows.Next() {
		stored, err := scanStoredZone(rows)
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
