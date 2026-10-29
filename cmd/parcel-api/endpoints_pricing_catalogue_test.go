package main

func init() {
	registerUnwired(pricingCatalogueWiring{
		priceCards:               unwiredPricingCatalogue{},
		referenceSeries:          unwiredPricingCatalogue{},
		priceCardContent:         unwiredPriceCardContent{},
		referenceSeriesCoverage:  unwiredReferenceSeriesCoverage{},
		pendingSeriesEvaluations: unwiredPendingSeriesEvaluations{},
		pricingEvaluations:       unwiredPricingEvaluations{},
	})
}
