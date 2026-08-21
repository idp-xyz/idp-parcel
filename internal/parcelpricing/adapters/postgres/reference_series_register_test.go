package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证计价参考序列登记册（票 08 件①②）：登记行只增不改、
// 取值更正走新版本不静默替换、按计价基准时点解析且断言强度随答案带出、库内 CHECK 把
// 领域不变量再守一遍。夹具全部为 SYN-PRC 合成序列（S 级），零期次生产默认。

var (
	registerWeekOne   = time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)
	registerWeekTwo   = time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	registerWeekThree = time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC)
)

func newSeriesRegister(t *testing.T) (*adapter.ReferenceSeriesVersions, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	register, err := adapter.NewReferenceSeriesVersions(db)
	if err != nil {
		t.Fatalf("构造序列登记册：%v", err)
	}
	return register, db.Transactor(), pool
}

func registerPeriod(t *testing.T, from, to time.Time, value, evidence string) domain.SeriesPeriodValue {
	t.Helper()
	period, err := domain.NewSeriesPeriodValue(from, to, evaluationValue(t, domain.ParseDecimal, value), evidence)
	if err != nil {
		t.Fatalf("构造期次：%v", err)
	}
	return period
}

func seriesReference(t *testing.T, id, version string) domain.VersionReference {
	t.Helper()
	reference, err := domain.NewVersionReference(domain.ArtifactReferenceSeries, id, version, "sha256:syn-"+id+"-"+version)
	if err != nil {
		t.Fatalf("构造序列引用：%v", err)
	}
	return reference
}

func fuelRegistration(t *testing.T, tenant, seriesID, version string, periods ...domain.SeriesPeriodValue) domain.ReferenceSeriesRegistration {
	t.Helper()
	if len(periods) == 0 {
		periods = []domain.SeriesPeriodValue{
			registerPeriod(t, registerWeekOne, registerWeekTwo, "0.22", "SYN-EVIDENCE/fuel-2026-W32"),
			registerPeriod(t, registerWeekTwo, registerWeekThree, "0.24", ""),
		}
	}
	registration, err := domain.NewReferenceSeriesRegistration(domain.ReferenceSeriesRegistrationSpec{
		Tenant:           evaluationValue(t, domain.NewTenantID, tenant),
		Kind:             domain.ReferenceSeriesFuelRate,
		Reference:        seriesReference(t, seriesID, version),
		SourceIdentifier: "SYN-CARRIER/fuel-weekly-bulletin",
		Registrant:       "SYN-PRC-SERIES-REGISTRAR",
		Periods:          periods,
	})
	if err != nil {
		t.Fatalf("构造燃油序列登记：%v", err)
	}
	return registration
}

func registerSeries(t *testing.T, register *adapter.ReferenceSeriesVersions, transactor bentoapp.Transactor, ctx context.Context, registration domain.ReferenceSeriesRegistration) ports.ReferenceSeriesRegistrationOutcome {
	t.Helper()
	var outcome ports.ReferenceSeriesRegistrationOutcome
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		saved, err := register.Register(txCtx, registration)
		outcome = saved
		return err
	}); err != nil {
		t.Fatalf("事务内登记失败：%v", err)
	}
	return outcome
}

// TestReferenceSeriesRegisterAndResolveRoundTrip 证登记后按计价基准时点原样解析：
// 命中期次给出取值与凭证，缺凭证期次的断言强度随答案带出，缺口与期外不给答案。
func TestReferenceSeriesRegisterAndResolveRoundTrip(t *testing.T) {
	register, transactor, _ := newSeriesRegister(t)
	ctx := t.Context()

	registration := fuelRegistration(t, "tenant-a", "SYN-PRC-FUEL-WEEKLY", "v1")
	if outcome := registerSeries(t, register, transactor, ctx, registration); outcome != ports.ReferenceSeriesRegistered {
		t.Fatalf("首登 outcome = %d, 想要 ReferenceSeriesRegistered", outcome)
	}

	tenant := evaluationValue(t, domain.NewTenantID, "tenant-a")
	reference := seriesReference(t, "SYN-PRC-FUEL-WEEKLY", "v1")

	reading, found, err := register.ResolveAt(ctx, tenant, reference, registerWeekOne.Add(48*time.Hour))
	if err != nil || !found {
		t.Fatalf("首期解析失败：found=%v err=%v", found, err)
	}
	if reading.Value().Value().String() != "0.22" || !reading.Verifiable() {
		t.Fatalf("首期取值变形：%s verifiable=%v", reading.Value().Value(), reading.Verifiable())
	}

	asserted, found, err := register.ResolveAt(ctx, tenant, reference, registerWeekTwo.Add(time.Hour))
	if err != nil || !found {
		t.Fatalf("第二期解析失败：found=%v err=%v", found, err)
	}
	if asserted.Verifiable() {
		t.Fatal("缺凭证期次不该报可复核——断言强度只准隔离验证")
	}

	if _, found, err := register.ResolveAt(ctx, tenant, reference, registerWeekThree.Add(time.Hour)); err != nil || found {
		t.Fatalf("期外时点不该有答案：found=%v err=%v", found, err)
	}
	if _, found, err := register.ResolveAt(ctx, tenant, seriesReference(t, "SYN-PRC-FUEL-WEEKLY", "v9"), registerWeekOne.Add(time.Hour)); err != nil || found {
		t.Fatalf("未登记版本不该有答案：found=%v err=%v", found, err)
	}
	other := evaluationValue(t, domain.NewTenantID, "tenant-b")
	if _, found, err := register.ResolveAt(ctx, other, reference, registerWeekOne.Add(time.Hour)); err != nil || found {
		t.Fatalf("别的租户不该看见该序列：found=%v err=%v", found, err)
	}
}

