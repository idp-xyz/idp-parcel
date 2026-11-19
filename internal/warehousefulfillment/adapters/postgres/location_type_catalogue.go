package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// LocationTypeCatalogue 实现库位类型册的读口：各库位类型当前修订的分页列表（ADR-0077、ADR-0144）与一个库位类型的修订史。读的都是
// 已落册的修订，不读在途的写；每条语句显式带租户条件（作用域是身份的一部分，不是过滤器）。
type LocationTypeCatalogue struct {
	db *bentopg.DB
}

func NewLocationTypeCatalogue(db *bentopg.DB) (*LocationTypeCatalogue, error) {
	if db == nil {
		return nil, fmt.Errorf("warehouse fulfillment postgres: db is nil")
	}
	return &LocationTypeCatalogue{db: db}, nil
}

var (
	_ ports.LocationTypeCatalogueRead = (*LocationTypeCatalogue)(nil)
	_ ports.LocationTypeHistoryRead   = (*LocationTypeCatalogue)(nil)
)

// locationTypeListed 是总数与取页共用的前半句：每个库位类型取它修订号最大的那一笔。停用标记另导出一列文本 'true' / 'false'，供是否停用
// 这个筛选维按词表比（布尔维的落列约定见 cataloguepage.BooleanVocabulary）。
const locationTypeListed = `WITH latest AS (
        SELECT DISTINCT ON (location_type_code) *
          FROM warehouse_fulfillment.location_type_revision
         WHERE tenant_id = $1
         ORDER BY location_type_code, revision DESC
     ), listed AS (
        SELECT latest.*, latest.deactivated::text AS deactivated_word
          FROM latest
     )`

// locationTypeListBinding：排序维 locationTypeCode 落库位类型编码列，行标识就是它；q 覆盖编码与名称——登记表单与下游挑库位类型时认的
// 就是它们。
var locationTypeListBinding = ports.LocationTypePages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"locationTypeCode": "location_type_code"},
	Identity: []string{"location_type_code"},
	Filters:  map[string]string{"deactivated": "deactivated_word"},
	Keyword:  []string{"location_type_code", "location_type_name"},
})

// ListLocationTypes 取一页各库位类型的当前修订。总数与本页是两条语句，ReadExecutor 不保证同一快照：两句之间新登的一笔会让 total 与本页
// 差一行，下一次取页即对上。
func (catalogue *LocationTypeCatalogue) ListLocationTypes(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.LocationTypeRow], error) {
	const operation = "list location types"
	fail := func(err error) (cataloguepage.Result[ports.LocationTypeRow], error) {
		return cataloguepage.Result[ports.LocationTypeRow]{}, fmt.Errorf("%s: %w", operation, err)
	}
	if tenant.String() == "" {
		return fail(fmt.Errorf("tenant is required"))
	}
	// 把「忘了传页大小」挡在读口上：静默答一页会把缺参变成一个没人决定过的页大小（ADR-0077 决定五）。
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := locationTypeListBinding.Plan(query, "", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, locationTypeListed+" "+plan.CountSQL("listed"), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	fetched, err := collectLocationTypeRows(ctx, querier,
		locationTypeListed+" "+plan.PageSQL(locationTypeColumns, "listed"), plan.PageArgs, limit+1)
	if err != nil {
		return fail(err)
	}
	// 位置的值取自扫描到的行，不取领域对象里的副本：键集比较要的是 ORDER BY 那几列原样的值。
	page, err := cataloguepage.Finish(query, fetched, limit, total,
		func(row ports.LocationTypeRow, _ string) cataloguepage.Position {
			return cataloguepage.Position{Value: row.LocationTypeCode, Identity: []string{row.LocationTypeCode}}
		})
	if err != nil {
		return fail(err)
	}
	return page, nil
}

// ListLocationTypeRevisions 取一个库位类型册上的全部修订，按修订号升序。不在册与别的租户的同编码库位类型同形：空切片、无错。
func (catalogue *LocationTypeCatalogue) ListLocationTypeRevisions(
	ctx context.Context,
	tenant domain.TenantID,
	code domain.LocationTypeCode,
) ([]ports.LocationTypeRow, error) {
	const operation = "list location type revisions"
	if tenant.String() == "" || code.String() == "" {
		return nil, fmt.Errorf("%s: tenant and location type code are required", operation)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	rows, err := collectLocationTypeRows(ctx, querier,
		`SELECT `+locationTypeColumns+`
		   FROM warehouse_fulfillment.location_type_revision
		  WHERE tenant_id = $1 AND location_type_code = $2
		  ORDER BY revision ASC`,
		[]any{tenant.String(), code.String()}, 4)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	return rows, nil
}

func collectLocationTypeRows(
	ctx context.Context,
	querier bentopg.Querier,
	statement string,
	args []any,
	capacity int,
) ([]ports.LocationTypeRow, error) {
	rows, err := querier.Query(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	collected := make([]ports.LocationTypeRow, 0, capacity)
	for rows.Next() {
		stored, err := scanStoredLocationType(rows)
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
