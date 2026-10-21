package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

type fulfillmentCorrectionsWiring struct {
	chargeOccurrenceInvalidation tfhttp.ChargeOccurrenceInvalidator
}

func init() {
	registerEndpoints(endpointFeature[fulfillmentCorrectionsWiring]{
		lines: map[string]endpointLine{
			"/transport-fulfillment-charge-occurrence-invalidations": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionTransportChargeOccurrenceInvalidation),
			},
		},
		build: func(sources endpointSources) (fulfillmentCorrectionsWiring, error) {
			// 运输收费发生项失效口（ADR-0232 决定四）。
			invalidation, err := buildChargeOccurrenceInvalidation(sources.db)
			return fulfillmentCorrectionsWiring{chargeOccurrenceInvalidation: invalidation}, err
		},
		mount: func(channels endpointChannels, wiring fulfillmentCorrectionsWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 运输收费发生项失效口（ADR-0232 决定四）：说一项已登记的发生项所说的范围不成立，是操作者做出的生产决定，挂「运营决定」
				// 能力面的运输收费发生项失效一种；能登记的不因此能失效。写开关换不了它（ADR-0150 决定三）。
				{Pattern: "/transport-fulfillment-charge-occurrence-invalidations", Handler: tfhttp.NewInvalidateChargeOccurrenceEndpoint(channels.operatorDecisions.transport, wiring.chargeOccurrenceInvalidation)},
			}
		},
	})
}
