package main

func init() {
	registerUnwired(settlementCatalogueWiring{
		settlementCharges:           unwiredSettlementCharges{},
		settlementStatements:        unwiredSettlementStatements{},
		settlementDisputed:          unwiredSettlementStatements{},
		settlementStatementLines:    unwiredSettlementStatements{},
		supplierBillLines:           unwiredSettlementStatements{},
		auditedPayables:             unwiredSettlementStatements{},
		settlementFundsApplications: unwiredSettlementFundsApplications{},
		settlementOperatingResults:  unwiredSettlementOperatingResults{},
	})
}
