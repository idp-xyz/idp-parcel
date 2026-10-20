package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

type departureGatePreviewWiring struct {
	departureGatePreview tfhttp.DepartureGatePreviewer
}

func init() {
	registerEndpoints(endpointFeature[departureGatePreviewWiring]{
		lines: map[string]endpointLine{
			"/transport-fulfillment/departure-gate-previews": {method: http.MethodPost, face: workFactFace(accessidentity.WorkScopeOffsite)},
		},
		build: func(sources endpointSources) (departureGatePreviewWiring, error) {
			// 设备出发前门禁问询（ADR-0219）：与出发登记同一处读数构造，只读。
			preview, err := buildDepartureGatePreview(sources.db)
			return departureGatePreviewWiring{departureGatePreview: preview}, err
		},
		mount: func(channels endpointChannels, wiring departureGatePreviewWiring) []httpapi.BusinessEndpoint {
			// 出发前门禁问询只读、不形成事实，随移动事实口挂这一面（ADR-0219 决定四）。
			departureGatePreviewIntake := tfhttp.DepartureGatePreviewIntake(tfhttp.UnconfiguredIntake{})
			if channels.workFactTransport != nil {
				departureGatePreviewIntake = channels.workFactTransport
			}
			return []httpapi.BusinessEndpoint{
				// 设备出发前门禁问询口（ADR-0219）：只读，交回与随后同参出发同一套读数，本上下文不落任何东西。挂作业事实渠道、与移动事实口
				// 同一份认证与授予——能报出发就能问出发前那一问，不另立能力种类；写开关换不了它（ADR-0150 决定三）。
				{Pattern: "/transport-fulfillment/departure-gate-previews", Handler: tfhttp.NewPreviewDepartureGateEndpoint(departureGatePreviewIntake, wiring.departureGatePreview)},
			}
		},
	})
}