// TestExchangeRateSeriesCarriesItsQuoteBasis 证汇率序列带口径入册、带口径解析：
// 解析出的取值携带声明口径的商业价格政策版本，可直接冻结进评价输入。
func TestExchangeRateSeriesCarriesItsQuoteBasis(t *testing.T) {
	register, transactor, _ := newSeriesRegister(t)
	ctx := t.Context()

	policy, err := domain.NewVersionReference(domain.ArtifactCommercialPolicy, "SYN-PRC-FX-POLICY", "v1", "sha256:syn-fx-policy")
	if err != nil {
		t.Fatalf("构造口径引用：%v", err)
	}
	registration, err := domain.NewReferenceSeriesRegistration(domain.ReferenceSeriesRegistrationSpec{
		Tenant:           evaluationValue(t, domain.NewTenantID, "tenant-a"),
		Kind:             domain.ReferenceSeriesExchangeRate,
		Reference:        seriesReference(t, "SYN-PRC-USD-CNY", "v1"),
		SourceIdentifier: "SYN-FINANCE/usd-cny-daily",
		Registrant:       "SYN-PRC-SERIES-REGISTRAR",
		QuoteBasis:       policy,
		Periods: []domain.SeriesPeriodValue{
			registerPeriod(t, registerWeekOne, registerWeekTwo, "7.2", "SYN-EVIDENCE/fx-2026-W32"),
		},
	})
	if err != nil {
		t.Fatalf("构造汇率序列登记：%v", err)
	}
	if outcome := registerSeries(t, register, transactor, ctx, registration); outcome != ports.ReferenceSeriesRegistered {
		t.Fatalf("汇率首登 outcome = %d", outcome)
	}

	reading, found, err := register.ResolveAt(ctx,
		evaluationValue(t, domain.NewTenantID, "tenant-a"),
		seriesReference(t, "SYN-PRC-USD-CNY", "v1"),
		registerWeekOne.Add(time.Hour))
	if err != nil || !found {
		t.Fatalf("汇率解析失败：found=%v err=%v", found, err)
	}
	basis, declared := reading.Value().QuoteBasis()
	if !declared || basis.ID() != "SYN-PRC-FX-POLICY" {
		t.Fatalf("解析取值丢了口径：declared=%v basis=%s", declared, basis.ID())
	}
}

// TestReferenceSeriesDuplicateAndConflict 证结果代数：同内容重登是幂等重放；同版本
// 装不同取值答内容冲突且原行不被顶替——取值更正必须走新版本。
func TestReferenceSeriesDuplicateAndConflict(t *testing.T) {
	register, transactor, _ := newSeriesRegister(t)
	ctx := t.Context()

	original := fuelRegistration(t, "tenant-a", "SYN-PRC-FUEL-DUP", "v1")
	if outcome := registerSeries(t, register, transactor, ctx, original); outcome != ports.ReferenceSeriesRegistered {
		t.Fatalf("首登 outcome = %d", outcome)
	}
	if outcome := registerSeries(t, register, transactor, ctx, original); outcome != ports.ReferenceSeriesAlreadyRegistered {
		t.Fatalf("重登 outcome = %d, 想要 ReferenceSeriesAlreadyRegistered", outcome)
	}

	impostor := fuelRegistration(t, "tenant-a", "SYN-PRC-FUEL-DUP", "v1",
		registerPeriod(t, registerWeekOne, registerWeekTwo, "0.99", "SYN-EVIDENCE/fuel-2026-W32"))
	if impostor.ContentDigest() == original.ContentDigest() {
		t.Fatal("夹具没造出内容分歧")
	}
	if outcome := registerSeries(t, register, transactor, ctx, impostor); outcome != ports.ReferenceSeriesContentConflict {
		t.Fatalf("冒名登记 outcome = %d, 想要 ReferenceSeriesContentConflict", outcome)
	}

	reading, found, err := register.ResolveAt(ctx,
		evaluationValue(t, domain.NewTenantID, "tenant-a"),
		seriesReference(t, "SYN-PRC-FUEL-DUP", "v1"),
		registerWeekOne.Add(time.Hour))
	if err != nil || !found || reading.Value().Value().String() != "0.22" {
		t.Fatalf("原行被顶替了：found=%v err=%v", found, err)
	}
}

