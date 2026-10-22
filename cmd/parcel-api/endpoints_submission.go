package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type submissionWiring struct {
	submission shipmenthttp.SubmissionHandler
}

func init() {
	registerEndpoints(endpointFeature[submissionWiring]{
		lines: map[string]endpointLine{
			"/v1/shipment-requests": {method: http.MethodPost, face: customerClientFace(accessidentity.CustomerCapabilityShipmentRequestSubmission)},
		},
		build: func(sources endpointSources) (submissionWiring, error) {
			submission, err := buildSubmissionOrchestration(sources.db, sources.isolatedWrite, sources.selfAuthority)
			return submissionWiring{submission: submission}, err
		},
		mount: func(channels endpointChannels, wiring submissionWiring) []httpapi.BusinessEndpoint {
			// 提交口挂首方客户渠道的 Intake（ADR-0139 决定四；票 first-party-api/00），两个隔离开关都换不了它（ADR-0150 决定三）。
			// 它单独一个变量，与 endpointChannels 那组查阅变量分开，免得日后有人顺手把它并进 shipmentViews 那个 if 里——读开关开不了写行。发行方
			// 参数未设时 Intake 仍在，答的是未配置，与字面量未配置即拒同字节。
			submissionIntake := shipmenthttp.SubmissionIntake(shipmenthttp.UnconfiguredIntake{})
			if channels.customerSubmission != nil {
				submissionIntake = channels.customerSubmission
			}
			return []httpapi.BusinessEndpoint{
				{Pattern: "/v1/shipment-requests", Handler: shipmenthttp.NewSubmitShipmentRequestEndpoint(submissionIntake, wiring.submission)},
			}
		},
	})
}
