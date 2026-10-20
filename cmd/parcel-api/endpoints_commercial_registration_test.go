package main

func init() {
	registerUnwired(commercialRegistrationWiring{
		commercialPublication:              unwiredCommercialPublication{},
		commercialPublicationPreview:       unwiredCommercialPublicationPreview{},
		commercialPublicationDrafts:        unwiredPublicationDrafts{},
		partyIdentityRegistration:          unwiredPartyIdentityRegistration{},
		productChannelRegistration:         unwiredProductChannelRegistration{},
		registrationNumberTypeRegistration: unwiredRegistrationNumberTypeRegistration{},
		legalEntityProfileRegistration:     unwiredLegalEntityProfileRegistration{},
		channelAccountUseRegistration:      unwiredChannelAccountUseRegistration{},
	})
}
