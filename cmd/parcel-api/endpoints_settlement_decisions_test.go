package main

func init() {
	registerUnwired(settlementDecisionsWiring{
		confirmer: unwiredChargeConfirmation{},
		publisher: unwiredStatementPublication{},
		receiver:  unwiredSupplierBill{},
		auditor:   unwiredSupplierBill{},
	})
}
