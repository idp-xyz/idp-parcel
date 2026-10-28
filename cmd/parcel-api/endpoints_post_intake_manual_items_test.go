package main

import (
	"context"
	"errors"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

func init() {
	registerUnwired(postIntakeManualItemWiring{registration: unwiredPostIntakeManualItemResult{}, items: unwiredPostIntakeManualItems{}})
}

type unwiredPostIntakeManualItemResult struct{}

func (unwiredPostIntakeManualItemResult) Handle(context.Context, shipmentapp.RegisterPostIntakeManualItemResultCommand) (shipmentapp.RegisterPostIntakeManualItemResultResult, error) {
	return shipmentapp.RegisterPostIntakeManualItemResultResult{}, errOrchestrationNotWired
}

type unwiredPostIntakeManualItems struct{}

var _ shipmenthttp.PostIntakeManualItemReader = unwiredPostIntakeManualItems{}

func (unwiredPostIntakeManualItems) ListPendingPostIntakeManualItems(context.Context, domain.TenantID, int) (ports.PendingPostIntakeManualItems, error) {
	return ports.PendingPostIntakeManualItems{}, errors.New("parcel-api: post-intake manual items not wired")
}

func (unwiredPostIntakeManualItems) ListPostIntakeManualItemsOfParcel(context.Context, domain.TenantID, domain.DeclaredParcelID) ([]ports.PostIntakeManualItem, error) {
	return nil, errors.New("parcel-api: post-intake manual items not wired")
}