// TestReferenceSeriesCanonicalizationDiffersIsAnswered 证形状不同摘要不可比：同键已
// 按另一套规范化形状在册时答`形状不同`，不是冲突也不是重放，原行保持原样。
func TestReferenceSeriesCanonicalizationDiffersIsAnswered(t *testing.T) {
	register, transactor, pool := newSeriesRegister(t)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO parcel_pricing.reference_series_version
			(tenant_id, series_id, series_version, kind, source_identifier, registrant,
			 effective_from, evidence_grade, canonicalization, content_digest, snapshot)
		 VALUES ('tenant-a', 'SYN-PRC-FUEL-SHAPE', 'v1', 'FUEL_RATE', 'SYN-CARRIER/legacy',
		         'SYN-PRC-SERIES-REGISTRAR', $1, 'ASSERTED', 'PRS-0', 'legacy-digest', '{}'::jsonb)`,
		registerWeekOne); err != nil {
		t.Fatalf("预插旧形状行：%v", err)
	}

	outcome := registerSeries(t, register, transactor, ctx,
		fuelRegistration(t, "tenant-a", "SYN-PRC-FUEL-SHAPE", "v1"))
	if outcome != ports.ReferenceSeriesCanonicalizationDiffers {
		t.Fatalf("outcome = %d, 想要 ReferenceSeriesCanonicalizationDiffers", outcome)
	}
}

// TestReferenceSeriesRegisterOutsideTransactionRejected 证事务纪律：登记必须在事务内。
func TestReferenceSeriesRegisterOutsideTransactionRejected(t *testing.T) {
	register, _, _ := newSeriesRegister(t)
	ctx := t.Context()

	if _, err := register.Register(ctx, fuelRegistration(t, "tenant-a", "SYN-PRC-FUEL-NOTX", "v1")); err == nil {
		t.Fatal("无事务登记被接受了")
	}
}

// TestReferenceSeriesCheckConstraintsRejectImpossibleRows 证领域不变量在库内再守一遍：
// 种类封闭、汇率必带口径、口径成对、区间有序、更正成对不自指、等级封闭。
func TestReferenceSeriesCheckConstraintsRejectImpossibleRows(t *testing.T) {
	_, _, pool := newSeriesRegister(t)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO parcel_pricing.reference_series_version
			(tenant_id, series_id, series_version, kind, source_identifier, registrant,
			 effective_from, evidence_grade, canonicalization, content_digest, snapshot)
		 VALUES ('tenant-a', 's-bad-1', 'v1', 'MOON_PHASE', 'src', 'reg', now(), 'ASSERTED',
		         'PRS-1', 'd', '{}'::jsonb)`); err == nil {
		t.Fatal("一行「未知序列种类」溜进了登记册")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO parcel_pricing.reference_series_version
			(tenant_id, series_id, series_version, kind, source_identifier, registrant,
			 effective_from, evidence_grade, canonicalization, content_digest, snapshot)
		 VALUES ('tenant-a', 's-bad-2', 'v1', 'EXCHANGE_RATE', 'src', 'reg', now(), 'ASSERTED',
		         'PRS-1', 'd', '{}'::jsonb)`); err == nil {
		t.Fatal("一行「裸汇率」溜进了登记册——不接受未声明口径的裸汇率")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO parcel_pricing.reference_series_version
			(tenant_id, series_id, series_version, kind, source_identifier, registrant,
			 quote_basis_id, effective_from, evidence_grade, canonicalization, content_digest, snapshot)
		 VALUES ('tenant-a', 's-bad-3', 'v1', 'FUEL_RATE', 'src', 'reg',
		         'policy-1', now(), 'ASSERTED', 'PRS-1', 'd', '{}'::jsonb)`); err == nil {
		t.Fatal("一行「只有口径 ID 没有版本」溜进了登记册")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO parcel_pricing.reference_series_version
			(tenant_id, series_id, series_version, kind, source_identifier, registrant,
			 effective_from, effective_to, evidence_grade, canonicalization, content_digest, snapshot)
		 VALUES ('tenant-a', 's-bad-4', 'v1', 'FUEL_RATE', 'src', 'reg',
		         now(), now() - interval '1 day', 'ASSERTED', 'PRS-1', 'd', '{}'::jsonb)`); err == nil {
		t.Fatal("一行「区间倒置」溜进了登记册")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO parcel_pricing.reference_series_version
			(tenant_id, series_id, series_version, kind, source_identifier, registrant,
			 effective_from, evidence_grade, prior_version, canonicalization, content_digest, snapshot)
		 VALUES ('tenant-a', 's-bad-5', 'v2', 'FUEL_RATE', 'src', 'reg',
		         now(), 'ASSERTED', 'v1', 'PRS-1', 'd', '{}'::jsonb)`); err == nil {
		t.Fatal("一行「只带回指不带依据」溜进了登记册")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO parcel_pricing.reference_series_version
			(tenant_id, series_id, series_version, kind, source_identifier, registrant,
			 effective_from, evidence_grade, prior_version, correction_basis,
			 canonicalization, content_digest, snapshot)
		 VALUES ('tenant-a', 's-bad-6', 'v2', 'FUEL_RATE', 'src', 'reg',
		         now(), 'ASSERTED', 'v2', 'why', 'PRS-1', 'd', '{}'::jsonb)`); err == nil {
		t.Fatal("一行「自指更正」溜进了登记册")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO parcel_pricing.reference_series_version
			(tenant_id, series_id, series_version, kind, source_identifier, registrant,
			 effective_from, evidence_grade, canonicalization, content_digest, snapshot)
		 VALUES ('tenant-a', 's-bad-7', 'v1', 'FUEL_RATE', 'src', 'reg', now(), 'GOSPEL',
		         'PRS-1', 'd', '{}'::jsonb)`); err == nil {
		t.Fatal("一行「未知证据等级」溜进了登记册")
	}
}

