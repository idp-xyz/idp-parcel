package main

import (
	"context"
	"errors"

	nrapplication "go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

func init() {
	registerUnwired(rerouteDecisionWiring{decider: unwiredRerouteDecision{}, suggestions: unwiredRerouteSuggestions{}})
}

type unwiredRerouteDecision struct{}

func (unwiredRerouteDecision) Decide(context.Context, nrapplication.DecideRerouteCommand) (nrapplication.DecideRerouteResult, error) {
	return nrapplication.DecideRerouteResult{}, errOrchestrationNotWired
}

type unwiredRerouteSuggestions struct{}

func (unwiredRerouteSuggestions) ListRerouteSuggestions(context.Context, domain.TenantID, int) ([]ports.RerouteSuggestionCatalogueRow, error) {
	return nil, errors.New("parcel-api: reroute suggestion catalogue not wired")
}
