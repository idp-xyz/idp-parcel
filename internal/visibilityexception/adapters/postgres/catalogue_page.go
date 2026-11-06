package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// pageSources 是一册取页用的取数来源与取列。数总数可以少连几张表：只为本页多带几格的左联与侧联每行至多补一行、不增不减
// 行数，数总数时不连它们；但筛选维与 q 用到的列在两个来源上都得在场，total 才与本页出自同一组条件（ADR-0144 决定五）。
type pageSources struct {
	count     string
	page      string
	selection string
}

// readPage 按一册的落列声明取一页：先在同一组条件下数总数，再取游标之后的一页，由 cataloguepage.Finish 切出本页与下一游标。
// tenantCondition 是以 $1 为租户的固定条件（ADR-0003）。scan 把一行读成本册的行；position 从行上 ORDER BY 用的那几格取游标
// 位置，所以那几格必须照列原值读回，不经领域重建。limit 非正是调用方编程错误，静默答一页会把「忘了传」变成一个没人决定过的
// 页大小。
func readPage[Row any](
	ctx context.Context,
	db *bentopg.DB,
	binding *cataloguepage.Binding,
	query cataloguepage.Query,
	tenant domain.TenantID,
	tenantCondition string,
	limit int,
	sources pageSources,
	scan func(pgx.Rows) (Row, error),
	position func(row Row, sortField string) cataloguepage.Position,
) (cataloguepage.Result[Row], error) {
	if limit < 1 {
		return cataloguepage.Result[Row]{}, fmt.Errorf("limit must be positive, got %d", limit)
	}
	plan, err := binding.Plan(query, tenantCondition, []any{tenant.String()}, limit)
	if err != nil {
		return cataloguepage.Result[Row]{}, err
	}
	querier, err := db.ReadExecutor(ctx)
	if err != nil {
		return cataloguepage.Result[Row]{}, err
	}

	var total int64
	if err := querier.QueryRow(ctx, plan.CountSQL(sources.count), plan.Args...).Scan(&total); err != nil {
		return cataloguepage.Result[Row]{}, err
	}
	rows, err := querier.Query(ctx, plan.PageSQL(sources.selection, sources.page), plan.PageArgs...)
	if err != nil {
		return cataloguepage.Result[Row]{}, err
	}
	defer rows.Close()

	var fetched []Row
	for rows.Next() {
		row, err := scan(rows)
		if err != nil {
			return cataloguepage.Result[Row]{}, err
		}
		fetched = append(fetched, row)
	}
	if err := rows.Err(); err != nil {
		return cataloguepage.Result[Row]{}, err
	}
	rows.Close()
	return cataloguepage.Finish(query, fetched, limit, total, position)
}
