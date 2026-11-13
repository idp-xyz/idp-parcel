package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
)

type customerDisclosureWiring struct {
	decider visibilityhttp.CustomerDisclosureDecider
}

func init() {
	registerEndpoints(endpointFeature[customerDisclosureWiring]{
		lines: map[string]endpointLine{
			"/customer-disclosure-decisions":      {method: http.MethodPost, face: decisionFace(accessidentity.DecisionCustomerDisclosureDecision)},
			"/customer-disclosure-authorizations": {method: http.MethodPost, face: decisionFace(accessidentity.DecisionCustomerDisclosureAuthorization)},
		},
		build: func(sources endpointSources) (customerDisclosureWiring, error) {
			faces, err := buildCustomerDisclosureFaces(sources.db)
			return customerDisclosureWiring{decider: faces}, err
		},
		mount: func(channels endpointChannels, wiring customerDisclosureWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 客户可见异常披露的两口（UC-VE-006 步 3–5）：发起披露判断与确认发布，各按自己那一种决定认证、分开授予。写开关
				// 换不了它们。
				{Pattern: "/customer-disclosure-decisions", Handler: visibilityhttp.NewDecideCustomerDisclosureEndpoint(channels.operatorDecisions.visibility, wiring.decider)},
				{Pattern: "/customer-disclosure-authorizations", Handler: visibilityhttp.NewAuthorizeCustomerDisclosureEndpoint(channels.operatorDecisions.visibility, wiring.decider)},
			}
		},
	})
}
