package main

import (
	"context"

	customsdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	customsports "go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

func init() {
	registerUnwired(customsRecordsWiring{
		cases:       unwiredCustomsCaseRecords{},
		submissions: unwiredSubmissionRecords{},
		results:     unwiredExternalResultViews{},
		judgements:  unwiredAdmissionJudgementViews{},
	})
}

type unwiredCustomsCaseRecords struct{}

func (unwiredCustomsCaseRecords) ListCustomsCases(
	context.Context, customsdomain.TenantID, string, int,
) ([]customsports.CustomsCaseRecord, error) {
	return nil, errOrchestrationNotWired
}

type unwiredSubmissionRecords struct{}

func (unwiredSubmissionRecords) ListSubmissionVersions(
	context.Context, customsdomain.TenantID, string, int,
) ([]customsports.SubmissionVersionRecord, error) {
	return nil, errOrchestrationNotWired
}

type unwiredExternalResultViews struct{}

func (unwiredExternalResultViews) ListExternalResults(
	context.Context, customsdomain.TenantID, string, int,
) ([]customsports.ExternalResultViewRecord, error) {
	return nil, errOrchestrationNotWired
}

type unwiredAdmissionJudgementViews struct{}

func (unwiredAdmissionJudgementViews) ListActionAdmissionJudgements(
	context.Context, customsdomain.TenantID, string, int,
) ([]customsports.ActionAdmissionJudgementRecord, error) {
	return nil, errOrchestrationNotWired
}
