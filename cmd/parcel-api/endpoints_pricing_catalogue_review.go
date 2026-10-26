package main

import (
	"net/http"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type pricingCatalogueReviewWiring struct {
	referenceCatalogueReview pricinghttp.ReferenceCatalogueReviewer
}

func init() {
	registerEndpoints(endpointFeature[pricingCatalogueReviewWiring]{
		lines: map[string]endpointLine{
			"/pricing-reference-catalogue-reviews": {method: http.MethodPost, face: registryWriteFace()},
		},
		build: func(sources endpointSources) (pricingCatalogueReviewWiring, error) {
			review, err := buildReferenceCatalogueReviewOrchestration(sources.db)
			return pricingCatalogueReviewWiring{referenceCatalogueReview: review}, err
		},
		mount: func(channels endpointChannels, wiring pricingCatalogueReviewWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 计价参考目录版本复核（ADR-0109 决定二、ADR-0262 决定一）：与序列复核同挂操作者渠道的登记册 Intake——复核人与
				// 他的授予集都取自信封。这一行在，规则要求某一格授予的租户才复核得了目录：受控登记口带不出授予集，那一路一律答
				// 复核者不合格（ADR-0262 越权风险点 4）。
				{Pattern: "/pricing-reference-catalogue-reviews", Handler: pricinghttp.NewReviewReferenceCatalogueEndpoint(channels.operatorRegistries.pricing, wiring.referenceCatalogueReview)},
			}
		},
	})
}
