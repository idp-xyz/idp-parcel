package main

func init() {
	registerUnwired(commercialCatalogueWiring{
		serviceProducts:            unwiredCommercialCatalogue{},
		commercialPolicies:         unwiredCommercialCatalogue{},
		commercialRelations:        unwiredCommercialCatalogue{},
		partyIdentities:            unwiredCommercialCatalogue{},
		customerAccounts:           unwiredCommercialCatalogue{},
		productChannelMappings:     unwiredCommercialCatalogue{},
		registrationNumberTypes:    unwiredCommercialCatalogue{},
		publicationDrafts:          unwiredPublicationDraftRead{},
		legalEntityProfiles:        unwiredLegalEntityProfileRevisions{},
		legalEntityProfileResolver: unwiredLegalEntityProfileResolver{},
		referenceEnablements:       unwiredReferenceEnablements{},
	})
}
