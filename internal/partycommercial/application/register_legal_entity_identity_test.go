package application_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// fakeNumberTypes 是 ports.RegistrationNumberTypeLookup 的内存替身：按（租户，国家 / 地区）交回整份目录，
// 没登记的国家 / 地区交回空目录——与真读口「一笔都没有即空目录」同形。
type fakeNumberTypes struct {
	byCountry map[string][]domain.RegistrationNumberTypeRegistration
}

var _ ports.RegistrationNumberTypeLookup = (*fakeNumberTypes)(nil)

func (lookup *fakeNumberTypes) LoadRegistrationNumberTypeCatalogue(
	_ context.Context,
	tenant domain.TenantID,
	country domain.RegistrationCountryCode,
) (domain.RegistrationNumberTypeCatalogue, error) {
	return domain.NewRegistrationNumberTypeCatalogue(tenant, country, lookup.byCountry[country.String()])
}

var numberTypeEffectiveFrom = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func numberType(
	t *testing.T,
	country, code string,
	layer domain.RegistrationNumberLayer,
	pattern string,
) domain.RegistrationNumberTypeRegistration {
	t.Helper()
	format, err := domain.NewRegistrationNumberFormat(pattern)
	if err != nil {
		t.Fatalf("new format %q: %v", pattern, err)
	}
	lifecycle, err := domain.NewRegistrationNumberTypeLifecycle(numberTypeEffectiveFrom)
	if err != nil {
		t.Fatalf("new lifecycle: %v", err)
	}
	registration, err := domain.NewRegistrationNumberTypeRegistration(
		identityValue(t, domain.NewTenantID, "tenant-1"),
		identityValue(t, domain.NewRegistrationCountryCode, country),
		identityValue(t, domain.NewRegistrationNumberTypeCode, code),
		1,
		domain.RegistrationNumberTypeSpec{
			Name:   identityValue(t, domain.NewRegistrationNumberTypeName, "合成类型 "+code),
			Layer:  layer,
			Format: format,
			Basis:  identityValue(t, domain.NewRegistrationNumberTypeBasisReference, "SYN-BASIS-"+code),
		},
		lifecycle,
	)
	if err != nil {
		t.Fatalf("new registration number type: %v", err)
	}
	return registration
}

// newFakeNumberTypes 登 XA 一国两类：身份层终身注册号与资料层税务登记号；XB 故意不登。国家 / 地区
// 取 ISO 3166 用户自定义码，类型与格式全为合成值。
func newFakeNumberTypes(t *testing.T) *fakeNumberTypes {
	t.Helper()
	return &fakeNumberTypes{byCountry: map[string][]domain.RegistrationNumberTypeRegistration{
		"XA": {
			numberType(t, "XA", "SYN-XA-LIFETIME", domain.RegistrationNumberIdentityLayer, `SYN-XA-[0-9]{6}`),
			numberType(t, "XA", "SYN-XA-TAX", domain.RegistrationNumberProfileLayer, `SYN-XA-TAX-[0-9]{4}`),
		},
	}}
}

