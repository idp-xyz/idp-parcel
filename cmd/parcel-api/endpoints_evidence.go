package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
)

type evidenceWiring struct {
	manager visibilityhttp.EvidenceManager
}

func init() {
	registerEndpoints(endpointFeature[evidenceWiring]{
		lines: map[string]endpointLine{
			"/evidence-submissions":             {method: http.MethodPost, face: decisionFace(accessidentity.DecisionEvidenceSubmission)},
			"/evidence-appraisals":              {method: http.MethodPost, face: decisionFace(accessidentity.DecisionEvidenceAppraisal)},
			"/evidence-disclosure-preparations": {method: http.MethodPost, face: decisionFace(accessidentity.DecisionEvidenceDisclosurePreparation)},
		},
		build: func(sources endpointSources) (evidenceWiring, error) {
			faces, err := buildEvidenceFaces(sources.db)
			return evidenceWiring{manager: faces}, err
		},
		mount: func(channels endpointChannels, wiring evidenceWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 证据的登记、评价与披露版本准备三口（UC-VE-007 步 2）：异常运营人员作出的生产决定，挂「运营决定」能力面，各按
				// 自己那一种认证。写开关换不了它们。
				{Pattern: "/evidence-submissions", Handler: visibilityhttp.NewSubmitEvidenceEndpoint(channels.operatorDecisions.visibility, wiring.manager)},
				{Pattern: "/evidence-appraisals", Handler: visibilityhttp.NewAppraiseEvidenceEndpoint(channels.operatorDecisions.visibility, wiring.manager)},
				{Pattern: "/evidence-disclosure-preparations", Handler: visibilityhttp.NewPrepareEvidenceDisclosureEndpoint(channels.operatorDecisions.visibility, wiring.manager)},
			}
		},
	})
}
