package main

func init() {
	registerUnwired(customsCatalogueWiring{
		complianceRules:    unwiredComplianceRules{},
		caseRegisters:      unwiredCaseRegisters{},
		gateConditions:     unwiredGateConditions{},
		portsPaths:         unwiredPortsPaths{},
		credentials:        unwiredCredentialCatalogue{},
		dutyCollaborations: unwiredDutyReconciliationCatalogue{},
		dutyVerifications:  unwiredDutyReconciliationCatalogue{},
	})
}
