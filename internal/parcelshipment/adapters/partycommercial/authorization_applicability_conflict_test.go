package partycommercial_test

import (
	"errors"
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件证各授权适配器把 party-commercial 的`适用冲突`（ADR-0261 决定三：同一刻多于一条授予允许、或委派方不同的多条
// 委派命中）译成 psports.AuthorizationApplicabilityConflict：不落`不允许`，不读作已授权，不带授权引用，也不留在 error。

func twoGrants(t *testing.T, action pcdomain.AuthorizedAction) *grantStoreDouble {
	t.Helper()
	return &grantStoreDouble{grants: []pcdomain.AuthorityGrant{
		rejectionGrant(t, "auth-a", action),
		rejectionGrant(t, "auth-b", action),
	}}
}

func assertConflictOutcome(t *testing.T, outcome psports.AuthorizationOutcome, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("authorize: %v——适用冲突是 PC 答得出的一格，不是未形成", err)
	}
	if outcome != psports.AuthorizationApplicabilityConflict {
		t.Fatalf("outcome = %s, want APPLICABILITY_CONFLICT", outcome)
	}
}

// Covers: ADR-0261 越权风险点 3 的消费方半边——主动拒绝、人工复核、撤回经 PC 裁定询问的适配器把适用冲突译成同名一格，
// 不带授权引用。
func TestTheGrantAdaptersTranslateAnApplicabilityConflict(t *testing.T) {
	t.Run("active rejection", func(t *testing.T) {
		authorizer := adapter.NewActiveRejectionAdapter(
			pcapplication.NewAdjudicateCommercialAuthorizationHandler(twoGrants(t, pcdomain.ActiveRejectionAction)),
			&rejectionRequestSource{request: rejectionPCRequest(t), formed: true},
		)
		result, err := authorizer.AuthorizeActiveRejection(t.Context(), rejectionQuery(t))
		assertConflictOutcome(t, result.Outcome, err)
		if result.Authority.String() != "" {
			t.Fatalf("authority = %q；适用冲突不该带授权引用", result.Authority)
		}
	})

	t.Run("manual review", func(t *testing.T) {
		authorizer := adapter.NewManualReviewAuthorizationAdapter(
			pcapplication.NewAdjudicateCommercialAuthorizationHandler(twoGrants(t, pcdomain.ManualReviewAction)),
			&reviewRequestSource{request: reviewPCRequest(t, pcdomain.ManualReviewAction), formed: true},
		)
		result, err := authorizer.AuthorizeManualReview(t.Context(), reviewQuery(t))
		assertConflictOutcome(t, result.Outcome, err)
		if result.Authority.String() != "" {
			t.Fatalf("authority = %q；适用冲突不该带授权引用", result.Authority)
		}
	})

	t.Run("withdrawal", func(t *testing.T) {
		authorizer := adapter.NewWithdrawalAuthorizationAdapter(
			pcapplication.NewAdjudicateCommercialAuthorizationHandler(twoGrants(t, pcdomain.ActiveRejectionAction)),
			&withdrawalRequestSource{request: rejectionPCRequest(t), formed: true},
		)
		result, err := authorizer.AuthorizeWithdrawal(t.Context(), withdrawalQuery(t))
		assertConflictOutcome(t, result.Outcome, err)
		if result.Authority.String() != "" {
			t.Fatalf("authority = %q；适用冲突不该带授权引用", result.Authority)
		}
	})
}

// Covers: 同上，受控关闭与重开共用的那一只：两条关闭授予都命中时答适用冲突，三件（依据、角色、决定方）一件不带。
func TestTheContinuedAttemptAdapterTranslatesAnApplicabilityConflict(t *testing.T) {
	authorizer := newContinuedAttemptAuthorizer(t,
		twoGrants(t, pcdomain.ControlledClosureAction),
		&continuedAttemptRequestSource{coordinates: continuedAttemptCoordinates(t), formed: true},
	)
	result, err := authorizer.AuthorizeContinuedAttemptDecision(t.Context(), continuedAttemptQuery(t, psdomain.ControlledClosureDecision))
	assertConflictOutcome(t, result.Outcome, err)
	if result.Authority.String() != "" || result.AuthorityRole.String() != "" || result.Decider.String() != "" {
		t.Fatalf("适用冲突带出了 authority=%q role=%q decider=%q", result.Authority, result.AuthorityRole, result.Decider)
	}
}

// Covers: 同上，资料修订那一只两种来路：客户账户自己请求而两条授予都命中；运营角色代录而两条委派的委派方不同。两种都
// 不带授权依据与实际决定方。
func TestTheSourceDataAmendmentAdapterTranslatesAnApplicabilityConflict(t *testing.T) {
	t.Run("two grants", func(t *testing.T) {
		authorizer := adapter.NewSourceDataAmendmentAuthorizationAdapter(
			pcapplication.NewAdjudicateCommercialAuthorizationHandlerWithDelegations(
				twoGrants(t, pcdomain.SourceDataAmendmentAction),
				&delegationViewDouble{err: errors.New("不该被问到")},
			),
			&amendmentRequestSource{coordinates: customerCoordinates(t), formed: true},
		)
		result, err := authorizer.AuthorizeSourceDataAmendment(t.Context(), amendmentAuthorizationQuery(t, "customer-1"))
		assertConflictOutcome(t, result.Outcome, err)
		if result.Decider.String() != "" {
			t.Fatalf("decider = %q；适用冲突不该带实际决定方", result.Decider)
		}
	})

	t.Run("delegations naming different delegators", func(t *testing.T) {
		authorizer := adapter.NewSourceDataAmendmentAuthorizationAdapter(
			pcapplication.NewAdjudicateCommercialAuthorizationHandlerWithDelegations(
				&grantStoreDouble{grants: []pcdomain.AuthorityGrant{amendmentGrant(t, "auth-amend")}},
				&delegationViewDouble{delegations: []pcdomain.ContractDelegation{
					amendmentDelegation(t, "contract-1", accountDelegator(t, "customer-1")),
					amendmentDelegation(t, "contract-2", legalEntityDelegator(t, "legal-1")),
				}},
			),
			&amendmentRequestSource{coordinates: operatorCoordinates(t, "ops-1"), formed: true},
		)
		result, err := authorizer.AuthorizeSourceDataAmendment(t.Context(), amendmentAuthorizationQuery(t, "ops-1"))
		assertConflictOutcome(t, result.Outcome, err)
		if result.Decider.String() != "" {
			t.Fatalf("decider = %q；适用冲突不该带实际决定方", result.Decider)
		}
	})
}
