package application_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件证往来单位识别信息的用例门（ADR-0367 决定一至三）：注册号目录、改号要更正依据、我方公司不在往来单位上登号、
// 全租户一号一主（往来单位与公司主体一并判）、外部编码同码一主，以及这些门不拦重放。

func partyCommand(t *testing.T, id, name string, revision int) application.RegisterBusinessPartyCommand {
	t.Helper()
	return application.RegisterBusinessPartyCommand{
		Party:         testParty(t, "tenant-1", id, name),
		Revision:      revision,
		Basis:         identityValue(t, domain.NewIdentityBasisReference, "basis-"+id),
		EffectiveFrom: partyEffectiveFrom,
	}
}

func withPartyNumber(t *testing.T, command application.RegisterBusinessPartyCommand, country, code, number string) application.RegisterBusinessPartyCommand {
	t.Helper()
	registrationCountry := identityValue(t, domain.NewRegistrationCountryCode, country)
	lifetime, err := domain.NewLifetimeRegistrationNumber(
		identityValue(t, domain.NewRegistrationNumberTypeCode, code),
		identityValue(t, domain.NewRegistrationNumber, number),
	)
	if err != nil {
		t.Fatalf("new lifetime registration number: %v", err)
	}
	command.RegistrationCountry = &registrationCountry
	command.LifetimeNumbers = []domain.LifetimeRegistrationNumber{lifetime}
	return command
}

func externalReference(t *testing.T, system, purpose, code string) domain.ExternalReference {
	t.Helper()
	reference, err := domain.ParseExternalReference(system, purpose, code)
	if err != nil {
		t.Fatalf("parse external reference: %v", err)
	}
	return reference
}

func expectOutcome(t *testing.T, result application.PartyRegistryResult, err error, want application.PartyRegistryOutcome) {
	t.Helper()
	if err != nil || result.Outcome() != want {
		t.Fatalf("result = (%v, %v, cause %v), want %v", result.Outcome(), err, result.Cause(), want)
	}
}

func newIdentificationHandler(t *testing.T) (*fakePartyRegistry, *application.RegisterPartyIdentityHandler) {
	t.Helper()
	registry := newFakePartyRegistry()
	return registry, application.NewRegisterPartyIdentityHandler(registry, newFakeNumberTypes(t))
}

// Covers: 识别号可缺；给了就过注册号目录，判法与法人身份层同一道门；缺一格是漏填。
func TestBusinessPartyNumbersPassTheCatalogue(t *testing.T) {
	registry, handler := newIdentificationHandler(t)
	ctx := context.Background()

	result, err := handler.RegisterBusinessParty(ctx, partyCommand(t, "party-a", "单位甲", 1))
	expectOutcome(t, result, err, application.PartyIdentityRegistered)

	bad := withPartyNumber(t, partyCommand(t, "party-b", "单位乙", 1), "CN", "SYN-XA-LIFETIME", "SYN-XA-12")
	result, err = handler.RegisterBusinessParty(ctx, bad)
	expectNotAccepted(t, result, err, "不合类型")

	profileLayer := withPartyNumber(t, partyCommand(t, "party-b", "单位乙", 1), "CN", "SYN-XA-TAX", "SYN-XA-TAX-0001")
	result, err = handler.RegisterBusinessParty(ctx, profileLayer)
	expectNotAccepted(t, result, err, "属资料层")

	onlyCountry := withPartyNumber(t, partyCommand(t, "party-b", "单位乙", 1), "CN", "SYN-XA-LIFETIME", "SYN-XA-000001")
	onlyCountry.LifetimeNumbers = nil
	result, err = handler.RegisterBusinessParty(ctx, onlyCountry)
	expectNotAccepted(t, result, err, "缺终身注册号")
	if _, found, _ := registry.LoadLatestBusinessParty(ctx, onlyCountry.Party.Tenant(), onlyCountry.Party.ID()); found {
		t.Fatal("refused registrations must not write a byte")
	}

	good := withPartyNumber(t, partyCommand(t, "party-b", "单位乙", 1), "CN", "SYN-XA-LIFETIME", "SYN-XA-000001")
	good.ExternalReferences = []domain.ExternalReference{
		externalReference(t, "SYN-ERP", "SUPPLIER", "V-0001"),
		externalReference(t, "SYN-ERP", "CUSTOMER", "C-0001"),
	}
	result, err = handler.RegisterBusinessParty(ctx, good)
	expectOutcome(t, result, err, application.PartyIdentityRegistered)
	stored, _, _ := registry.LoadLatestBusinessParty(ctx, good.Party.Tenant(), good.Party.ID())
	if layer, has := stored.IdentityLayer(); !has || layer.Country().String() != "CN" {
		t.Fatalf("stored identity = %+v (has %v)", layer, has)
	}
	if references := stored.ExternalReferences(); len(references) != 2 || references[0].Purpose() != domain.ExternalCustomerCode {
		t.Fatalf("external references should be ordered by system then purpose: %+v", references)
	}
}

