package postgres_test

import (
	"context"
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

// 本文件对真实 PostgreSQL 16 证启用范围登记册（0043）：落册往返、撞键重放与冲突、最新修订装载、按种类整册读出、
// 最新生效导出（停用后再启用）、跨租户隔离、无事务拒，以及迁移里 CHECK 对形状的把关。租户与登记责任方全为合成值。

var enablementEffective = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func newReferenceEnablements(t *testing.T) (*adapter.ReferenceEnablements, bentoapp.Transactor, bentopg.Executor) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registry, err := adapter.NewReferenceEnablements(db)
	if err != nil {
		t.Fatalf("构造启用范围登记册：%v", err)
	}
	return registry, db.Transactor(), pool
}

func enablementFixture(
	t *testing.T,
	tenant string,
	kind domain.ReferenceKind,
	code string,
	revision int,
	status domain.ReferenceEnablementStatus,
	effectiveFrom time.Time,
	source, operator string,
) domain.ReferenceEnablementRegistration {
	t.Helper()
	registration, err := domain.NewReferenceEnablementRegistration(
		pcTenant(t, tenant), kind, pcValue(t, domain.NewReferenceCode, code), revision,
		domain.ReferenceEnablementSpec{
			Status:        status,
			EffectiveFrom: effectiveFrom,
			Source:        pcValue(t, domain.NewReferenceEnablementSource, source),
		},
		pcValue(t, domain.NewOperatorSubjectReference, operator),
		time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("new reference enablement: %v", err)
	}
	return registration
}

func saveEnablement(
	t *testing.T,
	transactor bentoapp.Transactor,
	registry *adapter.ReferenceEnablements,
	registration domain.ReferenceEnablementRegistration,
) ports.ReferenceEnablementSaveOutcome {
	t.Helper()
	var outcome ports.ReferenceEnablementSaveOutcome
	mustWithinPublicationTransaction(t, transactor, t.Context(), func(txCtx context.Context) error {
		var err error
		outcome, err = registry.SaveReferenceEnablement(txCtx, registration)
		return err
	})
	return outcome
}

// Covers: ADR-0345 决定三——修订 1 落册、同键同内容重放答已登记（审计两件不同也算同内容）、同键异内容冲突不覆盖；
// 读回时往返不走样，登记责任方与登记时间照落库值原样回来。
func TestReferenceEnablementRegistersReplaysAndConflicts(t *testing.T) {
	registry, transactor, _ := newReferenceEnablements(t)
	ctx := t.Context()
	tenant, kind, code := pcTenant(t, "tenant-enable-1"), domain.ReferenceKindCountry, pcValue(t, domain.NewReferenceCode, "CN")

	first := enablementFixture(t, "tenant-enable-1", kind, "CN", 1, domain.ReferenceEnablementEnabled, enablementEffective, "MEMO-1", "https://idp.example#operator-1")
	if outcome := saveEnablement(t, transactor, registry, first); outcome != ports.ReferenceEnablementSaved {
		t.Fatalf("修订 1 = %s, want SAVED", outcome)
	}
	otherOperator := enablementFixture(t, "tenant-enable-1", kind, "CN", 1, domain.ReferenceEnablementEnabled, enablementEffective, "MEMO-1", "https://idp.example#operator-2")
	if outcome := saveEnablement(t, transactor, registry, otherOperator); outcome != ports.ReferenceEnablementAlreadyRegistered {
		t.Fatalf("重放修订 1 = %s, want ALREADY_REGISTERED", outcome)
	}
	conflicting := enablementFixture(t, "tenant-enable-1", kind, "CN", 1, domain.ReferenceEnablementDisabled, enablementEffective, "MEMO-1", "https://idp.example#operator-1")
	if outcome := saveEnablement(t, transactor, registry, conflicting); outcome != ports.ReferenceEnablementContentConflict {
		t.Fatalf("同修订异状态 = %s, want CONTENT_CONFLICT", outcome)
	}
	differentSource := enablementFixture(t, "tenant-enable-1", kind, "CN", 1, domain.ReferenceEnablementEnabled, enablementEffective, "MEMO-2", "https://idp.example#operator-1")
	if outcome := saveEnablement(t, transactor, registry, differentSource); outcome != ports.ReferenceEnablementContentConflict {
		t.Fatalf("同修订异来源单据 = %s, want CONTENT_CONFLICT", outcome)
	}

	loaded, found, err := registry.LoadLatestReferenceEnablement(ctx, tenant, kind, code)
	if err != nil || !found {
		t.Fatalf("读回 = (%v, %v)", found, err)
	}
	if loaded.Revision() != 1 || loaded.Status() != domain.ReferenceEnablementEnabled ||
		!loaded.EffectiveFrom().Equal(enablementEffective) || loaded.Source().String() != "MEMO-1" ||
		loaded.Registrant().String() != "https://idp.example#operator-1" ||
		!loaded.RecordedAt().Equal(first.RecordedAt()) {
		t.Fatalf("往返走样：r%d %s %v %q %s %v", loaded.Revision(), loaded.Status(), loaded.EffectiveFrom(),
			loaded.Source(), loaded.Registrant(), loaded.RecordedAt())
	}
}

