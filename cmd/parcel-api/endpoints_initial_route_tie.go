package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	networkhttp "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/http"
	nrpostgres "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type initialRouteTieWiring struct {
	decider networkhttp.InitialRouteTieDecider
	ties    networkhttp.InitialRouteTieCatalogueReader
}

func init() {
	registerEndpoints(endpointFeature[initialRouteTieWiring]{
		lines: map[string]endpointLine{
			"/route-initial-route-decisions": {method: http.MethodPost, face: decisionFace(accessidentity.DecisionAuthorizedReroute)},
			"/route-initial-route-ties":      {method: http.MethodGet, face: operationsReadFace()},
		},
		build: func(sources endpointSources) (initialRouteTieWiring, error) {
			decider, err := buildInitialRouteTieDecision(sources.db)
			if err != nil {
				return initialRouteTieWiring{}, err
			}
			ties, err := nrpostgres.NewInitialRouteTieDecisions(sources.db)
			return initialRouteTieWiring{decider: decider, ties: ties}, err
		},
		mount: func(channels endpointChannels, wiring initialRouteTieWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 并列裁决口（ADR-0309 决定二、五）：授权角色在最低成本并列的候选之间择一、形成首个计划，是操作者做出的生产决定，
				// 挂「运营决定」能力面，与授权改路同一种授予。写开关换不了它（ADR-0150 决定三）。
				{Pattern: "/route-initial-route-decisions", Handler: networkhttp.NewDecideInitialRouteEndpoint(channels.operatorDecisions.networkRouting, wiring.decider)},
				// 并列读面（ADR-0309 决定六）同属运营查阅，与路由判断两册共用网络目录查阅那一只 Intake。
				{Pattern: "/route-initial-route-ties", Handler: networkhttp.NewQueryInitialRouteTiesEndpoint(channels.networkCatalog(), wiring.ties)},
			}
		},
	})
}
