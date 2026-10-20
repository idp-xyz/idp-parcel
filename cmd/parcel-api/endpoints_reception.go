package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type receptionWiring struct {
	reception nodeopshttp.ReceptionHandler
}

func init() {
	registerEndpoints(endpointFeature[receptionWiring]{
		lines: map[string]endpointLine{
			"/node-operations/receptions": {method: http.MethodPost, face: workFactFace(accessidentity.WorkScopeNode)},
		},
		build: func(sources endpointSources) (receptionWiring, error) {
			reception, err := buildReceptionOrchestration(sources.db)
			return receptionWiring{reception: reception}, err
		},
		mount: func(channels endpointChannels, wiring receptionWiring) []httpapi.BusinessEndpoint {
			receptionIntake := nodeopshttp.ReceptionIntake(nodeopshttp.UnconfiguredIntake{})
			if channels.workFactReception != nil {
				receptionIntake = channels.workFactReception
			}
			return []httpapi.BusinessEndpoint{
				{Pattern: "/node-operations/receptions", Handler: nodeopshttp.NewReceiveDeliveredUnitEndpoint(receptionIntake, wiring.reception)},
			}
		},
	})
}
