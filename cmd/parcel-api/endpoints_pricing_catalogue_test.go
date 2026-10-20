package main

func init() {
	registerUnwired(pricingCatalogueWiring{
		priceCards:               unwiredPricingCatalogue{},
		referenceSeries:          unwiredPricingCatalogue{},
		referenceSeriesCoverage:  unwiredReferenceSeriesCoverage{},
		pendingSeriesEvaluations: unwiredPendingSeriesEvaluations{},
		pricingEvaluations:       unwiredPricingEvaluations{},
	})
}
