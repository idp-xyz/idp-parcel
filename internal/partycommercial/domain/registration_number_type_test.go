package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件证注册号类型目录的领域面（ADR-0145 决定一）：国家 / 地区代码只收一种形状、格式按整串
// 匹配、修订与停用的门，以及给身份登记与法人资料用的校验答案代数。国家 / 地区一律取 ISO 3166
// 的用户自定义码（XA、XB），类型与格式一律合成——目录不带任何真实国家 / 地区的格式。

var registrationTypeEffectiveFrom = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func registrationTypeFixture(
	t *testing.T,
	tenant, country, code string,
	layer domain.RegistrationNumberLayer,
	pattern string,
	effectiveFrom time.Time,
) domain.RegistrationNumberTypeRegistration {
	t.Helper()
	countryCode, err := domain.NewRegistrationCountryCode(country)
	if err != nil {
		t.Fatalf("NewRegistrationCountryCode(%q)：%v", country, err)
	}
	format, err := domain.NewRegistrationNumberFormat(pattern)
	if err != nil {
		t.Fatalf("NewRegistrationNumberFormat(%q)：%v", pattern, err)
	}
	lifecycle, err := domain.NewRegistrationNumberTypeLifecycle(effectiveFrom)
	if err != nil {
		t.Fatalf("NewRegistrationNumberTypeLifecycle：%v", err)
	}
	registration, err := domain.NewRegistrationNumberTypeRegistration(
		commercialValue(t, domain.NewTenantID, tenant),
		countryCode,
		commercialValue(t, domain.NewRegistrationNumberTypeCode, code),
		1,
		domain.RegistrationNumberTypeSpec{
			Name:   commercialValue(t, domain.NewRegistrationNumberTypeName, "合成类型 "+code),
			Layer:  layer,
			Format: format,
			Basis:  commercialValue(t, domain.NewRegistrationNumberTypeBasisReference, "SYN-BASIS-"+code),
		},
		lifecycle,
	)
	if err != nil {
		t.Fatalf("NewRegistrationNumberTypeRegistration：%v", err)
	}
	return registration
}

func registrationCountry(t *testing.T, value string) domain.RegistrationCountryCode {
	t.Helper()
	code, err := domain.NewRegistrationCountryCode(value)
	if err != nil {
		t.Fatalf("NewRegistrationCountryCode(%q)：%v", value, err)
	}
	return code
}

// Covers: 注册国家 / 地区只收两位大写拉丁字母；小写、三位码、夹数字或空白一律拒——同一个
// 国家 / 地区在目录里只能有一个键。
func TestRegistrationCountryCodeTakesOnlyTheTwoUpperLetterShape(t *testing.T) {
	for _, accepted := range []string{"XA", "XB", "ZZ"} {
		if _, err := domain.NewRegistrationCountryCode(accepted); err != nil {
			t.Fatalf("NewRegistrationCountryCode(%q) = %v, want accepted", accepted, err)
		}
	}
	for _, refused := range []string{"", "xa", "XAA", "X1", " XA", "XA "} {
		if _, err := domain.NewRegistrationCountryCode(refused); !errors.Is(err, domain.ErrInvalidRegistrationCountry) {
			t.Fatalf("NewRegistrationCountryCode(%q) = %v, want ErrInvalidRegistrationCountry", refused, err)
		}
	}
}

