package main

import (
	"context"

	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

func init() {
	registerUnwired(fulfillmentCorrectionsWiring{
		chargeOccurrenceInvalidation: unwiredChargeOccurrenceInvalidation{},
		segmentCommissionWithdrawal:  unwiredSegmentCommissionWithdrawal{},
	})
}

// unwiredChargeOccurrenceInvalidation 是失效口的未接线占位：装配测试只看接入面，编排走不到。
type unwiredChargeOccurrenceInvalidation struct{}

func (unwiredChargeOccurrenceInvalidation) Invalidate(
	context.Context, tfapp.InvalidateChargeOccurrenceCommand,
) (tfapp.ChargeOccurrenceInvalidationResult, error) {
	return tfapp.ChargeOccurrenceInvalidationResult{}, errOrchestrationNotWired
}

// unwiredSegmentCommissionWithdrawal 是解除口的未接线占位。
type unwiredSegmentCommissionWithdrawal struct{}

func (unwiredSegmentCommissionWithdrawal) Withdraw(
	context.Context, tfapp.WithdrawSegmentCommissionAdoptionCommand,
) (tfapp.AdoptionWithdrawalResult, error) {
	return tfapp.AdoptionWithdrawalResult{}, errOrchestrationNotWired
}
