package main

func init() {
	registerUnwired(pricingCatalogueWiring{
		priceCards:               unwiredPricingCatalogue{},
		referenceSeries:          unwiredPricingCatalogue{},
		priceCardContent:         unwiredPriceCardContent{},
		priceCardTemplateSpec:    unwiredPriceCardTemplateSpec{},
		referenceSeriesCoverage:  unwiredReferenceSeriesCoverage{},
		pendingSeriesEvaluations: unwiredPendingSeriesEvaluations{},
		pricingEvaluations:       unwiredPricingEvaluations{},
	})
}
