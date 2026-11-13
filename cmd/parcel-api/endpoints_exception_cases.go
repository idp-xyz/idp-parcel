package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
)

type exceptionCasesWiring struct {
	manager visibilityhttp.CaseManager
}

func init() {
	registerEndpoints(endpointFeature[exceptionCasesWiring]{
		lines: map[string]endpointLine{
			"/exception-case-take-ups": {method: http.MethodPost, face: decisionFace(accessidentity.DecisionExceptionCaseTakeUp)},
			"/exception-case-closures": {method: http.MethodPost, face: decisionFace(accessidentity.DecisionExceptionCaseClosure)},
			"/exception-case-merges":   {method: http.MethodPost, face: decisionFace(accessidentity.DecisionExceptionCaseMerge)},
		},
		build: func(sources endpointSources) (exceptionCasesWiring, error) {
			faces, err := buildExceptionCaseFaces(sources.db)
			return exceptionCasesWiring{manager: faces}, err
		},
		mount: func(channels endpointChannels, wiring exceptionCasesWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 异常案件的接单、关闭与归并三口（UC-VE-005）：异常运营人员作出的生产决定，挂「运营决定」能力面，各按自己那一种
				// 认证。写开关换不了它们。
				{Pattern: "/exception-case-take-ups", Handler: visibilityhttp.NewTakeUpExceptionCaseEndpoint(channels.operatorDecisions.visibility, wiring.manager)},
				{Pattern: "/exception-case-closures", Handler: visibilityhttp.NewCloseExceptionCaseEndpoint(channels.operatorDecisions.visibility, wiring.manager)},
				{Pattern: "/exception-case-merges", Handler: visibilityhttp.NewMergeExceptionCaseEndpoint(channels.operatorDecisions.visibility, wiring.manager)},
			}
		},
	})
}
