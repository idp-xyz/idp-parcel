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

// UnclaimedDisposalRules 是待认领处置规则修订册的 PostgreSQL 适配器（读写共用）。
// 修订只追加，绝不 UPDATE；撞键不覆盖，按内容答重放或冲突。
type UnclaimedDisposalRules struct {
	db *bentopg.DB
}

func NewUnclaimedDisposalRules(db *bentopg.DB) (*UnclaimedDisposalRules, error) {
	if db == nil {
		return nil, fmt.Errorf("warehouse fulfillment postgres: db is nil")
	}
	return &UnclaimedDisposalRules{db: db}, nil
}

var (
	_ ports.DisposalRuleRegistry      = (*UnclaimedDisposalRules)(nil)
	_ ports.DisposalRuleLookup        = (*UnclaimedDisposalRules)(nil)
	_ ports.DisposalRuleCatalogueRead = (*UnclaimedDisposalRules)(nil)
	_ ports.DisposalRuleHistoryRead   = (*UnclaimedDisposalRules)(nil)
)

const disposalRuleColumns = `tenant_id, warehouse_code, revision, claim_window_seconds, registrant, registered_at`

const disposalRuleListed = `WITH listed AS (
	SELECT DISTINCT ON (warehouse_code) ` + disposalRuleColumns + `
	  FROM warehouse_fulfillment.unclaimed_disposal_rule_revision
	 WHERE tenant_id = $1
	 ORDER BY warehouse_code, revision DESC
)`

var disposalRuleListBinding = ports.UnclaimedDisposalRulePages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"warehouseCode": "warehouse_code"},
	Identity: []string{"warehouse_code"},
	Filters:  map[string]string{"warehouseCode": "warehouse_code"},
	Keyword:  []string{"warehouse_code"},
})

