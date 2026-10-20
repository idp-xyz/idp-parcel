package main

func init() {
	registerUnwired(veRegistrationWiring{
		milestoneMappingRegistration:         unwiredMilestoneMappingRegistration{},
		triageRulesRegistration:              unwiredTriageRulesRegistration{},
		notificationPolicyRegistration:       unwiredNotificationPolicyRegistration{},
		claimEligibilityRegistration:         unwiredClaimEligibilityRegistration{},
		claimAuthorizationRegistration:       unwiredClaimAuthorizationRegistration{},
		disclosurePolicyRegistration:         unwiredDisclosurePolicyRegistration{},
		exceptionDisclosureRulesRegistration: unwiredExceptionDisclosureRulesRegistration{},
		conflictSignalRuleRegistration:       unwiredConflictSignalRuleRegistration{},
	})
}
