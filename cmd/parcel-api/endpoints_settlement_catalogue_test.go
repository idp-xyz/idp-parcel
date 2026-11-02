package main

func init() {
	registerUnwired(settlementCatalogueWiring{
		settlementCharges:           unwiredSettlementCharges{},
		settlementStatements:        unwiredSettlementStatements{},
		settlementDisputed:          unwiredSettlementStatements{},
		settlementStatementLines:    unwiredSettlementStatements{},
		settlementFundsApplications: unwiredSettlementFundsApplications{},
		settlementOperatingResults:  unwiredSettlementOperatingResults{},
	})
}
