package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
)

type etaEntryWiring struct {
	former visibilityhttp.ETAFormer
}

func init() {
	registerEndpoints(endpointFeature[etaEntryWiring]{
		lines: map[string]endpointLine{
			"/eta-prediction-manual-entries": {method: http.MethodPost, face: decisionFace(accessidentity.DecisionETAEntry)},
		},
		build: func(sources endpointSources) (etaEntryWiring, error) {
			former, err := buildETAEntry(sources.db)
			return etaEntryWiring{former: former}, err
		},
		mount: func(channels endpointChannels, wiring etaEntryWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 预计到达的手工录入口（dox/decisions.md「产品形态」）：没有预测来源对接时，操作员把承运商告知的或自己估的
				// 预计到达录进来。挂「运营决定」能力面；写开关换不了它。
				{Pattern: "/eta-prediction-manual-entries", Handler: visibilityhttp.NewEnterETAEndpoint(channels.operatorDecisions.visibility, wiring.former)},
			}
		},
	})
}
