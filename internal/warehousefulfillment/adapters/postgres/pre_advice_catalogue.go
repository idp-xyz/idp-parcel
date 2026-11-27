package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// PreAdviceCatalogue 实现入库预报的查阅口。列表在一个货主之内；按单据号找只比本租户的 document_number，
// 没有号的行那一列是 NULL，逐字相等碰不到。
type PreAdviceCatalogue struct {
	db *bentopg.DB
}

func NewPreAdviceCatalogue(db *bentopg.DB) (*PreAdviceCatalogue, error) {
	if db == nil {
		return nil, fmt.Errorf("warehouse fulfillment postgres: db is nil")
	}
	return &PreAdviceCatalogue{db: db}, nil
}

var _ ports.PreAdviceRead = (*PreAdviceCatalogue)(nil)

const preAdviceListed = `WITH listed AS (
        SELECT advice.pre_advice_id, advice.warehouse_code, advice.status, notice.expected_at
          FROM warehouse_fulfillment.inbound_pre_advice AS advice
          JOIN warehouse_fulfillment.inbound_pre_advice_notice AS notice
            ON notice.tenant_id = advice.tenant_id
           AND notice.pre_advice_id = advice.pre_advice_id
           AND notice.version = (
                SELECT MAX(later.version)
                  FROM warehouse_fulfillment.inbound_pre_advice_notice AS later
                 WHERE later.tenant_id = advice.tenant_id
                   AND later.pre_advice_id = advice.pre_advice_id
           )
         WHERE advice.tenant_id = $1 AND advice.owner_account_id = $2
     )`

var preAdviceListBinding = ports.PreAdvicePages.MustBind(cataloguepage.Columns{
	Sorts:       map[string]string{"preAdviceId": "pre_advice_id"},
	Identity:    []string{"pre_advice_id"},
	Keyword:     []string{"pre_advice_id", "warehouse_code"},
	Filters:     map[string]string{"status": "status", "warehouseCode": "warehouse_code", "expectedFrom": "expected_at", "expectedUntil": "expected_at"},
	Comparisons: map[string]string{"expectedFrom": ">=", "expectedUntil": "<="},
})

func (catalogue *PreAdviceCatalogue) ListPreAdvices(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[domain.PreAdvice], error) {
	const operation = "list inbound pre-advices"
	fail := func(err error) (cataloguepage.Result[domain.PreAdvice], error) {
		return cataloguepage.Result[domain.PreAdvice]{}, fmt.Errorf("%s: %w", operation, err)
	}
	if tenant.String() == "" || owner.String() == "" {
		return fail(fmt.Errorf("tenant and owner are required"))
	}
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := preAdviceListBinding.Plan(query, "", []any{tenant.String(), owner.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, preAdviceListed+" "+plan.CountSQL("listed"), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	identifiers, err := collectPreAdviceIDs(ctx, querier, preAdviceListed+" "+plan.PageSQL("pre_advice_id", "listed"), plan.PageArgs)
	if err != nil {
		return fail(err)
	}
	fetched := make([]domain.PreAdvice, 0, len(identifiers))
	for _, identifier := range identifiers {
		advice, found, err := loadPreAdviceWhere(ctx, querier,
			`tenant_id = $1 AND owner_account_id = $2 AND pre_advice_id = $3`,
			tenant.String(), owner.String(), identifier)
		if err != nil {
			return fail(err)
		}
		if !found {
			return fail(fmt.Errorf("listed pre-advice %s did not load", identifier))
		}
		fetched = append(fetched, advice)
	}
	// 预报标识逐字落在 pre_advice_id 上，ID().String() 就是排序用的那一列。
	page, err := cataloguepage.Finish(query, fetched, limit, total, func(advice domain.PreAdvice, _ string) cataloguepage.Position {
		identifier := advice.ID().String()
		return cataloguepage.Position{Value: identifier, Identity: []string{identifier}}
	})
	if err != nil {
		return fail(err)
	}
	return page, nil
}

func (catalogue *PreAdviceCatalogue) PreAdviceForOwner(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	id domain.PreAdviceID,
) (domain.PreAdvice, bool, error) {
	const operation = "load inbound pre-advice for owner"
	if tenant.String() == "" || owner.String() == "" || id.String() == "" {
		return domain.PreAdvice{}, false, fmt.Errorf("%s: tenant, owner and pre-advice are required", operation)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return domain.PreAdvice{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	advice, found, err := loadPreAdviceWhere(ctx, querier,
		`tenant_id = $1 AND owner_account_id = $2 AND pre_advice_id = $3`,
		tenant.String(), owner.String(), id.String())
	if err != nil {
		return domain.PreAdvice{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return advice, found, nil
}

func (catalogue *PreAdviceCatalogue) PreAdviceByDocumentNumber(
	ctx context.Context,
	tenant domain.TenantID,
	number domain.DocumentNumber,
) (domain.PreAdvice, bool, error) {
	const operation = "load inbound pre-advice by document number"
	if tenant.String() == "" || !number.Assigned() {
		return domain.PreAdvice{}, false, fmt.Errorf("%s: tenant and document number are required", operation)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return domain.PreAdvice{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	advice, found, err := loadPreAdviceWhere(ctx, querier,
		`tenant_id = $1 AND document_number = $2`, tenant.String(), number.String())
	if err != nil {
		return domain.PreAdvice{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return advice, found, nil
}

func collectPreAdviceIDs(ctx context.Context, querier bentopg.Querier, sql string, args []any) ([]string, error) {
	rows, err := querier.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var identifiers []string
	for rows.Next() {
		var identifier string
		if err := rows.Scan(&identifier); err != nil {
			return nil, err
		}
		identifiers = append(identifiers, identifier)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return identifiers, nil
}
