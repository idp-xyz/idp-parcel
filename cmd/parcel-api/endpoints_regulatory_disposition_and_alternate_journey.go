package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

type regulatoryDispositionAndJourneyWiring struct {
	acceptance tfhttp.RegulatoryDispositionAcceptor
	journey    tfhttp.AlternateJourneyStarter
}

func init() {
	registerEndpoints(endpointFeature[regulatoryDispositionAndJourneyWiring]{
		lines: map[string]endpointLine{
			"/transport-fulfillment-regulatory-disposition-acceptances": {method: http.MethodPost, face: decisionFace(accessidentity.DecisionRegulatoryDispositionAcceptance)},
			"/transport-fulfillment-alternate-journey-starts":           {method: http.MethodPost, face: decisionFace(accessidentity.DecisionAlternateJourneyStart)},
		},
		build: func(sources endpointSources) (regulatoryDispositionAndJourneyWiring, error) {
			acceptance, err := buildRegulatoryDispositionAcceptance(sources.db)
			if err != nil {
				return regulatoryDispositionAndJourneyWiring{}, err
			}
			journey, err := buildAlternateJourneyStart(sources.db)
			return regulatoryDispositionAndJourneyWiring{acceptance: acceptance, journey: journey}, err
		},
		mount: func(channels endpointChannels, wiring regulatoryDispositionAndJourneyWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 监管运输处置承接口（UC-TF-001，ADR-0265、ADR-0337）：运输运营对关务协作事项形成承接决定，是管理台上作出的生产决定，挂
				// 操作者渠道「运营决定」能力面的监管运输处置承接一种。待补充是答复、不落决定（ADR-0265）；写开关换不了它（ADR-0150 决定三）。
				{Pattern: "/transport-fulfillment-regulatory-disposition-acceptances", Handler: tfhttp.NewAcceptRegulatoryDispositionEndpoint(channels.operatorDecisions.transport, wiring.acceptance)},
				// 替代/退运旅程启动口（UC-TF-007，与 UC-TF-001 共用，ADR-0337）：授权业务角色采用备用伙伴或退运，挂「运营决定」能力面的
				// 替代或退运旅程启动一种，与承接分开授予。
				{Pattern: "/transport-fulfillment-alternate-journey-starts", Handler: tfhttp.NewStartAlternateJourneyEndpoint(channels.operatorDecisions.transport, wiring.journey)},
			}
		},
	})
}
