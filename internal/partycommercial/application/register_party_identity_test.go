package application_test

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// fakePartyRegistry 是 ports.PartyIdentityRegistry 的内存替身：同键同内容答重放、
// 异内容答冲突，与真适配器的 ADR-0031 代数同形。
type fakePartyRegistry struct {
	parties       map[string][]domain.BusinessPartyRegistration
	entities      map[string][]domain.LegalEntityRegistration
	accounts      map[string][]domain.CustomerAccountRegistration
	relationships map[string][]domain.PartyRelationshipRegistration
}

func newFakePartyRegistry() *fakePartyRegistry {
	return &fakePartyRegistry{
		parties:       map[string][]domain.BusinessPartyRegistration{},
		entities:      map[string][]domain.LegalEntityRegistration{},
		accounts:      map[string][]domain.CustomerAccountRegistration{},
		relationships: map[string][]domain.PartyRelationshipRegistration{},
	}
}

func registryKey(tenant, id string) string { return tenant + "\x00" + id }

func saveRevision[T any](
	store map[string][]T, key string, revision func(T) int, next T,
) (ports.PartyRegistrySaveOutcome, error) {
	for _, existing := range store[key] {
		if revision(existing) == revision(next) {
			if reflect.DeepEqual(existing, next) {
				return ports.PartyRegistryAlreadyRegistered, nil
			}
			return ports.PartyRegistryContentConflict, nil
		}
	}
	store[key] = append(store[key], next)
	return ports.PartyRegistrySaved, nil
}

func latestRevision[T any](store map[string][]T, key string, revision func(T) int) (T, bool) {
	var latest T
	found := false
	for _, existing := range store[key] {
		if !found || revision(existing) > revision(latest) {
			latest = existing
			found = true
		}
	}
	return latest, found
}

func (registry *fakePartyRegistry) SaveBusinessParty(
	_ context.Context, registration domain.BusinessPartyRegistration,
) (ports.PartyRegistrySaveOutcome, error) {
	key := registryKey(registration.Party().Tenant().String(), registration.Party().ID().String())
	return saveRevision(registry.parties, key,
		domain.BusinessPartyRegistration.Revision, registration)
}

func (registry *fakePartyRegistry) SaveLegalEntity(
	_ context.Context, registration domain.LegalEntityRegistration,
) (ports.PartyRegistrySaveOutcome, error) {
	key := registryKey(registration.Entity().Tenant().String(), registration.Entity().ID().String())
	return saveRevision(registry.entities, key,
		domain.LegalEntityRegistration.Revision, registration)
}

func (registry *fakePartyRegistry) SaveCustomerAccount(
	_ context.Context, registration domain.CustomerAccountRegistration,
) (ports.PartyRegistrySaveOutcome, error) {
	key := registryKey(registration.Account().Tenant().String(), registration.Account().ID().String())
	return saveRevision(registry.accounts, key,
		domain.CustomerAccountRegistration.Revision, registration)
}

func (registry *fakePartyRegistry) SaveRelationship(
	_ context.Context, registration domain.PartyRelationshipRegistration,
) (ports.PartyRegistrySaveOutcome, error) {
	key := registryKey(registration.Tenant().String(), registration.ID().String())
	return saveRevision(registry.relationships, key,
		domain.PartyRelationshipRegistration.Revision, registration)
}

func (registry *fakePartyRegistry) LoadLatestBusinessParty(
	_ context.Context, tenant domain.TenantID, party domain.PartyID,
) (domain.BusinessPartyRegistration, bool, error) {
	latest, found := latestRevision(registry.parties,
		registryKey(tenant.String(), party.String()), domain.BusinessPartyRegistration.Revision)
	return latest, found, nil
}

func (registry *fakePartyRegistry) LoadLatestLegalEntity(
	_ context.Context, tenant domain.TenantID, entity domain.LegalEntityReference,
) (domain.LegalEntityRegistration, bool, error) {
	latest, found := latestRevision(registry.entities,
		registryKey(tenant.String(), entity.String()), domain.LegalEntityRegistration.Revision)
	return latest, found, nil
}

