package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type regulatoryRestrictionsWiring struct {
	establisher customshttp.RegulatoryRestrictionEstablisher
	releaser    customshttp.RegulatoryRestrictionReleaser
}

func init() {
	registerEndpoints(endpointFeature[regulatoryRestrictionsWiring]{
		lines: map[string]endpointLine{
			"/customs-regulatory-restrictions":         {method: http.MethodPost, face: decisionFace(accessidentity.DecisionRegulatoryRestrictionEstablishment)},
			"/customs-regulatory-restriction-releases": {method: http.MethodPost, face: decisionFace(accessidentity.DecisionRegulatoryRestrictionRelease)},
		},
		build: func(sources endpointSources) (regulatoryRestrictionsWiring, error) {
			faces, err := buildRegulatoryRestrictionFaces(sources.db)
			return regulatoryRestrictionsWiring{establisher: faces, releaser: faces}, err
		},
		mount: func(channels endpointChannels, wiring regulatoryRestrictionsWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 监管限制的建立与解除两口：关务人员作出的生产决定，挂「运营决定」能力面，各按自己那一种认证。依据是一条已接收的
				// 外部监管结果，由编排对外部结果册核；写开关换不了它们。
				{Pattern: "/customs-regulatory-restrictions", Handler: customshttp.NewEstablishRegulatoryRestrictionEndpoint(channels.operatorDecisions.customs, wiring.establisher)},
				{Pattern: "/customs-regulatory-restriction-releases", Handler: customshttp.NewReleaseRegulatoryRestrictionEndpoint(channels.operatorDecisions.customs, wiring.releaser)},
			}
		},
	})
}
