package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

type deliveryGatePreviewWiring struct {
	deliveryGatePreview tfhttp.DeliveryGatePreviewer
}

func init() {
	registerEndpoints(endpointFeature[deliveryGatePreviewWiring]{
		lines: map[string]endpointLine{
			"/transport-fulfillment/delivery-gate-previews": {method: http.MethodPost, face: workFactFace(accessidentity.WorkScopeOffsite)},
		},
		build: func(sources endpointSources) (deliveryGatePreviewWiring, error) {
			// 派送出发前的交付门禁问询（ADR-0238）：与有效交付登记同一处读数构造，只读。
			preview, err := buildDeliveryGatePreview(sources.db)
			return deliveryGatePreviewWiring{deliveryGatePreview: preview}, err
		},
		mount: func(channels endpointChannels, wiring deliveryGatePreviewWiring) []httpapi.BusinessEndpoint {
			// 交付前问询只读、不形成事实，随有效交付口挂作业事实这一面（ADR-0238 决定四）。
			deliveryGatePreviewIntake := tfhttp.DeliveryGatePreviewIntake(tfhttp.UnconfiguredIntake{})
			if channels.workFactTransport != nil {
				deliveryGatePreviewIntake = channels.workFactTransport
			}
			return []httpapi.BusinessEndpoint{
				// 派送出发前的交付门禁问询口（ADR-0238）：只读，交回与随后同参有效交付同一套读数，本上下文不落任何东西。挂作业事实渠道、
				// 与有效交付口同一份认证与授予——能登交付就能问派送出发前那一问，不另立能力种类；写开关换不了它（ADR-0150 决定三）。
				{Pattern: "/transport-fulfillment/delivery-gate-previews", Handler: tfhttp.NewPreviewDeliveryGateEndpoint(deliveryGatePreviewIntake, wiring.deliveryGatePreview)},
			}
		},
	})
}