func (registry *fakePartyRegistry) LoadLatestCustomerAccount(
	_ context.Context, tenant domain.TenantID, account domain.CustomerAccountID,
) (domain.CustomerAccountRegistration, bool, error) {
	latest, found := latestRevision(registry.accounts,
		registryKey(tenant.String(), account.String()), domain.CustomerAccountRegistration.Revision)
	return latest, found, nil
}

func (registry *fakePartyRegistry) LoadLatestRelationship(
	_ context.Context, tenant domain.TenantID, relationship domain.RelationshipID,
) (domain.PartyRelationshipRegistration, bool, error) {
	latest, found := latestRevision(registry.relationships,
		registryKey(tenant.String(), relationship.String()), domain.PartyRelationshipRegistration.Revision)
	return latest, found, nil
}

var _ ports.PartyIdentityRegistry = (*fakePartyRegistry)(nil)

var (
	partyEffectiveFrom  = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	entityEffectiveFrom = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	relationStartsAt    = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	deactivateAt        = time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
)

func identityValue[T interface{ String() string }](t *testing.T, constructor func(string) (T, error), raw string) T {
	t.Helper()
	value, err := constructor(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

func testParty(t *testing.T, tenant, id, name string) domain.BusinessParty {
	t.Helper()
	party, err := domain.NewBusinessParty(
		identityValue(t, domain.NewTenantID, tenant),
		identityValue(t, domain.NewPartyID, id),
		identityValue(t, domain.NewPartyName, name),
	)
	if err != nil {
		t.Fatalf("new business party: %v", err)
	}
	return party
}

func registerParty(
	t *testing.T,
	handler *application.RegisterPartyIdentityHandler,
	tenant, id, name string,
) {
	t.Helper()
	result, err := handler.RegisterBusinessParty(context.Background(), application.RegisterBusinessPartyCommand{
		Party:         testParty(t, tenant, id, name),
		Revision:      1,
		Basis:         identityValue(t, domain.NewIdentityBasisReference, "basis-"+id),
		EffectiveFrom: partyEffectiveFrom,
	})
	if err != nil {
		t.Fatalf("register party %s: %v", id, err)
	}
	if result.Outcome() != application.PartyIdentityRegistered {
		t.Fatalf("register party %s outcome = %s", id, result.Outcome())
	}
}

// Covers: 票 01「登记→生效→停用」写侧闭环——登记落册、重放答重复、停用落下一修订、
// 停用后不再收新修订。
func TestPartyRegistrationDeactivationRoundTrip(t *testing.T) {
	registry := newFakePartyRegistry()
	handler := application.NewRegisterPartyIdentityHandler(registry)
	ctx := context.Background()

	command := application.RegisterBusinessPartyCommand{
		Party:         testParty(t, "tenant-1", "party-1", "参与方一号"),
		Revision:      1,
		Basis:         identityValue(t, domain.NewIdentityBasisReference, "basis-1"),
		EffectiveFrom: partyEffectiveFrom,
	}
	result, err := handler.RegisterBusinessParty(ctx, command)
	if err != nil || result.Outcome() != application.PartyIdentityRegistered {
		t.Fatalf("first registration = (%v, %v)", result.Outcome(), err)
	}

	replay, err := handler.RegisterBusinessParty(ctx, command)
	if err != nil || replay.Outcome() != application.PartyIdentityAlreadyRegistered {
		t.Fatalf("replay = (%v, %v), want ALREADY_REGISTERED", replay.Outcome(), err)
	}

	deactivate := application.DeactivatePartyIdentityCommand{
		Tenant:   identityValue(t, domain.NewTenantID, "tenant-1"),
		Kind:     application.BusinessPartyIdentity,
		ID:       "party-1",
		Revision: 2,
		Basis:    identityValue(t, domain.NewIdentityBasisReference, "basis-deact"),
		At:       deactivateAt,
	}
	deactivated, err := handler.Deactivate(ctx, deactivate)
	if err != nil || deactivated.Outcome() != application.PartyIdentityDeactivated {
		t.Fatalf("deactivate = (%v, %v)", deactivated.Outcome(), err)
	}

	deactivateReplay, err := handler.Deactivate(ctx, deactivate)
	if err != nil || deactivateReplay.Outcome() != application.PartyIdentityAlreadyRegistered {
		t.Fatalf("deactivate replay = (%v, %v), want ALREADY_REGISTERED", deactivateReplay.Outcome(), err)
	}

	conflicting := deactivate
	conflicting.Basis = identityValue(t, domain.NewIdentityBasisReference, "basis-other")
	conflicted, err := handler.Deactivate(ctx, conflicting)
	if err != nil || conflicted.Outcome() != application.PartyIdentityContentConflict {
		t.Fatalf("conflicting deactivation = (%v, %v), want CONTENT_CONFLICT", conflicted.Outcome(), err)
	}

	afterDeactivation := command
	afterDeactivation.Revision = 3
	rejected, err := handler.RegisterBusinessParty(ctx, afterDeactivation)
	if err != nil || rejected.Outcome() != application.PartyIdentityNotAccepted {
		t.Fatalf("registration after deactivation = (%v, %v), want NOT_ACCEPTED", rejected.Outcome(), err)
	}
	if cause := rejected.Cause(); cause == nil || !strings.Contains(cause.Error(), "已停用") {
		t.Fatalf("cause = %v，未点名身份已停用", rejected.Cause())
	}
}

// Covers: 修订连续性门——首笔必须是 1，跳号被拒且一个字节没写。
func TestRevisionSlotsMustBeContiguous(t *testing.T) {
	registry := newFakePartyRegistry()
	handler := application.NewRegisterPartyIdentityHandler(registry)
	ctx := context.Background()

	first := application.RegisterBusinessPartyCommand{
		Party:         testParty(t, "tenant-1", "party-1", "参与方一号"),
		Revision:      2,
		Basis:         identityValue(t, domain.NewIdentityBasisReference, "basis-1"),
		EffectiveFrom: partyEffectiveFrom,
	}
	result, err := handler.RegisterBusinessParty(ctx, first)
	if err != nil || result.Outcome() != application.PartyIdentityNotAccepted {
		t.Fatalf("first registration at revision 2 = (%v, %v), want NOT_ACCEPTED", result.Outcome(), err)
	}

	registerParty(t, handler, "tenant-1", "party-1", "参与方一号")

	gap := application.RegisterBusinessPartyCommand{
		Party:         testParty(t, "tenant-1", "party-1", "参与方一号改名"),
		Revision:      4,
		Basis:         identityValue(t, domain.NewIdentityBasisReference, "basis-2"),
		EffectiveFrom: partyEffectiveFrom,
	}
	skipped, err := handler.RegisterBusinessParty(ctx, gap)
	if err != nil || skipped.Outcome() != application.PartyIdentityNotAccepted {
		t.Fatalf("gapped revision = (%v, %v), want NOT_ACCEPTED", skipped.Outcome(), err)
	}
	if len(registry.parties[registryKey("tenant-1", "party-1")]) != 1 {
		t.Fatal("rejected registration still landed on the registry")
	}
}

// Covers: 票 01「法人必须钉在已登记且届时已生效的参与方上」——悬空参与方与生效前
// 时点都被拒；钉住有效参与方后登记落册。
func TestLegalEntityDemandsAnEffectiveParty(t *testing.T) {
	registry := newFakePartyRegistry()
	handler := application.NewRegisterPartyIdentityHandler(registry)
	ctx := context.Background()

	command := application.RegisterLegalEntityCommand{
		Tenant:        identityValue(t, domain.NewTenantID, "tenant-1"),
		Entity:        identityValue(t, domain.NewLegalEntityReference, "le-1"),
		Party:         identityValue(t, domain.NewPartyID, "party-le"),
		Revision:      1,
		Basis:         identityValue(t, domain.NewIdentityBasisReference, "basis-le"),
		EffectiveFrom: entityEffectiveFrom,
	}
	dangling, err := handler.RegisterLegalEntity(ctx, command)
	if err != nil || dangling.Outcome() != application.PartyIdentityNotAccepted {
		t.Fatalf("dangling party = (%v, %v), want NOT_ACCEPTED", dangling.Outcome(), err)
	}

	registerParty(t, handler, "tenant-1", "party-le", "运营法人参与方")

	early := command
	early.EffectiveFrom = partyEffectiveFrom.Add(-24 * time.Hour)
	notYet, err := handler.RegisterLegalEntity(ctx, early)
	if err != nil || notYet.Outcome() != application.PartyIdentityNotAccepted {
		t.Fatalf("entity before party effectiveness = (%v, %v), want NOT_ACCEPTED", notYet.Outcome(), err)
	}

	landed, err := handler.RegisterLegalEntity(ctx, command)
	if err != nil || landed.Outcome() != application.PartyIdentityRegistered {
		t.Fatalf("legal entity registration = (%v, %v)", landed.Outcome(), err)
	}
}

// Covers: 票 01 客户账户登记（ADR-0003 第三级）——同一守卫适用于客户参与方引用。
func TestCustomerAccountDemandsAnEffectiveCustomerParty(t *testing.T) {
	registry := newFakePartyRegistry()
	handler := application.NewRegisterPartyIdentityHandler(registry)
	ctx := context.Background()

	command := application.RegisterCustomerAccountCommand{
		Tenant:        identityValue(t, domain.NewTenantID, "tenant-1"),
		Account:       identityValue(t, domain.NewCustomerAccountID, "account-1"),
		CustomerParty: identityValue(t, domain.NewPartyID, "party-cust"),
		Revision:      1,
		Basis:         identityValue(t, domain.NewIdentityBasisReference, "basis-acct"),
		EffectiveFrom: entityEffectiveFrom,
	}
	dangling, err := handler.RegisterCustomerAccount(ctx, command)
	if err != nil || dangling.Outcome() != application.PartyIdentityNotAccepted {
		t.Fatalf("dangling customer party = (%v, %v), want NOT_ACCEPTED", dangling.Outcome(), err)
	}

	registerParty(t, handler, "tenant-1", "party-cust", "货主客户参与方")
	landed, err := handler.RegisterCustomerAccount(ctx, command)
	if err != nil || landed.Outcome() != application.PartyIdentityRegistered {
		t.Fatalf("customer account registration = (%v, %v)", landed.Outcome(), err)
	}
}

// Covers: 关系登记——双方都要已生效；带批准事实走真转换落已生效，缺批准落候选。
func TestRelationshipRegistrationChecksBothPartiesAndApproval(t *testing.T) {
	registry := newFakePartyRegistry()
	handler := application.NewRegisterPartyIdentityHandler(registry)
	ctx := context.Background()

	registerParty(t, handler, "tenant-1", "party-cust", "货主客户参与方")

	interval, err := domain.NewEffectiveInterval(relationStartsAt, time.Time{})
	if err != nil {
		t.Fatalf("new interval: %v", err)
	}
	spec := domain.PartyRelationshipSpec{
		Holder:       identityValue(t, domain.NewPartyID, "party-cust"),
		Counterparty: identityValue(t, domain.NewPartyID, "party-le"),
		Role:         domain.CustomerRole,
		Scope:        identityValue(t, domain.NewCommercialScopeReference, "scope-1"),
		Basis:        identityValue(t, domain.NewRelationshipBasisReference, "contract-1"),
		Effective:    interval,
	}
	command := application.RegisterPartyRelationshipCommand{
		Tenant:   identityValue(t, domain.NewTenantID, "tenant-1"),
		ID:       identityValue(t, domain.NewRelationshipID, "rel-1"),
		Revision: 1,
		Spec:     spec,
		Approval: &application.RelationshipApproval{
			Reference:  identityValue(t, domain.NewApprovalReference, "approval-1"),
			ApprovedAt: relationStartsAt,
		},
	}

	dangling, err := handler.RegisterRelationship(ctx, command)
	if err != nil || dangling.Outcome() != application.PartyIdentityNotAccepted {
		t.Fatalf("relationship with dangling counterparty = (%v, %v), want NOT_ACCEPTED", dangling.Outcome(), err)
	}

	registerParty(t, handler, "tenant-1", "party-le", "运营法人参与方")

	landed, err := handler.RegisterRelationship(ctx, command)
	if err != nil || landed.Outcome() != application.PartyIdentityRegistered {
		t.Fatalf("approved relationship = (%v, %v)", landed.Outcome(), err)
	}
	stored, found, err := registry.LoadLatestRelationship(ctx,
		identityValue(t, domain.NewTenantID, "tenant-1"),
		identityValue(t, domain.NewRelationshipID, "rel-1"))
	if err != nil || !found {
		t.Fatalf("load stored relationship: (%v, %v)", found, err)
	}
	if stored.Relationship().Status() != domain.RelationshipEffective {
		t.Fatalf("stored status = %v, want EFFECTIVE", stored.Relationship().Status())
	}

	candidate := command
	candidate.ID = identityValue(t, domain.NewRelationshipID, "rel-2")
	candidate.Approval = nil
	landedCandidate, err := handler.RegisterRelationship(ctx, candidate)
	if err != nil || landedCandidate.Outcome() != application.PartyIdentityRegistered {
		t.Fatalf("candidate relationship = (%v, %v)", landedCandidate.Outcome(), err)
	}
	storedCandidate, _, err := registry.LoadLatestRelationship(ctx,
		identityValue(t, domain.NewTenantID, "tenant-1"),
		identityValue(t, domain.NewRelationshipID, "rel-2"))
	if err != nil {
		t.Fatalf("load candidate: %v", err)
	}
	if storedCandidate.Relationship().Status() != domain.RelationshipCandidate {
		t.Fatalf("candidate status = %v, want CANDIDATE", storedCandidate.Relationship().Status())
	}
}

// Covers: 停用命令的落点声明——册上没有的身份答未找到，修订错位被拒。
func TestDeactivationChecksTargetAndRevision(t *testing.T) {
	registry := newFakePartyRegistry()
	handler := application.NewRegisterPartyIdentityHandler(registry)
	ctx := context.Background()

	missing, err := handler.Deactivate(ctx, application.DeactivatePartyIdentityCommand{
		Tenant:   identityValue(t, domain.NewTenantID, "tenant-1"),
		Kind:     application.CustomerAccountIdentity,
		ID:       "account-9",
		Revision: 2,
		Basis:    identityValue(t, domain.NewIdentityBasisReference, "basis-deact"),
		At:       deactivateAt,
	})
	if err != nil || missing.Outcome() != application.PartyIdentityNotFound {
		t.Fatalf("deactivate missing account = (%v, %v), want NOT_FOUND", missing.Outcome(), err)
	}

	registerParty(t, handler, "tenant-1", "party-1", "参与方一号")
	mismatched, err := handler.Deactivate(ctx, application.DeactivatePartyIdentityCommand{
		Tenant:   identityValue(t, domain.NewTenantID, "tenant-1"),
		Kind:     application.BusinessPartyIdentity,
		ID:       "party-1",
		Revision: 5,
		Basis:    identityValue(t, domain.NewIdentityBasisReference, "basis-deact"),
		At:       deactivateAt,
	})
	if err != nil || mismatched.Outcome() != application.PartyIdentityNotAccepted {
		t.Fatalf("mismatched revision = (%v, %v), want NOT_ACCEPTED", mismatched.Outcome(), err)
	}
}