// Covers: 格式按整串匹配——漏写锚点的正文不能让「含一段像号的子串」过门；带选择分支的正文整体
// 被包住，不会一支锚头、一支锚尾；不配平的正文单独编不过即拒，不靠包裹「修好」它。
func TestRegistrationNumberFormatMatchesTheWholeNumber(t *testing.T) {
	tenant := "SYN-TENANT-01"
	digits := registrationTypeFixture(t, tenant, "XA", "SYN-DIGITS", domain.RegistrationNumberIdentityLayer,
		`[0-9]{3}`, registrationTypeEffectiveFrom)
	branches := registrationTypeFixture(t, tenant, "XA", "SYN-BRANCH", domain.RegistrationNumberIdentityLayer,
		`A[0-9]|B[0-9]{2}`, registrationTypeEffectiveFrom)
	catalogue, err := domain.NewRegistrationNumberTypeCatalogue(
		commercialValue(t, domain.NewTenantID, tenant), registrationCountry(t, "XA"),
		[]domain.RegistrationNumberTypeRegistration{digits, branches},
	)
	if err != nil {
		t.Fatalf("NewRegistrationNumberTypeCatalogue：%v", err)
	}
	at := registrationTypeEffectiveFrom.Add(time.Hour)
	cases := []struct {
		code   string
		number string
		want   domain.RegistrationNumberCheckOutcome
	}{
		{code: "SYN-DIGITS", number: "123", want: domain.RegistrationNumberAccepted},
		{code: "SYN-DIGITS", number: "1234", want: domain.RegistrationNumberFormatMismatch},
		{code: "SYN-DIGITS", number: "a123", want: domain.RegistrationNumberFormatMismatch},
		{code: "SYN-BRANCH", number: "A1", want: domain.RegistrationNumberAccepted},
		{code: "SYN-BRANCH", number: "B12", want: domain.RegistrationNumberAccepted},
		{code: "SYN-BRANCH", number: "A1B12", want: domain.RegistrationNumberFormatMismatch},
		{code: "SYN-BRANCH", number: "xB12", want: domain.RegistrationNumberFormatMismatch},
	}
	for _, tc := range cases {
		check, err := catalogue.Check(
			commercialValue(t, domain.NewRegistrationNumberTypeCode, tc.code),
			domain.RegistrationNumberIdentityLayer,
			commercialValue(t, domain.NewRegistrationNumber, tc.number),
			at,
		)
		if err != nil {
			t.Fatalf("Check(%s, %q)：%v", tc.code, tc.number, err)
		}
		if check.Outcome() != tc.want {
			t.Fatalf("Check(%s, %q) = %s, want %s", tc.code, tc.number, check.Outcome(), tc.want)
		}
	}

	if got := digits.Format().Pattern(); got != `[0-9]{3}` {
		t.Fatalf("Pattern() = %q, want 登记原文 `[0-9]{3}`", got)
	}
	for _, refused := range []string{"", "   ", `a)|(b`, `[`} {
		if _, err := domain.NewRegistrationNumberFormat(refused); !errors.Is(err, domain.ErrInvalidRegistrationNumberFormat) {
			t.Fatalf("NewRegistrationNumberFormat(%q) = %v, want ErrInvalidRegistrationNumberFormat", refused, err)
		}
	}
}

// Covers: 层的反查与 String() 是同一份名单；空串与集外词答不认识，落空交零值。
func TestRegistrationNumberLayerNamedRoundTripsAndRefusesOutsiders(t *testing.T) {
	for _, layer := range []domain.RegistrationNumberLayer{
		domain.RegistrationNumberIdentityLayer, domain.RegistrationNumberProfileLayer,
	} {
		if got, known := domain.RegistrationNumberLayerNamed(layer.String()); !known || got != layer {
			t.Fatalf("RegistrationNumberLayerNamed(%q) = (%d, %v), want (%d, true)", layer.String(), got, known, layer)
		}
	}
	for _, outsider := range []string{"", "TAX", "identity", "LIFETIME"} {
		if got, known := domain.RegistrationNumberLayerNamed(outsider); known || got != domain.RegistrationNumberLayerInvalid {
			t.Fatalf("RegistrationNumberLayerNamed(%q) = (%d, %v), want (0, false)", outsider, got, known)
		}
	}
}

// Covers: 登记信封缺任何一格都立不住——名称、层、格式、依据、生效时点、修订号，一格都不代填。
func TestRegistrationNumberTypeRegistrationRefusesAnIncompleteEnvelope(t *testing.T) {
	complete := registrationTypeFixture(t, "SYN-TENANT-01", "XA", "SYN-LIFETIME",
		domain.RegistrationNumberIdentityLayer, `[0-9]{6}`, registrationTypeEffectiveFrom)
	spec := domain.RegistrationNumberTypeSpec{
		Name:   complete.Name(),
		Layer:  complete.Layer(),
		Format: complete.Format(),
		Basis:  complete.Basis(),
	}
	withoutName, withoutLayer, withoutFormat, withoutBasis := spec, spec, spec, spec
	withoutName.Name = domain.RegistrationNumberTypeName{}
	withoutLayer.Layer = domain.RegistrationNumberLayerInvalid
	withoutFormat.Format = domain.RegistrationNumberFormat{}
	withoutBasis.Basis = domain.RegistrationNumberTypeBasisReference{}

	cases := map[string]struct {
		spec      domain.RegistrationNumberTypeSpec
		revision  int
		lifecycle domain.RegistrationNumberTypeLifecycle
	}{
		"缺名称":  {spec: withoutName, revision: 1, lifecycle: complete.Lifecycle()},
		"缺层":   {spec: withoutLayer, revision: 1, lifecycle: complete.Lifecycle()},
		"缺格式":  {spec: withoutFormat, revision: 1, lifecycle: complete.Lifecycle()},
		"缺依据":  {spec: withoutBasis, revision: 1, lifecycle: complete.Lifecycle()},
		"修订为零": {spec: spec, revision: 0, lifecycle: complete.Lifecycle()},
		"缺生效":  {spec: spec, revision: 1, lifecycle: domain.RegistrationNumberTypeLifecycle{}},
	}
	for name, tc := range cases {
		_, err := domain.NewRegistrationNumberTypeRegistration(
			complete.Tenant(), complete.Country(), complete.Code(), tc.revision, tc.spec, tc.lifecycle,
		)
		if !errors.Is(err, domain.ErrInvalidRegistrationNumberType) {
			t.Fatalf("%s：err = %v, want ErrInvalidRegistrationNumberType", name, err)
		}
	}
	if _, err := domain.NewRegistrationNumberTypeLifecycle(time.Time{}); !errors.Is(err, domain.ErrInvalidRegistrationNumberType) {
		t.Fatalf("零生效时点：err = %v, want ErrInvalidRegistrationNumberType", err)
	}
}

