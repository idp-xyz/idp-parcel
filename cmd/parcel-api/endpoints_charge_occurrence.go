package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

type chargeOccurrenceWiring struct {
	chargeOccurrenceRegistration tfhttp.ChargeOccurrenceRegistrar
}

func init() {
	registerEndpoints(endpointFeature[chargeOccurrenceWiring]{
		lines: map[string]endpointLine{
			"/transport-fulfillment-charge-occurrence-registrations": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionTransportChargeOccurrenceRegistration),
			},
		},
		build: func(sources endpointSources) (chargeOccurrenceWiring, error) {
			// 运输收费发生项登记口（ADR-0207）。
			registration, err := buildChargeOccurrenceRegistration(sources.db)
			return chargeOccurrenceWiring{chargeOccurrenceRegistration: registration}, err
		},
		mount: func(channels endpointChannels, wiring chargeOccurrenceWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 运输收费发生项登记口（ADR-0207）：对已发生的履约事实作一次采购判断，是操作者做出的生产决定，挂「运营决定」能力面的
				// 运输收费发生项登记一种（ADR-0151 定族判据），不挂作业事实。写开关换不了它（ADR-0150 决定三）。
				{Pattern: "/transport-fulfillment-charge-occurrence-registrations", Handler: tfhttp.NewRegisterChargeOccurrenceEndpoint(channels.operatorDecisions.transport, wiring.chargeOccurrenceRegistration)},
			}
		},
	})
}
