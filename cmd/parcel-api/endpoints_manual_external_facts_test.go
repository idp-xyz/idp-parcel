package main

func init() {
	registerUnwired(manualExternalFactsWiring{
		externalFundsFactAdoption:           unwiredExternalFundsFactRegistration{},
		externalFundsFactCorrectionAdoption: unwiredExternalFundsFactCorrectionRegistration{},
		regulatoryCredentialRegistration:    unwiredRegulatoryCredentialRegistration{},
	})
}
