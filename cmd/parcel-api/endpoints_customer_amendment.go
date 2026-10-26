package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type customerAmendmentWiring struct {
	// customerAmendment 缺席即该行字面量未配置（判 nil，不把带类型的空指针塞进接口）。
	customerAmendment *shipmenthttp.CustomerAmendmentIntake
	amendment         shipmenthttp.AmendmentHandler
}

func init() {
	registerEndpoints(endpointFeature[customerAmendmentWiring]{
		lines: map[string]endpointLine{
			"/v1/shipment-requests/source-data-amendments": {
				method: http.MethodPost, face: customerClientFace(accessidentity.CustomerCapabilitySourceDataAmendment), ownIntake: true,
			},
		},
		build: func(sources endpointSources) (customerAmendmentWiring, error) {
			customerAmendment, err := buildCustomerAmendmentIntake(sources.customerClientMinter)
			if err != nil {
				return customerAmendmentWiring{}, err
			}
			// 资料修订编排（UC-PS-002；票 ps-port-remainder/04）。授权的商业坐标映射由首方直客的映射给出（ADR-0311）。
			amendment, err := buildCustomerAmendmentOrchestration(sources.db)
			if err == nil {
				var requests *pspostgres.ShipmentRequests
				if requests, err = pspostgres.NewShipmentRequests(sources.db); err == nil {
					amendment = shipmenthttp.NewCustomerAmendmentHandler(requests, amendment)
				}
			}
			return customerAmendmentWiring{customerAmendment: customerAmendment, amendment: amendment}, err
		},
		mount: func(_ endpointChannels, wiring customerAmendmentWiring) []httpapi.BusinessEndpoint {
			// 资料修订口挂首方客户渠道的 Intake（ADR-0310 取舍 B；票 first-party-api/10），理由同 endpoints_customer_supplement.go：
			// 单独的变量，两个隔离开关都换不了它。
			intake := shipmenthttp.SourceDataAmendmentIntake(shipmenthttp.UnconfiguredIntake{})
			if wiring.customerAmendment != nil {
				intake = wiring.customerAmendment
			}
			return []httpapi.BusinessEndpoint{
				// 资料修订命令口（UC-PS-002）：客户本人在`已接受`委托上形成客户原始资料新版本。
				{Pattern: "/v1/shipment-requests/source-data-amendments", Handler: shipmenthttp.NewAmendCustomerSourceDataEndpoint(intake, wiring.amendment)},
			}
		},
	})
}
