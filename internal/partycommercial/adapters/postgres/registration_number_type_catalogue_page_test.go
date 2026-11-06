package postgres_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件对真实 PostgreSQL 证注册号类型目录按 ADR-0144 分页（票 catalogue-read-pagination-rollout/11）：翻页中途登记新行
// 不重不漏、状态、层、国家各维与 q 收窄的总数与本页同一组条件、同刻登记的行按行标识与排序同向收尾、空册答空页。国家 /
// 地区取 ISO 3166 用户自定义码，判据同 registration_number_type_registry_test.go 的文件头。

const registrationTypePageTenant = "SYN-TENANT-01"

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
		pcTenant(t, registrationTypePageTenant),
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

// saveRegistrationTypesTogether 在同一个事务里登记几行：登记时刻取事务开始的那一刻，这几行的 registeredAt 因此逐位相同，
// 次序只能由行标识定。
func saveRegistrationTypesTogether(
	t *testing.T,
	transactor bentoapp.Transactor,
	types *adapter.RegistrationNumberTypes,
	registrations ...domain.RegistrationNumberTypeRegistration,
) {
	t.Helper()
	mustWithinPublicationTransaction(t, transactor, t.Context(), func(txCtx context.Context) error {
		for _, registration := range registrations {
			outcome, err := types.SaveRegistrationNumberType(txCtx, registration)
			if err != nil {
				return err
			}
			if outcome != ports.RegistrationNumberTypeRegistrySaved {
				return fmt.Errorf("登记 %s/%s = %s", registration.Country(), registration.Code(), outcome)
			}
		}
		return nil
	})
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

	assertPagesWithoutRepeatsOrGaps(t, ports.RegistrationNumberTypePages, list(pcTenant(t, registrationTypePageTenant)), func(code string) {
		registration := registrationTypeFixture(t, registrationTypePageTenant, "XA", code, 1, domain.RegistrationNumberIdentityLayer, `SYN-[0-9]{6}`)
		if outcome := saveRegistrationType(t, transactor, types, registration); outcome != ports.RegistrationNumberTypeRegistrySaved {
			t.Fatalf("登记 %s = %s", code, outcome)
		}
	}, func(row ports.RegistrationNumberTypeRow) string { return row.TypeCode }, "SYN-TYPE-")
	assertEmptyCatalogue(t, "注册号类型", ports.RegistrationNumberTypePages, list(pcTenant(t, "SYN-TENANT-EMPTY")))
}

// Covers: 完成判据「筛选维内取并、维间取交，total 随筛选与 q 变；q 里的 % 按字面」。状态词表每一格都有实例；XB 的两笔资料层类型
// 同一事务登记、登记时刻相同，倒序时按行标识倒序、正序时按行标识正序（决胜键与排序同向）。
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
	saveRegistrationTypesTogether(t, transactor, types,
		namedRegistrationTypeFixture(t, "XB", "SYN-CUSTOMS", "合成海关编码 100%", domain.RegistrationNumberProfileLayer, registrationTypeEffectiveFrom),
		namedRegistrationTypeFixture(t, "XB", "SYN-VAT", "合成增值税登记号", domain.RegistrationNumberProfileLayer, registrationTypeEffectiveFrom),
	)

	tenant := pcTenant(t, registrationTypePageTenant)
	assertFilteredPages(t, "注册号类型", ports.RegistrationNumberTypePages,
		func(query cataloguepage.Query, limit int) (cataloguepage.Result[ports.RegistrationNumberTypeRow], error) {
			return catalogue.ListRegistrationNumberTypes(ctx, tenant, limit, query)
		},
		func(row ports.RegistrationNumberTypeRow) string { return row.CountryCode + "/" + row.TypeCode },
		[]pageCase{
			{"", "XB/SYN-VAT,XB/SYN-CUSTOMS,XB/SYN-LIFETIME,XA/SYN-TAX,XA/SYN-LIFETIME"},
			{"sort=registeredAt", "XA/SYN-LIFETIME,XA/SYN-TAX,XB/SYN-LIFETIME,XB/SYN-CUSTOMS,XB/SYN-VAT"},
			{"countryCode=XB", "XB/SYN-VAT,XB/SYN-CUSTOMS,XB/SYN-LIFETIME"},
			{"layer=PROFILE", "XB/SYN-VAT,XB/SYN-CUSTOMS,XA/SYN-TAX"},
			{"status=EFFECTIVE", "XB/SYN-VAT,XB/SYN-CUSTOMS,XA/SYN-LIFETIME"},
			{"status=REGISTERED&status=DEACTIVATED", "XB/SYN-LIFETIME,XA/SYN-TAX"},
			{"layer=IDENTITY&countryCode=XA&countryCode=XB&status=EFFECTIVE", "XA/SYN-LIFETIME"},
			{"q=税号", "XA/SYN-TAX"},
			{"q=syn-lifetime", "XB/SYN-LIFETIME,XA/SYN-LIFETIME"},
			{"q=xb", "XB/SYN-VAT,XB/SYN-CUSTOMS,XB/SYN-LIFETIME"},
			{"q=%25", "XB/SYN-CUSTOMS"},
		})
}