// Covers: 第一次补登不带更正依据；改号或去号要带；没改却带更正依据拒；更名照带原号是普通修订。
func TestBusinessPartyNumberSuccession(t *testing.T) {
	_, handler := newIdentificationHandler(t)
	ctx := context.Background()
	correction := identityValue(t, domain.NewIdentityBasisReference, "SYN-CORRECTION-1")

	result, err := handler.RegisterBusinessParty(ctx, partyCommand(t, "party-a", "单位甲", 1))
	expectOutcome(t, result, err, application.PartyIdentityRegistered)

	firstNumbers := withPartyNumber(t, partyCommand(t, "party-a", "单位甲", 2), "CN", "SYN-XA-LIFETIME", "SYN-XA-000001")
	withCorrection := firstNumbers
	withCorrection.IdentityCorrectionBasis = &correction
	result, err = handler.RegisterBusinessParty(ctx, withCorrection)
	expectNotAccepted(t, result, err, "第一次补登号")
	result, err = handler.RegisterBusinessParty(ctx, firstNumbers)
	expectOutcome(t, result, err, application.PartyIdentityRegistered)

	renamed := withPartyNumber(t, partyCommand(t, "party-a", "单位甲（更名）", 3), "CN", "SYN-XA-LIFETIME", "SYN-XA-000001")
	result, err = handler.RegisterBusinessParty(ctx, renamed)
	expectOutcome(t, result, err, application.PartyIdentityRegistered)

	changed := withPartyNumber(t, partyCommand(t, "party-a", "单位甲（更名）", 4), "CN", "SYN-XA-LIFETIME", "SYN-XA-000002")
	result, err = handler.RegisterBusinessParty(ctx, changed)
	expectNotAccepted(t, result, err, "不作变更")
	changed.IdentityCorrectionBasis = &correction
	result, err = handler.RegisterBusinessParty(ctx, changed)
	expectOutcome(t, result, err, application.PartyIdentityRegistered)

	removed := partyCommand(t, "party-a", "单位甲（更名）", 5)
	result, err = handler.RegisterBusinessParty(ctx, removed)
	expectNotAccepted(t, result, err, "不作变更")
	removed.IdentityCorrectionBasis = &correction
	result, err = handler.RegisterBusinessParty(ctx, removed)
	expectOutcome(t, result, err, application.PartyIdentityRegistered)
}

// Covers: 一号一主——第二家往来单位登同一个号拒并点名第一家；第一家停用后放行；同号撞上公司主体也拒；
// 公司主体登一个已被往来单位持有的号拒；两家公司主体同号拒。
func TestRegistrationNumbersAreHeldByOneLiveIdentity(t *testing.T) {
	_, handler := newIdentificationHandler(t)
	ctx := context.Background()

	first := withPartyNumber(t, partyCommand(t, "party-a", "单位甲", 1), "CN", "SYN-XA-LIFETIME", "SYN-XA-000001")
	result, err := handler.RegisterBusinessParty(ctx, first)
	expectOutcome(t, result, err, application.PartyIdentityRegistered)

	second := withPartyNumber(t, partyCommand(t, "party-b", "单位乙", 1), "CN", "SYN-XA-LIFETIME", "SYN-XA-000001")
	result, err = handler.RegisterBusinessParty(ctx, second)
	expectNotAccepted(t, result, err, "已登在往来单位 party-a 上")

	// 公司主体要钉的参与方本身不带号；它的号撞上 party-a。
	registerParty(t, handler, "tenant-1", "party-le", "运营法人参与方")
	entity := withLegalEntityIdentity(t, legalEntityCommand(t, 1), "CN", "SYN-XA-LIFETIME", "SYN-XA-000001")
	result, err = handler.RegisterLegalEntity(ctx, entity)
	expectNotAccepted(t, result, err, "已登在往来单位 party-a 上")

	deactivated, err := handler.Deactivate(ctx, application.DeactivatePartyIdentityCommand{
		Tenant:   identityValue(t, domain.NewTenantID, "tenant-1"),
		Kind:     application.BusinessPartyIdentity,
		ID:       "party-a",
		Revision: 2,
		Basis:    identityValue(t, domain.NewIdentityBasisReference, "basis-stop-a"),
		At:       time.Now().Add(-time.Hour),
	})
	expectOutcome(t, deactivated, err, application.PartyIdentityDeactivated)
	result, err = handler.RegisterLegalEntity(ctx, entity)
	expectOutcome(t, result, err, application.PartyIdentityRegistered)

	// 号如今在公司主体 le-1 上：往来单位乙再登它拒并点名公司主体；另一家公司主体登它也拒。
	result, err = handler.RegisterBusinessParty(ctx, second)
	expectNotAccepted(t, result, err, "已登在公司主体 le-1")
	registerParty(t, handler, "tenant-1", "party-le-2", "第二法人参与方")
	other := withLegalEntityIdentity(t, legalEntityCommand(t, 1), "CN", "SYN-XA-LIFETIME", "SYN-XA-000001")
	other.Entity = identityValue(t, domain.NewLegalEntityReference, "le-2")
	other.Party = identityValue(t, domain.NewPartyID, "party-le-2")
	result, err = handler.RegisterLegalEntity(ctx, other)
	expectNotAccepted(t, result, err, "已登在公司主体 le-1")

	// 同一公司主体自己的后续修订不算撞号：同内容重放答重复。
	result, err = handler.RegisterLegalEntity(ctx, entity)
	expectOutcome(t, result, err, application.PartyIdentityAlreadyRegistered)
}

