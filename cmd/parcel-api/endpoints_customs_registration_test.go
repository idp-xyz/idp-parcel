package main

func init() {
	registerUnwired(customsRegistrationWiring{
		interpretationRuleRegistration:      unwiredInterpretationRuleRegistration{},
		gateCatalogRegistration:             unwiredGateCatalogRegistration{},
		candidatePortRegistration:           unwiredCandidatePortRegistration{},
		declarationPathRegistration:         unwiredDeclarationPathRegistration{},
		caseRequirementRegistration:         unwiredCaseRequirementRegistration{},
		regulatoryCredentialRegistration:    unwiredRegulatoryCredentialRegistration{},
		dutyCollaborationRegistration:       unwiredDutyCollaborationRegistration{},
		dutyPaymentVerificationRegistration: unwiredDutyPaymentVerificationRegistration{},
	})
}
