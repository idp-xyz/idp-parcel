package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

type capacityPoolWiring struct {
	capacityPoolRegistration tfhttp.CapacityPoolRegistrar
}

func init() {
	registerEndpoints(endpointFeature[capacityPoolWiring]{
		lines: map[string]endpointLine{
			"/transport-fulfillment-capacity-pool-registrations": {method: http.MethodPost, face: registryWriteFace()},
		},
		build: func(sources endpointSources) (capacityPoolWiring, error) {
			registration, err := buildCapacityPoolRegistration(sources.db)
			return capacityPoolWiring{capacityPoolRegistration: registration}, err
		},
		mount: func(channels endpointChannels, wiring capacityPoolWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 容量池登记口（ADR-0201 决定一）：在一个已登记的班次上登一个维度的池，族与周期安排、具体班次登记口同；
				// 池身份由班次与维度派生，不由登记方起名。受控 CLI parcel-transport-register 的 capacity-pool 命令调的是同一个编排。
				{Pattern: "/transport-fulfillment-capacity-pool-registrations", Handler: tfhttp.NewRegisterCapacityPoolEndpoint(channels.operatorRegistries.transport, wiring.capacityPoolRegistration)},
			}
		},
	})
}
