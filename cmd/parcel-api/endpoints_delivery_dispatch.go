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
			"/transport-fulfillment-delivery-dispatch-triggers": {method: http.MethodPost, face: decisionFace(accessidentity.DecisionDeliveryDispatchReask)},
		},
		build: func(sources endpointSources) (deliveryDispatchWiring, error) {
			// 末端派送任务内部触发执行器（ADR-0114 决定二），只给补拍口用：进程内的那一拍由派发进程随交接登记信封发起
			// （ADR-0216）。三条派送要求缝的接法见 buildDeliveryDispatchTrigger。
			trigger, err := buildDeliveryDispatchTrigger(sources.db)
			return deliveryDispatchWiring{deliveryDispatchTrigger: trigger}, err
		},
		mount: func(channels endpointChannels, wiring deliveryDispatchWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 末端派送任务内部触发执行器的补拍口（ADR-0216 决定四）：进程内的那一拍由派发进程随交接登记信封发起，这一口留给
				// 那一拍之后还要再发起的情形——所有者补齐了要求，运营人员按派送待形成册对单个（段，对象）再问一次。挂操作者渠道的
				// 「运营决定」能力面，决定种类与按册重问同一种（ADR-0230 决定七）：补拍的人在管理台上，手里没有设备。与建派送任务
				// 那一口手工建任务是两件事。写开关换不了这一行（ADR-0150 决定三）。
				{Pattern: "/transport-fulfillment-delivery-dispatch-triggers", Handler: tfhttp.NewTriggerDeliveryDispatchEndpoint(channels.operatorDecisions.transport, wiring.deliveryDispatchTrigger)},
			}
		},
	})
}
