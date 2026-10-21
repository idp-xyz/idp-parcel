package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

type commissionAgreementRevisionWiring struct {
	reviser tfhttp.CommissionAgreementReviser
}

func init() {
	registerEndpoints(endpointFeature[commissionAgreementRevisionWiring]{
		lines: map[string]endpointLine{
			"/transport-fulfillment-transport-commission-agreement-revisions": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionTransportCommissionAgreementRevision),
			},
		},
		build: func(sources endpointSources) (commissionAgreementRevisionWiring, error) {
			reviser, err := buildCommissionAgreementRevision(sources.db)
			return commissionAgreementRevisionWiring{reviser: reviser}, err
		},
		mount: func(channels endpointChannels, wiring commissionAgreementRevisionWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 委托协议换版口（ADR-0231 决定三）：指名一份运输委托自哪一业务时刻起改按同一协议对象的后继版本采购，是操作者做出的生产决定，
				// 挂「运营决定」能力面的委托协议换版一种。写开关换不了它（ADR-0150 决定三）。
				{Pattern: "/transport-fulfillment-transport-commission-agreement-revisions", Handler: tfhttp.NewReviseCommissionAgreementEndpoint(channels.operatorDecisions.transport, wiring.reviser)},
			}
		},
	})
}
