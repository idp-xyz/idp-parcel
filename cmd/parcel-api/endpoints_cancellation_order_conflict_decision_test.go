package main

import (
	"context"

	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
)

func init() {
	registerUnwired(cancellationOrderConflictDecisionWiring{decision: unwiredCancellationOrderConflictDecision{}})
}

type unwiredCancellationOrderConflictDecision struct{}

func (unwiredCancellationOrderConflictDecision) Handle(context.Context, shipmentapp.DecideCancellationOrderConflictCommand) (shipmentapp.DecideCancellationOrderConflictResult, error) {
	return shipmentapp.DecideCancellationOrderConflictResult{}, errOrchestrationNotWired
}
