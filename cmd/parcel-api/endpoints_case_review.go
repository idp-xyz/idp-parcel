package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
)

type caseReviewWiring struct {
	exceptionTriageRecords visibilityhttp.TriageReviewReader
	exceptionCaseRecords   visibilityhttp.CaseReviewReader
	claimsRecoveryRecords  visibilityhttp.ClaimsRecoveryReviewReader
}

func init() {
	registerEndpoints(endpointFeature[caseReviewWiring]{
		lines: map[string]endpointLine{
			"/exception-triage-records": {method: http.MethodGet, face: operationsReadFace(), probe: "/exception-triage-records?registry=signal-episode"},
			"/exception-case-records":   {method: http.MethodGet, face: operationsReadFace()},
			"/claims-recovery-records":  {method: http.MethodGet, face: operationsReadFace(), probe: "/claims-recovery-records?registry=customer-notification"},
		},
		build: func(sources endpointSources) (caseReviewWiring, error) {
			// VE 案件侧三页读面（票 admin-skeleton-closure-batch/06）：一个适配器实现三读口，
			// 分诊、案件与理赔追偿六本册子照行转写。
			caseReview, err := vepostgres.NewCaseReview(sources.db)
			return caseReviewWiring{
				exceptionTriageRecords: caseReview,
				exceptionCaseRecords:   caseReview,
				claimsRecoveryRecords:  caseReview,
			}, err
		},
		mount: func(channels endpointChannels, wiring caseReviewWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// VE 案件侧三页（票 admin-skeleton-closure-batch/06）：分诊两册、案件单册、
				// 理赔追偿三册，与目录及追踪查阅同族同 Intake 变量。生产装配三口共用一个
				// CaseReview 读适配器（六方法一型），此处三参分收是为了让装配测试盖得住
				// 「某一口接错适配器」——参数分、实现合，两侧各取所需。
				{Pattern: "/exception-triage-records", Handler: visibilityhttp.NewQueryExceptionTriageRecordsEndpoint(channels.trackingProjections(), wiring.exceptionTriageRecords)},
				{Pattern: "/exception-case-records", Handler: visibilityhttp.NewQueryExceptionCaseRecordsEndpoint(channels.trackingProjections(), wiring.exceptionCaseRecords)},
				{Pattern: "/claims-recovery-records", Handler: visibilityhttp.NewQueryClaimsRecoveryRecordsEndpoint(channels.trackingProjections(), wiring.claimsRecoveryRecords)},
			}
		},
	})
}
