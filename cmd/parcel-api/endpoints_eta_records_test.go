package main

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	visibilitydomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	visibilityports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

func init() {
	registerUnwired(etaRecordsWiring{predictions: unwiredETAPredictionReview{}})
}

type unwiredETAPredictionReview struct{}

func (unwiredETAPredictionReview) ListCurrentETAPredictions(
	context.Context, visibilitydomain.TenantID, int, cataloguepage.Query,
) (cataloguepage.Result[visibilityports.ETAPredictionReviewRow], error) {
	return cataloguepage.Result[visibilityports.ETAPredictionReviewRow]{}, errOrchestrationNotWired
}
