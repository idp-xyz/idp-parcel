package main

import (
	"net/http"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type cancellationWiring struct {
	cancellation shipmenthttp.CancellationHandler
}

func init() {
	registerEndpoints(endpointFeature[cancellationWiring]{
		lines: map[string]endpointLine{
			"/shipment-requests/parcel-cancellations": {method: http.MethodPost, face: unconfiguredFace()},
		},
		build: func(sources endpointSources) (cancellationWiring, error) {
			cancellation, err := buildCancellationOrchestration(sources.db)
			return cancellationWiring{cancellation: cancellation}, err
		},
		mount: func(_ endpointChannels, wiring cancellationWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				{Pattern: "/shipment-requests/parcel-cancellations", Handler: shipmenthttp.NewCancelParcelEndpoint(shipmenthttp.UnconfiguredIntake{}, wiring.cancellation)},
			}
		},
	})
}
