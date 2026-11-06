package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件对真实 PostgreSQL 证注册号类型目录按 ADR-0144 分页（票 catalogue-read-pagination-rollout/11）：翻页中途登记新行
// 不重不漏、状态 / 层 / 国家三维与 q 收窄的总数与本页同一组条件、两段行标识在倒序下也同向收尾、空册答空页。国家 / 地区取
// ISO 3166 用户自定义码，判据同 registration_number_type_registry_test.go 的文件头。

// namedRegistrationTypeFixture 同 registrationTypeFixture，另可给类型名称与生效时点：q 要命中名称，`已登记`那一格要一笔
// 生效时点在未来的条目。
func namedRegistrationTypeFixture(
	t *testing.T,
	country, code, name string,
	layer domain.RegistrationNumberLayer,
	effectiveFrom time.Time,
) domain.RegistrationNumberTypeRegistration {
	t.Helper()
	format, err := domain.NewRegistrationNumberFormat(`SYN-[0-9]{6}`)
	if err != nil {
		t.Fatalf("new format: %v", err)
	}
	lifecycle, err := domain.NewRegistrationNumberTypeLifecycle(effectiveFrom)
	if err != nil {
		t.Fatalf("new lifecycle: %v", err)
	}
	registration, err := domain.NewRegistrationNumberTypeRegistration(
		pcTenant(t, "tenant-1"),
		pcValue(t, domain.NewRegistrationCountryCode, country),
		pcValue(t, domain.NewRegistrationNumberTypeCode, code),
		1,
		domain.RegistrationNumberTypeSpec{
			Name:   pcValue(t, domain.NewRegistrationNumberTypeName, name),
			Layer:  layer,
			Format: format,
			Basis:  pcValue(t, domain.NewRegistrationNumberTypeBasisReference, "SYN-BASIS-"+code),
		},
		lifecycle,
	)
	if err != nil {
		t.Fatalf("new registration number type: %v", err)
	}
	return registration
}

// Covers: 完成判据「翻页期间插入新行不重不漏」与「空册答空页」。
func TestRegistrationNumberTypeCataloguePagesWithoutRepeatsOrGapsWhileRowsArrive(t *testing.T) {
	types, catalogue, transactor := newRegistrationNumberTypes(t)
	ctx := t.Context()
	list := func(tenant domain.TenantID) pageLister[ports.RegistrationNumberTypeRow] {
		return func(query cataloguepage.Query, limit int) (cataloguepage.Result[ports.RegistrationNumberTypeRow], error) {
			return catalogue.ListRegistrationNumberTypes(ctx, tenant, limit, query)
		}
	}

	assertPagesWithoutRepeatsOrGaps(t, ports.RegistrationNumberTypePages, list(pcTenant(t, "tenant-1")), func(code string) {
		registration := registrationTypeFixture(t, "tenant-1", "XA", code, 1, domain.RegistrationNumberIdentityLayer, `SYN-[0-9]{6}`)
		if outcome := saveRegistrationType(t, transactor, types, registration); outcome != ports.RegistrationNumberTypeRegistrySaved {
			t.Fatalf("登记 %s = %s", code, outcome)
		}
	}, func(row ports.RegistrationNumberTypeRow) string { return row.TypeCode }, "SYN-TYPE-")
	assertEmptyCatalogue(t, "注册号类型", ports.RegistrationNumberTypePages, list(pcTenant(t, "SYN-TENANT-EMPTY")))
}

// Covers: 完成判据「筛选维内取并、维间取交，total 随筛选与 q 变；q 里的 % 按字面」。状态三格各有实例；倒序时同国家的两笔按
// 类型代码倒序收尾（决胜键与排序同向）。
func TestRegistrationNumberTypeCatalogueFiltersAndSearches(t *testing.T) {
	types, catalogue, transactor := newRegistrationNumberTypes(t)
	ctx := t.Context()
	save := func(registration domain.RegistrationNumberTypeRegistration) {
		if outcome := saveRegistrationType(t, transactor, types, registration); outcome != ports.RegistrationNumberTypeRegistrySaved {
			t.Fatalf("登记 = %s", outcome)
		}
	}

	save(namedRegistrationTypeFixture(t, "XA", "SYN-LIFETIME", "合成终身注册号", domain.RegistrationNumberIdentityLayer, registrationTypeEffectiveFrom))
	tax := namedRegistrationTypeFixture(t, "XA", "SYN-TAX", "合成税号", domain.RegistrationNumberProfileLayer, registrationTypeEffectiveFrom)
	save(tax)
	retired, err := tax.Deactivate(pcValue(t, domain.NewRegistrationNumberTypeBasisReference, "SYN-BASIS-RETIRE"), registrationTypeEffectiveFrom.AddDate(0, 1, 0))
	if err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	save(retired)
	save(namedRegistrationTypeFixture(t, "XB", "SYN-LIFETIME", "合成终身注册号", domain.RegistrationNumberIdentityLayer, time.Now().UTC().Add(72*time.Hour)))
	save(namedRegistrationTypeFixture(t, "XB", "SYN-CUSTOMS", "合成海关编码 100%", domain.RegistrationNumberProfileLayer, registrationTypeEffectiveFrom))

	tenant := pcTenant(t, "tenant-1")
	assertFilteredPages(t, "注册号类型", ports.RegistrationNumberTypePages,
		func(query cataloguepage.Query, limit int) (cataloguepage.Result[ports.RegistrationNumberTypeRow], error) {
			return catalogue.ListRegistrationNumberTypes(ctx, tenant, limit, query)
		},
		func(row ports.RegistrationNumberTypeRow) string { return row.CountryCode + "/" + row.TypeCode },
		[]pageCase{
			{"sort=countryCode", "XA/SYN-LIFETIME,XA/SYN-TAX,XB/SYN-CUSTOMS,XB/SYN-LIFETIME"},
			{"sort=-countryCode", "XB/SYN-LIFETIME,XB/SYN-CUSTOMS,XA/SYN-TAX,XA/SYN-LIFETIME"},
			{"sort=countryCode&countryCode=XB", "XB/SYN-CUSTOMS,XB/SYN-LIFETIME"},
			{"sort=countryCode&layer=PROFILE", "XA/SYN-TAX,XB/SYN-CUSTOMS"},
			{"sort=countryCode&status=EFFECTIVE", "XA/SYN-LIFETIME,XB/SYN-CUSTOMS"},
			{"sort=countryCode&status=REGISTERED&status=DEACTIVATED", "XA/SYN-TAX,XB/SYN-LIFETIME"},
			{"sort=countryCode&layer=IDENTITY&countryCode=XA&countryCode=XB&status=EFFECTIVE", "XA/SYN-LIFETIME"},
			{"sort=countryCode&q=税号", "XA/SYN-TAX"},
			{"sort=countryCode&q=syn-lifetime", "XA/SYN-LIFETIME,XB/SYN-LIFETIME"},
			{"sort=countryCode&q=xb", "XB/SYN-CUSTOMS,XB/SYN-LIFETIME"},
			{"sort=countryCode&q=%25", "XB/SYN-CUSTOMS"},
		})
}