// Covers: ADR-0345 决定三「当前状态由生效时点不晚于此刻的最新一行导出」与停用后再启用——三笔追加后整册读出带全部修订，
// 导出的状态是最后一笔的启用；LoadLatest 取最高修订；来源单据可空，空读回仍是空。
func TestReferenceEnablementDisableThenEnableAgainDerivesTheLatestEffectiveRow(t *testing.T) {
	registry, transactor, _ := newReferenceEnablements(t)
	ctx := t.Context()
	tenant, kind := pcTenant(t, "tenant-enable-2"), domain.ReferenceKindCurrency
	day := func(n int) time.Time { return enablementEffective.AddDate(0, 0, n) }

	saveEnablement(t, transactor, registry, enablementFixture(t, "tenant-enable-2", kind, "USD", 1, domain.ReferenceEnablementEnabled, day(0), "", "idp#a"))
	saveEnablement(t, transactor, registry, enablementFixture(t, "tenant-enable-2", kind, "USD", 2, domain.ReferenceEnablementDisabled, day(10), "停用单", "idp#a"))
	saveEnablement(t, transactor, registry, enablementFixture(t, "tenant-enable-2", kind, "USD", 3, domain.ReferenceEnablementEnabled, day(20), "", "idp#b"))
	saveEnablement(t, transactor, registry, enablementFixture(t, "tenant-enable-2", kind, "EUR", 1, domain.ReferenceEnablementDisabled, day(0), "", "idp#a"))

	rows, err := registry.ListReferenceEnablements(ctx, tenant, kind)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("整册应读出 4 行，实际 %d", len(rows))
	}
	if rows[0].Code().String() != "EUR" || rows[1].Code().String() != "USD" || rows[1].Revision() != 1 || rows[3].Revision() != 3 {
		t.Fatalf("应按码、修订升序：%s/%d %s/%d", rows[0].Code(), rows[0].Revision(), rows[3].Code(), rows[3].Revision())
	}
	if rows[1].Source().String() != "" || rows[2].Source().String() != "停用单" {
		t.Fatalf("来源单据往返走样：%q %q", rows[1].Source(), rows[2].Source())
	}

	usd := []domain.ReferenceEnablementRegistration{rows[1], rows[2], rows[3]}
	for _, tc := range []struct {
		at     time.Time
		found  bool
		status domain.ReferenceEnablementStatus
	}{
		{day(-1), false, 0},
		{day(5), true, domain.ReferenceEnablementEnabled},
		{day(10), true, domain.ReferenceEnablementDisabled},
		{day(25), true, domain.ReferenceEnablementEnabled},
	} {
		standing, found := domain.EnablementStandingAt(usd, tc.at)
		if found != tc.found || (found && standing.Status() != tc.status) {
			t.Fatalf("at %v: (%v, %v)，want (%v, %v)", tc.at, found, standing.Status(), tc.found, tc.status)
		}
	}

	latest, found, err := registry.LoadLatestReferenceEnablement(ctx, tenant, kind, pcValue(t, domain.NewReferenceCode, "USD"))
	if err != nil || !found || latest.Revision() != 3 {
		t.Fatalf("LoadLatest 应取最高修订 3：%d %v %v", latest.Revision(), found, err)
	}
	if _, found, _ := registry.LoadLatestReferenceEnablement(ctx, tenant, kind, pcValue(t, domain.NewReferenceCode, "JPY")); found {
		t.Fatal("从未登记的码 found=false")
	}
}

