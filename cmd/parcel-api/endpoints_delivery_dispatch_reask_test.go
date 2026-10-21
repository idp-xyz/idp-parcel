package main

import (
	"context"

	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

func init() {
	registerUnwired(deliveryDispatchReaskWiring{reask: unwiredDeliveryDispatchReask{}, cadence: unwiredReaskCadenceRegistration{}})
}

type unwiredDeliveryDispatchReask struct{}

func (unwiredDeliveryDispatchReask) Reask(
	context.Context,
	tfapp.ReaskPendingDeliveryDispatchesCommand,
) (tfapp.ReaskPendingDeliveryDispatchesResult, error) {
	return tfapp.ReaskPendingDeliveryDispatchesResult{}, errOrchestrationNotWired
}

type unwiredReaskCadenceRegistration struct{}

func (unwiredReaskCadenceRegistration) Register(
	context.Context,
	tfapp.RegisterReaskCadenceCommand,
) (tfapp.RegisterReaskCadenceResult, error) {
	return tfapp.RegisterReaskCadenceResult{}, errOrchestrationNotWired
}
