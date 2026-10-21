package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type customsDecisionsWiring struct {
	caseEstablishment       customshttp.CaseEstablisher
	submissionAuthorization customshttp.SubmissionAuthorizationConfirmer
	declarationSubmission   customshttp.DeclarationSubmitter
}

func init() {
	registerEndpoints(endpointFeature[customsDecisionsWiring]{
		lines: map[string]endpointLine{
			"/customs-cases": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionCustomsCaseEstablishment),
			},
			"/customs-submission-authorizations": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionCustomsSubmissionAuthorization),
			},
			"/customs-declaration-submissions": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionCustomsDeclarationSubmission),
			},
		},
		build: func(sources endpointSources) (customsDecisionsWiring, error) {
			faces, err := buildCustomsDecisionFaces(sources.db)
			return customsDecisionsWiring{
				caseEstablishment:       faces.caseEstablishment,
				submissionAuthorization: faces.submissionAuthorization,
				declarationSubmission:   faces.declarationSubmission,
			}, err
		},
		mount: func(channels endpointChannels, wiring customsDecisionsWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 关务建案、提交授权确认与申报提交执行三口（ADR-0227）：都是运营人员作出的生产决定，挂「运营决定」能力面，各按自己
				// 那一种认证——能确认授权的不因此能发起提交。生产进程里提交只从第三口进，没有任何后台触发会形成提交版本；写开关
				// 换不了它们（ADR-0150 决定三），合成租户与真实租户走同一条命令面。
				{Pattern: "/customs-cases", Handler: customshttp.NewEstablishCaseEndpoint(channels.operatorDecisions.customs, wiring.caseEstablishment)},
				{Pattern: "/customs-submission-authorizations", Handler: customshttp.NewConfirmSubmissionAuthorizationEndpoint(channels.operatorDecisions.customs, wiring.submissionAuthorization)},
				{Pattern: "/customs-declaration-submissions", Handler: customshttp.NewSubmitDeclarationEndpoint(channels.operatorDecisions.customs, wiring.declarationSubmission)},
			}
		},
	})
}