func (store *UnclaimedDisposalRules) SaveDisposalRuleRevision(
	ctx context.Context,
	revision domain.DisposalRuleRevision,
) (ports.RegistrySaveOutcome, error) {
	const operation = "save unclaimed disposal rule revision"
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return ports.RegistrySaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.unclaimed_disposal_rule_revision
		    (tenant_id, warehouse_code, revision, claim_window_seconds, registrant, registered_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (tenant_id, warehouse_code, revision) DO NOTHING`,
		revision.Tenant().String(), revision.Warehouse().String(), revision.Revision(),
		revision.Content().ClaimWindowSeconds(), revision.Registrant().String(), revision.RegisteredAt(),
	)
	if err != nil {
		return ports.RegistrySaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() > 0 {
		for index, method := range revision.Content().Methods() {
			if _, err := executor.Exec(ctx,
				`INSERT INTO warehouse_fulfillment.unclaimed_disposal_rule_method
				    (tenant_id, warehouse_code, revision, position, method)
				 VALUES ($1, $2, $3, $4, $5)`,
				revision.Tenant().String(), revision.Warehouse().String(), revision.Revision(), index, method.String(),
			); err != nil {
				return ports.RegistrySaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
			}
		}
		return ports.RegistryRevisionSaved, nil
	}

	existing, found, err := loadDisposalRuleRevision(ctx, executor, revision.Tenant(), revision.Warehouse(), revision.Revision())
	if err != nil {
		return ports.RegistrySaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	if !found {
		return ports.RegistrySaveOutcomeInvalid, fmt.Errorf("%s: 撞键后读不回既有行", operation)
	}
	if existing.SameContentAs(revision) {
		return ports.RegistryRevisionAlreadyRegistered, nil
	}
	return ports.RegistryRevisionContentConflict, nil
}

func (store *UnclaimedDisposalRules) LoadLatestDisposalRule(
	ctx context.Context,
	tenant domain.TenantID,
	warehouse domain.WarehouseCode,
) (domain.DisposalRuleRevision, bool, error) {
	const operation = "load latest unclaimed disposal rule"
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return domain.DisposalRuleRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	var (
		tenantText, whText, registrantText string
		rev                                int
		windowSeconds                      int64
		registeredAt                       time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT `+disposalRuleColumns+`
		   FROM warehouse_fulfillment.unclaimed_disposal_rule_revision
		  WHERE tenant_id = $1 AND warehouse_code = $2
		  ORDER BY revision DESC
		  LIMIT 1`,
		tenant.String(), warehouse.String(),
	).Scan(&tenantText, &whText, &rev, &windowSeconds, &registrantText, &registeredAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DisposalRuleRevision{}, false, nil
	}
	if err != nil {
		return domain.DisposalRuleRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	rule, err := buildDisposalRuleFromHeader(ctx, querier, tenantText, whText, rev, windowSeconds, registrantText, registeredAt)
	if err != nil {
		return domain.DisposalRuleRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return rule, true, nil
}

func (store *UnclaimedDisposalRules) LoadDisposalRuleRevision(
	ctx context.Context,
	tenant domain.TenantID,
	warehouse domain.WarehouseCode,
	revision int,
) (domain.DisposalRuleRevision, bool, error) {
	const operation = "load unclaimed disposal rule revision"
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return domain.DisposalRuleRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	rule, found, err := loadDisposalRuleRevision(ctx, querier, tenant, warehouse, revision)
	if err != nil {
		return domain.DisposalRuleRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return rule, found, nil
}

func (store *UnclaimedDisposalRules) ListDisposalRules(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.DisposalRuleRow], error) {
	const operation = "list unclaimed disposal rules"
	fail := func(err error) (cataloguepage.Result[ports.DisposalRuleRow], error) {
		return cataloguepage.Result[ports.DisposalRuleRow]{}, fmt.Errorf("%s: %w", operation, err)
	}
	if tenant.String() == "" {
		return fail(errors.New("tenant is required"))
	}
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := disposalRuleListBinding.Plan(query, "", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, disposalRuleListed+" "+plan.CountSQL("listed"), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	fetched, err := collectDisposalRuleRows(ctx, querier, disposalRuleListed+" "+plan.PageSQL(disposalRuleColumns, "listed"), plan.PageArgs)
	if err != nil {
		return fail(err)
	}
	page, err := cataloguepage.Finish(query, fetched, limit, total,
		func(row ports.DisposalRuleRow, _ string) cataloguepage.Position {
			return cataloguepage.Position{Value: row.WarehouseCode, Identity: []string{row.WarehouseCode}}
		})
	if err != nil {
		return fail(err)
	}
	return page, nil
}

func (store *UnclaimedDisposalRules) ListDisposalRuleRevisions(
	ctx context.Context,
	tenant domain.TenantID,
	warehouse domain.WarehouseCode,
) ([]ports.DisposalRuleRow, error) {
	const operation = "list unclaimed disposal rule revisions"
	if tenant.String() == "" || warehouse.String() == "" {
		return nil, fmt.Errorf("%s: tenant and warehouse code are required", operation)
	}
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	rows, err := collectDisposalRuleRows(ctx, querier,
		`SELECT `+disposalRuleColumns+`
		   FROM warehouse_fulfillment.unclaimed_disposal_rule_revision
		  WHERE tenant_id = $1 AND warehouse_code = $2
		  ORDER BY revision ASC`,
		[]any{tenant.String(), warehouse.String()})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	return rows, nil
}

func loadDisposalRuleRevision(
	ctx context.Context,
	querier bentopg.Querier,
	tenant domain.TenantID,
	warehouse domain.WarehouseCode,
	revision int,
) (domain.DisposalRuleRevision, bool, error) {
	var (
		tenantText, whText, registrantText string
		rev                                int
		windowSeconds                      int64
		registeredAt                       time.Time
	)
	err := querier.QueryRow(ctx,
		`SELECT `+disposalRuleColumns+`
		   FROM warehouse_fulfillment.unclaimed_disposal_rule_revision
		  WHERE tenant_id = $1 AND warehouse_code = $2 AND revision = $3`,
		tenant.String(), warehouse.String(), revision,
	).Scan(&tenantText, &whText, &rev, &windowSeconds, &registrantText, &registeredAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DisposalRuleRevision{}, false, nil
	}
	if err != nil {
		return domain.DisposalRuleRevision{}, false, err
	}
	rule, err := buildDisposalRuleFromHeader(ctx, querier, tenantText, whText, rev, windowSeconds, registrantText, registeredAt)
	if err != nil {
		return domain.DisposalRuleRevision{}, false, err
	}
	return rule, true, nil
}

func collectDisposalRuleRows(
	ctx context.Context,
	querier bentopg.Querier,
	sql string,
	args []any,
) ([]ports.DisposalRuleRow, error) {
	rows, err := querier.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	type header struct {
		tenant, wh, registrant string
		rev                    int
		windowSeconds          int64
		registeredAt           time.Time
	}
	var headers []header
	for rows.Next() {
		var h header
		if err := rows.Scan(&h.tenant, &h.wh, &h.rev, &h.windowSeconds, &h.registrant, &h.registeredAt); err != nil {
			rows.Close()
			return nil, err
		}
		headers = append(headers, h)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	result := make([]ports.DisposalRuleRow, 0, len(headers))
	for _, h := range headers {
		rule, err := buildDisposalRuleFromHeader(ctx, querier, h.tenant, h.wh, h.rev, h.windowSeconds, h.registrant, h.registeredAt)
		if err != nil {
			return nil, err
		}
		methods := make([]string, 0, len(rule.Content().Methods()))
		for _, m := range rule.Content().Methods() {
			methods = append(methods, m.String())
		}
		result = append(result, ports.DisposalRuleRow{
			WarehouseCode:      rule.Warehouse().String(),
			Revision:           rule.Revision(),
			ClaimWindowSeconds: rule.Content().ClaimWindowSeconds(),
			DisposalMethods:    methods,
			Registrant:         rule.Registrant().String(),
			RegisteredAt:       rule.RegisteredAt(),
		})
	}
	return result, nil
}

func buildDisposalRuleFromHeader(
	ctx context.Context,
	querier bentopg.Querier,
	tenantText, whText string,
	rev int,
	windowSeconds int64,
	registrantText string,
	registeredAt time.Time,
) (domain.DisposalRuleRevision, error) {
	rows, err := querier.Query(ctx,
		`SELECT method
		   FROM warehouse_fulfillment.unclaimed_disposal_rule_method
		  WHERE tenant_id = $1 AND warehouse_code = $2 AND revision = $3
		  ORDER BY position ASC`,
		tenantText, whText, rev)
	if err != nil {
		return domain.DisposalRuleRevision{}, err
	}
	defer rows.Close()
	var methods []domain.UnclaimedDisposalMethod
	for rows.Next() {
		var text string
		if err := rows.Scan(&text); err != nil {
			return domain.DisposalRuleRevision{}, err
		}
		method, err := domain.NewUnclaimedDisposalMethod(text)
		if err != nil {
			return domain.DisposalRuleRevision{}, err
		}
		methods = append(methods, method)
	}
	if err := rows.Err(); err != nil {
		return domain.DisposalRuleRevision{}, err
	}
	tenant, err := domain.NewTenantID(tenantText)
	if err != nil {
		return domain.DisposalRuleRevision{}, err
	}
	warehouse, err := domain.NewWarehouseCode(whText)
	if err != nil {
		return domain.DisposalRuleRevision{}, err
	}
	registrant, err := domain.NewRegistrant(registrantText)
	if err != nil {
		return domain.DisposalRuleRevision{}, err
	}
	return domain.NewDisposalRuleRevision(
		tenant, warehouse, rev, domain.NewDisposalRuleContent(windowSeconds, methods), registrant, registeredAt,
	)
}
