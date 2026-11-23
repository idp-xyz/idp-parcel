package main

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	settlementapp "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	settlementdomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
	warehousedomain "go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

type warehouseAdoptionBridge struct {
	handler *settlementapp.WarehouseChargeHandler
}

func (bridge warehouseAdoptionBridge) Adopt(ctx context.Context, tenant warehousedomain.TenantID, command warehousehttp.WarehouseAdoptionCommand) (string, error) {
	settlementTenant, err := settlementdomain.NewTenantID(tenant.String())
	if err != nil {
		return "", err
	}
	outcome, err := bridge.handler.Adopt(ctx, settlementTenant, settlementapp.WarehouseAdoption{
		Owner: command.Owner, Warehouse: command.Warehouse, Revision: command.Revision, Effective: command.Effective,
		Account: command.Account, Scope: command.Scope, Basis: command.Basis, Registrant: command.Registrant,
	})
	if err != nil {
		return "", err
	}
	return string(outcome), nil
}

func buildWarehouseAdoptionService(db *bentopg.DB) (warehousehttp.WarehouseAdoptionService, error) {
	store, err := sapostgres.NewWarehouseCharges(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: warehouse charge adoption: %w", err)
	}
	accounts, err := sapostgres.NewSettlementAccounts(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: warehouse charge adoption: %w", err)
	}
	return warehouseAdoptionBridge{handler: settlementapp.NewWarehouseChargeHandler(
		store, nil, accounts, nil, nil, nil, nil, nil, nil, nil, nil,
	)}, nil
}
