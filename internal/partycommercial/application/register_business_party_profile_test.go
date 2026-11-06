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

// fakeBusinessPartyProfiles 是参与方资料登记册的内存替身：同键同内容答重放、异内容答冲突。
type fakeBusinessPartyProfiles struct {
	revisions map[string][]domain.BusinessPartyProfileRevision
}

func (fake *fakeBusinessPartyProfiles) SaveBusinessPartyProfile(
	_ context.Context, revision domain.BusinessPartyProfileRevision,
) (ports.BusinessPartyProfileSaveOutcome, error) {
	key := registryKey(revision.Tenant().String(), revision.Party().String())
	for _, existing := range fake.revisions[key] {
		if existing.Revision() == revision.Revision() {
			if reflect.DeepEqual(existing, revision) {
				return ports.BusinessPartyProfileAlreadyRegistered, nil
			}
			return ports.BusinessPartyProfileContentConflict, nil
		}
	}
	fake.revisions[key] = append(fake.revisions[key], revision)
	return ports.BusinessPartyProfileSaved, nil
}

func (fake *fakeBusinessPartyProfiles) LoadLatestBusinessPartyProfile(
	_ context.Context, tenant domain.TenantID, party domain.PartyID,
) (domain.BusinessPartyProfileRevision, bool, error) {
	latest, found := latestRevision(fake.revisions, registryKey(tenant.String(), party.String()), domain.BusinessPartyProfileRevision.Revision)
	return latest, found, nil
}

var _ ports.BusinessPartyProfileRegistry = (*fakeBusinessPartyProfiles)(nil)

type profileFixture struct {
	parties  *fakePartyRegistry
	profiles *fakeBusinessPartyProfiles
	identity *application.RegisterPartyIdentityHandler
	handler  *application.RegisterBusinessPartyProfileHandler
}

func newProfileFixture(t *testing.T) profileFixture {
	t.Helper()
	parties := newFakePartyRegistry()
	profiles := &fakeBusinessPartyProfiles{revisions: map[string][]domain.BusinessPartyProfileRevision{}}
	types := newFakeNumberTypes(t)
	types.byCountry["CN"] = append(types.byCountry["CN"], numberType(t, "CN", "SYN-XA-VAT", domain.RegistrationNumberProfileLayer, `SYN-XA-VAT-[0-9]{4}`))
	return profileFixture{
		parties:  parties,
		profiles: profiles,
		identity: application.NewRegisterPartyIdentityHandler(parties, types),
		handler:  application.NewRegisterBusinessPartyProfileHandler(profiles, parties, types),
	}
}

func partyProfileCommand(t *testing.T, party string, revision int, parts domain.BusinessPartyProfileParts) application.RegisterBusinessPartyProfileCommand {
	t.Helper()
	return application.RegisterBusinessPartyProfileCommand{
		Tenant:        identityValue(t, domain.NewTenantID, "tenant-1"),
		Party:         identityValue(t, domain.NewPartyID, party),
		Revision:      revision,
		Basis:         identityValue(t, domain.NewBusinessPartyProfileBasisReference, "basis-profile"),
		EffectiveFrom: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		Parts:         parts,
	}
}

func cnAddress(t *testing.T) *domain.RegisteredAddress {
	t.Helper()
	address, err := domain.NewRegisteredAddress(identityValue(t, domain.NewRegistrationCountryCode, "CN"), []string{"上海市 SYN 路 1 号"})
	if err != nil {
		t.Fatal(err)
	}
	return &address
}

func vat(t *testing.T, number string) domain.TaxRegistrationNumber {
	t.Helper()
	tax, err := domain.NewTaxRegistrationNumber(identityValue(t, domain.NewRegistrationNumberTypeCode, "SYN-XA-VAT"), identityValue(t, domain.NewRegistrationNumber, number))
	if err != nil {
		t.Fatal(err)
	}
	return tax
}

func expectProfileOutcome(t *testing.T, result application.BusinessPartyProfileResult, err error, want application.BusinessPartyProfileOutcome, mentions string) {
	t.Helper()
	if err != nil || result.Outcome() != want {
		t.Fatalf("result = (%v, %v, cause %v), want %v", result.Outcome(), err, result.Cause(), want)
	}
	if mentions != "" && (result.Cause() == nil || !strings.Contains(result.Cause().Error(), mentions)) {
		t.Fatalf("cause = %v, want it to mention %q", result.Cause(), mentions)
	}
}

