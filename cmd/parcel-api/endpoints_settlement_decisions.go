package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
)

type settlementDecisionsWiring struct {
	confirmer settlementhttp.ChargeConfirmer
	publisher settlementhttp.StatementPublisher
}

func init() {
	registerEndpoints(endpointFeature[settlementDecisionsWiring]{
		lines: map[string]endpointLine{
			"/settlement-charge-confirmations": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionChargeConfirmation),
			},
			"/settlement-statement-publications": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionStatementPublication),
			},
		},
		build: func(sources endpointSources) (settlementDecisionsWiring, error) {
			orchestrations, err := buildSettlementOrchestrations(sources.db)
			return settlementDecisionsWiring{confirmer: orchestrations, publisher: orchestrations}, err
		},
		mount: func(channels endpointChannels, wiring settlementDecisionsWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 运营确认口与截单发布口（ADR-0339 决定一、二）：两种结算运营决定，进的是 ADR-0170 装在这里的确认与截单编排，先过结算
				// 时点那一道门。写开关换不了它们（ADR-0150 决定三）。
				{Pattern: "/settlement-charge-confirmations", Handler: settlementhttp.NewConfirmChargeEndpoint(channels.operatorDecisions.settlement, wiring.confirmer)},
				{Pattern: "/settlement-statement-publications", Handler: settlementhttp.NewPublishStatementEndpoint(channels.operatorDecisions.settlement, wiring.publisher, systemClock{}.Now)},
			}
		},
	})
}
