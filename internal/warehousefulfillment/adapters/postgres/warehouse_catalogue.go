package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// WarehouseCatalogue 实现仓册的两个读口：各仓当前修订的分页列表（ADR-0077、ADR-0144）与一个仓的修订史。读的都是已落册的
// 修订，不读在途的写；每条语句显式带租户条件（作用域是身份的一部分，不是过滤器）。
type WarehouseCatalogue struct {
	db *bentopg.DB
}

func NewWarehouseCatalogue(db *bentopg.DB) (*WarehouseCatalogue, error) {
	if db == nil {
		return nil, fmt.Errorf("warehouse fulfillment postgres: db is nil")
	}
	return &WarehouseCatalogue{db: db}, nil
}

var (
	_ ports.WarehouseCatalogueRead = (*WarehouseCatalogue)(nil)
	_ ports.WarehouseHistoryRead   = (*WarehouseCatalogue)(nil)
)

// warehouseListed 是总数与取页共用的前半句：每个仓取它修订号最大的那一笔。停用标记另导出一列文本 'true' / 'false'，
// 供是否停用这个筛选维按词表比（布尔维的落列约定见 cataloguepage.BooleanVocabulary）。筛选维、q 与键集都落在 listed 的列上。
const warehouseListed = `WITH latest AS (
        SELECT DISTINCT ON (warehouse_code) *
          FROM warehouse_fulfillment.warehouse_revision
         WHERE tenant_id = $1
         ORDER BY warehouse_code, revision DESC
     ), listed AS (
        SELECT latest.*, latest.deactivated::text AS deactivated_word
          FROM latest
     )`

// warehouseListedRestricted 在租户条件上再加仓编码集。$2 是调用方放进 Plan 参数前部的编码数组；
// 空数组不匹配任何行，那是什么都看不见，不是全部。
const warehouseListedRestricted = `WITH latest AS (
        SELECT DISTINCT ON (warehouse_code) *
          FROM warehouse_fulfillment.warehouse_revision
         WHERE tenant_id = $1 AND warehouse_code = ANY($2)
         ORDER BY warehouse_code, revision DESC
     ), listed AS (
        SELECT latest.*, latest.deactivated::text AS deactivated_word
          FROM latest
     )`

// warehouseListBinding：q 覆盖仓编码与名称——登记表单与下游挑仓时认的就是这两格，地址正文不进检索。
var warehouseListBinding = ports.WarehousePages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"warehouseCode": "warehouse_code"},
	Identity: []string{"warehouse_code"},
	Filters:  map[string]string{"deactivated": "deactivated_word"},
	Keyword:  []string{"warehouse_code", "warehouse_name"},
})

// ListWarehouses 取一页各仓的当前修订。总数与本页是两条语句，ReadExecutor 不保证同一快照：两句之间新登的一笔会让 total
// 与本页差一行，下一次取页即对上。
func (catalogue *WarehouseCatalogue) ListWarehouses(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
	scope ports.WarehouseScope,
) (cataloguepage.Result[ports.WarehouseRow], error) {
	const operation = "list warehouses"
	fail := func(err error) (cataloguepage.Result[ports.WarehouseRow], error) {
		return cataloguepage.Result[ports.WarehouseRow]{}, fmt.Errorf("%s: %w", operation, err)
	}
	if tenant.String() == "" {
		return fail(fmt.Errorf("tenant is required"))
	}
	// 把「忘了传页大小」挡在读口上：静默答一页会把缺参变成一个没人决定过的页大小（ADR-0077 决定五）。
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	listed := warehouseListed
	args := []any{tenant.String()}
	if scope.Restricted {
		listed = warehouseListedRestricted
		codes := scope.Codes
		if codes == nil {
			codes = []string{}
		}
		args = append(args, codes)
	}
	plan, err := warehouseListBinding.Plan(query, "", args, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, listed+" "+plan.CountSQL("listed"), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	fetched, err := collectWarehouseRows(ctx, querier,
		listed+" "+plan.PageSQL(warehouseColumns, "listed"), plan.PageArgs, limit+1)
	if err != nil {
		return fail(err)
	}
	// 位置的值取自扫描到的行，不取领域对象里的副本：键集比较要的是 ORDER BY 那一列原样的值。
	page, err := cataloguepage.Finish(query, fetched, limit, total,
		func(row ports.WarehouseRow, _ string) cataloguepage.Position {
			return cataloguepage.Position{Value: row.WarehouseCode, Identity: []string{row.WarehouseCode}}
		})
	if err != nil {
		return fail(err)
	}
	return page, nil
}

// ListWarehouseRevisions 取一个仓册上的全部修订，按修订号升序。不在册与别的租户的同编码仓同形：空切片、无错。
func (catalogue *WarehouseCatalogue) ListWarehouseRevisions(
	ctx context.Context,
	tenant domain.TenantID,
	code domain.WarehouseCode,
) ([]ports.WarehouseRow, error) {
	const operation = "list warehouse revisions"
	if tenant.String() == "" || code.String() == "" {
		return nil, fmt.Errorf("%s: tenant and warehouse code are required", operation)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	rows, err := collectWarehouseRows(ctx, querier,
		`SELECT `+warehouseColumns+`
		   FROM warehouse_fulfillment.warehouse_revision
		  WHERE tenant_id = $1 AND warehouse_code = $2
		  ORDER BY revision ASC`,
		[]any{tenant.String(), code.String()}, 4)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	return rows, nil
}

func collectWarehouseRows(
	ctx context.Context,
	querier bentopg.Querier,
	statement string,
	args []any,
	capacity int,
) ([]ports.WarehouseRow, error) {
	rows, err := querier.Query(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	collected := make([]ports.WarehouseRow, 0, capacity)
	for rows.Next() {
		stored, err := scanStoredWarehouse(rows)
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
