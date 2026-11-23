package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
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
		    tenant_id, occurrence_id, version, status, charge_id, evaluation_id)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		tenant.String(), request.Occurrence, request.Version, request.Status, nullable(request.ChargeID), nullable(request.EvaluationID)); err != nil {
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
	var charge, evaluation *string
	err = executor.QueryRow(ctx, `
		SELECT status, charge_id, evaluation_id
		  FROM settlement_accounting.warehouse_charge_request
		 WHERE tenant_id = $1 AND occurrence_id = $2 AND version = $3`,
		tenant.String(), occurrence, version).Scan(&request.Status, &charge, &evaluation)
	if errors.Is(err, pgx.ErrNoRows) {
		return application.WarehouseChargeRequest{}, false, nil
	}
	if err != nil {
		return application.WarehouseChargeRequest{}, false, fmt.Errorf("load warehouse charge request: %w", err)
	}
	if charge != nil {
		request.ChargeID = *charge
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
