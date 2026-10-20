package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

type deliveryDispatchWiring struct {
	deliveryDispatchTrigger tfhttp.DeliveryDispatchTriggerer
}

func init() {
	registerEndpoints(endpointFeature[deliveryDispatchWiring]{
		lines: map[string]endpointLine{
			"/transport-fulfillment-delivery-dispatch-triggers": {method: http.MethodPost, face: workFactFace(accessidentity.WorkScopeOffsite)},
		},
		build: func(sources endpointSources) (deliveryDispatchWiring, error) {
			// 末端派送任务内部触发执行器（ADR-0114 决定二），只给补拍口用：进程内的那一拍由派发进程随交接登记信封发起
			// （ADR-0216）。三条派送要求缝的接法见 buildDeliveryDispatchTrigger。
			trigger, err := buildDeliveryDispatchTrigger(sources.db)
			return deliveryDispatchWiring{deliveryDispatchTrigger: trigger}, err
		},
		mount: func(channels endpointChannels, wiring deliveryDispatchWiring) []httpapi.BusinessEndpoint {
			deliveryDispatchTriggerIntake := tfhttp.DeliveryDispatchTriggerIntake(tfhttp.UnconfiguredIntake{})
			if channels.workFactTransport != nil {
				deliveryDispatchTriggerIntake = channels.workFactTransport
			}
			return []httpapi.BusinessEndpoint{
				// 末端派送任务内部触发执行器的补拍口（ADR-0216 决定四）：进程内的那一拍由派发进程随交接登记信封发起，这一口留给
				// 那一拍之后还要再发起的情形——所有者补齐了要求，没有别的东西会回头再发起。挂作业事实渠道（ADR-0149；它不在
				// ADR-0151 决定六改归运营决定的那几口里）。与建派送任务那一口手工建任务是两件事。
				{Pattern: "/transport-fulfillment-delivery-dispatch-triggers", Handler: tfhttp.NewTriggerDeliveryDispatchEndpoint(deliveryDispatchTriggerIntake, wiring.deliveryDispatchTrigger)},
			}
		},
	})
}
