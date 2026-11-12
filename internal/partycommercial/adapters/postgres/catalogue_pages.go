package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// catalogueRead 是一册目录在本包里的取数形状（ADR-0144；票 catalogue-read-pagination-rollout/11）：WITH 前半句（导出名一律
// 叫 listed，租户与读面自己的固定条件落在里面）、要取的列、一行的扫描与本页末行的位置。筛选维、q 与键集都落在 listed
// 的导出列上：状态这类导出列只在前半句里才有，条件只能接在它后面。
type catalogueRead[Row any] struct {
	binding   *cataloguepage.Binding
	listed    string
	selection string
	scan      func(pgx.Rows) (Row, error)
	// position 给 Finish 编游标：值取自扫描到的 ORDER BY 那几列，不取领域对象里的副本（理由在 Finish 头注）。
	position func(Row, string) cataloguepage.Position
}

// page 照查询对象取一页：先按同一组条件数总数，再取游标之后的 limit+1 行，交 Finish 切页、编下一页的游标。args 是前半句
// 已占用的参数（$1 起），筛选维、q、键集与页大小的参数接在后面。
//
// 总数与本页是两条语句，ReadExecutor 不保证同一快照：两句之间新登的一笔会让 total 与本页差一行，下一次取页即对上。
func (read catalogueRead[Row]) page(
	ctx context.Context,
	db *bentopg.DB,
	operation string,
	limit int,
	query cataloguepage.Query,
	args ...any,
) (cataloguepage.Result[Row], error) {
	fail := func(err error) (cataloguepage.Result[Row], error) {
		return cataloguepage.Result[Row]{}, fmt.Errorf("%s: %w", operation, err)
	}
	if err := requirePositiveLimit(operation, limit); err != nil {
		return cataloguepage.Result[Row]{}, err
	}
	plan, err := read.binding.Plan(query, "", args, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, read.listed+" "+plan.CountSQL("listed"), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	fetched, err := read.rows(ctx, querier, read.listed+" "+plan.PageSQL(read.selection, "listed"), plan.PageArgs, limit+1)
	if err != nil {
		return fail(err)
	}
	page, err := cataloguepage.Finish(query, fetched, limit, total, read.position)
	if err != nil {
		return fail(err)
	}
	facets, err := read.facets(ctx, querier, query, args)
	if err != nil {
		return fail(err)
	}
	page.Facets = facets
	return page, nil
}

// facets 数本册点名的各分面维（ADR-0378 决定五）：每维一句，条件取本次的 q 与其余各维、不含该维自己。没点名的册一句
// 都不发，交回 nil——答复里就没有 facets 这一格。与总数、取页一样各是一条语句，不保证同一快照（理由见 page 头注）。
func (read catalogueRead[Row]) facets(
	ctx context.Context,
	querier bentopg.Querier,
	query cataloguepage.Query,
	args []any,
) (cataloguepage.Facets, error) {
	plans, err := read.binding.FacetPlans(query, "", args)
	if err != nil || len(plans) == 0 {
		return nil, err
	}
	facets := make(cataloguepage.Facets, len(plans))
	for _, plan := range plans {
		found, err := facetCounts(ctx, querier, read.listed+" "+plan.SQL("listed"), plan.Args)
		if err != nil {
			return nil, err
		}
		facets[plan.Dimension] = plan.Counts(found)
	}
	return facets, nil
}

func facetCounts(ctx context.Context, querier bentopg.Querier, statement string, args []any) (map[string]int64, error) {
	rows, err := querier.Query(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	found := make(map[string]int64)
	for rows.Next() {
		var value string
		var count int64
		if err := rows.Scan(&value, &count); err != nil {
			return nil, err
		}
		found[value] = count
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return found, nil
}

// find 按身份读回一行（ADR-0361）：与取页同一段 listed、同一份列与扫描，只多 key 那两格键过滤——另写一条语句就会长出
// 第二套行转写，理由在 versionKey 头注。args 同 page。
func (read catalogueRead[Row]) find(
	ctx context.Context,
	db *bentopg.DB,
	operation string,
	key *versionKey,
	args ...any,
) (Row, bool, error) {
	var none Row
	querier, err := db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("%s: %w", operation, err)
	}
	filter, args := key.filter(args)
	fetched, err := read.rows(ctx, querier, read.listed+" SELECT "+read.selection+" FROM listed WHERE TRUE"+filter, args, 1)
	if err != nil {
		return none, false, fmt.Errorf("%s: %w", operation, err)
	}
	return firstRow(fetched, nil)
}

func (read catalogueRead[Row]) rows(
	ctx context.Context,
	querier bentopg.Querier,
	statement string,
	args []any,
	capacity int,
) ([]Row, error) {
	rows, err := querier.Query(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	fetched := make([]Row, 0, capacity)
	for rows.Next() {
		row, err := read.scan(rows)
		if err != nil {
			return nil, err
		}
		fetched = append(fetched, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return fetched, nil
}
