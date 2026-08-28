package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

var (
	identityFrom       = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	beforeIdentityFrom = time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	afterIdentityFrom  = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	deactivationAt     = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
)

func identityLifecycle(t *testing.T) domain.IdentityLifecycle {
	t.Helper()
	lifecycle, err := domain.NewIdentityLifecycle(identityFrom)
	if err != nil {
		t.Fatalf("new identity lifecycle: %v", err)
	}
	return lifecycle
}

func identityBasis(t *testing.T, value string) domain.IdentityBasisReference {
	t.Helper()
	return commercialValue(t, domain.NewIdentityBasisReference, value)
}

// Covers: CONTEXT 参与方身份「已登记 → 已生效」——生效时点未到的登记不支持新决定，
// 生效时点一到状态即为已生效。
func TestIdentityLifecycleDerivesStatusFromInstants(t *testing.T) {
	lifecycle := identityLifecycle(t)

	if status := lifecycle.StatusAt(beforeIdentityFrom); status != domain.IdentityRegistered {
		t.Fatalf("status before effectiveFrom = %v, want REGISTERED", status)
	}
	if status := lifecycle.StatusAt(identityFrom); status != domain.IdentityEffective {
		t.Fatalf("status at effectiveFrom = %v, want EFFECTIVE", status)
	}
	if status := lifecycle.StatusAt(afterIdentityFrom); status != domain.IdentityEffective {
		t.Fatalf("status after effectiveFrom = %v, want EFFECTIVE", status)
	}
}

// Covers: CONTEXT「已生效 → 已停用：停用必须携带显式依据与停用时点」——无依据、无
// 时点与二次停用都被拒。
func TestIdentityDeactivationDemandsBasisAndInstantOnce(t *testing.T) {
	lifecycle := identityLifecycle(t)

	if _, err := lifecycle.Deactivate(domain.IdentityBasisReference{}, deactivationAt); !errors.Is(err, domain.ErrInvalidIdentityTransition) {
		t.Fatalf("deactivate without basis: error = %v, want ErrInvalidIdentityTransition", err)
	}
	if _, err := lifecycle.Deactivate(identityBasis(t, "basis-deact"), time.Time{}); !errors.Is(err, domain.ErrInvalidIdentityTransition) {
		t.Fatalf("deactivate without instant: error = %v, want ErrInvalidIdentityTransition", err)
	}

	deactivated, err := lifecycle.Deactivate(identityBasis(t, "basis-deact"), deactivationAt)
	if err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	if _, err := deactivated.Deactivate(identityBasis(t, "basis-again"), deactivationAt.Add(time.Hour)); !errors.Is(err, domain.ErrInvalidIdentityTransition) {
		t.Fatalf("second deactivation: error = %v, want ErrInvalidIdentityTransition", err)
	}

	if status := deactivated.StatusAt(deactivationAt); status != domain.IdentityDeactivated {
		t.Fatalf("status at deactivation = %v, want DEACTIVATED", status)
	}
	if status := deactivated.StatusAt(afterIdentityFrom); status != domain.IdentityEffective {
		t.Fatalf("status before deactivation = %v, want EFFECTIVE（停用不改写历史）", status)
	}
	basis, at, has := deactivated.Deactivation()
	if !has || basis.String() != "basis-deact" || !at.Equal(deactivationAt) {
		t.Fatalf("deactivation facts = (%v, %v, %v)", basis, at, has)
	}
}

// Covers: CONTEXT「生效时点未到的登记也可以被停用——那是撤下一笔尚未生效的登记」——
// 撤下后自撤下时点起即为已停用，不会先「生效」一下。
func TestNotYetEffectiveIdentityCanBeWithdrawn(t *testing.T) {
	lifecycle := identityLifecycle(t)
	withdrawn, err := lifecycle.Deactivate(identityBasis(t, "basis-withdraw"), beforeIdentityFrom)
	if err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	if status := withdrawn.StatusAt(afterIdentityFrom); status != domain.IdentityDeactivated {
		t.Fatalf("status after withdrawn instant = %v, want DEACTIVATED", status)
	}
}

// Covers: CONTEXT「责任法人登记必须显式关联其业务参与方身份…且账户与参与方同租户
// （ADR-0003）」——法人绑他租参与方被拒，与客户账户的跨租守卫同一条。
func TestLegalEntityRejectsPartyFromAnotherTenant(t *testing.T) {
	foreign := partyInTenant(t, "tenant-2", "party-9", "他租参与方")
	_, err := domain.NewResponsibleLegalEntity(
		commercialValue(t, domain.NewTenantID, "tenant-1"),
		commercialValue(t, domain.NewLegalEntityReference, "le-1"),
		foreign,
	)
	if !errors.Is(err, domain.ErrCrossTenantLegalEntityParty) {
		t.Fatalf("error = %v, want ErrCrossTenantLegalEntityParty", err)
	}

	local := party(t, "party-1", "本租参与方")
	entity, err := domain.NewResponsibleLegalEntity(
		commercialValue(t, domain.NewTenantID, "tenant-1"),
		commercialValue(t, domain.NewLegalEntityReference, "le-1"),
		local,
	)
	if err != nil {
		t.Fatalf("new legal entity: %v", err)
	}
	if entity.Party() != local.ID() {
		t.Fatalf("entity party = %v, want %v", entity.Party(), local.ID())
	}
}

