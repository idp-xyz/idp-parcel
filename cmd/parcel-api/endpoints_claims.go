package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
)

type claimsWiring struct {
	claims visibilityhttp.ClaimReceiver
}

func init() {
	registerEndpoints(endpointFeature[claimsWiring]{
		lines: map[string]endpointLine{
			"/claims": {method: http.MethodPost, face: unconfiguredFace()},
		},
		build: func(sources endpointSources) (claimsWiring, error) {
			claims, err := buildClaimsOrchestration(sources.db)
			return claimsWiring{claims: claims}, err
		},
		mount: func(_ endpointChannels, wiring claimsWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				{Pattern: "/claims", Handler: visibilityhttp.NewReceiveClaimEndpoint(visibilityhttp.UnconfiguredIntake{}, wiring.claims)},
			}
		},
	})
}
