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

type delegationViewDouble struct {
	delegations []domain.ContractDelegation
	err         error
	asked       bool
	tenant      domain.TenantID
	scope       domain.CommercialScopeReference
	at          time.Time
}

func (view *delegationViewDouble) LoadEffectiveDelegations(
	_ context.Context,
	tenant domain.TenantID,
	scope domain.CommercialScopeReference,
	at time.Time,
) ([]domain.ContractDelegation, error) {
	view.asked = true
	view.tenant, view.scope, view.at = tenant, scope, at
	if view.err != nil {
		return nil, view.err
	}
	return view.delegations, nil
}

// Covers: ADR-0116 Decision 三 —— 运营角色代客户请求资料修订时，编排装载该范围该时点的有效委派，
// 由 domain.Authorize 解出委派方作实际决定方。
func TestAdjudicationLoadsDelegationsForAnOperatorAmendingOnBehalfOfACustomer(t *testing.T) {
	grant := authorityGrant(t, "auth-amend", domain.SourceDataAmendmentAction, "level-commercial", "scope-a")
	view := &delegationViewDouble{delegations: []domain.ContractDelegation{
		contractDelegation(t, "contract-1", "account-1", "level-commercial", "scope-a"),
	}}
	handler := application.NewAdjudicateCommercialAuthorizationHandlerWithDelegations(
		&grantStoreDouble{grants: []domain.AuthorityGrant{grant}}, view)

	authorized, err := handler.Handle(t.Context(), adjTenant(t, "tenant-1"),
		operatorAmendmentRequest(t, "operator-1", "account-1"))
	if err != nil {
		t.Fatalf("adjudicate: %v", err)
	}
	decider, named := authorized.Decider()
	if !named || decider.Kind() != domain.CustomerAccountDecider || decider.Reference() != "account-1" {
		t.Fatalf("decider = %s %q (named=%v), want CUSTOMER_ACCOUNT account-1", decider.Kind(), decider.Reference(), named)
	}
	if !view.asked || view.tenant.String() != "tenant-1" || view.scope.String() != "scope-a" || !view.at.Equal(adjudicateAt) {
		t.Fatalf("委派装载键 = asked %v tenant %q scope %q at %s；应按询问的范围与业务时点装载",
			view.asked, view.tenant, view.scope, view.at)
	}
}

// Covers: ADR-0116 Decision 三「grant 在场而委派缺席 → ErrNotAuthorized，不是未配置」在编排层原样交回。
func TestAdjudicationWithAGrantButNoDelegationRefusesTheOperator(t *testing.T) {
	grant := authorityGrant(t, "auth-amend", domain.SourceDataAmendmentAction, "level-commercial", "scope-a")
	handler := application.NewAdjudicateCommercialAuthorizationHandlerWithDelegations(
		&grantStoreDouble{grants: []domain.AuthorityGrant{grant}}, &delegationViewDouble{})

	_, err := handler.Handle(t.Context(), adjTenant(t, "tenant-1"), operatorAmendmentRequest(t, "operator-1", "account-1"))
	if !errors.Is(err, domain.ErrNotAuthorized) || !errors.Is(err, domain.ErrDelegationAbsent) {
		t.Fatalf("error = %v, want ErrDelegationAbsent (Is ErrNotAuthorized)", err)
	}
}

// Covers: 委派只在要用到它时装载——客户自己请求、以及既有两格动作，决定方不经委派解出，不多问一次库。
func TestAdjudicationDoesNotLoadDelegationsWhenTheDeciderIsTheRequester(t *testing.T) {
	view := &delegationViewDouble{err: errors.New("不该被问到")}

	t.Run("a customer amending its own data", func(t *testing.T) {
		grant := authorityGrant(t, "auth-amend", domain.SourceDataAmendmentAction, "level-commercial", "scope-a")
		handler := application.NewAdjudicateCommercialAuthorizationHandlerWithDelegations(
			&grantStoreDouble{grants: []domain.AuthorityGrant{grant}}, view)
		authorized, err := handler.Handle(t.Context(), adjTenant(t, "tenant-1"), customerAmendmentRequest(t, "account-1"))
		if err != nil {
			t.Fatalf("adjudicate: %v", err)
		}
		if decider, named := authorized.Decider(); !named || decider.Reference() != "account-1" {
			t.Fatalf("decider = %q (named=%v), want account-1", decider.Reference(), named)
		}
	})

	t.Run("an operator rejecting under its own grant", func(t *testing.T) {
		grant := authorityGrant(t, "auth-reject", domain.ActiveRejectionAction, "level-commercial", "scope-a")
		handler := application.NewAdjudicateCommercialAuthorizationHandlerWithDelegations(
			&grantStoreDouble{grants: []domain.AuthorityGrant{grant}}, view)
		if _, err := handler.Handle(t.Context(), adjTenant(t, "tenant-1"), rejectionRequest(t)); err != nil {
			t.Fatalf("adjudicate: %v", err)
		}
	})

	if view.asked {
		t.Fatal("委派读口被问到了，而这几格的决定方不经委派解出")
	}
}

