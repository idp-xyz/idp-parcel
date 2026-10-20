package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

type effectiveTimeRuleWiring struct {
	effectiveTimeRuleRegistration tfhttp.EffectiveTimeRuleRegistrar
}

func init() {
	registerEndpoints(endpointFeature[effectiveTimeRuleWiring]{
		lines: map[string]endpointLine{
			"/transport-fulfillment-effective-time-rule-registrations": {method: http.MethodPost, face: registryWriteFace()},
		},
		build: func(sources endpointSources) (effectiveTimeRuleWiring, error) {
			registration, err := buildEffectiveTimeRuleRegistration(sources.db)
			return effectiveTimeRuleWiring{effectiveTimeRuleRegistration: registration}, err
		},
		mount: func(channels endpointChannels, wiring effectiveTimeRuleWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 有效时间规则登记（label-channel/19）与凭证两行同一格：一版规则登进去会让收编执行器替该源
				// 此后每一条素材形成有效时间，挂操作者渠道的「登记册配置写」能力面（ADR-0100 决定四）。
				{Pattern: "/transport-fulfillment-effective-time-rule-registrations", Handler: tfhttp.NewRegisterEffectiveTimeRuleEndpoint(channels.operatorRegistries.transport, wiring.effectiveTimeRuleRegistration)},
			}
		},
	})
}
