package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// GoodsCatalogue 实现商品册的读口：一个货主名下各商品的当前修订、一个商品的当前修订与修订史，以及按码找商品。
// 别的货主的商品与没登记同形。按码找的是各商品的当前修订：商品编码两端无界，别名起含、止不含，没给的一端无界。
type GoodsCatalogue struct {
	db *bentopg.DB
}

func NewGoodsCatalogue(db *bentopg.DB) (*GoodsCatalogue, error) {
	if db == nil {
		return nil, fmt.Errorf("warehouse fulfillment postgres: db is nil")
	}
	return &GoodsCatalogue{db: db}, nil
}

var (
	_ ports.GoodsCatalogueRead = (*GoodsCatalogue)(nil)
	_ ports.GoodsCurrentRead   = (*GoodsCatalogue)(nil)
	_ ports.GoodsHistoryRead   = (*GoodsCatalogue)(nil)
)

const goodsListed = `WITH latest AS (
        SELECT DISTINCT ON (goods_code) *
          FROM warehouse_fulfillment.goods_revision
         WHERE tenant_id = $1 AND owner_account_id = $2
         ORDER BY goods_code, revision DESC
     ), listed AS (
        SELECT latest.* FROM latest
     )`

var goodsListBinding = ports.GoodsPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"goodsCode": "goods_code"},
	Identity: []string{"goods_code"},
	Keyword:  []string{"goods_code", "goods_name"},
})

