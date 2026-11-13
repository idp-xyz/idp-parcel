package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
)

type settlementFollowUpWiring struct {
	adjuster settlementhttp.ChargeAdjuster
	followUp settlementhttp.StatementFollowUp
}

func init() {
	registerEndpoints(endpointFeature[settlementFollowUpWiring]{
		lines: map[string]endpointLine{
			"/settlement-charge-adjustments":     {method: http.MethodPost, face: decisionFace(accessidentity.DecisionChargeAdjustmentRegistration)},
			"/settlement-statement-voids":        {method: http.MethodPost, face: decisionFace(accessidentity.DecisionStatementVoid)},
			"/settlement-late-charge-inclusions": {method: http.MethodPost, face: decisionFace(accessidentity.DecisionLateChargeInclusion)},
			"/settlement-adjustment-inclusions":  {method: http.MethodPost, face: decisionFace(accessidentity.DecisionAdjustmentInclusion)},
		},
		build: func(sources endpointSources) (settlementFollowUpWiring, error) {
			orchestrations, err := buildSettlementOrchestrations(sources.db, sources.documentNumbers.Assigner)
			return settlementFollowUpWiring{adjuster: orchestrations, followUp: orchestrations}, err
		},
		mount: func(channels endpointChannels, wiring settlementFollowUpWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 对账单发布之后的四口（UC-SA-003 步 6–7）：费用调整登记、整单作废、后到费用与既有调整纳入后续账期。四种分开授予
				// 的结算运营决定，进的是确认与截单口装的同一组编排，不过结算时点那一道门。写开关换不了它们。
				{Pattern: "/settlement-charge-adjustments", Handler: settlementhttp.NewRecordChargeAdjustmentEndpoint(channels.operatorDecisions.settlement, wiring.adjuster)},
				{Pattern: "/settlement-statement-voids", Handler: settlementhttp.NewVoidStatementEndpoint(channels.operatorDecisions.settlement, wiring.followUp)},
				{Pattern: "/settlement-late-charge-inclusions", Handler: settlementhttp.NewIncludeLateChargeEndpoint(channels.operatorDecisions.settlement, wiring.followUp)},
				{Pattern: "/settlement-adjustment-inclusions", Handler: settlementhttp.NewIncludeAdjustmentEndpoint(channels.operatorDecisions.settlement, wiring.followUp)},
			}
		},
	})
}