// Covers: ADR-0345 决定四——册面按租户隔离，种类互不串；同码各租户各登各的首笔。
func TestReferenceEnablementIsolatesTenantsAndKinds(t *testing.T) {
	registry, transactor, _ := newReferenceEnablements(t)
	ctx := t.Context()

	saveEnablement(t, transactor, registry, enablementFixture(t, "tenant-enable-3a", domain.ReferenceKindCountry, "SG", 1, domain.ReferenceEnablementEnabled, enablementEffective, "", "idp#a"))
	if outcome := saveEnablement(t, transactor, registry, enablementFixture(t, "tenant-enable-3b", domain.ReferenceKindCountry, "SG", 1, domain.ReferenceEnablementDisabled, enablementEffective, "", "idp#a")); outcome != ports.ReferenceEnablementSaved {
		t.Fatalf("另一租户同码的修订 1 是它自己的首笔，实际 %s", outcome)
	}

	for tenant, want := range map[string]domain.ReferenceEnablementStatus{
		"tenant-enable-3a": domain.ReferenceEnablementEnabled,
		"tenant-enable-3b": domain.ReferenceEnablementDisabled,
	} {
		rows, err := registry.ListReferenceEnablements(ctx, pcTenant(t, tenant), domain.ReferenceKindCountry)
		if err != nil || len(rows) != 1 || rows[0].Status() != want {
			t.Fatalf("%s 只应读到自己的一行 %s：%v %v", tenant, want, rows, err)
		}
	}
	if rows, err := registry.ListReferenceEnablements(ctx, pcTenant(t, "tenant-enable-3a"), domain.ReferenceKindCurrency); err != nil || len(rows) != 0 {
		t.Fatalf("国家 / 地区的行不串进币种册：%v %v", rows, err)
	}
	if rows, err := registry.ListReferenceEnablements(ctx, pcTenant(t, "tenant-enable-3c"), domain.ReferenceKindCountry); err != nil || len(rows) != 0 {
		t.Fatalf("没登记过的租户读空册：%v %v", rows, err)
	}
}

// Covers: 登记写口无环境事务即拒（判据同其余登记册）；迁移的 CHECK 守形状与封闭集——绕过领域门的写入也进不去。
func TestReferenceEnablementWriteNeedsATransactionAndTheTableGuardsShape(t *testing.T) {
	registry, _, pool := newReferenceEnablements(t)
	ctx := t.Context()

	_, err := registry.SaveReferenceEnablement(ctx, enablementFixture(t, "tenant-enable-4", domain.ReferenceKindCountry, "CN", 1, domain.ReferenceEnablementEnabled, enablementEffective, "", "idp#a"))
	if err == nil {
		t.Fatal("无环境事务应拒")
	}

	for name, args := range map[string][]any{
		"国家码形状":    {"tenant-enable-4", "COUNTRY", "CHN", 1, "ENABLED", ""},
		"币种码形状":    {"tenant-enable-4", "CURRENCY", "US", 1, "ENABLED", ""},
		"种类封闭集":    {"tenant-enable-4", "REGION", "CN", 1, "ENABLED", ""},
		"状态封闭集":    {"tenant-enable-4", "COUNTRY", "CN", 1, "PENDING", ""},
		"修订从 1 起":  {"tenant-enable-4", "COUNTRY", "CN", 0, "ENABLED", ""},
		"来源单据首尾空白": {"tenant-enable-4", "COUNTRY", "CN", 1, "ENABLED", " x "},
	} {
		_, err := pool.Exec(ctx,
			`INSERT INTO party_commercial.reference_enablement_registration
				(tenant_id, kind, code, revision, status, effective_from, source_ref, registered_by, recorded_at)
			 VALUES ($1, $2, $3, $4, $5, now(), $6, 'idp#a', now())`, args...)
		if err == nil {
			t.Fatalf("%s：绕过领域门的行应被 CHECK 拒", name)
		}
	}
}
