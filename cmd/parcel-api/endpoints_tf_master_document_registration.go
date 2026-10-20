package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

type masterDocumentRegistrationWiring struct {
	masterDocumentRegistration tfhttp.MasterDocumentRegistrar
}

func init() {
	registerEndpoints(endpointFeature[masterDocumentRegistrationWiring]{
		lines: map[string]endpointLine{
			"/transport-fulfillment-carrier-master-document-registrations": {method: http.MethodPost, face: registryWriteFace()},
			"/transport-fulfillment-carrier-master-document-revisions":     {method: http.MethodPost, face: registryWriteFace()},
		},
		build: func(sources endpointSources) (masterDocumentRegistrationWiring, error) {
			registration, err := buildMasterDocumentRegistration(sources.db)
			return masterDocumentRegistrationWiring{masterDocumentRegistration: registration}, err
		},
		mount: func(channels endpointChannels, wiring masterDocumentRegistrationWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 总单登记两口（ADR-0113 决定五；票 tf-carrier-master-document-register/01）：登记一份总单的首版，与对它此刻的
				// 当前版形成一次撤销 / 替代 / 关联重述。运营登记不是承运方回传口，路径取读面册名前缀 `transport-fulfillment-`；
				// 新版本口不叫 `-corrections`，理由同凭证。挂操作者渠道的「登记册配置写」能力面（ADR-0100 决定四）：一份总单登进去
				// 就成了 parcel-pricing 主单级评价可引的身份（ADR-0111），这两行不能让隔离读开关换值。
				{Pattern: "/transport-fulfillment-carrier-master-document-registrations", Handler: tfhttp.NewRegisterCarrierMasterDocumentEndpoint(channels.operatorRegistries.transport, wiring.masterDocumentRegistration)},
				{Pattern: "/transport-fulfillment-carrier-master-document-revisions", Handler: tfhttp.NewReviseCarrierMasterDocumentEndpoint(channels.operatorRegistries.transport, wiring.masterDocumentRegistration)},
			}
		},
	})
}
