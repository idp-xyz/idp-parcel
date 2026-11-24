package main

func init() {
	registerUnwired(pricingCatalogueWiring{
		priceCards:               unwiredPricingCatalogue{},
		referenceSeries:          unwiredPricingCatalogue{},
		priceCardContent:         unwiredPriceCardContent{},
		priceCardWorkbook:        unwiredPriceCardWorkbook{},
		sellPriceScopes:          unwiredSellPriceScope{},
		priceCardTemplateSpec:    unwiredPriceCardTemplateSpec{},
		referenceSeriesCoverage:  unwiredReferenceSeriesCoverage{},
		pendingSeriesEvaluations: unwiredPendingSeriesEvaluations{},
		pricingEvaluations:       unwiredPricingEvaluations{},
		warehousePlans:           unwiredWarehousePlans{},
	})
}
