package main

import (
	"context"
	"errors"

	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

func init() {
	registerUnwired(postIntakeDispositionWiring{decision: unwiredPostIntakeDisposition{}, requests: unwiredPostIntakeDispositionRequests{}})
}

type unwiredPostIntakeDisposition struct{}

func (unwiredPostIntakeDisposition) Handle(context.Context, shipmentapp.DecidePostIntakeDispositionCommand) (shipmentapp.DecidePostIntakeDispositionResult, error) {
	return shipmentapp.DecidePostIntakeDispositionResult{}, errOrchestrationNotWired
}

type unwiredPostIntakeDispositionRequests struct{}

func (unwiredPostIntakeDispositionRequests) ListUnansweredPostIntakeDispositionRequests(context.Context, domain.TenantID, int) (ports.UnansweredPostIntakeDispositionRequests, error) {
	return ports.UnansweredPostIntakeDispositionRequests{}, errors.New("parcel-api: post-intake disposition requests not wired")
}
