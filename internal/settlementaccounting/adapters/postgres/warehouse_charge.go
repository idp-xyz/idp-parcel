package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// WarehouseCharges 记下仓储计费采用和这一路的请求。
type WarehouseCharges struct {
	db *bentopg.DB
}

func NewWarehouseCharges(db *bentopg.DB) (*WarehouseCharges, error) {
	if db == nil {
		return nil, errors.New("settlement accounting postgres: warehouse charges need a db")
	}
	return &WarehouseCharges{db: db}, nil
}

var _ application.WarehouseChargeStore = (*WarehouseCharges)(nil)

func (store *WarehouseCharges) SaveAdoption(ctx context.Context, tenant domain.TenantID, adoption application.WarehouseAdoption) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save warehouse adoption: %w", err)
	}
	if _, err := executor.Exec(ctx, `
		INSERT INTO settlement_accounting.warehouse_charge_adoption (
		    tenant_id, owner_account_id, warehouse_code, revision, effective_at, account_id, pricing_scope, basis, registrant)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		tenant.String(), adoption.Owner, adoption.Warehouse, adoption.Revision, adoption.Effective,
		adoption.Account, adoption.Scope, adoption.Basis, adoption.Registrant); err != nil {
		return fmt.Errorf("save warehouse adoption: %w", err)
	}
	return nil
}

func (store *WarehouseCharges) LoadAdoptions(ctx context.Context, tenant domain.TenantID, owner, warehouse string) ([]application.WarehouseAdoption, error) {
	executor, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("load warehouse adoptions: %w", err)
	}
	rows, err := executor.Query(ctx, `
		SELECT revision, effective_at, account_id, pricing_scope, basis, registrant
		  FROM settlement_accounting.warehouse_charge_adoption
		 WHERE tenant_id = $1 AND owner_account_id = $2 AND warehouse_code = $3`,
		tenant.String(), owner, warehouse)
	if err != nil {
		return nil, fmt.Errorf("load warehouse adoptions: %w", err)
	}
	defer rows.Close()
	var adoptions []application.WarehouseAdoption
	for rows.Next() {
		adoption := application.WarehouseAdoption{Owner: owner, Warehouse: warehouse}
		if err := rows.Scan(&adoption.Revision, &adoption.Effective, &adoption.Account, &adoption.Scope, &adoption.Basis, &adoption.Registrant); err != nil {
			return nil, err
		}
		adoptions = append(adoptions, adoption)
	}
	return adoptions, rows.Err()
}

func (store *WarehouseCharges) SaveRequest(ctx context.Context, tenant domain.TenantID, request application.WarehouseChargeRequest) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save warehouse charge request: %w", err)
	}
	if _, err := executor.Exec(ctx, `
		INSERT INTO settlement_accounting.warehouse_charge_request (
		    tenant_id, occurrence_id, version, status, charge_id, evaluation_id,
		    request_id, account_id, settlement_currency, pricing_scope, pricing_basis_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		tenant.String(), request.Occurrence, request.Version, request.Status,
		nullable(request.ChargeID), nullable(request.EvaluationID), nullable(request.RequestID),
		nullable(request.Account), nullable(request.Currency), nullable(request.Scope), nullableTime(request.BasisAt)); err != nil {
		return fmt.Errorf("save warehouse charge request: %w", err)
	}
	return nil
}

func (store *WarehouseCharges) LoadRequest(ctx context.Context, tenant domain.TenantID, occurrence string, version int) (application.WarehouseChargeRequest, bool, error) {
	executor, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return application.WarehouseChargeRequest{}, false, fmt.Errorf("load warehouse charge request: %w", err)
	}
	request := application.WarehouseChargeRequest{Occurrence: occurrence, Version: version}
	var charge, evaluation, requestID, account, currency, scope *string
	var basis *time.Time
	err = executor.QueryRow(ctx, `
		SELECT status, charge_id, evaluation_id, request_id, account_id, settlement_currency, pricing_scope, pricing_basis_at
		  FROM settlement_accounting.warehouse_charge_request
		 WHERE tenant_id = $1 AND occurrence_id = $2 AND version = $3`,
		tenant.String(), occurrence, version).Scan(
		&request.Status, &charge, &evaluation, &requestID, &account, &currency, &scope, &basis)
	if errors.Is(err, pgx.ErrNoRows) {
		return application.WarehouseChargeRequest{}, false, nil
	}
	if err != nil {
		return application.WarehouseChargeRequest{}, false, fmt.Errorf("load warehouse charge request: %w", err)
	}
	request.ChargeID = valueOf(charge)
	request.EvaluationID = valueOf(evaluation)
	request.RequestID = valueOf(requestID)
	request.Account = valueOf(account)
	request.Currency = valueOf(currency)
	request.Scope = valueOf(scope)
	if basis != nil {
		request.BasisAt = *basis
	}
	return request, true, nil
}

func (store *WarehouseCharges) LoadCharges(ctx context.Context, tenant domain.TenantID, occurrence string) ([]application.WarehouseChargeRequest, error) {
	executor, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("load warehouse charge requests: %w", err)
	}
	rows, err := executor.Query(ctx, `
		SELECT version, status, charge_id
		  FROM settlement_accounting.warehouse_charge_request
		 WHERE tenant_id = $1 AND occurrence_id = $2`, tenant.String(), occurrence)
	if err != nil {
		return nil, fmt.Errorf("load warehouse charge requests: %w", err)
	}
	defer rows.Close()
	var requests []application.WarehouseChargeRequest
	for rows.Next() {
		request := application.WarehouseChargeRequest{Occurrence: occurrence}
		var charge *string
		if err := rows.Scan(&request.Version, &request.Status, &charge); err != nil {
			return nil, err
		}
		if charge != nil {
			request.ChargeID = *charge
		}
		requests = append(requests, request)
	}
	return requests, rows.Err()
}