// Covers: CONTEXT「身份登记版本化不可覆盖…停用同样以新的登记修订表达」——登记信封
// 要求修订从 1 起，Deactivate 交回修订号加一的下一笔，不改写原修订。
func TestRegistrationRevisionsAdvanceWithoutRewriting(t *testing.T) {
	registration, err := domain.NewBusinessPartyRegistration(
		party(t, "party-1", "参与方一号"),
		1,
		identityBasis(t, "basis-reg"),
		identityLifecycle(t),
	)
	if err != nil {
		t.Fatalf("new registration: %v", err)
	}

	deactivated, err := registration.Deactivate(identityBasis(t, "basis-deact"), deactivationAt)
	if err != nil {
		t.Fatalf("deactivate registration: %v", err)
	}
	if deactivated.Revision() != 2 {
		t.Fatalf("deactivated revision = %d, want 2", deactivated.Revision())
	}
	if registration.Revision() != 1 {
		t.Fatalf("original revision mutated to %d", registration.Revision())
	}
	if _, _, has := registration.Lifecycle().Deactivation(); has {
		t.Fatal("original registration gained a deactivation")
	}

	if _, err := domain.NewBusinessPartyRegistration(
		party(t, "party-1", "参与方一号"), 0,
		identityBasis(t, "basis-reg"), identityLifecycle(t),
	); !errors.Is(err, domain.ErrInvalidIdentityRegistration) {
		t.Fatalf("revision 0: error = %v, want ErrInvalidIdentityRegistration", err)
	}
}

// Covers: 登记信封的完整性门——法人与客户账户登记同样拒缺件（依据、生命周期、修订）。
func TestEntityAndAccountRegistrationsRejectIncompleteContent(t *testing.T) {
	entity, err := domain.NewResponsibleLegalEntity(
		commercialValue(t, domain.NewTenantID, "tenant-1"),
		commercialValue(t, domain.NewLegalEntityReference, "le-1"),
		party(t, "party-1", "参与方一号"),
	)
	if err != nil {
		t.Fatalf("new legal entity: %v", err)
	}
	if _, err := domain.NewLegalEntityRegistration(
		entity, 1, domain.IdentityBasisReference{}, identityLifecycle(t),
	); !errors.Is(err, domain.ErrInvalidIdentityRegistration) {
		t.Fatalf("entity registration without basis: error = %v, want ErrInvalidIdentityRegistration", err)
	}

	account, err := domain.NewCustomerAccount(
		commercialValue(t, domain.NewTenantID, "tenant-1"),
		commercialValue(t, domain.NewCustomerAccountID, "account-1"),
		party(t, "party-2", "客户参与方"),
	)
	if err != nil {
		t.Fatalf("new customer account: %v", err)
	}
	if _, err := domain.NewCustomerAccountRegistration(
		account, 1, identityBasis(t, "basis-reg"), domain.IdentityLifecycle{},
	); !errors.Is(err, domain.ErrInvalidIdentityRegistration) {
		t.Fatalf("account registration without lifecycle: error = %v, want ErrInvalidIdentityRegistration", err)
	}

	registration, err := domain.NewCustomerAccountRegistration(
		account, 1, identityBasis(t, "basis-reg"), identityLifecycle(t),
	)
	if err != nil {
		t.Fatalf("new account registration: %v", err)
	}
	if registration.Account().CustomerParty().String() != "party-2" {
		t.Fatalf("account customer party = %v", registration.Account().CustomerParty())
	}
}

// Covers: 关系登记信封——关系正文沿既有 PartyRelationship 模型；零值关系（没经构造门）
// 与坏修订进不了登记册。
func TestRelationshipRegistrationDemandsConstructedRelationship(t *testing.T) {
	tenant := commercialValue(t, domain.NewTenantID, "tenant-1")
	id := commercialValue(t, domain.NewRelationshipID, "rel-1")

	if _, err := domain.NewPartyRelationshipRegistration(
		tenant, id, 1, domain.PartyRelationship{},
	); !errors.Is(err, domain.ErrInvalidPartyRelationship) {
		t.Fatalf("zero relationship: error = %v, want ErrInvalidPartyRelationship", err)
	}

	relationship := effectiveRelationship(t, "party-1", "party-2", domain.CustomerRole)
	if _, err := domain.NewPartyRelationshipRegistration(
		tenant, id, 0, relationship,
	); !errors.Is(err, domain.ErrInvalidPartyRelationship) {
		t.Fatalf("revision 0: error = %v, want ErrInvalidPartyRelationship", err)
	}

	registration, err := domain.NewPartyRelationshipRegistration(tenant, id, 1, relationship)
	if err != nil {
		t.Fatalf("new relationship registration: %v", err)
	}
	if registration.Relationship().Status() != domain.RelationshipEffective {
		t.Fatalf("registered relationship status = %v", registration.Relationship().Status())
	}
}
