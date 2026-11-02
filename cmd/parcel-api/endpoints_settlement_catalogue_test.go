package main

func init() {
	registerUnwired(settlementCatalogueWiring{
		settlementCharges:           unwiredSettlementCharges{},
		settlementStatements:        unwiredSettlementStatements{},
		settlementDisputed:          unwiredSettlementStatements{},
		settlementFundsApplications: unwiredSettlementFundsApplications{},
		settlementOperatingResults:  unwiredSettlementOperatingResults{},
	})
}