func nullable(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func nullableTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}

func valueOf(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

const warehouseAdoptionListColumns = `owner_account_id, warehouse_code, revision, effective_at, account_id, pricing_scope, basis, registrant`

const warehouseAdoptionListed = `WITH listed AS (
	SELECT ` + warehouseAdoptionListColumns + `
	  FROM settlement_accounting.warehouse_charge_adoption
	 WHERE tenant_id = $1
)`

var warehouseAdoptionListBinding = ports.WarehouseAdoptionPages.MustBind(cataloguepage.Columns{
	Sorts: map[string]string{
		"effectiveAt":  "effective_at",
		"ownerAccount": "owner_account_id",
	},
	Identity: []string{"owner_account_id", "warehouse_code", "revision"},
	Filters: map[string]string{
		"ownerAccount":  "owner_account_id",
		"warehouseCode": "warehouse_code",
		"pricingScope":  "pricing_scope",
	},
	Keyword: []string{"owner_account_id", "warehouse_code", "account_id", "pricing_scope", "basis"},
})

// ListAdoptions 按分页条件列出本租户的仓储计费采用修订。
func (store *WarehouseCharges) ListAdoptions(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[application.WarehouseAdoption], error) {
	const operation = "list warehouse adoptions"
	fail := func(err error) (cataloguepage.Result[application.WarehouseAdoption], error) {
		return cataloguepage.Result[application.WarehouseAdoption]{}, fmt.Errorf("%s: %w", operation, err)
	}
	if tenant.String() == "" || limit < 1 {
		return fail(errors.New("tenant and a positive limit are required"))
	}
	plan, err := warehouseAdoptionListBinding.Plan(query, "", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	executor, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := executor.QueryRow(ctx, warehouseAdoptionListed+" "+plan.CountSQL("listed"), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	rows, err := executor.Query(ctx, warehouseAdoptionListed+" "+plan.PageSQL(warehouseAdoptionListColumns, "listed"), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()
	var items []application.WarehouseAdoption
	for rows.Next() {
		var item application.WarehouseAdoption
		if err := rows.Scan(
			&item.Owner, &item.Warehouse, &item.Revision, &item.Effective,
			&item.Account, &item.Scope, &item.Basis, &item.Registrant,
		); err != nil {
			return fail(err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	return cataloguepage.Finish(query, items, limit, total, func(item application.WarehouseAdoption, sortField string) cataloguepage.Position {
		identity := []string{item.Owner, item.Warehouse, cataloguepage.FormatInteger(int64(item.Revision))}
		if sortField == "ownerAccount" {
			return cataloguepage.Position{Value: item.Owner, Identity: identity}
		}
		return cataloguepage.Position{Value: cataloguepage.FormatInstant(item.Effective), Identity: identity}
	})
}

const warehouseChargeRequestListColumns = `
	occurrence_id, version, status, charge_id, evaluation_id,
	request_id, account_id, settlement_currency, pricing_scope, pricing_basis_at`

const warehouseChargeRequestListed = `WITH listed AS (
	SELECT ` + warehouseChargeRequestListColumns + `
	  FROM settlement_accounting.warehouse_charge_request
	 WHERE tenant_id = $1
)`

var warehouseChargeRequestListBinding = ports.WarehouseChargeRequestPages.MustBind(cataloguepage.Columns{
	Sorts: map[string]string{
		"occurrenceId": "occurrence_id",
	},
	Identity: []string{"occurrence_id", "version"},
	Filters: map[string]string{
		"status":       "status",
		"pricingScope": "pricing_scope",
	},
	Keyword: []string{"occurrence_id", "request_id", "charge_id", "evaluation_id", "account_id", "pricing_scope"},
})

// ListRequests 按分页条件列出本租户的仓储 SELL 计费请求与结清状态。
func (store *WarehouseCharges) ListRequests(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[application.WarehouseChargeRequest], error) {
	const operation = "list warehouse charge requests"
	fail := func(err error) (cataloguepage.Result[application.WarehouseChargeRequest], error) {
		return cataloguepage.Result[application.WarehouseChargeRequest]{}, fmt.Errorf("%s: %w", operation, err)
	}
	if tenant.String() == "" || limit < 1 {
		return fail(errors.New("tenant and a positive limit are required"))
	}
	plan, err := warehouseChargeRequestListBinding.Plan(query, "", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	executor, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := executor.QueryRow(ctx, warehouseChargeRequestListed+" "+plan.CountSQL("listed"), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	rows, err := executor.Query(ctx, warehouseChargeRequestListed+" "+plan.PageSQL(warehouseChargeRequestListColumns, "listed"), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()
	var items []application.WarehouseChargeRequest
	for rows.Next() {
		var (
			item                                            application.WarehouseChargeRequest
			charge, evaluation, reqID, account, curr, scope *string
			basis                                           *time.Time
		)
		if err := rows.Scan(
			&item.Occurrence, &item.Version, &item.Status, &charge, &evaluation,
			&reqID, &account, &curr, &scope, &basis,
		); err != nil {
			return fail(err)
		}
		item.ChargeID = valueOf(charge)
		item.EvaluationID = valueOf(evaluation)
		item.RequestID = valueOf(reqID)
		item.Account = valueOf(account)
		item.Currency = valueOf(curr)
		item.Scope = valueOf(scope)
		if basis != nil {
			item.BasisAt = *basis
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	return cataloguepage.Finish(query, items, limit, total, func(item application.WarehouseChargeRequest, _ string) cataloguepage.Position {
		return cataloguepage.Position{
			Value:    item.Occurrence,
			Identity: []string{item.Occurrence, cataloguepage.FormatInteger(int64(item.Version))},
		}
	})
}
