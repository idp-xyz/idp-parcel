package main

import (
	"context"

	customsdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	customsports "go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

func init() {
	registerUnwired(customsRecordsWiring{
		cases:       unwiredCustomsCaseRecords{},
		submissions: unwiredSubmissionRecords{},
		results:     unwiredExternalResultViews{},
		judgements:  unwiredAdmissionJudgementViews{},
		unadmitted:  unwiredUnadmittedObjects{},
		manifests:   unwiredExternalManifestReferences{},
	})
}

type unwiredCustomsCaseRecords struct{}

func (unwiredCustomsCaseRecords) ListCustomsCases(
	context.Context, customsdomain.TenantID, int, cataloguepage.Query,
) (cataloguepage.Result[customsports.CustomsCaseRecord], error) {
	return cataloguepage.Result[customsports.CustomsCaseRecord]{}, errOrchestrationNotWired
}

type unwiredSubmissionRecords struct{}

func (unwiredSubmissionRecords) ListSubmissionVersions(
	context.Context, customsdomain.TenantID, int, cataloguepage.Query,
) (cataloguepage.Result[customsports.SubmissionVersionRecord], error) {
	return cataloguepage.Result[customsports.SubmissionVersionRecord]{}, errOrchestrationNotWired
}

type unwiredExternalResultViews struct{}

func (unwiredExternalResultViews) ListExternalResults(
	context.Context, customsdomain.TenantID, string, int, cataloguepage.Query,
) (cataloguepage.Result[customsports.ExternalResultViewRecord], error) {
	return cataloguepage.Result[customsports.ExternalResultViewRecord]{}, errOrchestrationNotWired
}

type unwiredAdmissionJudgementViews struct{}

func (unwiredAdmissionJudgementViews) ListActionAdmissionJudgements(
	context.Context, customsdomain.TenantID, int, cataloguepage.Query,
) (cataloguepage.Result[customsports.ActionAdmissionJudgementRecord], error) {
	return cataloguepage.Result[customsports.ActionAdmissionJudgementRecord]{}, errOrchestrationNotWired
}

type unwiredUnadmittedObjects struct{}

func (unwiredUnadmittedObjects) ListUnadmittedObjects(
	context.Context, customsdomain.TenantID, int, cataloguepage.Query,
) (customsports.UnadmittedObjects, error) {
	return customsports.UnadmittedObjects{}, errOrchestrationNotWired
}

type unwiredExternalManifestReferences struct{}

func (unwiredExternalManifestReferences) ListExternalManifestReferences(
	context.Context, customsdomain.TenantID, int, cataloguepage.Query,
) (cataloguepage.Result[customsports.ExternalManifestReferenceRecord], error) {
	return cataloguepage.Result[customsports.ExternalManifestReferenceRecord]{}, errOrchestrationNotWired
}
