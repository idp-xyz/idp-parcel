package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	networkhttp "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/http"
	nrpostgres "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type rerouteDecisionWiring struct {
	decider     networkhttp.RerouteDecider
	suggestions networkhttp.RerouteSuggestionCatalogueReader
}

func init() {
	registerEndpoints(endpointFeature[rerouteDecisionWiring]{
		lines: map[string]endpointLine{
			"/route-reroute-decisions":   {method: http.MethodPost, face: decisionFace(accessidentity.DecisionAuthorizedReroute)},
			"/route-reroute-suggestions": {method: http.MethodGet, face: operationsReadFace()},
		},
		build: func(sources endpointSources) (rerouteDecisionWiring, error) {
			decider, err := buildRerouteDecision(sources.db)
			if err != nil {
				return rerouteDecisionWiring{}, err
			}
			suggestions, err := nrpostgres.NewRerouteSuggestionCatalogue(sources.db)
			return rerouteDecisionWiring{decider: decider, suggestions: suggestions}, err
		},
		mount: func(channels endpointChannels, wiring rerouteDecisionWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 授权改路口（ADR-0284 决定一）：授权角色据一份改路建议择一候选、形成改路决定，是操作者做出的生产决定，挂「运营决定」
				// 能力面的授权改路一种。写开关换不了它（ADR-0150 决定三）。
				{Pattern: "/route-reroute-decisions", Handler: networkhttp.NewDecideRerouteEndpoint(channels.operatorDecisions.networkRouting, wiring.decider)},
				// 改路建议读面（ADR-0284 决定六）同属运营查阅，与路由判断两册共用网络目录查阅那一只 Intake。
				{Pattern: "/route-reroute-suggestions", Handler: networkhttp.NewQueryRerouteSuggestionsEndpoint(channels.networkCatalog(), wiring.suggestions)},
			}
		},
	})
}
