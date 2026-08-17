package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

var adjudicateAt = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

type grantStoreDouble struct {
	grants []domain.AuthorityGrant
	err    error
	tenant domain.TenantID
	scope  domain.CommercialScopeReference
	at     time.Time
}

func (store *grantStoreDouble) LoadEffectiveGrants(
	_ context.Context,
	tenant domain.TenantID,
	scope domain.CommercialScopeReference,
	at time.Time,
) ([]domain.AuthorityGrant, error) {
	store.tenant, store.scope, store.at = tenant, scope, at
	if store.err != nil {
		return nil, store.err
	}
	return store.grants, nil
}

func (store *grantStoreDouble) SaveGrant(context.Context, domain.AuthorityGrant) (ports.GrantSaveOutcome, error) {
	return ports.GrantSaveOutcomeInvalid, errors.New("save is not used by adjudication")
}

func TestAdjudicationWithNoGrantsIsNotConfigured(t *testing.T) {
	store := &grantStoreDouble{}
	handler := application.NewAdjudicateCommercialAuthorizationHandler(store)

	_, err := handler.Handle(t.Context(), adjTenant(t, "tenant-1"), rejectionRequest(t))
	if !errors.Is(err, domain.ErrAuthorityRulesNotConfigured) {
		t.Fatalf("error = %v, want ErrAuthorityRulesNotConfigured", err)
	}
	if store.tenant.String() != "tenant-1" || store.scope.String() != "scope-a" || !store.at.Equal(adjudicateAt) {
		t.Fatalf("装载键 = tenant %q scope %q at %s；应按询问的范围与业务时点装载",
			store.tenant, store.scope, store.at)
	}
}

func TestAdjudicationWithAMatchingGrantIsAuthorized(t *testing.T) {
	grant := authorityGrant(t, "auth-reject", domain.ActiveRejectionAction, "level-commercial", "scope-a")
	handler := application.NewAdjudicateCommercialAuthorizationHandler(&grantStoreDouble{
		grants: []domain.AuthorityGrant{grant},
	})

	authorized, err := handler.Handle(t.Context(), adjTenant(t, "tenant-1"), rejectionRequest(t))
	if err != nil {
		t.Fatalf("adjudicate: %v", err)
	}
	if authorized.GrantVersion().ObjectID().String() != "auth-reject" {
		t.Fatal("已授权却没有指名所采用的授权规则版本")
	}
}

func TestAdjudicationWithAReviewGrantRefusesActiveRejection(t *testing.T) {
	grant := authorityGrant(t, "auth-review", domain.ManualReviewAction, "level-commercial", "scope-a")
	handler := application.NewAdjudicateCommercialAuthorizationHandler(&grantStoreDouble{
		grants: []domain.AuthorityGrant{grant},
	})

	_, err := handler.Handle(t.Context(), adjTenant(t, "tenant-1"), rejectionRequest(t))
	if !errors.Is(err, domain.ErrNotAuthorized) {
		t.Fatalf("error = %v, want ErrNotAuthorized", err)
	}
}

func TestAdjudicationDoesNotQueryWhenTenantIsMissing(t *testing.T) {
	store := &grantStoreDouble{err: errors.New("不该被问到")}
	handler := application.NewAdjudicateCommercialAuthorizationHandler(store)

	_, err := handler.Handle(t.Context(), domain.TenantID{}, rejectionRequest(t))
	if err == nil {
		t.Fatal("缺租户仍去问了权威")
	}
	if store.tenant.String() != "" {
		t.Fatal("缺租户时发出了装载查询")
	}
}

func TestAdjudicationSurfacesLoaderFailure(t *testing.T) {
	unavailable := errors.New("权威不可读")
	handler := application.NewAdjudicateCommercialAuthorizationHandler(&grantStoreDouble{err: unavailable})

	_, err := handler.Handle(t.Context(), adjTenant(t, "tenant-1"), rejectionRequest(t))
	if !errors.Is(err, unavailable) {
		t.Fatalf("error = %v, want wrapped loader failure", err)
	}
}

func adjTenant(t *testing.T, raw string) domain.TenantID {
	t.Helper()
	tenant, err := domain.NewTenantID(raw)
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	return tenant
}

func adjValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

func rejectionRequest(t *testing.T) domain.AuthorizationRequest {
	t.Helper()
	request, err := domain.NewAuthorizationRequest(
		domain.ActiveRejectionAction,
		adjValue(t, domain.NewLegalEntityReference, "legal-1"),
		adjValue(t, domain.NewAuthorityLevel, "level-commercial"),
		adjValue(t, domain.NewCommercialScopeReference, "scope-a"),
		adjValue(t, domain.NewStructuredReason, "COMMERCIAL_RISK"),
		adjValue(t, domain.NewEvidenceReference, "evidence-1"),
		adjudicateAt,
	)
	if err != nil {
		t.Fatalf("authorization request: %v", err)
	}
	return request
}

func authorityGrant(t *testing.T, objectID string, action domain.AuthorizedAction, level, scope string) domain.AuthorityGrant {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("interval: %v", err)
	}
	approval, err := domain.NewApprovalBasis(
		adjValue(t, domain.NewApprovalReference, "approval-"+objectID),
		adjValue(t, domain.NewCommercialSourceReference, "source-"+objectID),
		time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("approval: %v", err)
	}
	version, err := domain.RehydrateCommercialVersion(domain.RehydrateCommercialVersionSpec{
		TenantID:      adjTenant(t, "tenant-1"),
		Kind:          domain.AuthorizationRuleObject,
		ObjectID:      adjValue(t, domain.NewCommercialObjectID, objectID),
		Version:       adjValue(t, domain.NewCommercialVersionLabel, "v1"),
		Scope:         adjValue(t, domain.NewCommercialScopeReference, scope),
		ContentDigest: adjValue(t, domain.NewCommercialContentDigest, "sha256:"+objectID),
		Effective:     interval,
		Status:        domain.CommercialVersionEffective,
		Approval:      approval,
		PublishedAt:   time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC),
		EffectiveAt:   time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("rehydrate authorization rule: %v", err)
	}
	grant, err := domain.NewAuthorityGrant(
		version,
		action,
		adjValue(t, domain.NewLegalEntityReference, "legal-1"),
		adjValue(t, domain.NewAuthorityLevel, level),
		adjValue(t, domain.NewCommercialScopeReference, scope),
		interval,
	)
	if err != nil {
		t.Fatalf("authority grant: %v", err)
	}
	return grant
}
