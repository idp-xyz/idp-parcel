package main

import (
	"context"

	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

func init() {
	registerUnwired(commissionAgreementRevisionWiring{reviser: unwiredCommissionAgreementRevision{}})
}

type unwiredCommissionAgreementRevision struct{}

func (unwiredCommissionAgreementRevision) Revise(
	context.Context,
	tfapp.ReviseCommissionAgreementCommand,
) (tfapp.CommissionAgreementRevisionResult, error) {
	return tfapp.CommissionAgreementRevisionResult{}, errOrchestrationNotWired
}