// withLegalEntityIdentity 给命令补上身份层：一个国家 / 地区、一个终身注册号。
func withLegalEntityIdentity(
	t *testing.T,
	command application.RegisterLegalEntityCommand,
	country, code, number string,
) application.RegisterLegalEntityCommand {
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

func legalEntityCommand(t *testing.T, revision int) application.RegisterLegalEntityCommand {
	t.Helper()
	return application.RegisterLegalEntityCommand{
		Tenant:        identityValue(t, domain.NewTenantID, "tenant-1"),
		Entity:        identityValue(t, domain.NewLegalEntityReference, "le-1"),
		Party:         identityValue(t, domain.NewPartyID, "party-le"),
		Revision:      revision,
		Basis:         identityValue(t, domain.NewIdentityBasisReference, "basis-le"),
		EffectiveFrom: entityEffectiveFrom,
	}
}

func expectNotAccepted(t *testing.T, result application.PartyRegistryResult, err error, mentions string) {
	t.Helper()
	if err != nil || result.Outcome() != application.PartyIdentityNotAccepted {
		t.Fatalf("result = (%v, %v), want NOT_ACCEPTED", result.Outcome(), err)
	}
	if result.Cause() == nil || !strings.Contains(result.Cause().Error(), mentions) {
		t.Fatalf("cause = %v, want it to mention %q", result.Cause(), mentions)
	}
}

// Covers: 票 legal-entity-profile/02「新登记缺一拒登，号经 01 的校验」——新登记缺国家 / 地区、缺号、国家 / 地区
// 未登记、类型未登记、号属资料层、格式不符各拒一条（理由各自说清续办），合格即落册、同内容重放答重复。
func TestLegalEntityIdentityLayerGates(t *testing.T) {
	registry := newFakePartyRegistry()
	handler := application.NewRegisterPartyIdentityHandler(registry, newFakeNumberTypes(t))
	ctx := context.Background()
	registerParty(t, handler, "tenant-1", "party-le", "运营法人参与方")

	bare := legalEntityCommand(t, 1)
	result, err := handler.RegisterLegalEntity(ctx, bare)
	expectNotAccepted(t, result, err, "必须带注册国家 / 地区")

	onlyNumbers := withLegalEntityIdentity(t, bare, "XA", "SYN-XA-LIFETIME", "SYN-XA-000001")
	onlyNumbers.RegistrationCountry = nil
	result, err = handler.RegisterLegalEntity(ctx, onlyNumbers)
	expectNotAccepted(t, result, err, "缺注册国家 / 地区")

	onlyCountry := withLegalEntityIdentity(t, bare, "XA", "SYN-XA-LIFETIME", "SYN-XA-000001")
	onlyCountry.LifetimeNumbers = nil
	result, err = handler.RegisterLegalEntity(ctx, onlyCountry)
	expectNotAccepted(t, result, err, "缺终身注册号")

	cases := []struct {
		name, country, code, number, mentions string
	}{
		{"国家未登记", "XB", "SYN-XB-LIFETIME", "SYN-XB-000001", "未登记：先在目录登记"},
		{"类型未登记", "XA", "SYN-XA-OTHER", "SYN-XA-000001", "SYN-XA-OTHER"},
		{"号属资料层", "XA", "SYN-XA-TAX", "SYN-XA-TAX-0001", "属资料层"},
		{"格式不符", "XA", "SYN-XA-LIFETIME", "SYN-XA-12", "不合类型"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			result, err := handler.RegisterLegalEntity(ctx,
				withLegalEntityIdentity(t, bare, testCase.country, testCase.code, testCase.number))
			expectNotAccepted(t, result, err, testCase.mentions)
		})
	}
	if _, found, _ := registry.LoadLatestLegalEntity(ctx, bare.Tenant, bare.Entity); found {
		t.Fatal("refused registrations must not write a byte")
	}

	good := withLegalEntityIdentity(t, bare, "XA", "SYN-XA-LIFETIME", "SYN-XA-000001")
	result, err = handler.RegisterLegalEntity(ctx, good)
	if err != nil || result.Outcome() != application.PartyIdentityRegistered {
		t.Fatalf("registration = (%v, %v), want REGISTERED", result.Outcome(), err)
	}
	result, err = handler.RegisterLegalEntity(ctx, good)
	if err != nil || result.Outcome() != application.PartyIdentityAlreadyRegistered {
		t.Fatalf("replay = (%v, %v), want ALREADY_REGISTERED", result.Outcome(), err)
	}
	stored, _, _ := registry.LoadLatestLegalEntity(ctx, bare.Tenant, bare.Entity)
	if layer, has := stored.IdentityLayer(); !has || layer.Country().String() != "XA" {
		t.Fatalf("stored identity layer = %+v (has %v), want country XA", layer, has)
	}
}

