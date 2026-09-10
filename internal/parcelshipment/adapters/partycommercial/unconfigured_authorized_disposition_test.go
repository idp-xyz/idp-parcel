package partycommercial_test

import (
	"context"
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// Covers: 票 sa-preacceptance-policy-view/04 第 2 步「PC 侧授权动作词汇未加之前如实答 AUTHORITY_RULES_NOT_CONFIGURED」
// ——不读询问、不采信处置人，对谁都答`授权规则未配置`且不带授权引用；不是`不允许`（那要有一份规则作依据），也不是
// error（重试改不了一个还没人建的规则）。
func TestUnconfiguredAuthorizedDispositionAuthorizerAnswersRulesNotConfiguredIdentically(t *testing.T) {
	authorizer := adapter.UnconfiguredAuthorizedDispositionAuthorizer{}
	queries := []psports.AuthorizedDispositionAuthorizationQuery{
		{},
		{
			Identity:          resolutionKeyIdentity(t, "tenant-1", "customer-1"),
			ShipmentRequestID: value(t, psdomain.NewShipmentRequestID, "request-1"),
			SubmissionVersion: value(t, psdomain.NewSubmissionVersionID, "version-1"),
			Disposer:          value(t, psdomain.NewDisposerReference, "CREDIT-OFFICER-1"),
			Reason:            value(t, psdomain.NewDispositionReasonReference, "CREDIT_LIMIT_NOT_EXTENDED"),
			Evidence:          value(t, psdomain.NewDispositionEvidenceReference, "EVID-1"),
		},
	}
	for _, query := range queries {
		answer, err := authorizer.AuthorizeDisposition(context.Background(), query)
		if err != nil {
			t.Fatalf("authorize disposition: %v", err)
		}
		if answer.Outcome != psports.AuthorizationRulesNotConfigured {
			t.Fatalf("outcome = %s, want RULES_NOT_CONFIGURED", answer.Outcome)
		}
		if answer.Authority.String() != "" {
			t.Fatal("未配置的答复带上了授权引用")
		}
	}
}