// Covers: 修订从 1 起连续；空正文成立；重放答重复、同修订异内容答冲突；往来单位未登记、已停用、是我方公司都拒。
func TestBusinessPartyProfileRevisionsAndPartyGates(t *testing.T) {
	fixture := newProfileFixture(t)
	ctx := context.Background()

	result, err := fixture.handler.Register(ctx, partyProfileCommand(t, "party-a", 1, domain.BusinessPartyProfileParts{}))
	expectProfileOutcome(t, result, err, application.BusinessPartyProfileNotAccepted, "未登记")

	registerParty(t, fixture.identity, "tenant-1", "party-a", "单位甲")
	result, err = fixture.handler.Register(ctx, partyProfileCommand(t, "party-a", 2, domain.BusinessPartyProfileParts{}))
	expectProfileOutcome(t, result, err, application.BusinessPartyProfileNotAccepted, "首笔资料修订必须是 1")
	first := partyProfileCommand(t, "party-a", 1, domain.BusinessPartyProfileParts{
		Names: []domain.LocalizedPartyName{{Language: "en", Name: "Party A Co., Ltd."}}, RegisteredAddress: cnAddress(t),
	})
	result, err = fixture.handler.Register(ctx, first)
	expectProfileOutcome(t, result, err, application.BusinessPartyProfileRegistered, "")
	result, err = fixture.handler.Register(ctx, first)
	expectProfileOutcome(t, result, err, application.BusinessPartyProfileAlreadyRegistered, "")
	conflict := partyProfileCommand(t, "party-a", 1, domain.BusinessPartyProfileParts{})
	result, err = fixture.handler.Register(ctx, conflict)
	expectProfileOutcome(t, result, err, application.BusinessPartyProfileContentConflict, "")
	result, err = fixture.handler.Register(ctx, partyProfileCommand(t, "party-a", 3, domain.BusinessPartyProfileParts{}))
	expectProfileOutcome(t, result, err, application.BusinessPartyProfileNotAccepted, "修订必须连续")
	result, err = fixture.handler.Register(ctx, partyProfileCommand(t, "party-a", 2, domain.BusinessPartyProfileParts{}))
	expectProfileOutcome(t, result, err, application.BusinessPartyProfileRegistered, "")

	registerParty(t, fixture.identity, "tenant-1", "party-le", "运营法人参与方")
	entity := withLegalEntityIdentity(t, legalEntityCommand(t, 1), "CN", "SYN-XA-LIFETIME", "SYN-XA-000001")
	if result, err := fixture.identity.RegisterLegalEntity(ctx, entity); err != nil || result.Outcome() != application.PartyIdentityRegistered {
		t.Fatalf("legal entity = %v %v", result.Outcome(), result.Cause())
	}
	result, err = fixture.handler.Register(ctx, partyProfileCommand(t, "party-le", 1, domain.BusinessPartyProfileParts{}))
	expectProfileOutcome(t, result, err, application.BusinessPartyProfileNotAccepted, "只登在公司主体的法人资料上")

	registerParty(t, fixture.identity, "tenant-1", "party-stop", "单位停")
	stopped, err := fixture.identity.Deactivate(ctx, application.DeactivatePartyIdentityCommand{
		Tenant: identityValue(t, domain.NewTenantID, "tenant-1"), Kind: application.BusinessPartyIdentity, ID: "party-stop",
		Revision: 2, Basis: identityValue(t, domain.NewIdentityBasisReference, "basis-stop"), At: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil || stopped.Outcome() != application.PartyIdentityDeactivated {
		t.Fatalf("deactivate = %v %v", stopped.Outcome(), err)
	}
	result, err = fixture.handler.Register(ctx, partyProfileCommand(t, "party-stop", 1, domain.BusinessPartyProfileParts{}))
	expectProfileOutcome(t, result, err, application.BusinessPartyProfileNotAccepted, "已停用")
}

// Covers: 反方向的门（ADR-0369 决定二）——登着带内容的参与方资料的往来单位，不能被公司主体钉住；撤成空资料后放行。
func TestALegalEntityDoesNotPinAPartyCarryingAProfile(t *testing.T) {
	fixture := newProfileFixture(t)
	ctx := context.Background()
	registerParty(t, fixture.identity, "tenant-1", "party-le", "运营法人参与方")
	fixture.parties.profiled[registryKey("tenant-1", "party-le")] = true
	entity := withLegalEntityIdentity(t, legalEntityCommand(t, 1), "CN", "SYN-XA-LIFETIME", "SYN-XA-000001")
	result, err := fixture.identity.RegisterLegalEntity(ctx, entity)
	expectNotAccepted(t, result, err, "登着参与方资料")
	fixture.parties.profiled[registryKey("tenant-1", "party-le")] = false
	result, err = fixture.identity.RegisterLegalEntity(ctx, entity)
	if err != nil || result.Outcome() != application.PartyIdentityRegistered {
		t.Fatalf("撤下资料后 = %v %v", result.Outcome(), result.Cause())
	}
}

// Covers: 注册地址国家对着往来单位的注册国家判；监管登记号按注册国家、没有则按注册地址国家的资料层目录判，两者都没有拒；
// 资料层以外的类型拒。
func TestBusinessPartyProfileCountryAndNumberGates(t *testing.T) {
	fixture := newProfileFixture(t)
	ctx := context.Background()

	registerParty(t, fixture.identity, "tenant-1", "party-plain", "无号单位")
	result, err := fixture.handler.Register(ctx, partyProfileCommand(t, "party-plain", 1, domain.BusinessPartyProfileParts{
		TaxNumbers: []domain.TaxRegistrationNumber{vat(t, "SYN-XA-VAT-0001")},
	}))
	expectProfileOutcome(t, result, err, application.BusinessPartyProfileNotAccepted, "没有可判的国家")
	result, err = fixture.handler.Register(ctx, partyProfileCommand(t, "party-plain", 1, domain.BusinessPartyProfileParts{
		RegisteredAddress: cnAddress(t), TaxNumbers: []domain.TaxRegistrationNumber{vat(t, "SYN-XA-VAT-0001")},
	}))
	expectProfileOutcome(t, result, err, application.BusinessPartyProfileRegistered, "")
	result, err = fixture.handler.Register(ctx, partyProfileCommand(t, "party-plain", 2, domain.BusinessPartyProfileParts{
		RegisteredAddress: cnAddress(t), TaxNumbers: []domain.TaxRegistrationNumber{vat(t, "SYN-XA-VAT-12")},
	}))
	expectProfileOutcome(t, result, err, application.BusinessPartyProfileNotAccepted, "不合类型")

	numbered := withPartyNumber(t, partyCommand(t, "party-sg", "新加坡单位", 1), "CN", "SYN-XA-LIFETIME", "SYN-XA-000077")
	if result, err := fixture.identity.RegisterBusinessParty(ctx, numbered); err != nil || result.Outcome() != application.PartyIdentityRegistered {
		t.Fatalf("party = %v %v", result.Outcome(), result.Cause())
	}
	sgAddress, err := domain.NewRegisteredAddress(identityValue(t, domain.NewRegistrationCountryCode, "SG"), []string{"SYN Road 1"})
	if err != nil {
		t.Fatal(err)
	}
	result, err = fixture.handler.Register(ctx, partyProfileCommand(t, "party-sg", 1, domain.BusinessPartyProfileParts{RegisteredAddress: &sgAddress}))
	expectProfileOutcome(t, result, err, application.BusinessPartyProfileNotAccepted, "不一致")
	// 办公地址不对注册国家比：分支机构可以在别处。
	result, err = fixture.handler.Register(ctx, partyProfileCommand(t, "party-sg", 1, domain.BusinessPartyProfileParts{OfficeAddress: &sgAddress}))
	expectProfileOutcome(t, result, err, application.BusinessPartyProfileRegistered, "")
	lifetime, err := domain.NewTaxRegistrationNumber(identityValue(t, domain.NewRegistrationNumberTypeCode, "SYN-XA-LIFETIME"), identityValue(t, domain.NewRegistrationNumber, "SYN-XA-000077"))
	if err != nil {
		t.Fatal(err)
	}
	result, err = fixture.handler.Register(ctx, partyProfileCommand(t, "party-sg", 2, domain.BusinessPartyProfileParts{TaxNumbers: []domain.TaxRegistrationNumber{lifetime}}))
	expectProfileOutcome(t, result, err, application.BusinessPartyProfileNotAccepted, "属身份层")
}
