package main

func init() {
	registerUnwired(settlementCatalogueWiring{
		settlementCharges:           unwiredSettlementCharges{},
		settlementStatements:        unwiredSettlementStatements{},
		settlementDisputed:          unwiredSettlementStatements{},
		settlementStatementLines:    unwiredSettlementStatements{},
		preAcceptanceEstimates:      unwiredPreAcceptanceEstimates{},
		supplierBillLines:           unwiredSettlementStatements{},
		auditedPayables:             unwiredSettlementStatements{},
		settlementFundsApplications: unwiredSettlementFundsApplications{},
		settlementOperatingResults:  unwiredSettlementOperatingResults{},
	})
}
