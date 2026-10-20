package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type dispositionWiring struct {
	disposition shipmenthttp.AuthorizedDispositionHandler
}

func init() {
	registerEndpoints(endpointFeature[dispositionWiring]{
		lines: map[string]endpointLine{
			"/shipment-requests/authorized-dispositions": {method: http.MethodPost, face: decisionFace(accessidentity.DecisionAuthorizedDisposition)},
		},
		build: func(sources endpointSources) (dispositionWiring, error) {
			// 授权处置编排（ADR-0132）：处置授权那一格今天如实未配置（PC 授权动作词汇没有这一格），接进装配后
			// 停在`等待授权处置`的委托才有生产入口，停点从「没有入口」变成「说得出停在授权未配置」。
			disposition, err := buildDispositionOrchestration(sources.db)
			return dispositionWiring{disposition: disposition}, err
		},
		mount: func(channels endpointChannels, wiring dispositionWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 授权处置命令口（票 sa-preacceptance-policy-view/04；ADR-0132）：授权角色对停在`等待授权处置`的
				// 委托选去向。运营侧写行，挂操作者渠道的「运营决定」能力面（ADR-0151）：处置人取认证出的提交操作者，有没有权由
				// 编排问 party-commercial。
				{Pattern: "/shipment-requests/authorized-dispositions", Handler: shipmenthttp.NewDisposeShipmentRequestEndpoint(channels.operatorDecisions.shipment, wiring.disposition)},
			}
		},
	})
}
