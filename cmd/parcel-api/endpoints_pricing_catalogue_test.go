package main

func init() {
	registerUnwired(pricingCatalogueWiring{
		priceCards:               unwiredPricingCatalogue{},
		referenceSeries:          unwiredPricingCatalogue{},
		priceCardContent:         unwiredPriceCardContent{},
		sellPriceScopes:          unwiredSellPriceScope{},
		priceCardTemplateSpec:    unwiredPriceCardTemplateSpec{},
		referenceSeriesCoverage:  unwiredReferenceSeriesCoverage{},
		pendingSeriesEvaluations: unwiredPendingSeriesEvaluations{},
		pricingEvaluations:       unwiredPricingEvaluations{},
	})
}
