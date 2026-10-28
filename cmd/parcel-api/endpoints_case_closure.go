package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type caseClosureWiring struct {
	closer customshttp.CaseCloser
}

func init() {
	registerEndpoints(endpointFeature[caseClosureWiring]{
		lines: map[string]endpointLine{
			"/customs-case-closures": {method: http.MethodPost, face: decisionFace(accessidentity.DecisionCustomsCaseClosure)},
		},
		build: func(sources endpointSources) (caseClosureWiring, error) {
			closer, err := buildCaseClosure(sources.db)
			return caseClosureWiring{closer: closer}, err
		},
		mount: func(channels endpointChannels, wiring caseClosureWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 关务案件关闭口（UC-CC-010）：有权角色对进行中的案件请求关闭，是管理台上作出的生产决定，挂「运营决定」能力面的关务案件关闭一种，
				// 与建案分开授予。有未解决或冲突项时答被阻并列出各项，不关；写开关换不了它（ADR-0150 决定三）。
				{Pattern: "/customs-case-closures", Handler: customshttp.NewCloseCaseEndpoint(channels.operatorDecisions.customs, wiring.closer)},
			}
		},
	})
}
