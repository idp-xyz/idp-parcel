package main

import (
	"net/http"

	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type customsAdmissionWiring struct {
	nodeCustomsZoneRegistration customshttp.NodeCustomsZoneRegistrar
}

func init() {
	registerEndpoints(endpointFeature[customsAdmissionWiring]{
		lines: map[string]endpointLine{
			"/customs-node-zone-registrations": {method: http.MethodPost, face: registryWriteFace()},
		},
		build: func(sources endpointSources) (customsAdmissionWiring, error) {
			// 节点关务区域登记口与动作准入判断（ADR-0206）。
			customsAdmission, err := buildCustomsAdmissionOrchestration(sources.db)
			return customsAdmissionWiring{nodeCustomsZoneRegistration: customsAdmission.nodeZoneRegistration}, err
		},
		mount: func(channels endpointChannels, wiring customsAdmissionWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 节点关务区域登记口（ADR-0206；ADR-0085 决定一）：关务登记写面族的一格，挂操作者渠道的登记册 Intake，写开关换
				// 不了它（ADR-0150 决定三）。事物词取登记 CLI 的命令名 node-zone，判据同关务各登记口。
				{Pattern: "/customs-node-zone-registrations", Handler: customshttp.NewRegisterNodeCustomsZoneEndpoint(channels.operatorRegistries.customs, wiring.nodeCustomsZoneRegistration)},
			}
		},
	})
}
