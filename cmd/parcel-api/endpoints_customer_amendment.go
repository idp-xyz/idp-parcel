package main

import (
	"net/http"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type customerAmendmentWiring struct {
	amendment shipmenthttp.AmendmentHandler
}

func init() {
	registerEndpoints(endpointFeature[customerAmendmentWiring]{
		lines: map[string]endpointLine{
			"/shipment-requests/source-data-amendments": {method: http.MethodPost, face: unconfiguredFace()},
		},
		build: func(sources endpointSources) (customerAmendmentWiring, error) {
			// 资料修订编排（UC-PS-002；票 ps-port-remainder/04）：此前它只活在测试里，接进装配后修订请求
			// 才有生产入口——今天越过 Intake 后停在授权未决，那是提供方那两半（PC）未立的如实停点。
			amendment, err := buildCustomerAmendmentOrchestration(sources.db)
			return customerAmendmentWiring{amendment: amendment}, err
		},
		mount: func(_ endpointChannels, wiring customerAmendmentWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 资料修订命令口（UC-PS-002；票 ps-port-remainder/04）：客户或其授权代表在`已接受`委托上形成客户原始
				// 资料新版本。客户渠道的写行，同挂字面量 UnconfiguredIntake{}；请求方怎么认证属客户渠道
				// （`PAR-INT-01`；首方渠道见 ADR-0139 草案），实际决定方怎么判属 `BD-PS-009`，隔离读准入与
				// 隔离提交放行都换不了这一行。编排接的是未配置的授权与矩阵答复，越过 Intake 后如实停在授权
				// 未决——停点从「没有入口」变成「说得出停在哪」。
				{Pattern: "/shipment-requests/source-data-amendments", Handler: shipmenthttp.NewAmendCustomerSourceDataEndpoint(shipmenthttp.UnconfiguredIntake{}, wiring.amendment)},
			}
		},
	})
}
