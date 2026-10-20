package main

func init() {
	registerUnwired(pricingRegistrationWiring{
		priceCardRegistration:          unwiredPriceCardRegistration{},
		referenceSeriesRegistration:    unwiredReferenceSeriesRegistration{},
		referenceSeriesReview:          unwiredReferenceSeriesReview{},
		referenceSeriesPreview:         unwiredReferenceSeriesPreview{},
		referenceCatalogueRegistration: unwiredReferenceCatalogueRegistration{},
	})
}