// Covers: 三步法旧构造器——没有委派读口的编排走到要委派的那一格时报错，不装成空切片：空切片会被
// 读成「客户没委派」（业务拒绝），而这里缺的是接线。
func TestAdjudicationWithoutADelegationViewCannotAnswerAnOperatorAmendment(t *testing.T) {
	grant := authorityGrant(t, "auth-amend", domain.SourceDataAmendmentAction, "level-commercial", "scope-a")
	handler := application.NewAdjudicateCommercialAuthorizationHandler(
		&grantStoreDouble{grants: []domain.AuthorityGrant{grant}})

	_, err := handler.Handle(t.Context(), adjTenant(t, "tenant-1"), operatorAmendmentRequest(t, "operator-1", "account-1"))
	if err == nil {
		t.Fatal("没有委派读口却答出了结果")
	}
	if errors.Is(err, domain.ErrNotAuthorized) || errors.Is(err, domain.ErrAuthorityRulesNotConfigured) {
		t.Fatalf("error = %v；未接线被报成了业务答复", err)
	}
}

// Covers: 委派读口读取失败上抛，不折成空切片（那会说成「客户没委派」）。
func TestAdjudicationSurfacesDelegationLoaderFailure(t *testing.T) {
	grant := authorityGrant(t, "auth-amend", domain.SourceDataAmendmentAction, "level-commercial", "scope-a")
	unavailable := errors.New("委派不可读")
	handler := application.NewAdjudicateCommercialAuthorizationHandlerWithDelegations(
		&grantStoreDouble{grants: []domain.AuthorityGrant{grant}}, &delegationViewDouble{err: unavailable})

	_, err := handler.Handle(t.Context(), adjTenant(t, "tenant-1"), operatorAmendmentRequest(t, "operator-1", "account-1"))
	if !errors.Is(err, unavailable) {
		t.Fatalf("error = %v, want wrapped delegation loader failure", err)
	}
}

func customerAmendmentRequest(t *testing.T, account string) domain.AuthorizationRequest {
	t.Helper()
	requester, err := domain.RequestedByCustomerAccount(adjValue(t, domain.NewCustomerAccountID, account))
	if err != nil {
		t.Fatalf("requester: %v", err)
	}
	return amendmentRequestBy(t, requester)
}

func operatorAmendmentRequest(t *testing.T, operator, onBehalfOf string) domain.AuthorizationRequest {
	t.Helper()
	requester, err := domain.RequestedByOperatorRole(
		adjValue(t, domain.NewOperatorRoleReference, operator),
		adjValue(t, domain.NewCustomerAccountID, onBehalfOf),
	)
	if err != nil {
		t.Fatalf("requester: %v", err)
	}
	return amendmentRequestBy(t, requester)
}

func amendmentRequestBy(t *testing.T, requester domain.AuthorizationRequester) domain.AuthorizationRequest {
	t.Helper()
	request, err := domain.NewAuthorizationRequestBy(
		requester,
		domain.SourceDataAmendmentAction,
		adjValue(t, domain.NewLegalEntityReference, "legal-1"),
		adjValue(t, domain.NewAuthorityLevel, "level-commercial"),
		adjValue(t, domain.NewCommercialScopeReference, "scope-a"),
		adjValue(t, domain.NewStructuredReason, "CUSTOMER_CORRECTION"),
		adjValue(t, domain.NewEvidenceReference, "evidence-1"),
		adjudicateAt,
	)
	if err != nil {
		t.Fatalf("authorization request: %v", err)
	}
	return request
}

func contractDelegation(t *testing.T, contractID, account, level, scope string) domain.ContractDelegation {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("interval: %v", err)
	}
	approval, err := domain.NewApprovalBasis(
		adjValue(t, domain.NewApprovalReference, "approval-"+contractID),
		adjValue(t, domain.NewCommercialSourceReference, "source-"+contractID),
		time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("approval: %v", err)
	}
	contract, err := domain.RehydrateCommercialVersion(domain.RehydrateCommercialVersionSpec{
		TenantID:      adjTenant(t, "tenant-1"),
		Kind:          domain.CustomerContractObject,
		ObjectID:      adjValue(t, domain.NewCommercialObjectID, contractID),
		Version:       adjValue(t, domain.NewCommercialVersionLabel, "v1"),
		Scope:         adjValue(t, domain.NewCommercialScopeReference, scope),
		ContentDigest: adjValue(t, domain.NewCommercialContentDigest, "sha256:"+contractID),
		Effective:     interval,
		Status:        domain.CommercialVersionEffective,
		Approval:      approval,
		PublishedAt:   time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC),
		EffectiveAt:   time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("rehydrate customer contract: %v", err)
	}
	delegator, err := domain.DelegatedByCustomerAccount(adjValue(t, domain.NewCustomerAccountID, account))
	if err != nil {
		t.Fatalf("delegator: %v", err)
	}
	delegation, err := domain.NewContractDelegation(
		contract,
		delegator,
		domain.SourceDataAmendmentAction,
		adjValue(t, domain.NewCommercialScopeReference, scope),
		adjValue(t, domain.NewAuthorityLevel, level),
		interval,
	)
	if err != nil {
		t.Fatalf("contract delegation: %v", err)
	}
	return delegation
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
