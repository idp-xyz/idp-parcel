package main

func init() {
	registerUnwired(settlementCatalogueWiring{
		settlementCharges:           unwiredSettlementCharges{},
		settlementStatements:        unwiredSettlementStatements{},
		settlementFundsApplications: unwiredSettlementFundsApplications{},
		settlementOperatingResults:  unwiredSettlementOperatingResults{},
	})
}