// Covers: 停用形成下一笔修订，状态自停用时点起才转已停用；二次停用、无依据停用、零时点停用拒绝。
func TestRegistrationNumberTypeDeactivationFormsTheNextRevision(t *testing.T) {
	registration := registrationTypeFixture(t, "SYN-TENANT-01", "XA", "SYN-LIFETIME",
		domain.RegistrationNumberIdentityLayer, `[0-9]{6}`, registrationTypeEffectiveFrom)
	deactivatedAt := registrationTypeEffectiveFrom.Add(30 * 24 * time.Hour)
	basis := commercialValue(t, domain.NewRegistrationNumberTypeBasisReference, "SYN-BASIS-RETIRE")

	next, err := registration.Deactivate(basis, deactivatedAt)
	if err != nil {
		t.Fatalf("Deactivate：%v", err)
	}
	if next.Revision() != 2 {
		t.Fatalf("停用修订 = %d, want 2", next.Revision())
	}
	if got := next.Lifecycle().StatusAt(deactivatedAt.Add(-time.Second)); got != domain.RegistrationNumberTypeEffective {
		t.Fatalf("停用前一刻状态 = %s, want EFFECTIVE", got)
	}
	if got := next.Lifecycle().StatusAt(deactivatedAt); got != domain.RegistrationNumberTypeDeactivated {
		t.Fatalf("停用时点状态 = %s, want DEACTIVATED", got)
	}
	if got := registration.Lifecycle().StatusAt(registrationTypeEffectiveFrom.Add(-time.Second)); got != domain.RegistrationNumberTypeRegistered {
		t.Fatalf("生效前状态 = %s, want REGISTERED", got)
	}
	if gotBasis, gotAt, has := next.Lifecycle().Deactivation(); !has || gotBasis != basis || !gotAt.Equal(deactivatedAt) {
		t.Fatalf("Deactivation() = (%s, %v, %v), want (%s, %v, true)", gotBasis, gotAt, has, basis, deactivatedAt)
	}

	if _, err := next.Deactivate(basis, deactivatedAt.Add(time.Hour)); !errors.Is(err, domain.ErrInvalidRegistrationNumberTypeTransition) {
		t.Fatalf("二次停用：err = %v, want ErrInvalidRegistrationNumberTypeTransition", err)
	}
	if _, err := registration.Deactivate(domain.RegistrationNumberTypeBasisReference{}, deactivatedAt); !errors.Is(err, domain.ErrInvalidRegistrationNumberTypeTransition) {
		t.Fatalf("无依据停用：err = %v, want ErrInvalidRegistrationNumberTypeTransition", err)
	}
	if _, err := registration.Deactivate(basis, time.Time{}); !errors.Is(err, domain.ErrInvalidRegistrationNumberTypeTransition) {
		t.Fatalf("零时点停用：err = %v, want ErrInvalidRegistrationNumberTypeTransition", err)
	}
}

