package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

type externalCarrierCredentialWiring struct {
	credentialRegistration tfhttp.CredentialRegistrar
}

func init() {
	registerEndpoints(endpointFeature[externalCarrierCredentialWiring]{
		lines: map[string]endpointLine{
			"/transport-fulfillment-external-carrier-credential-registrations":         {method: http.MethodPost, face: registryWriteFace()},
			"/transport-fulfillment-external-carrier-credential-applicability-changes": {method: http.MethodPost, face: registryWriteFace()},
		},
		build: func(sources endpointSources) (externalCarrierCredentialWiring, error) {
			registration, err := buildExternalCarrierCredentialRegistration(sources.db)
			return externalCarrierCredentialWiring{credentialRegistration: registration}, err
		},
		mount: func(channels endpointChannels, wiring externalCarrierCredentialWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 外部承运凭证登记两口（ADR-0085，票 label-channel/18）：登记一份凭证的首版，与对它此刻的当前版落
				// 一次作废 / 失效 / 替代。它们是运营登记不是承运方回传口，路径取读面册名前缀 `transport-fulfillment-`；
				// 改变口不叫 `-corrections`——作废、失效、替代改变的是适用关系而不是更正一个判断，原版本一字不动。
				// 挂操作者渠道的「登记册配置写」能力面（ADR-0100 决定四；票 operator-channel/04）：一份凭证登进去就会被收编执行器
				// 用来把外部轨迹认到某个载运对象上，这两行比查阅行更不能让隔离读开关换值。
				{Pattern: "/transport-fulfillment-external-carrier-credential-registrations", Handler: tfhttp.NewRegisterExternalCarrierCredentialEndpoint(channels.operatorRegistries.transport, wiring.credentialRegistration)},
				{Pattern: "/transport-fulfillment-external-carrier-credential-applicability-changes", Handler: tfhttp.NewChangeExternalCarrierCredentialApplicabilityEndpoint(channels.operatorRegistries.transport, wiring.credentialRegistration)},
			}
		},
	})
}
