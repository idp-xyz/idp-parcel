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
		legalEntityProfiles:        unwiredLegalEntityProfileRevisions{},
		legalEntityProfileResolver: unwiredLegalEntityProfileResolver{},
	})
}