// Covers: 目录只装同一租户、同一国家 / 地区的条目，且每个类型代码只一笔——同代码两笔说明读侧没有
// 只取最新修订，收下它会让校验随取数次序变答案。
func TestRegistrationNumberTypeCatalogueRefusesMixedOrDuplicatedEntries(t *testing.T) {
	tenant := commercialValue(t, domain.NewTenantID, "SYN-TENANT-01")
	country := registrationCountry(t, "XA")
	own := registrationTypeFixture(t, "SYN-TENANT-01", "XA", "SYN-LIFETIME",
		domain.RegistrationNumberIdentityLayer, `[0-9]{6}`, registrationTypeEffectiveFrom)
	otherTenant := registrationTypeFixture(t, "SYN-TENANT-02", "XA", "SYN-OTHER",
		domain.RegistrationNumberIdentityLayer, `[0-9]{6}`, registrationTypeEffectiveFrom)
	otherCountry := registrationTypeFixture(t, "SYN-TENANT-01", "XB", "SYN-OTHER",
		domain.RegistrationNumberIdentityLayer, `[0-9]{6}`, registrationTypeEffectiveFrom)

	cases := map[string][]domain.RegistrationNumberTypeRegistration{
		"他租户":   {own, otherTenant},
		"他国家":   {own, otherCountry},
		"同代码两笔": {own, own},
	}
	for name, entries := range cases {
		if _, err := domain.NewRegistrationNumberTypeCatalogue(tenant, country, entries); !errors.Is(err, domain.ErrInvalidRegistrationNumberTypeCatalogue) {
			t.Fatalf("%s：err = %v, want ErrInvalidRegistrationNumberTypeCatalogue", name, err)
		}
	}
	if _, err := domain.NewRegistrationNumberTypeCatalogue(tenant, country, nil); err != nil {
		t.Fatalf("空目录是「未登记」的如实形状，不该拒：%v", err)
	}
}

// Covers: 校验答案代数逐格——国家 / 地区未登记、类型未登记、层不符、未生效、已停用、格式不符、
// 合格；层不符排在生效期之前；有可对照类型的格交回那笔类型修订。
func TestRegistrationNumberCheckAnswersEachGrade(t *testing.T) {
	tenantValue := "SYN-TENANT-01"
	tenant := commercialValue(t, domain.NewTenantID, tenantValue)
	identity := registrationTypeFixture(t, tenantValue, "XA", "SYN-LIFETIME",
		domain.RegistrationNumberIdentityLayer, `SYN-[0-9]{6}`, registrationTypeEffectiveFrom)
	profile := registrationTypeFixture(t, tenantValue, "XA", "SYN-TAX",
		domain.RegistrationNumberProfileLayer, `SYN-TAX-[0-9]{4}`, registrationTypeEffectiveFrom)
	future := registrationTypeFixture(t, tenantValue, "XA", "SYN-FUTURE",
		domain.RegistrationNumberIdentityLayer, `SYN-[0-9]{6}`, registrationTypeEffectiveFrom.AddDate(1, 0, 0))
	retiredAt := registrationTypeEffectiveFrom.AddDate(0, 1, 0)
	retired, err := registrationTypeFixture(t, tenantValue, "XA", "SYN-RETIRED",
		domain.RegistrationNumberIdentityLayer, `SYN-[0-9]{6}`, registrationTypeEffectiveFrom).
		Deactivate(commercialValue(t, domain.NewRegistrationNumberTypeBasisReference, "SYN-BASIS-RETIRE"), retiredAt)
	if err != nil {
		t.Fatalf("Deactivate：%v", err)
	}
	catalogue, err := domain.NewRegistrationNumberTypeCatalogue(tenant, registrationCountry(t, "XA"),
		[]domain.RegistrationNumberTypeRegistration{identity, profile, future, retired})
	if err != nil {
		t.Fatalf("NewRegistrationNumberTypeCatalogue：%v", err)
	}
	unregistered, err := domain.NewRegistrationNumberTypeCatalogue(tenant, registrationCountry(t, "XB"), nil)
	if err != nil {
		t.Fatalf("NewRegistrationNumberTypeCatalogue（空）：%v", err)
	}

	at := retiredAt.AddDate(0, 1, 0)
	cases := []struct {
		name         string
		catalogue    domain.RegistrationNumberTypeCatalogue
		code         string
		layer        domain.RegistrationNumberLayer
		number       string
		want         domain.RegistrationNumberCheckOutcome
		wantRevision int
	}{
		{name: "国家未登记", catalogue: unregistered, code: "SYN-LIFETIME",
			layer: domain.RegistrationNumberIdentityLayer, number: "SYN-000001",
			want: domain.RegistrationCountryNotRegistered},
		{name: "类型未登记", catalogue: catalogue, code: "SYN-GHOST",
			layer: domain.RegistrationNumberIdentityLayer, number: "SYN-000001",
			want: domain.RegistrationNumberTypeNotRegistered},
		{name: "资料层号填进身份层", catalogue: catalogue, code: "SYN-TAX",
			layer: domain.RegistrationNumberIdentityLayer, number: "SYN-TAX-0001",
			want: domain.RegistrationNumberLayerMismatch, wantRevision: 1},
		{name: "层不符先于已停用", catalogue: catalogue, code: "SYN-RETIRED",
			layer: domain.RegistrationNumberProfileLayer, number: "SYN-000001",
			want: domain.RegistrationNumberLayerMismatch, wantRevision: 2},
		{name: "未到生效", catalogue: catalogue, code: "SYN-FUTURE",
			layer: domain.RegistrationNumberIdentityLayer, number: "SYN-000001",
			want: domain.RegistrationNumberTypeNotEffective, wantRevision: 1},
		{name: "已停用", catalogue: catalogue, code: "SYN-RETIRED",
			layer: domain.RegistrationNumberIdentityLayer, number: "SYN-000001",
			want: domain.RegistrationNumberTypeNotEffective, wantRevision: 2},
		{name: "格式不符", catalogue: catalogue, code: "SYN-LIFETIME",
			layer: domain.RegistrationNumberIdentityLayer, number: "SYN-00001",
			want: domain.RegistrationNumberFormatMismatch, wantRevision: 1},
		{name: "合格", catalogue: catalogue, code: "SYN-LIFETIME",
			layer: domain.RegistrationNumberIdentityLayer, number: "SYN-000001",
			want: domain.RegistrationNumberAccepted, wantRevision: 1},
		{name: "资料层合格", catalogue: catalogue, code: "SYN-TAX",
			layer: domain.RegistrationNumberProfileLayer, number: "SYN-TAX-0001",
			want: domain.RegistrationNumberAccepted, wantRevision: 1},
	}
	for _, tc := range cases {
		check, err := tc.catalogue.Check(
			commercialValue(t, domain.NewRegistrationNumberTypeCode, tc.code),
			tc.layer,
			commercialValue(t, domain.NewRegistrationNumber, tc.number),
			at,
		)
		if err != nil {
			t.Fatalf("%s：Check：%v", tc.name, err)
		}
		if check.Outcome() != tc.want {
			t.Fatalf("%s：Outcome = %s, want %s", tc.name, check.Outcome(), tc.want)
		}
		numberType, hasType := check.Type()
		if tc.wantRevision == 0 {
			if hasType {
				t.Fatalf("%s：没有可对照类型的格交回了类型 %s", tc.name, numberType.Code())
			}
			continue
		}
		if !hasType || numberType.Code().String() != tc.code || numberType.Revision() != tc.wantRevision {
			t.Fatalf("%s：Type() = (%s r%d, %v), want (%s r%d, true)",
				tc.name, numberType.Code(), numberType.Revision(), hasType, tc.code, tc.wantRevision)
		}
	}
}

