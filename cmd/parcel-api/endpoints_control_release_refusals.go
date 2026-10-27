package main

import (
	"net/http"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type controlReleaseRefusalsWiring struct {
	refusals shipmenthttp.ControlReleaseRefusalCatalogueReader
}

func init() {
	registerEndpoints(endpointFeature[controlReleaseRefusalsWiring]{
		lines: map[string]endpointLine{
			"/control-release-refusals": {method: http.MethodGet, face: operationsReadFace()},
		},
		build: func(sources endpointSources) (controlReleaseRefusalsWiring, error) {
			// 读口就是释放未受理册上「没有对应处理留痕」的行（ADR-0293 决定一）：读面不是编排，不触发处理。
			refusals, err := pspostgres.NewControlReleaseRefusalCatalogue(sources.db)
			return controlReleaseRefusalsWiring{refusals: refusals}, err
		},
		mount: func(channels endpointChannels, wiring controlReleaseRefusalsWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 释放未受理册查阅（ADR-0293 决定一）：运营查阅，授权边界是租户；与面单交易、渠道择优决定两口共用委托上下文那一只操作者查阅
				// Intake。处理是「运营决定」能力面上另一个端点，查阅面里没有动作。
				{Pattern: "/control-release-refusals", Handler: shipmenthttp.NewQueryControlReleaseRefusalsEndpoint(channels.controlReleaseRefusals(), wiring.refusals)},
			}
		},
	})
}

func (channels endpointChannels) controlReleaseRefusals() shipmenthttp.ControlReleaseRefusalQueryIntake {
	if channels.operatorRegistries.shipmentOperationsRead != nil {
		return channels.operatorRegistries.shipmentOperationsRead
	}
	return shipmenthttp.UnconfiguredIntake{}
}
