package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

type fulfillmentCorrectionsWiring struct {
	chargeOccurrenceInvalidation tfhttp.ChargeOccurrenceInvalidator
	segmentCommissionWithdrawal  tfhttp.SegmentCommissionAdoptionWithdrawer
}

func init() {
	registerEndpoints(endpointFeature[fulfillmentCorrectionsWiring]{
		lines: map[string]endpointLine{
			"/transport-fulfillment-charge-occurrence-invalidations": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionTransportChargeOccurrenceInvalidation),
			},
			"/transport-fulfillment-segment-commission-adoption-withdrawals": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionFulfillmentSegmentCommissionAdoptionWithdrawal),
			},
		},
		build: func(sources endpointSources) (fulfillmentCorrectionsWiring, error) {
			// 运输收费发生项失效口（ADR-0232 决定四）。
			invalidation, err := buildChargeOccurrenceInvalidation(sources.db)
			if err != nil {
				return fulfillmentCorrectionsWiring{}, err
			}
			// 解除履约段采用口（ADR-0232 决定一至三）。
			withdrawal, err := buildSegmentCommissionAdoptionWithdrawal(sources.db)
			return fulfillmentCorrectionsWiring{chargeOccurrenceInvalidation: invalidation, segmentCommissionWithdrawal: withdrawal}, err
		},
		mount: func(channels endpointChannels, wiring fulfillmentCorrectionsWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 运输收费发生项失效口（ADR-0232 决定四）：说一项已登记的发生项所说的范围不成立，是操作者做出的生产决定，挂「运营决定」
				// 能力面的运输收费发生项失效一种；能登记的不因此能失效。写开关换不了它（ADR-0150 决定三）。
				{Pattern: "/transport-fulfillment-charge-occurrence-invalidations", Handler: tfhttp.NewInvalidateChargeOccurrenceEndpoint(channels.operatorDecisions.transport, wiring.chargeOccurrenceInvalidation)},
				// 解除履约段采用口（ADR-0232 决定一至三）：撤回一段与一份委托之间的声明，委托的开始随之撤回或重新派生；段上已按这份委托
				// 形成的发生项要先经上一口失效。挂「运营决定」能力面的解除履约段采用一种；能采用的不因此能解除。写开关换不了它（ADR-0150 决定三）。
				{Pattern: "/transport-fulfillment-segment-commission-adoption-withdrawals", Handler: tfhttp.NewWithdrawSegmentCommissionAdoptionEndpoint(channels.operatorDecisions.transport, wiring.segmentCommissionWithdrawal)},
			}
		},
	})
}