// Covers: 我方公司的号只登在公司主体上——被公司主体钉着的往来单位不收号；公司主体不钉一个带号的往来单位。
func TestOurCompanyNumbersLiveOnlyOnTheLegalEntity(t *testing.T) {
	_, handler := newIdentificationHandler(t)
	ctx := context.Background()

	registerParty(t, handler, "tenant-1", "party-le", "运营法人参与方")
	entity := withLegalEntityIdentity(t, legalEntityCommand(t, 1), "CN", "SYN-XA-LIFETIME", "SYN-XA-000001")
	result, err := handler.RegisterLegalEntity(ctx, entity)
	expectOutcome(t, result, err, application.PartyIdentityRegistered)

	pinned := withPartyNumber(t, partyCommand(t, "party-le", "运营法人参与方", 2), "CN", "SYN-XA-LIFETIME", "SYN-XA-000009")
	result, err = handler.RegisterBusinessParty(ctx, pinned)
	expectNotAccepted(t, result, err, "是公司主体 le-1 的参与方")

	numbered := withPartyNumber(t, partyCommand(t, "party-x", "带号单位", 1), "CN", "SYN-XA-LIFETIME", "SYN-XA-000007")
	result, err = handler.RegisterBusinessParty(ctx, numbered)
	expectOutcome(t, result, err, application.PartyIdentityRegistered)
	onNumbered := withLegalEntityIdentity(t, legalEntityCommand(t, 1), "CN", "SYN-XA-LIFETIME", "SYN-XA-000008")
	onNumbered.Entity = identityValue(t, domain.NewLegalEntityReference, "le-x")
	onNumbered.Party = identityValue(t, domain.NewPartyID, "party-x")
	result, err = handler.RegisterLegalEntity(ctx, onNumbered)
	expectNotAccepted(t, result, err, "自己登着注册号")
}

// Covers: 外部编码同码一主；同一单位同一（系统，用途）给两个码拒；换码是普通修订；查重门不拦重放。
func TestExternalReferencesAreHeldByOneLiveParty(t *testing.T) {
	_, handler := newIdentificationHandler(t)
	ctx := context.Background()

	first := partyCommand(t, "party-a", "单位甲", 1)
	first.ExternalReferences = []domain.ExternalReference{externalReference(t, "SYN-ERP", "CUSTOMER", "C-0001")}
	result, err := handler.RegisterBusinessParty(ctx, first)
	expectOutcome(t, result, err, application.PartyIdentityRegistered)

	second := partyCommand(t, "party-b", "单位乙", 1)
	second.ExternalReferences = []domain.ExternalReference{externalReference(t, "SYN-ERP", "CUSTOMER", "C-0001")}
	result, err = handler.RegisterBusinessParty(ctx, second)
	expectNotAccepted(t, result, err, "已登在往来单位 party-a 上")

	// 同一个码换一本主档就不是同一个键：供应商编码 C-0001 与客户编码 C-0001 不相撞。
	second.ExternalReferences = []domain.ExternalReference{externalReference(t, "SYN-ERP", "SUPPLIER", "C-0001")}
	result, err = handler.RegisterBusinessParty(ctx, second)
	expectOutcome(t, result, err, application.PartyIdentityRegistered)

	twice := partyCommand(t, "party-c", "单位丙", 1)
	twice.ExternalReferences = []domain.ExternalReference{
		externalReference(t, "SYN-ERP", "CUSTOMER", "C-0003"),
		externalReference(t, "SYN-ERP", "CUSTOMER", "C-0004"),
	}
	result, err = handler.RegisterBusinessParty(ctx, twice)
	expectNotAccepted(t, result, err, "同一系统、同一用途只收一个编码")

	recoded := partyCommand(t, "party-a", "单位甲", 2)
	recoded.ExternalReferences = []domain.ExternalReference{externalReference(t, "SYN-ERP", "CUSTOMER", "C-1001")}
	result, err = handler.RegisterBusinessParty(ctx, recoded)
	expectOutcome(t, result, err, application.PartyIdentityRegistered)

	// 修订 1 的重放照册面比对：C-0001 如今已不在 party-a 的最新修订上，重放仍答重复，不被今天的门拦。
	result, err = handler.RegisterBusinessParty(ctx, first)
	expectOutcome(t, result, err, application.PartyIdentityAlreadyRegistered)
}
