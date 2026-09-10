package partycommercial_test

import (
	"context"
	"errors"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

type rejectionRequestSource struct {
	request pcdomain.AuthorizationRequest
	formed  bool
	err     error
}

func (source *rejectionRequestSource) FormAuthorizationRequest(
	_ context.Context,
	_ psports.ActiveRejectionAuthorizationQuery,
) (pcdomain.AuthorizationRequest, bool, error) {
	return source.request, source.formed, source.err
}

type grantStoreDouble struct {
	grants []pcdomain.AuthorityGrant
	err    error
}

func (store *grantStoreDouble) LoadEffectiveGrants(
	context.Context, pcdomain.TenantID, pcdomain.CommercialScopeReference, time.Time,
) ([]pcdomain.AuthorityGrant, error) {
	if store.err != nil {
		return nil, store.err
	}
	return store.grants, nil
}

func (store *grantStoreDouble) SaveGrant(context.Context, pcdomain.AuthorityGrant) (pcports.GrantSaveOutcome, error) {
	return pcports.GrantSaveOutcomeInvalid, errors.New("save is not used by the adapter")
}

func TestActiveRejectionIsGrantedWhenAMatchingRuleExists(t *testing.T) {
	grant := rejectionGrant(t, "auth-reject", pcdomain.ActiveRejectionAction)
	authorizer := adapter.NewActiveRejectionAdapter(
		pcapplication.NewAdjudicateCommercialAuthorizationHandler(&grantStoreDouble{grants: []pcdomain.AuthorityGrant{grant}}),
		&rejectionRequestSource{request: rejectionPCRequest(t), formed: true},
	)

	result, err := authorizer.AuthorizeActiveRejection(t.Context(), rejectionQuery(t))
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if result.Outcome != psports.AuthorizationGranted {
		t.Fatalf("outcome = %s, want GRANTED", result.Outcome)
	}
	if result.Authority.String() != "auth-reject/v1" {
		t.Fatalf("authority = %q, want the adopted grant version", result.Authority)
	}
}

func TestActiveRejectionIsRefusedWhenTheScopeHasOtherRules(t *testing.T) {
	grant := rejectionGrant(t, "auth-review", pcdomain.ManualReviewAction)
	authorizer := adapter.NewActiveRejectionAdapter(
		pcapplication.NewAdjudicateCommercialAuthorizationHandler(&grantStoreDouble{grants: []pcdomain.AuthorityGrant{grant}}),
		&rejectionRequestSource{request: rejectionPCRequest(t), formed: true},
	)

	result, err := authorizer.AuthorizeActiveRejection(t.Context(), rejectionQuery(t))
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if result.Outcome != psports.AuthorizationRefused {
		t.Fatalf("outcome = %s, want REFUSED", result.Outcome)
	}
	if result.Authority.String() != "" {
		t.Fatal("不允许时仍带了授权引用")
	}
}

func TestActiveRejectionReportsRulesNotConfiguredWhenTheBookIsEmpty(t *testing.T) {
	authorizer := adapter.NewActiveRejectionAdapter(
		pcapplication.NewAdjudicateCommercialAuthorizationHandler(&grantStoreDouble{}),
		&rejectionRequestSource{request: rejectionPCRequest(t), formed: true},
	)

	result, err := authorizer.AuthorizeActiveRejection(t.Context(), rejectionQuery(t))
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if result.Outcome != psports.AuthorizationRulesNotConfigured {
		t.Fatalf("outcome = %s, want RULES_NOT_CONFIGURED", result.Outcome)
	}
}

func TestActiveRejectionSurfacesAuthorityReadFailure(t *testing.T) {
	unavailable := errors.New("权威不可读")
	authorizer := adapter.NewActiveRejectionAdapter(
		pcapplication.NewAdjudicateCommercialAuthorizationHandler(&grantStoreDouble{err: unavailable}),
		&rejectionRequestSource{request: rejectionPCRequest(t), formed: true},
	)

	_, err := authorizer.AuthorizeActiveRejection(t.Context(), rejectionQuery(t))
	if !errors.Is(err, unavailable) {
		t.Fatalf("error = %v, want wrapped loader failure", err)
	}
}

func TestActiveRejectionDoesNotCallTheProviderWhenTheRequestCannotBeFormed(t *testing.T) {
	authorizer := adapter.NewActiveRejectionAdapter(
		pcapplication.NewAdjudicateCommercialAuthorizationHandler(&grantStoreDouble{
			err: errors.New("不该被问到"),
		}),
		&rejectionRequestSource{formed: false},
	)

	_, err := authorizer.AuthorizeActiveRejection(t.Context(), rejectionQuery(t))
	if err == nil {
		t.Fatal("范围或时点折不成时仍去问了提供方")
	}
}

// Covers: 本口只问「许不许主动拒绝」——映射若折出了别的动作（如人工复核），一条只授复核权的规则会被读成
// 拒绝权，而获准复核并不等于获准直接拒掉这单业务（PC CONTEXT）。这是词汇表之外的输入，按
// ErrUntranslatableAnswer 拒且不问提供方；折出拒绝动作的映射则照常交给提供方（用读失败的哨兵证它到了那里）。
// 守卫与 ManualReviewAuthorizationAdapter 那道同形（票 ps-port-remainder/08 第 3 件）。
func TestActiveRejectionOnlyTranslatesAMappingThatFormsTheRejectionAction(t *testing.T) {
	providerAsked := errors.New("提供方被问到了")

	t.Run("a mapping forming the rejection action reaches the provider", func(t *testing.T) {
		authorizer := adapter.NewActiveRejectionAdapter(
			pcapplication.NewAdjudicateCommercialAuthorizationHandler(&grantStoreDouble{err: providerAsked}),
			&rejectionRequestSource{request: rejectionPCRequest(t), formed: true},
		)

		_, err := authorizer.AuthorizeActiveRejection(t.Context(), rejectionQuery(t))
		if !errors.Is(err, providerAsked) {
			t.Fatalf("error = %v, want the provider to have been asked", err)
		}
		if errors.Is(err, adapter.ErrUntranslatableAnswer) {
			t.Fatal("折出的是拒绝动作，守卫却拒译了")
		}
	})

	t.Run("a mapping forming another action is refused before the provider", func(t *testing.T) {
		authorizer := adapter.NewActiveRejectionAdapter(
			pcapplication.NewAdjudicateCommercialAuthorizationHandler(&grantStoreDouble{err: providerAsked}),
			&rejectionRequestSource{request: reviewPCRequest(t, pcdomain.ManualReviewAction), formed: true},
		)

		_, err := authorizer.AuthorizeActiveRejection(t.Context(), rejectionQuery(t))
		if !errors.Is(err, adapter.ErrUntranslatableAnswer) {
			t.Fatalf("error = %v, want ErrUntranslatableAnswer——映射折出的不是主动拒绝动作", err)
		}
		if errors.Is(err, providerAsked) {
			t.Fatal("映射折出别的动作，适配器仍去问了提供方")
		}
	})
}

func rejectionQuery(t *testing.T) psports.ActiveRejectionAuthorizationQuery {
	t.Helper()
	identity, err := psdomain.NewSourceIdentity(
		value(t, psdomain.NewTenantID, "tenant-1"),
		value(t, psdomain.NewCustomerAccountID, "customer-1"),
		value(t, psdomain.NewSource, "source-a"),
		value(t, psdomain.NewSourceRequestKey, "key-1"),
	)
	if err != nil {
		t.Fatalf("source identity: %v", err)
	}
	return psports.ActiveRejectionAuthorizationQuery{
		Identity:          identity,
		ShipmentRequestID: value(t, psdomain.NewShipmentRequestID, "request-1"),
		SubmissionVersion: value(t, psdomain.NewSubmissionVersionID, "version-1"),
		Decider:           value(t, psdomain.NewDeciderReference, "OPERATOR-1"),
		Reason:            value(t, psdomain.NewRejectionReasonReference, "COMMERCIAL_RISK"),
	}
}

func rejectionPCRequest(t *testing.T) pcdomain.AuthorizationRequest {
	t.Helper()
	request, err := pcdomain.NewAuthorizationRequest(
		pcdomain.ActiveRejectionAction,
		pcValue(t, pcdomain.NewLegalEntityReference, "legal-1"),
		pcValue(t, pcdomain.NewAuthorityLevel, "level-commercial"),
		pcValue(t, pcdomain.NewCommercialScopeReference, "scope-a"),
		pcValue(t, pcdomain.NewStructuredReason, "COMMERCIAL_RISK"),
		pcValue(t, pcdomain.NewEvidenceReference, "evidence-1"),
		time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("authorization request: %v", err)
	}
	return request
}

func rejectionGrant(t *testing.T, objectID string, action pcdomain.AuthorizedAction) pcdomain.AuthorityGrant {
	t.Helper()
	interval, err := pcdomain.NewEffectiveInterval(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("interval: %v", err)
	}
	approval, err := pcdomain.NewApprovalBasis(
		pcValue(t, pcdomain.NewApprovalReference, "approval-"+objectID),
		pcValue(t, pcdomain.NewCommercialSourceReference, "source-"+objectID),
		time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("approval: %v", err)
	}
	version, err := pcdomain.RehydrateCommercialVersion(pcdomain.RehydrateCommercialVersionSpec{
		TenantID:      pcValue(t, pcdomain.NewTenantID, "tenant-1"),
		Kind:          pcdomain.AuthorizationRuleObject,
		ObjectID:      pcValue(t, pcdomain.NewCommercialObjectID, objectID),
		Version:       pcValue(t, pcdomain.NewCommercialVersionLabel, "v1"),
		Scope:         pcValue(t, pcdomain.NewCommercialScopeReference, "scope-a"),
		ContentDigest: pcValue(t, pcdomain.NewCommercialContentDigest, "sha256:"+objectID),
		Effective:     interval,
		Status:        pcdomain.CommercialVersionEffective,
		Approval:      approval,
		PublishedAt:   time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC),
		EffectiveAt:   time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	grant, err := pcdomain.NewAuthorityGrant(
		version,
		action,
		pcValue(t, pcdomain.NewLegalEntityReference, "legal-1"),
		pcValue(t, pcdomain.NewAuthorityLevel, "level-commercial"),
		pcValue(t, pcdomain.NewCommercialScopeReference, "scope-a"),
		interval,
	)
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	return grant
}

func pcValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}
