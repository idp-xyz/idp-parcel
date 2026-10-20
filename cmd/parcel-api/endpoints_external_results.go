package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type externalResultsWiring struct {
	results customshttp.ResultHandler
}

func init() {
	registerEndpoints(endpointFeature[externalResultsWiring]{
		lines: map[string]endpointLine{
			"/customs/external-results": {method: http.MethodPost, face: integrationClientFace(accessidentity.FactCustomsExternalResult)},
		},
		build: func(sources endpointSources) (externalResultsWiring, error) {
			results, err := buildExternalResultsOrchestration(sources.db)
			return externalResultsWiring{results: results}, err
		},
		mount: func(channels endpointChannels, wiring externalResultsWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				{Pattern: "/customs/external-results", Handler: customshttp.NewReceiveExternalResultEndpoint(customshttp.ResultIntake(channels.integrationClients.customs), wiring.results)},
			}
		},
	})
}
