package main

func init() {
	registerUnwired(pricingImportWiring{
		priceCardPreview:          unwiredPriceCardPreview{},
		priceCardDraftSubmission:  unwiredPriceCardDraftSubmission{},
		priceCardDrafts:           unwiredPriceCardDrafts{},
		priceCardDraftApproval:    unwiredPriceCardDraftApproval{},
		priceCardDraftPublication: unwiredPriceCardDraftPublication{},
	})
}