// TestReferenceSeriesCorrectionVersionKeepsItsRelation 证更正版本带关系入册并可解析：
// 新版本入册不动原版本，两版各自按基准时点给自己的取值。
func TestReferenceSeriesCorrectionVersionKeepsItsRelation(t *testing.T) {
	register, transactor, _ := newSeriesRegister(t)
	ctx := t.Context()

	original := fuelRegistration(t, "tenant-a", "SYN-PRC-FUEL-FIX", "v1")
	registerSeries(t, register, transactor, ctx, original)

	corrected, err := domain.NewReferenceSeriesRegistration(domain.ReferenceSeriesRegistrationSpec{
		Tenant:           evaluationValue(t, domain.NewTenantID, "tenant-a"),
		Kind:             domain.ReferenceSeriesFuelRate,
		Reference:        seriesReference(t, "SYN-PRC-FUEL-FIX", "v2"),
		SourceIdentifier: "SYN-CARRIER/fuel-weekly-bulletin",
		Registrant:       "SYN-PRC-SERIES-REGISTRAR",
		Periods: []domain.SeriesPeriodValue{
			registerPeriod(t, registerWeekOne, registerWeekTwo, "0.23", "SYN-EVIDENCE/fuel-2026-W32-corrected"),
		},
		PriorVersion:    seriesReference(t, "SYN-PRC-FUEL-FIX", "v1"),
		CorrectionBasis: "SYN-CORRECTION/fuel-2026-W32-transcription",
	})
	if err != nil {
		t.Fatalf("构造更正登记：%v", err)
	}
	if outcome := registerSeries(t, register, transactor, ctx, corrected); outcome != ports.ReferenceSeriesRegistered {
		t.Fatalf("更正登记 outcome = %d", outcome)
	}

	tenant := evaluationValue(t, domain.NewTenantID, "tenant-a")
	was, found, err := register.ResolveAt(ctx, tenant, seriesReference(t, "SYN-PRC-FUEL-FIX", "v1"), registerWeekOne.Add(time.Hour))
	if err != nil || !found || was.Value().Value().String() != "0.22" {
		t.Fatalf("原版本被追溯改写了：found=%v err=%v", found, err)
	}
	now, found, err := register.ResolveAt(ctx, tenant, seriesReference(t, "SYN-PRC-FUEL-FIX", "v2"), registerWeekOne.Add(time.Hour))
	if err != nil || !found || now.Value().Value().String() != "0.23" {
		t.Fatalf("更正版本解析失败：found=%v err=%v", found, err)
	}
}
