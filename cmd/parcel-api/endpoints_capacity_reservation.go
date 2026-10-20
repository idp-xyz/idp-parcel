package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

type capacityReservationWiring struct {
	capacityReservation tfhttp.CapacityReserver
	capacityRelease     tfhttp.CapacityReleaser
}

func init() {
	registerEndpoints(endpointFeature[capacityReservationWiring]{
		lines: map[string]endpointLine{
			"/transport-fulfillment-capacity-reservations": {method: http.MethodPost, face: decisionFace(accessidentity.DecisionCapacityReservation)},
			"/transport-fulfillment-capacity-releases":     {method: http.MethodPost, face: decisionFace(accessidentity.DecisionCapacityRelease)},
		},
		build: func(sources endpointSources) (capacityReservationWiring, error) {
			// 容量预占口与释放口（ADR-0203）。
			decisions, err := buildCapacityDecisions(sources.db)
			return capacityReservationWiring{capacityReservation: decisions, capacityRelease: decisions}, err
		},
		mount: func(channels endpointChannels, wiring capacityReservationWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 容量预占口与释放口（ADR-0203 决定三）：为一个业务范围占住、或还回一个池的一段容量，是操作者做出的生产决定，挂
				// 「运营决定」能力面、各按自己那一种认证（ADR-0151 定族判据）；建池是登记册配置写，在容量池登记口那一行。写开关
				// 换不了它们（ADR-0150 决定三）。
				{Pattern: "/transport-fulfillment-capacity-reservations", Handler: tfhttp.NewReserveCapacityEndpoint(channels.operatorDecisions.transport, wiring.capacityReservation)},
				{Pattern: "/transport-fulfillment-capacity-releases", Handler: tfhttp.NewReleaseCapacityEndpoint(channels.operatorDecisions.transport, wiring.capacityRelease)},
			}
		},
	})
}
