package main

import (
	"net/http"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type withdrawalWiring struct {
	withdrawal shipmenthttp.WithdrawalHandler
}

func init() {
	registerEndpoints(endpointFeature[withdrawalWiring]{
		lines: map[string]endpointLine{
			"/shipment-requests/withdrawals": {method: http.MethodPost, face: unconfiguredFace()},
		},
		build: func(sources endpointSources) (withdrawalWiring, error) {
			withdrawal, err := buildWithdrawalOrchestration(sources.db)
			return withdrawalWiring{withdrawal: withdrawal}, err
		},
		mount: func(_ endpointChannels, wiring withdrawalWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				{Pattern: "/shipment-requests/withdrawals", Handler: shipmenthttp.NewWithdrawShipmentRequestEndpoint(shipmenthttp.UnconfiguredIntake{}, wiring.withdrawal)},
			}
		},
	})
}
