package main

func init() {
	registerUnwired(caseReviewWiring{
		exceptionTriageRecords: unwiredCaseReview{},
		exceptionCaseRecords:   unwiredCaseReview{},
		claimsRecoveryRecords:  unwiredCaseReview{},
	})
}
