package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

type deliveryDispatchReaskWiring struct {
	reask   tfhttp.DeliveryDispatchReasker
	cadence tfhttp.ReaskCadenceRegistrar
}

func init() {
	registerEndpoints(endpointFeature[deliveryDispatchReaskWiring]{
		lines: map[string]endpointLine{
			"/transport-fulfillment-delivery-dispatch-reasks":                      {method: http.MethodPost, face: decisionFace(accessidentity.DecisionDeliveryDispatchReask)},
			"/transport-fulfillment-delivery-dispatch-reask-cadence-registrations": {method: http.MethodPost, face: registryWriteFace()},
		},
		build: func(sources endpointSources) (deliveryDispatchReaskWiring, error) {
			reask, err := buildDeliveryDispatchReask(sources.db)
			if err != nil {
				return deliveryDispatchReaskWiring{}, err
			}
			cadence, err := buildReaskCadenceRegistration(sources.db)
			return deliveryDispatchReaskWiring{reask: reask, cadence: cadence}, err
		},
		mount: func(channels endpointChannels, wiring deliveryDispatchReaskWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 按派送待形成册批量重问（ADR-0230 决定四）：运营人员把册上待形成的那几件交回派送发起执行器再问一次所有者。挂操作者渠道的
				// 「运营决定」能力面，与补拍口同一种决定；写开关换不了这一行（ADR-0150 决定三）。
				{Pattern: "/transport-fulfillment-delivery-dispatch-reasks", Handler: tfhttp.NewReaskDeliveryDispatchesEndpoint(channels.operatorDecisions.transport, wiring.reask)},
				// 派送重问间隔登记（ADR-0230 决定五）：租户取值，挂登记册配置写；没登过的租户到期重问答未配置。
				{Pattern: "/transport-fulfillment-delivery-dispatch-reask-cadence-registrations", Handler: tfhttp.NewRegisterReaskCadenceEndpoint(channels.operatorRegistries.transport, wiring.cadence)},
			}
		},
	})
}
