package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// readCataloguePage 跑一册的总数与取页两句，切出本页（ADR-0144 决定五、六）。两句须由同一份 Plan 拼出，总数与本页才出自同一组
// 条件。scan 读回取页那一句的一行；position 给出一行的游标位置，它的值须与 ORDER BY 用的那几列逐位相同——装回时改了写法或精度，
// 键集比较就会把本页末行再取回来，或漏掉与它同值的行（cataloguepage.Finish 头注）。
func readCataloguePage[Row any](
	ctx context.Context,
	querier bentopg.Querier,
	query cataloguepage.Query,
	plan cataloguepage.Plan,
	limit int,
	countSQL, pageSQL string,
	scan func(pgx.Row) (Row, error),
	position func(row Row, sortField string) cataloguepage.Position,
) (cataloguepage.Result[Row], error) {
	var total int64
	if err := querier.QueryRow(ctx, countSQL, plan.Args...).Scan(&total); err != nil {
		return cataloguepage.Result[Row]{}, err
	}
	rows, err := querier.Query(ctx, pageSQL, plan.PageArgs...)
	if err != nil {
		return cataloguepage.Result[Row]{}, err
	}
	defer rows.Close()
	// 空页答空切片而不是 nil，与迁移前各读口一致：直接读读口的调用方（批处理、用例）拿到的空册不变形。
	fetched := make([]Row, 0, limit+1)
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
	return cataloguepage.Finish(query, fetched, limit, total, position)
}
