package main

import (
	"context"

	pricingapp "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
)

func init() {
	registerUnwired(pricingCatalogueReviewWiring{referenceCatalogueReview: unwiredReferenceCatalogueReview{}})
}

// unwiredReferenceCatalogueReview 只给装配测试用，答法同 unwired_orchestration.go 里的各占位：不交回零值答复，稳定错误让
// 「越过了 Intake」可观察为 NO_ANSWER_FORMED。不与目录登记占位合并，判据同生产侧的事务包装。
type unwiredReferenceCatalogueReview struct{}

func (unwiredReferenceCatalogueReview) Handle(
	context.Context,
	pricingapp.ReviewReferenceCatalogueCommand,
) (pricingapp.ReviewReferenceCatalogueOutcome, error) {
	return pricingapp.ReviewReferenceCatalogueOutcomeInvalid, errOrchestrationNotWired
}
