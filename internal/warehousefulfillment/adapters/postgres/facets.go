package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// facetQuerier 是读面执行一句分面计数要的口。ReadExecutor 交回的就是它。
type facetQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// listedFacets 数本册点名的分面（ADR-0378 决定五）。没点名的册一句都不发，交回 nil。
// listed 是取页用的同一段 WITH，导出名是 listed；args 是这段已经占用的参数。
func listedFacets(
	ctx context.Context,
	querier facetQuerier,
	binding *cataloguepage.Binding,
	listed string,
	query cataloguepage.Query,
	args []any,
) (cataloguepage.Facets, error) {
	plans, err := binding.FacetPlans(query, "", args)
	if err != nil || len(plans) == 0 {
		return nil, err
	}
	facets := make(cataloguepage.Facets, len(plans))
	for _, plan := range plans {
		found, err := facetValueCounts(ctx, querier, listed+" "+plan.SQL("listed"), plan.Args)
		if err != nil {
			return nil, err
		}
		facets[plan.Dimension] = plan.Counts(found)
	}
	return facets, nil
}

func facetValueCounts(ctx context.Context, querier facetQuerier, statement string, args []any) (map[string]int64, error) {
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

// withListedFacets 把分面计数补进已经切好的一页。读口没点名分面时原样交回。
func withListedFacets[Row any](
	ctx context.Context,
	querier facetQuerier,
	binding *cataloguepage.Binding,
	listed string,
	query cataloguepage.Query,
	args []any,
	page cataloguepage.Result[Row],
) (cataloguepage.Result[Row], error) {
	facets, err := listedFacets(ctx, querier, binding, listed, query, args)
	if err != nil {
		return cataloguepage.Result[Row]{}, err
	}
	page.Facets = facets
	return page, nil
}
