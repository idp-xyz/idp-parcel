package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	settlementapp "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	settlementdomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
	warehousedomain "go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

type warehouseAdoptionBridge struct {
	transactor bentoapp.Transactor
	handler    *settlementapp.WarehouseChargeHandler
	store      *sapostgres.WarehouseCharges
}

func (bridge warehouseAdoptionBridge) Adopt(ctx context.Context, tenant warehousedomain.TenantID, command warehousehttp.WarehouseAdoptionCommand) (string, error) {
	settlementTenant, err := settlementdomain.NewTenantID(tenant.String())
	if err != nil {
		return "", err
	}
	var outcome settlementapp.WarehouseChargeOutcome
	err = bridge.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		var runErr error
		outcome, runErr = bridge.handler.Adopt(txCtx, settlementTenant, settlementapp.WarehouseAdoption{
			Owner: command.Owner, Warehouse: command.Warehouse, Revision: command.Revision, Effective: command.Effective,
			Account: command.Account, Scope: command.Scope, Basis: command.Basis, Registrant: command.Registrant,
		})
		return runErr
	})
	if err != nil {
		return "", err
	}
	return string(outcome), nil
}

func (bridge warehouseAdoptionBridge) ListAdoptions(
	ctx context.Context,
	tenant warehousedomain.TenantID,
	limit int,
	rawQuery string,
) (cataloguepage.Result[warehousehttp.WarehouseAdoptionReadRow], error) {
	pageQuery, err := saports.WarehouseAdoptionPages.DecodeRaw(rawQuery)
	if err != nil {
		return cataloguepage.Result[warehousehttp.WarehouseAdoptionReadRow]{}, err
	}
	if pageQuery.PageSize > 0 {
		limit = pageQuery.PageSize
	}
	settlementTenant, err := settlementdomain.NewTenantID(tenant.String())
	if err != nil {
		return cataloguepage.Result[warehousehttp.WarehouseAdoptionReadRow]{}, err
	}
	page, err := bridge.store.ListAdoptions(ctx, settlementTenant, limit, pageQuery)
	if err != nil {
		return cataloguepage.Result[warehousehttp.WarehouseAdoptionReadRow]{}, err
	}
	rows := make([]warehousehttp.WarehouseAdoptionReadRow, 0, len(page.Rows))
	for _, row := range page.Rows {
		rows = append(rows, warehousehttp.WarehouseAdoptionReadRow{
			OwnerAccount:  row.Owner,
			WarehouseCode: row.Warehouse,
			Revision:      row.Revision,
			EffectiveAt:   row.Effective,
			Account:       row.Account,
			PricingScope:  row.Scope,
			Basis:         row.Basis,
			Registrant:    row.Registrant,
		})
	}
	return cataloguepage.Result[warehousehttp.WarehouseAdoptionReadRow]{
		Rows:   rows,
		Next:   page.Next,
		Total:  page.Total,
		Facets: page.Facets,
	}, nil
}

func (bridge warehouseAdoptionBridge) ListRequests(
	ctx context.Context,
	tenant warehousedomain.TenantID,
	limit int,
	rawQuery string,
) (cataloguepage.Result[warehousehttp.WarehouseChargeRequestReadRow], error) {
	pageQuery, err := saports.WarehouseChargeRequestPages.DecodeRaw(rawQuery)
	if err != nil {
		return cataloguepage.Result[warehousehttp.WarehouseChargeRequestReadRow]{}, err
	}
	if pageQuery.PageSize > 0 {
		limit = pageQuery.PageSize
	}
	settlementTenant, err := settlementdomain.NewTenantID(tenant.String())
	if err != nil {
		return cataloguepage.Result[warehousehttp.WarehouseChargeRequestReadRow]{}, err
	}
	page, err := bridge.store.ListRequests(ctx, settlementTenant, limit, pageQuery)
	if err != nil {
		return cataloguepage.Result[warehousehttp.WarehouseChargeRequestReadRow]{}, err
	}
	rows := make([]warehousehttp.WarehouseChargeRequestReadRow, 0, len(page.Rows))
	for _, row := range page.Rows {
		rows = append(rows, warehousehttp.WarehouseChargeRequestReadRow{
			OccurrenceID:       row.Occurrence,
			Version:            row.Version,
			Status:             row.Status,
			ChargeID:           row.ChargeID,
			EvaluationID:       row.EvaluationID,
			RequestID:          row.RequestID,
			Account:            row.Account,
			SettlementCurrency: row.Currency,
			PricingScope:       row.Scope,
			PricingBasisAt:     row.BasisAt,
		})
	}
	return cataloguepage.Result[warehousehttp.WarehouseChargeRequestReadRow]{
		Rows:   rows,
		Next:   page.Next,
		Total:  page.Total,
		Facets: page.Facets,
	}, nil
}

func buildWarehouseAdoptionService(db *bentopg.DB) (warehouseAdoptionBridge, error) {
	store, err := sapostgres.NewWarehouseCharges(db)
	if err != nil {
		return warehouseAdoptionBridge{}, fmt.Errorf("parcel-api: warehouse charge adoption: %w", err)
	}
	accounts, err := sapostgres.NewSettlementAccounts(db)
	if err != nil {
		return warehouseAdoptionBridge{}, fmt.Errorf("parcel-api: warehouse charge adoption: %w", err)
	}
	return warehouseAdoptionBridge{
		transactor: db.Transactor(),
		store:      store,
		handler: settlementapp.NewWarehouseChargeHandler(
			store, nil, accounts, nil, nil, nil, nil, nil, nil, nil, nil,
		),
	}, nil
}