// Covers: 问题本身不完整（缺代码、缺层、缺号、零时点、零值目录）不是一种答案，交回错误。
func TestRegistrationNumberCheckRefusesAnIncompleteQuestion(t *testing.T) {
	tenant := commercialValue(t, domain.NewTenantID, "SYN-TENANT-01")
	catalogue, err := domain.NewRegistrationNumberTypeCatalogue(tenant, registrationCountry(t, "XA"), nil)
	if err != nil {
		t.Fatalf("NewRegistrationNumberTypeCatalogue：%v", err)
	}
	code := commercialValue(t, domain.NewRegistrationNumberTypeCode, "SYN-LIFETIME")
	number := commercialValue(t, domain.NewRegistrationNumber, "SYN-000001")
	at := registrationTypeEffectiveFrom

	checks := map[string]func() error{
		"缺代码": func() error {
			_, err := catalogue.Check(domain.RegistrationNumberTypeCode{}, domain.RegistrationNumberIdentityLayer, number, at)
			return err
		},
		"缺层": func() error {
			_, err := catalogue.Check(code, domain.RegistrationNumberLayerInvalid, number, at)
			return err
		},
		"缺号": func() error {
			_, err := catalogue.Check(code, domain.RegistrationNumberIdentityLayer, domain.RegistrationNumber{}, at)
			return err
		},
		"零时点": func() error {
			_, err := catalogue.Check(code, domain.RegistrationNumberIdentityLayer, number, time.Time{})
			return err
		},
		"零值目录": func() error {
			_, err := domain.RegistrationNumberTypeCatalogue{}.Check(code, domain.RegistrationNumberIdentityLayer, number, at)
			return err
		},
	}
	for name, check := range checks {
		if err := check(); !errors.Is(err, domain.ErrInvalidRegistrationNumberCheck) {
			t.Fatalf("%s：err = %v, want ErrInvalidRegistrationNumberCheck", name, err)
		}
	}
}
