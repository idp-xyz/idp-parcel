package main

func init() {
	registerUnwired(settlementRegistrationWiring{
		externalFundsFactRegistration:           unwiredExternalFundsFactRegistration{},
		externalFundsFactCorrectionRegistration: unwiredExternalFundsFactCorrectionRegistration{},
	})
}
