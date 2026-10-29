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

// newFakeNumberTypes 登 CN 一国两类：身份层终身注册号与资料层税务登记号；SG 故意不登。国家 / 地区
// 取目录里的真码，类型与格式全为合成值。
func newFakeNumberTypes(t *testing.T) *fakeNumberTypes {
	t.Helper()
	return &fakeNumberTypes{byCountry: map[string][]domain.RegistrationNumberTypeRegistration{
		"CN": {
			numberType(t, "CN", "SYN-XA-LIFETIME", domain.RegistrationNumberIdentityLayer, `SYN-XA-[0-9]{6}`),
			numberType(t, "CN", "SYN-XA-TAX", domain.RegistrationNumberProfileLayer, `SYN-XA-TAX-[0-9]{4}`),
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

	onlyNumbers := withLegalEntityIdentity(t, bare, "CN", "SYN-XA-LIFETIME", "SYN-XA-000001")
	onlyNumbers.RegistrationCountry = nil
	result, err = handler.RegisterLegalEntity(ctx, onlyNumbers)
	expectNotAccepted(t, result, err, "缺注册国家 / 地区")

	onlyCountry := withLegalEntityIdentity(t, bare, "CN", "SYN-XA-LIFETIME", "SYN-XA-000001")
	onlyCountry.LifetimeNumbers = nil
	result, err = handler.RegisterLegalEntity(ctx, onlyCountry)
	expectNotAccepted(t, result, err, "缺终身注册号")

	cases := []struct {
		name, country, code, number, mentions string
	}{
		{"国家未登记", "SG", "SYN-XB-LIFETIME", "SYN-XB-000001", "未登记：先在目录登记"},
		{"类型未登记", "CN", "SYN-XA-OTHER", "SYN-XA-000001", "SYN-XA-OTHER"},
		{"号属资料层", "CN", "SYN-XA-TAX", "SYN-XA-TAX-0001", "属资料层"},
		{"格式不符", "CN", "SYN-XA-LIFETIME", "SYN-XA-12", "不合类型"},
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

	good := withLegalEntityIdentity(t, bare, "CN", "SYN-XA-LIFETIME", "SYN-XA-000001")
	result, err = handler.RegisterLegalEntity(ctx, good)
	if err != nil || result.Outcome() != application.PartyIdentityRegistered {
		t.Fatalf("registration = (%v, %v), want REGISTERED", result.Outcome(), err)
	}
	result, err = handler.RegisterLegalEntity(ctx, good)
	if err != nil || result.Outcome() != application.PartyIdentityAlreadyRegistered {
		t.Fatalf("replay = (%v, %v), want ALREADY_REGISTERED", result.Outcome(), err)
	}
	stored, _, _ := registry.LoadLatestLegalEntity(ctx, bare.Tenant, bare.Entity)
	if layer, has := stored.IdentityLayer(); !has || layer.Country().String() != "CN" {
		t.Fatalf("stored identity layer = %+v (has %v), want country CN", layer, has)
	}
}

// Covers: `AT-PC-077`，ADR-0345 决定五——新登记法人的注册国家只收内置国家 / 地区目录里的码：形状合法（两位大写）而目录外的 ZZ，哪怕租户
// 已在注册号类型目录里为它登了类型，也在目录成员这道门上答未受理、一个字节不写；目录内的码照常落册。门只拦新登记与新修订，
// 已在册的目录外法人读回与重放不受牵连。
func TestLegalEntityRegistrationCountryMustBeInTheBuiltInCountryCatalogue(t *testing.T) {
	registry := newFakePartyRegistry()
	numberTypes := newFakeNumberTypes(t)
	numberTypes.byCountry["ZZ"] = []domain.RegistrationNumberTypeRegistration{
		numberType(t, "ZZ", "SYN-ZZ-LIFETIME", domain.RegistrationNumberIdentityLayer, `SYN-ZZ-[0-9]{6}`),
	}
	handler := application.NewRegisterPartyIdentityHandler(registry, numberTypes)
	ctx := context.Background()
	registerParty(t, handler, "tenant-1", "party-le", "运营法人参与方")

	outside := withLegalEntityIdentity(t, legalEntityCommand(t, 1), "ZZ", "SYN-ZZ-LIFETIME", "SYN-ZZ-000001")
	result, err := handler.RegisterLegalEntity(ctx, outside)
	expectNotAccepted(t, result, err, "不在产品内置的国家 / 地区目录里")
	if _, found, _ := registry.LoadLatestLegalEntity(ctx, outside.Tenant, outside.Entity); found {
		t.Fatal("目录外的注册国家不得落册一个字节")
	}

	inside := withLegalEntityIdentity(t, legalEntityCommand(t, 1), "CN", "SYN-XA-LIFETIME", "SYN-XA-000001")
	if result, err := handler.RegisterLegalEntity(ctx, inside); err != nil || result.Outcome() != application.PartyIdentityRegistered {
		t.Fatalf("目录内的注册国家应照常落册：(%v, %v)", result.Outcome(), err)
	}
}

// Covers: `AT-PC-077`，ADR-0345 决定五、六——收紧只在写入边界：收紧之前登进册的目录外法人，同内容重放照样答已登记（修订号落在已有修订上
// 的不经目录成员这道门），册面读回不动；对它登记新修订则要重新过这道门，恢复路径是带身份更正依据把国家更正成目录内的码。
func TestAnExistingLegalEntityOutsideTheCountryCatalogueStillReplays(t *testing.T) {
	registry := newFakePartyRegistry()
	numberTypes := newFakeNumberTypes(t)
	numberTypes.byCountry["ZZ"] = []domain.RegistrationNumberTypeRegistration{
		numberType(t, "ZZ", "SYN-ZZ-LIFETIME", domain.RegistrationNumberIdentityLayer, `SYN-ZZ-[0-9]{6}`),
	}
	handler := application.NewRegisterPartyIdentityHandler(registry, numberTypes)
	ctx := context.Background()
	registerParty(t, handler, "tenant-1", "party-le", "运营法人参与方")

	command := withLegalEntityIdentity(t, legalEntityCommand(t, 1), "ZZ", "SYN-ZZ-LIFETIME", "SYN-ZZ-000001")
	party, _, _ := registry.LoadLatestBusinessParty(ctx, command.Tenant, command.Party)
	entity, err := domain.NewResponsibleLegalEntity(command.Tenant, command.Entity, party.Party())
	if err != nil {
		t.Fatal(err)
	}
	lifecycle, err := domain.NewIdentityLifecycle(command.EffectiveFrom)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := domain.NewLegalEntityRegistration(entity, 1, command.Basis, lifecycle)
	if err != nil {
		t.Fatal(err)
	}
	layer, err := domain.NewLegalEntityIdentityLayer(*command.RegistrationCountry, command.LifetimeNumbers)
	if err != nil {
		t.Fatal(err)
	}
	if legacy, err = legacy.WithIdentityLayer(layer, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.SaveLegalEntity(ctx, legacy); err != nil {
		t.Fatal(err)
	}

	if result, err := handler.RegisterLegalEntity(ctx, command); err != nil || result.Outcome() != application.PartyIdentityAlreadyRegistered {
		t.Fatalf("收紧之前登进册的目录外法人，同内容重放 = (%v, %v)，want ALREADY_REGISTERED", result.Outcome(), err)
	}
	next := command
	next.Revision = 2
	result, err := handler.RegisterLegalEntity(ctx, next)
	expectNotAccepted(t, result, err, "不在产品内置的国家 / 地区目录里")
	// 拒绝文字要指出恢复路径，登记方不必去猜。
	expectNotAccepted(t, result, err, "身份更正依据")

	// 恢复路径：修订 2 把注册国家更正成目录内的码，带身份更正依据，答已登记。
	correction := identityValue(t, domain.NewIdentityBasisReference, "SYN-CORRECTION-COUNTRY")
	recovered := withLegalEntityIdentity(t, legalEntityCommand(t, 2), "CN", "SYN-XA-LIFETIME", "SYN-XA-000001")
	recovered.IdentityCorrectionBasis = &correction
	if result, err := handler.RegisterLegalEntity(ctx, recovered); err != nil || result.Outcome() != application.PartyIdentityRegistered {
		t.Fatalf("更正成目录内国家并带更正依据 = (%v, %v)，want REGISTERED", result.Outcome(), err)
	}
	stored, _, _ := registry.LoadLatestLegalEntity(ctx, command.Tenant, command.Entity)
	if layer, has := stored.IdentityLayer(); !has || layer.Country().String() != "CN" || stored.Revision() != 2 {
		t.Fatalf("更正后册上最新应是修订 2、国家 CN：r%d %+v", stored.Revision(), layer)
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
	first := withLegalEntityIdentity(t, legalEntityCommand(t, 1), "CN", "SYN-XA-LIFETIME", "SYN-XA-000001")
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

	changed := withLegalEntityIdentity(t, legalEntityCommand(t, 2), "CN", "SYN-XA-LIFETIME", "SYN-XA-000009")
	result, err = handler.RegisterLegalEntity(ctx, changed)
	expectNotAccepted(t, result, err, "不作变更")

	unchanged := withLegalEntityIdentity(t, legalEntityCommand(t, 2), "CN", "SYN-XA-LIFETIME", "SYN-XA-000001")
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

	backfill := withLegalEntityIdentity(t, legalEntityCommand(t, 2), "CN", "SYN-XA-LIFETIME", "SYN-XA-000001")
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
		withLegalEntityIdentity(t, legalEntityCommand(t, 1), "CN", "SYN-XA-LIFETIME", "SYN-XA-000001"))
	if err == nil || !strings.Contains(err.Error(), "未装配") {
		t.Fatalf("error = %v, want a technical failure naming the missing catalogue", err)
	}
}
