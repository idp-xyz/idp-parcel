package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// openedPage 是一次取页的前半段：按查询拼好的片段、读执行器与同一组条件下的总数（ADR-0144 决定五）。
type openedPage struct {
	plan    cataloguepage.Plan
	querier bentopg.Querier
	total   int64
}

// openPage 校验页大小、按查询拼出片段、数总数。countSource 是数总数的取数来源：取页时只为补列而 LEFT JOIN 进来的表
// 不改变行数，不必进这一句。limit 非正是调用方编程错误——静默答一页会把「忘了传」变成一个没人决定过的页大小。
func openPage(
	ctx context.Context,
	db *bentopg.DB,
	binding *cataloguepage.Binding,
	query cataloguepage.Query,
	limit int,
	countSource, base string,
	args ...any,
) (openedPage, error) {
	if limit < 1 {
		return openedPage{}, fmt.Errorf("limit must be positive, got %d", limit)
	}
	plan, err := binding.Plan(query, base, args, limit)
	if err != nil {
		return openedPage{}, err
	}
	querier, err := db.ReadExecutor(ctx)
	if err != nil {
		return openedPage{}, err
	}
	var total int64
	if err := querier.QueryRow(ctx, plan.CountSQL(countSource), plan.Args...).Scan(&total); err != nil {
		return openedPage{}, err
	}
	return openedPage{plan: plan, querier: querier, total: total}, nil
}

// rows 取至多 limit+1 行，多出的那一行由 cataloguepage.Finish 拿去判有没有下一页。
func (page openedPage) rows(ctx context.Context, selection, source string) (pgx.Rows, error) {
	return page.querier.Query(ctx, page.plan.PageSQL(selection, source), page.plan.PageArgs...)
}

// pageEntries 把扫描行换成端口行。游标在 Finish 时已按扫描行里 ORDER BY 那几列的原值编好，重建出来的领域对象不再参与。
func pageEntries[Scanned, Entry any](
	result cataloguepage.Result[Scanned],
	entry func(Scanned) Entry,
) cataloguepage.Result[Entry] {
	entries := make([]Entry, 0, len(result.Rows))
	for _, row := range result.Rows {
		entries = append(entries, entry(row))
	}
	return cataloguepage.Result[Entry]{Rows: entries, Next: result.Next, Total: result.Total}
}
