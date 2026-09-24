package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证注册号类型目录（0033）：登记、修订、停用落册往返；撞键重放与冲突；
// 按国家 / 地区取回的目录只见最新修订并据以判号——层不符、格式不符、国家 / 地区未登记三种拒绝
// 各一条；目录上列、跨租户隔离与无事务拒。国家 / 地区取 ISO 3166 用户自定义码，类型与格式全为
// 合成值。

var registrationTypeEffectiveFrom = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func newRegistrationNumberTypes(t *testing.T) (*adapter.RegistrationNumberTypes, *adapter.OperationsCatalogue, bentoapp.Transactor) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	types, err := adapter.NewRegistrationNumberTypes(db)
	if err != nil {
		t.Fatalf("构造注册号类型目录：%v", err)
	}
	catalogue, err := adapter.NewOperationsCatalogue(db)
	if err != nil {
		t.Fatalf("构造目录读口：%v", err)
	}
	return types, catalogue, db.Transactor()
}

func registrationTypeFixture(
	t *testing.T,
	tenant, country, code string,
	revision int,
	layer domain.RegistrationNumberLayer,
	pattern string,
) domain.RegistrationNumberTypeRegistration {
	t.Helper()
	format, err := domain.NewRegistrationNumberFormat(pattern)
	if err != nil {
		t.Fatalf("new format %q: %v", pattern, err)
	}
	lifecycle, err := domain.NewRegistrationNumberTypeLifecycle(registrationTypeEffectiveFrom)
	if err != nil {
		t.Fatalf("new lifecycle: %v", err)
	}
	registration, err := domain.NewRegistrationNumberTypeRegistration(
		pcTenant(t, tenant),
		pcValue(t, domain.NewRegistrationCountryCode, country),
		pcValue(t, domain.NewRegistrationNumberTypeCode, code),
		revision,
		domain.RegistrationNumberTypeSpec{
			Name:   pcValue(t, domain.NewRegistrationNumberTypeName, "合成类型 "+code),
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

func saveRegistrationType(
	t *testing.T,
	transactor bentoapp.Transactor,
	types *adapter.RegistrationNumberTypes,
	registration domain.RegistrationNumberTypeRegistration,
) ports.RegistrationNumberTypeSaveOutcome {
	t.Helper()
	var outcome ports.RegistrationNumberTypeSaveOutcome
	mustWithinPublicationTransaction(t, transactor, t.Context(), func(txCtx context.Context) error {
		var err error
		outcome, err = types.SaveRegistrationNumberType(txCtx, registration)
		return err
	})
	return outcome
}

func checkRegistrationNumber(
	t *testing.T,
	catalogue domain.RegistrationNumberTypeCatalogue,
	code string,
	layer domain.RegistrationNumberLayer,
	number string,
) domain.RegistrationNumberCheck {
	t.Helper()
	check, err := catalogue.Check(
		pcValue(t, domain.NewRegistrationNumberTypeCode, code),
		layer,
		pcValue(t, domain.NewRegistrationNumber, number),
		registrationTypeEffectiveFrom.AddDate(0, 6, 0),
	)
	if err != nil {
		t.Fatalf("check %s %q: %v", code, number, err)
	}
	return check
}

// Covers: 票 legal-entity-profile/01 完成判据「真库用例：登记、修订、停用」——修订 1 落册、同键同
// 内容重放、同键异内容冲突不覆盖；修订 2 更正格式落定不覆盖修订 1；停用成修订 3 携依据与时点往返；
// 目录上列只见最新修订并导出 DEACTIVATED。
func TestRegistrationNumberTypeRegistersRevisesAndDeactivates(t *testing.T) {
	types, catalogue, transactor := newRegistrationNumberTypes(t)
	ctx := t.Context()
	tenant, country, code := pcTenant(t, "tenant-1"),
		pcValue(t, domain.NewRegistrationCountryCode, "XA"),
		pcValue(t, domain.NewRegistrationNumberTypeCode, "SYN-LIFETIME")

	first := registrationTypeFixture(t, "tenant-1", "XA", "SYN-LIFETIME", 1,
		domain.RegistrationNumberIdentityLayer, `SYN-[0-9]{6}`)
	if outcome := saveRegistrationType(t, transactor, types, first); outcome != ports.RegistrationNumberTypeSaved {
		t.Fatalf("修订 1 = %s, want SAVED", outcome)
	}
	if outcome := saveRegistrationType(t, transactor, types, first); outcome != ports.RegistrationNumberTypeAlreadyRegistered {
		t.Fatalf("重放修订 1 = %s, want ALREADY_REGISTERED", outcome)
	}
	conflicting := registrationTypeFixture(t, "tenant-1", "XA", "SYN-LIFETIME", 1,
		domain.RegistrationNumberIdentityLayer, `SYN-[0-9]{7}`)
	if outcome := saveRegistrationType(t, transactor, types, conflicting); outcome != ports.RegistrationNumberTypeContentConflict {
		t.Fatalf("同修订异内容 = %s, want CONTENT_CONFLICT", outcome)
	}

	corrected := registrationTypeFixture(t, "tenant-1", "XA", "SYN-LIFETIME", 2,
		domain.RegistrationNumberIdentityLayer, `SYN-[0-9]{8}`)
	if outcome := saveRegistrationType(t, transactor, types, corrected); outcome != ports.RegistrationNumberTypeSaved {
		t.Fatalf("修订 2 = %s, want SAVED", outcome)
	}
	loaded, found, err := types.LoadLatestRegistrationNumberType(ctx, tenant, country, code)
	if err != nil || !found {
		t.Fatalf("读回修订 2 = (%v, %v)", found, err)
	}
	if loaded.Revision() != 2 || loaded.Format().Pattern() != `SYN-[0-9]{8}` ||
		loaded.Layer() != domain.RegistrationNumberIdentityLayer ||
		loaded.Name().String() != "合成类型 SYN-LIFETIME" || loaded.Basis().String() != "SYN-BASIS-SYN-LIFETIME" ||
		!loaded.Lifecycle().EffectiveFrom().Equal(registrationTypeEffectiveFrom) {
		t.Fatalf("修订 2 往返走样：r%d %q %s %q %q %v", loaded.Revision(), loaded.Format().Pattern(),
			loaded.Layer(), loaded.Name(), loaded.Basis(), loaded.Lifecycle().EffectiveFrom())
	}
	if _, _, has := loaded.Lifecycle().Deactivation(); has {
		t.Fatalf("修订 2 不该带停用")
	}

	deactivatedAt := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	retireBasis := pcValue(t, domain.NewRegistrationNumberTypeBasisReference, "SYN-BASIS-RETIRE")
	retired, err := loaded.Deactivate(retireBasis, deactivatedAt)
	if err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	if outcome := saveRegistrationType(t, transactor, types, retired); outcome != ports.RegistrationNumberTypeSaved {
		t.Fatalf("停用修订 = %s, want SAVED", outcome)
	}
	loaded, found, err = types.LoadLatestRegistrationNumberType(ctx, tenant, country, code)
	if err != nil || !found {
		t.Fatalf("读回停用修订 = (%v, %v)", found, err)
	}
	basis, at, has := loaded.Lifecycle().Deactivation()
	if loaded.Revision() != 3 || !has || basis != retireBasis || !at.Equal(deactivatedAt) {
		t.Fatalf("停用修订往返走样：r%d (%s, %v, %v)", loaded.Revision(), basis, at, has)
	}

	rows, err := catalogue.ListRegistrationNumberTypes(ctx, tenant, 10)
	if err != nil {
		t.Fatalf("list registration number types: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1（只上列最新修订）", len(rows))
	}
	row := rows[0]
	if row.CountryCode != "XA" || row.TypeCode != "SYN-LIFETIME" || row.Revision != 3 ||
		row.Layer != "IDENTITY" || row.FormatPattern != `SYN-[0-9]{8}` || row.Status != "DEACTIVATED" ||
		!row.HasDeactivation || row.DeactivationBasis != "SYN-BASIS-RETIRE" || !row.DeactivatedAt.Equal(deactivatedAt) {
		t.Fatalf("目录行 = %+v", row)
	}
	if _, err := catalogue.ListRegistrationNumberTypes(ctx, tenant, 0); err == nil {
		t.Fatalf("limit 0 应被拒")
	}
}

// Covers: 票 legal-entity-profile/01 完成判据「层不符、格式不符、国家未登记三种拒绝各一条」的真库
// 半边——目录经登记册按国家 / 地区取回、只取各类型最新修订（修订 1 合格而修订 2 不合格的号答格式
// 不符），并据以判号；国家 / 地区在册上一笔都没有时答未登记，不以默认格式代替。
func TestRegistrationNumberTypeLookupRefusesLayerFormatAndUnregisteredCountry(t *testing.T) {
	types, _, transactor := newRegistrationNumberTypes(t)
	ctx := t.Context()

	for _, registration := range []domain.RegistrationNumberTypeRegistration{
		registrationTypeFixture(t, "tenant-1", "XA", "SYN-LIFETIME", 1,
			domain.RegistrationNumberIdentityLayer, `SYN-[0-9]{6}`),
		registrationTypeFixture(t, "tenant-1", "XA", "SYN-LIFETIME", 2,
			domain.RegistrationNumberIdentityLayer, `SYN-[0-9]{8}`),
		registrationTypeFixture(t, "tenant-1", "XA", "SYN-TAX", 1,
			domain.RegistrationNumberProfileLayer, `SYN-TAX-[0-9]{4}`),
	} {
		if outcome := saveRegistrationType(t, transactor, types, registration); outcome != ports.RegistrationNumberTypeSaved {
			t.Fatalf("登记 %s r%d = %s, want SAVED", registration.Code(), registration.Revision(), outcome)
		}
	}

	catalogue, err := types.LoadRegistrationNumberTypeCatalogue(ctx, pcTenant(t, "tenant-1"),
		pcValue(t, domain.NewRegistrationCountryCode, "XA"))
	if err != nil {
		t.Fatalf("load catalogue XA: %v", err)
	}

	layerMismatch := checkRegistrationNumber(t, catalogue, "SYN-TAX", domain.RegistrationNumberIdentityLayer, "SYN-TAX-0001")
	if layerMismatch.Outcome() != domain.RegistrationNumberLayerMismatch {
		t.Fatalf("资料层号填进身份层 = %s, want LAYER_MISMATCH", layerMismatch.Outcome())
	}
	formatMismatch := checkRegistrationNumber(t, catalogue, "SYN-LIFETIME", domain.RegistrationNumberIdentityLayer, "SYN-000001")
	consulted, hasType := formatMismatch.Type()
	if formatMismatch.Outcome() != domain.RegistrationNumberFormatMismatch || !hasType || consulted.Revision() != 2 {
		t.Fatalf("按修订 1 合格、修订 2 不合格的号 = %s（对照 r%d, %v），want FORMAT_MISMATCH 对照 r2",
			formatMismatch.Outcome(), consulted.Revision(), hasType)
	}
	if accepted := checkRegistrationNumber(t, catalogue, "SYN-LIFETIME", domain.RegistrationNumberIdentityLayer, "SYN-00000001"); accepted.Outcome() != domain.RegistrationNumberAccepted {
		t.Fatalf("合格号 = %s, want ACCEPTED", accepted.Outcome())
	}

	unregistered, err := types.LoadRegistrationNumberTypeCatalogue(ctx, pcTenant(t, "tenant-1"),
		pcValue(t, domain.NewRegistrationCountryCode, "XB"))
	if err != nil {
		t.Fatalf("load catalogue XB: %v", err)
	}
	if check := checkRegistrationNumber(t, unregistered, "SYN-LIFETIME", domain.RegistrationNumberIdentityLayer, "SYN-00000001"); check.Outcome() != domain.RegistrationCountryNotRegistered {
		t.Fatalf("册上没有的国家 / 地区 = %s, want COUNTRY_NOT_REGISTERED", check.Outcome())
	}
}

// Covers: 目录按租户隔离——他租户按同一国家 / 地区取回空目录（答未登记）、点读与从未登记同形、上列
// 为空；一个租户登记的格式不会成为另一个租户的默认。
func TestRegistrationNumberTypeRegistryIsTenantScoped(t *testing.T) {
	types, catalogue, transactor := newRegistrationNumberTypes(t)
	ctx := t.Context()

	own := registrationTypeFixture(t, "tenant-1", "XA", "SYN-LIFETIME", 1,
		domain.RegistrationNumberIdentityLayer, `SYN-[0-9]{6}`)
	if outcome := saveRegistrationType(t, transactor, types, own); outcome != ports.RegistrationNumberTypeSaved {
		t.Fatalf("登记 = %s, want SAVED", outcome)
	}

	foreign := pcTenant(t, "tenant-b")
	country := pcValue(t, domain.NewRegistrationCountryCode, "XA")
	foreignCatalogue, err := types.LoadRegistrationNumberTypeCatalogue(ctx, foreign, country)
	if err != nil {
		t.Fatalf("foreign catalogue: %v", err)
	}
	if check := checkRegistrationNumber(t, foreignCatalogue, "SYN-LIFETIME", domain.RegistrationNumberIdentityLayer, "SYN-000001"); check.Outcome() != domain.RegistrationCountryNotRegistered {
		t.Fatalf("他租户判号 = %s, want COUNTRY_NOT_REGISTERED", check.Outcome())
	}
	if _, found, err := types.LoadLatestRegistrationNumberType(ctx, foreign, country,
		pcValue(t, domain.NewRegistrationNumberTypeCode, "SYN-LIFETIME")); err != nil || found {
		t.Fatalf("他租户点读 = (%v, %v), want (false, nil)", found, err)
	}
	if rows, err := catalogue.ListRegistrationNumberTypes(ctx, foreign, 10); err != nil || len(rows) != 0 {
		t.Fatalf("他租户上列 = (%d, %v), want (0, nil)", len(rows), err)
	}
}

func TestRegistrationNumberTypeWritesRefuseToRunOutsideATransaction(t *testing.T) {
	types, _, _ := newRegistrationNumberTypes(t)
	if _, err := types.SaveRegistrationNumberType(t.Context(),
		registrationTypeFixture(t, "tenant-1", "XA", "SYN-LIFETIME", 1,
			domain.RegistrationNumberIdentityLayer, `SYN-[0-9]{6}`),
	); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务 SaveRegistrationNumberType 应返回 ErrTransactionRequired，实得：%v", err)
	}
}
