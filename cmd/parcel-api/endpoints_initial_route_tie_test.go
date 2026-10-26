package main

import (
	"context"
	"errors"

	nrapplication "go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

func init() {
	registerUnwired(initialRouteTieWiring{decider: unwiredInitialRouteTieDecision{}, ties: unwiredInitialRouteTies{}})
}

type unwiredInitialRouteTieDecision struct{}

func (unwiredInitialRouteTieDecision) Decide(context.Context, nrapplication.DecideInitialRouteCommand) (nrapplication.DecideInitialRouteResult, error) {
	return nrapplication.DecideInitialRouteResult{}, errOrchestrationNotWired
}

type unwiredInitialRouteTies struct{}

func (unwiredInitialRouteTies) ListInitialRouteTies(context.Context, domain.TenantID, int) ([]ports.InitialRouteTieCatalogueRow, error) {
	return nil, errors.New("parcel-api: initial route tie catalogue not wired")
}