// Covers: 票 legal-entity-profile/02「不作变更，录错走更正」——修订改了号必须带身份更正依据，不带即拒；带了即落为
// 更正修订；没改却带更正依据同样拒；首笔登记之前没有可更正的号，带了即拒、一个字节不写，修订 1 在册后带着重放也拒。
func TestLegalEntityIdentityCorrectionNeedsABasis(t *testing.T) {
	registry := newFakePartyRegistry()
	handler := application.NewRegisterPartyIdentityHandler(registry, newFakeNumberTypes(t))
	ctx := context.Background()
	registerParty(t, handler, "tenant-1", "party-le", "运营法人参与方")

	correction := identityValue(t, domain.NewIdentityBasisReference, "SYN-CORRECTION-01")
	first := withLegalEntityIdentity(t, legalEntityCommand(t, 1), "XA", "SYN-XA-LIFETIME", "SYN-XA-000001")
	firstWithCorrection := first
	firstWithCorrection.IdentityCorrectionBasis = &correction
	result, err := handler.RegisterLegalEntity(ctx, firstWithCorrection)
	expectNotAccepted(t, result, err, "首笔登记")
	if _, found, _ := registry.LoadLatestLegalEntity(ctx, first.Tenant, first.Entity); found {
		t.Fatal("a refused first registration must not write a byte")
	}

	if result, err := handler.RegisterLegalEntity(ctx, first); err != nil || result.Outcome() != application.PartyIdentityRegistered {
		t.Fatalf("first = (%v, %v)", result.Outcome(), err)
	}
	result, err = handler.RegisterLegalEntity(ctx, firstWithCorrection)
	expectNotAccepted(t, result, err, "首笔登记")

	changed := withLegalEntityIdentity(t, legalEntityCommand(t, 2), "XA", "SYN-XA-LIFETIME", "SYN-XA-000009")
	result, err = handler.RegisterLegalEntity(ctx, changed)
	expectNotAccepted(t, result, err, "不作变更")

	unchanged := withLegalEntityIdentity(t, legalEntityCommand(t, 2), "XA", "SYN-XA-LIFETIME", "SYN-XA-000001")
	unchanged.IdentityCorrectionBasis = &correction
	result, err = handler.RegisterLegalEntity(ctx, unchanged)
	expectNotAccepted(t, result, err, "身份更正依据只随改了身份层的修订出现")

	changed.IdentityCorrectionBasis = &correction
	result, err = handler.RegisterLegalEntity(ctx, changed)
	if err != nil || result.Outcome() != application.PartyIdentityRegistered {
		t.Fatalf("correction = (%v, %v), want REGISTERED", result.Outcome(), err)
	}
	stored, _, _ := registry.LoadLatestLegalEntity(ctx, first.Tenant, first.Entity)
	if basis, has := stored.IdentityCorrectionBasis(); !has || basis.String() != "SYN-CORRECTION-01" {
		t.Fatalf("stored correction basis = %q (has %v)", basis, has)
	}
}

// Covers: 票 legal-entity-profile/02「既有修订不改写」——本格落地之前的历史修订没有身份层：原样重放答重复；
// 下一笔修订仍须带两格；第一次补登不带更正依据即落册。
func TestLegacyLegalEntityRevisionReplaysAndBackfills(t *testing.T) {
	registry := newFakePartyRegistry()
	handler := application.NewRegisterPartyIdentityHandler(registry, newFakeNumberTypes(t))
	ctx := context.Background()
	registerParty(t, handler, "tenant-1", "party-le", "运营法人参与方")

	legacy := legalEntityCommand(t, 1)
	entity, err := domain.RehydrateResponsibleLegalEntity(legacy.Tenant, legacy.Entity, legacy.Party)
	if err != nil {
		t.Fatalf("rehydrate entity: %v", err)
	}
	lifecycle, err := domain.NewIdentityLifecycle(legacy.EffectiveFrom)
	if err != nil {
		t.Fatalf("lifecycle: %v", err)
	}
	historical, err := domain.NewLegalEntityRegistration(entity, 1, legacy.Basis, lifecycle)
	if err != nil {
		t.Fatalf("historical registration: %v", err)
	}
	if _, err := registry.SaveLegalEntity(ctx, historical); err != nil {
		t.Fatalf("seed historical revision: %v", err)
	}

	result, err := handler.RegisterLegalEntity(ctx, legacy)
	if err != nil || result.Outcome() != application.PartyIdentityAlreadyRegistered {
		t.Fatalf("legacy replay = (%v, %v), want ALREADY_REGISTERED", result.Outcome(), err)
	}

	result, err = handler.RegisterLegalEntity(ctx, legalEntityCommand(t, 2))
	expectNotAccepted(t, result, err, "必须带注册国家 / 地区")

	backfill := withLegalEntityIdentity(t, legalEntityCommand(t, 2), "XA", "SYN-XA-LIFETIME", "SYN-XA-000001")
	result, err = handler.RegisterLegalEntity(ctx, backfill)
	if err != nil || result.Outcome() != application.PartyIdentityRegistered {
		t.Fatalf("backfill = (%v, %v), want REGISTERED", result.Outcome(), err)
	}
}

// Covers: 没装配注册号类型目录却去登法人，是装配缺陷：答技术失败，不替它放行、也不答成`未受理`。
func TestLegalEntityRegistrationWithoutNumberTypesFailsLoudly(t *testing.T) {
	registry := newFakePartyRegistry()
	handler := application.NewRegisterPartyIdentityHandler(registry, nil)
	ctx := context.Background()
	registerParty(t, handler, "tenant-1", "party-le", "运营法人参与方")

	_, err := handler.RegisterLegalEntity(ctx,
		withLegalEntityIdentity(t, legalEntityCommand(t, 1), "XA", "SYN-XA-LIFETIME", "SYN-XA-000001"))
	if err == nil || !strings.Contains(err.Error(), "未装配") {
		t.Fatalf("error = %v, want a technical failure naming the missing catalogue", err)
	}
}
