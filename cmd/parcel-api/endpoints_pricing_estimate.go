package main

import (
	"net/http"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type pricingEstimateWiring struct {
	pricingEstimate pricinghttp.EstimateFormer
}

func init() {
	registerEndpoints(endpointFeature[pricingEstimateWiring]{
		lines: map[string]endpointLine{
			"/pricing-estimates": {method: http.MethodPost, face: operationsReadFace()},
		},
		build: func(sources endpointSources) (pricingEstimateWiring, error) {
			// 运营试算编排（ADR-0152）：接真不等操作者 Intake，判据同回放——未配置 Intake 拒在编排之前。
			estimate, err := buildPricingEstimateOrchestration(sources.db)
			return pricingEstimateWiring{pricingEstimate: estimate}, err
		},
		mount: func(channels endpointChannels, wiring pricingEstimateWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 运营试算（ADR-0152 决定六，票 operator-workspace-gaps/05）：命令行，挂操作者渠道（票 operator-channel/06），能力面是
				// 主数据与运营查阅读而不是登记册配置写——试算不写任何册、不形成生产事实，所以挂 operatorRegistries.pricingOperationsRead，
				// 不挂 operatorRegistries.pricing。租户来自认证出的身份；不加隔离放行（ADR-0150）。编排只算不存，依赖结构上没有
				// 评价库与交付口。
				{Pattern: "/pricing-estimates", Handler: pricinghttp.NewEstimateEndpoint(channels.operatorRegistries.pricingOperationsRead, wiring.pricingEstimate)},
			}
		},
	})
}