func (catalogue *GoodsCatalogue) ListGoods(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.GoodsRow], error) {
	const operation = "list goods"
	fail := func(err error) (cataloguepage.Result[ports.GoodsRow], error) {
		return cataloguepage.Result[ports.GoodsRow]{}, fmt.Errorf("%s: %w", operation, err)
	}
	if tenant.String() == "" || owner.String() == "" {
		return fail(fmt.Errorf("tenant and owner are required"))
	}
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := goodsListBinding.Plan(query, "", []any{tenant.String(), owner.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, goodsListed+" "+plan.CountSQL("listed"), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	headers, err := collectGoodsHeaders(ctx, querier, goodsListed+" "+plan.PageSQL(goodsHeaderColumns, "listed"), plan.PageArgs)
	if err != nil {
		return fail(err)
	}
	fetched := make([]ports.GoodsRow, 0, len(headers))
	for _, header := range headers {
		revision, err := goodsRevisionOfStored(ctx, querier, header)
		if err != nil {
			return fail(err)
		}
		fetched = append(fetched, goodsRowOf(revision))
	}
	page, err := cataloguepage.Finish(query, fetched, limit, total,
		func(row ports.GoodsRow, _ string) cataloguepage.Position {
			return cataloguepage.Position{Value: row.GoodsCode, Identity: []string{row.GoodsCode}}
		})
	if err != nil {
		return fail(err)
	}
	return page, nil
}

func (catalogue *GoodsCatalogue) GetCurrentGoods(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	code domain.GoodsCode,
) (ports.GoodsRow, bool, error) {
	const operation = "get current goods"
	if tenant.String() == "" || owner.String() == "" || code.String() == "" {
		return ports.GoodsRow{}, false, fmt.Errorf("%s: tenant, owner and goods code are required", operation)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return ports.GoodsRow{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	revision, found, err := loadLatestGoodsRevision(ctx, querier, tenant, owner, code)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.GoodsRow{}, false, nil
	}
	if err != nil {
		return ports.GoodsRow{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	if !found {
		return ports.GoodsRow{}, false, nil
	}
	return goodsRowOf(revision), true, nil
}

func (catalogue *GoodsCatalogue) FindGoodsByCode(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	code domain.IdentifierCode,
	at time.Time,
) (ports.GoodsRow, bool, error) {
	const operation = "find goods by code"
	if tenant.String() == "" || owner.String() == "" || code.String() == "" {
		return ports.GoodsRow{}, false, fmt.Errorf("%s: tenant, owner and code are required", operation)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return ports.GoodsRow{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	rows, err := querier.Query(ctx,
		`WITH at_revision AS (
		    SELECT DISTINCT ON (goods_code) goods_code, revision
		      FROM warehouse_fulfillment.goods_revision
		     WHERE tenant_id = $1 AND owner_account_id = $2
		       AND registered_at <= $4
		     ORDER BY goods_code, revision DESC
		 )
		 SELECT at_revision.goods_code, at_revision.revision
		   FROM at_revision
		  WHERE at_revision.goods_code = $3
		     OR EXISTS (
		        SELECT 1 FROM warehouse_fulfillment.goods_alias alias
		         WHERE alias.tenant_id = $1 AND alias.owner_account_id = $2
		           AND alias.goods_code = at_revision.goods_code AND alias.revision = at_revision.revision
		           AND alias.alias_code = $3
		           AND (alias.valid_from IS NULL OR alias.valid_from <= $4)
		           AND (alias.valid_to IS NULL OR $4 < alias.valid_to)
		     )`,
		tenant.String(), owner.String(), code.String(), at.UTC())
	if err != nil {
		return ports.GoodsRow{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	defer rows.Close()
	type match struct {
		goodsCode string
		revision  int
	}
	var matched []match
	for rows.Next() {
		var item match
		if err := rows.Scan(&item.goodsCode, &item.revision); err != nil {
			return ports.GoodsRow{}, false, fmt.Errorf("%s: %w", operation, err)
		}
		matched = append(matched, item)
	}
	if err := rows.Err(); err != nil {
		return ports.GoodsRow{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	switch len(matched) {
	case 0:
		return ports.GoodsRow{}, false, nil
	case 1:
		goodsCode, err := domain.NewGoodsCode(matched[0].goodsCode)
		if err != nil {
			return ports.GoodsRow{}, false, fmt.Errorf("%s: %w", operation, err)
		}
		revision, found, err := loadGoodsRevision(ctx, querier, tenant, owner, goodsCode, matched[0].revision)
		if err != nil || !found {
			return ports.GoodsRow{}, false, fmt.Errorf("%s: 读商品 %q 修订 %d：%w", operation, matched[0].goodsCode, matched[0].revision, err)
		}
		return goodsRowOf(revision), true, nil
	default:
		return ports.GoodsRow{}, false, fmt.Errorf("%s: 码 %q 在同一时刻指向了 %d 个商品", operation, code.String(), len(matched))
	}
}

func (catalogue *GoodsCatalogue) ListGoodsRevisions(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	code domain.GoodsCode,
) ([]ports.GoodsRow, error) {
	const operation = "list goods revisions"
	if tenant.String() == "" || owner.String() == "" || code.String() == "" {
		return nil, fmt.Errorf("%s: tenant, owner and goods code are required", operation)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	headers, err := collectGoodsHeaders(ctx, querier,
		`SELECT `+goodsHeaderColumns+`
		   FROM warehouse_fulfillment.goods_revision
		  WHERE tenant_id = $1 AND owner_account_id = $2 AND goods_code = $3
		  ORDER BY revision ASC`,
		[]any{tenant.String(), owner.String(), code.String()})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	revisions := make([]ports.GoodsRow, 0, len(headers))
	for _, header := range headers {
		revision, err := goodsRevisionOfStored(ctx, querier, header)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", operation, err)
		}
		revisions = append(revisions, goodsRowOf(revision))
	}
	return revisions, nil
}

func collectGoodsHeaders(ctx context.Context, querier bentopg.Querier, statement string, args []any) ([]storedGoodsHeader, error) {
	rows, err := querier.Query(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var headers []storedGoodsHeader
	for rows.Next() {
		header, err := scanStoredGoodsHeader(rows)
		if err != nil {
			return nil, err
		}
		headers = append(headers, header)
	}
	return headers, rows.Err()
}
