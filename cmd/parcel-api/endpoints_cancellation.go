package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type cancellationWiring struct {
	// customerCancellation 缺席即该行字面量未配置（判 nil，不把带类型的空指针塞进接口）。
	customerCancellation *shipmenthttp.CustomerCancellationIntake
	cancellation         shipmenthttp.CancellationHandler
}

func init() {
	registerEndpoints(endpointFeature[cancellationWiring]{
		lines: map[string]endpointLine{
			"/v1/shipment-requests/parcel-cancellations": {
				method: http.MethodPost, face: customerClientFace(accessidentity.CustomerCapabilityParcelCancellation), ownIntake: true,
			},
		},
		build: func(sources endpointSources) (cancellationWiring, error) {
			customerCancellation, err := buildCustomerCancellationIntake(sources.customerClientMinter)
			if err != nil {
				return cancellationWiring{}, err
			}
			cancellation, err := buildCancellationOrchestration(sources.db)
			return cancellationWiring{customerCancellation: customerCancellation, cancellation: cancellation}, err
		},
		mount: func(_ endpointChannels, wiring cancellationWiring) []httpapi.BusinessEndpoint {
			// 取消口挂首方客户渠道的 Intake（ADR-0310 取舍 A、C；票 first-party-api/11），理由同 endpoints_submission.go 里
			// submissionIntake 那段注释：单独的变量，两个隔离开关都换不了它。
			intake := shipmenthttp.CancellationIntake(shipmenthttp.UnconfiguredIntake{})
			if wiring.customerCancellation != nil {
				intake = wiring.customerCancellation
			}
			return []httpapi.BusinessEndpoint{
				{Pattern: "/v1/shipment-requests/parcel-cancellations", Handler: shipmenthttp.NewCancelParcelEndpoint(intake, wiring.cancellation)},
			}
		},
	})
}
