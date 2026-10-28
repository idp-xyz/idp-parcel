package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type postIntakeManualItemWiring struct {
	registration shipmenthttp.PostIntakeManualItemResultHandler
	items        shipmenthttp.PostIntakeManualItemReader
}

func init() {
	registerEndpoints(endpointFeature[postIntakeManualItemWiring]{
		lines: map[string]endpointLine{
			"/post-intake-disposition-manual-item-results": {method: http.MethodPost, face: decisionFace(accessidentity.DecisionPostIntakeManualItemHandling)},
			"/post-intake-disposition-manual-items":        {method: http.MethodGet, face: operationsReadFace()},
		},
		build: func(sources endpointSources) (postIntakeManualItemWiring, error) {
			registration, err := buildPostIntakeManualItemResult(sources.db)
			if err != nil {
				return postIntakeManualItemWiring{}, err
			}
			items, err := pspostgres.NewPostIntakeManualItems(sources.db)
			return postIntakeManualItemWiring{registration: registration, items: items}, err
		},
		mount: func(channels endpointChannels, wiring postIntakeManualItemWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 待人工处理项的处理结果登记（ADR-0341）：运营人员办完收寄后服务处置决定留下的人工请求项，逐项登记已处理或无法处理；挂「运营决定」能力面的
				// 「处理结果登记」一种，与收寄后服务处置的授予分开。
				{Pattern: "/post-intake-disposition-manual-item-results", Handler: shipmenthttp.NewRegisterPostIntakeManualItemResultEndpoint(channels.operatorDecisions.shipment, wiring.registration)},
				// 待人工处理清单与按包裹读回：运营查阅，授权边界是租户，与等回应请求查阅共用那一只操作者查阅 Intake；查阅面里没有动作。
				{Pattern: "/post-intake-disposition-manual-items", Handler: shipmenthttp.NewQueryPostIntakeManualItemsEndpoint(channels.postIntakeManualItems(), wiring.items)},
			}
		},
	})
}

func (channels endpointChannels) postIntakeManualItems() shipmenthttp.PostIntakeManualItemQueryIntake {
	if channels.operatorRegistries.shipmentOperationsRead != nil {
		return channels.operatorRegistries.shipmentOperationsRead
	}
	return shipmenthttp.UnconfiguredIntake{}
}
