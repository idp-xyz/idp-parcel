package main

import (
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	pricingapp "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
)

// buildPricingEstimateOrchestration 装配 `/pricing-estimates` 的真编排（ADR-0152；票 operator-workspace-gaps/05）。价卡适配器
// 同时交装载与在用解析两口；补齐读数那两对与正式评价（parcel-dispatch 的评价请求消费者）同源同形——试算与正式评价在同一
// 时点、同一版本清单下给同一结果靠的就是这一点。
//
// 依赖里**没有评价库与交付口**：试算只算不存、不交结算，装配点想接也没有那一格（ADR-0152 决定二）。它只读，不包事务。
func buildPricingEstimateOrchestration(db *bentopg.DB) (*pricingapp.FormEstimateEvaluationsHandler, error) {
	cards, err := pppostgres.NewPriceCards(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: price cards for estimate: %w", err)
	}
	seriesVersions, err := pppostgres.NewReferenceSeriesVersions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: reference series register for estimate: %w", err)
	}
	seriesReviews, err := pppostgres.NewReferenceSeriesReviews(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: reference series in-force resolver for estimate: %w", err)
	}
	catalogueVersions, err := pppostgres.NewReferenceCatalogueVersions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: reference catalogue register for estimate: %w", err)
	}
	catalogueReviews, err := pppostgres.NewReferenceCatalogueReviews(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: reference catalogue in-force resolver for estimate: %w", err)
	}
	handler, err := pricingapp.NewFormEstimateEvaluationsHandler(pricingapp.FormEstimateEvaluationsDeps{
		PriceCards:       cards,
		InForce:          cards,
		Clock:            systemClock{},
		SeriesInForce:    seriesReviews,
		SeriesVersions:   seriesVersions,
		CatalogueInForce: catalogueReviews,
		Catalogues:       catalogueVersions,
	})
	if err != nil {
		return nil, fmt.Errorf("parcel-api: pricing estimate: %w", err)
	}
	return handler, nil
}
