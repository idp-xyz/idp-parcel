package main

import (
	"context"

	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
)

func init() {
	registerUnwired(controlReleaseRefusalHandlingWiring{handling: unwiredControlReleaseRefusalHandling{}})
}

type unwiredControlReleaseRefusalHandling struct{}

func (unwiredControlReleaseRefusalHandling) Handle(context.Context, shipmentapp.HandleControlReleaseRefusalCommand) (shipmentapp.HandleControlReleaseRefusalResult, error) {
	return shipmentapp.HandleControlReleaseRefusalResult{}, errOrchestrationNotWired
}
