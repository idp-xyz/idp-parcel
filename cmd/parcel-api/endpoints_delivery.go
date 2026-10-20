package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

type deliveryWiring struct {
	delivery        tfhttp.DeliveryHandler
	deliveryAttempt tfhttp.DeliveryAttemptHandler
}

func init() {
	registerEndpoints(endpointFeature[deliveryWiring]{
		lines: map[string]endpointLine{
			"/transport-fulfillment/deliveries":                 {method: http.MethodPost, face: workFactFace(accessidentity.WorkScopeOffsite)},
			"/transport-fulfillment/delivery-proof-corrections": {method: http.MethodPost, face: workFactFace(accessidentity.WorkScopeOffsite)},
			"/transport-fulfillment/delivery-attempts":          {method: http.MethodPost, face: workFactFace(accessidentity.WorkScopeOffsite)},
		},
		build: func(sources endpointSources) (deliveryWiring, error) {
			delivery, err := buildDeliveryOrchestration(sources.db)
			if err != nil {
				return deliveryWiring{}, err
			}
			deliveryAttempt, err := buildDeliveryAttemptOrchestration(sources.db)
			return deliveryWiring{delivery: delivery, deliveryAttempt: deliveryAttempt}, err
		},
		mount: func(channels endpointChannels, wiring deliveryWiring) []httpapi.BusinessEndpoint {
			deliveryRegistrationIntake := tfhttp.DeliveryRegistrationIntake(tfhttp.UnconfiguredIntake{})
			deliveryCorrectionIntake := tfhttp.DeliveryProofCorrectionIntake(tfhttp.UnconfiguredIntake{})
			deliveryAttemptIntake := tfhttp.DeliveryAttemptIntake(tfhttp.UnconfiguredIntake{})
			if channels.workFactTransport != nil {
				deliveryRegistrationIntake = channels.workFactTransport
				deliveryCorrectionIntake = channels.workFactTransport
				deliveryAttemptIntake = channels.workFactTransport
			}
			return []httpapi.BusinessEndpoint{
				{Pattern: "/transport-fulfillment/deliveries", Handler: tfhttp.NewRegisterEffectiveDeliveryEndpoint(deliveryRegistrationIntake, wiring.delivery)},
				{Pattern: "/transport-fulfillment/delivery-proof-corrections", Handler: tfhttp.NewCorrectDeliveryProofEndpoint(deliveryCorrectionIntake, wiring.delivery)},
				// 派送尝试登记（票 product-strategy-boundary/19）：执行方报来的每次到场与逐对象结果，交付生效只引用它登下的事实。
				// 作业事实，挂操作者渠道的「作业事实登记」能力面（ADR-0149 决定二）。
				{Pattern: "/transport-fulfillment/delivery-attempts", Handler: tfhttp.NewRecordDeliveryAttemptEndpoint(deliveryAttemptIntake, wiring.deliveryAttempt)},
			}
		},
	})
}
