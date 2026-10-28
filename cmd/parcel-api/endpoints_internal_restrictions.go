package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type internalRestrictionsWiring struct {
	establisher customshttp.InternalRestrictionEstablisher
	decider     customshttp.InternalRestrictionDecider
}

func init() {
	registerEndpoints(endpointFeature[internalRestrictionsWiring]{
		lines: map[string]endpointLine{
			"/customs-internal-restrictions":          {method: http.MethodPost, face: decisionFace(accessidentity.DecisionInternalRestrictionEstablishment)},
			"/customs-internal-restriction-decisions": {method: http.MethodPost, face: decisionFace(accessidentity.DecisionInternalRestrictionDecision)},
		},
		build: func(sources endpointSources) (internalRestrictionsWiring, error) {
			faces, err := buildInternalRestrictionFaces(sources.db)
			return internalRestrictionsWiring{establisher: faces, decider: faces}, err
		},
		mount: func(channels endpointChannels, wiring internalRestrictionsWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 内部合规限制的建立与决定两口（UC-CC-011，ADR-0334 决定七）：运营人员作出的生产决定，挂「运营决定」能力面，各按自己那一种
				// 认证——能建立的不因此能确认或解除。限制只能由原责任来源追加决定（领域判）；决定方对来源的当前授权核验不在本口（ADR-0334
				// 越权风险点 1 的后半），授权引用只被保存。写开关换不了它们（ADR-0150 决定三）。
				{Pattern: "/customs-internal-restrictions", Handler: customshttp.NewEstablishInternalRestrictionEndpoint(channels.operatorDecisions.customs, wiring.establisher)},
				{Pattern: "/customs-internal-restriction-decisions", Handler: customshttp.NewDecideInternalRestrictionEndpoint(channels.operatorDecisions.customs, wiring.decider)},
			}
		},
	})
}
