package main

func init() {
	registerUnwired(settlementDecisionsWiring{
		confirmer: unwiredChargeConfirmation{},
		publisher: unwiredStatementPublication{},
	})
}
