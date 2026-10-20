package main

import (
	"net/http"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type preAdviceWindowWiring struct {
	preAdviceWindowRegistration nodeopshttp.PreAdviceWindowRegistrar
}

func init() {
	registerEndpoints(endpointFeature[preAdviceWindowWiring]{
		lines: map[string]endpointLine{
			"/node-operations-pre-advice-window-registrations": {method: http.MethodPost, face: registryWriteFace()},
		},
		build: func(sources endpointSources) (preAdviceWindowWiring, error) {
			// 预报有效时窗登记口（ADR-0212 决定二末项）。
			registration, err := buildPreAdviceWindowRegistration(sources.db)
			return preAdviceWindowWiring{preAdviceWindowRegistration: registration}, err
		},
		mount: func(channels endpointChannels, wiring preAdviceWindowWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 预报有效时窗登记口（ADR-0212 决定二末项；ADR-0085 决定一）：节点作业登记写面的第一格，挂操作者渠道的登记册 Intake，写开关换
				// 不了它（ADR-0150 决定三）。事物词取登记 CLI parcel-node-operations-register 的族名 pre-advice-window。
				{Pattern: "/node-operations-pre-advice-window-registrations", Handler: nodeopshttp.NewRegisterPreAdviceWindowEndpoint(channels.operatorRegistries.nodeOperations, wiring.preAdviceWindowRegistration)},
			}
		},
	})
}
