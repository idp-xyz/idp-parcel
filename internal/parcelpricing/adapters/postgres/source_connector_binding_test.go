package postgres_test

import (
	"context"
	"errors"
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

// 本文件对真实 PostgreSQL 16 证来源连接器绑定登记册（迁移 0005；ADR-0099 决定六）：行只增不改、
// 同键重复按声明比对译成幂等或冲突、当前绑定 = 最近登记的一版、跨租户不可见、库内 CHECK 把
// 领域不变量再守一遍（免复核三格封闭无默认、汇率必带口径、口径三列成对）。夹具全部 SYN（S 级），
// 出厂零绑定由「不种任何行」这件事本身证。

func newBindingRegister(t *testing.T) (*adapter.SourceConnectorBindings, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	register, err := adapter.NewSourceConnectorBindings(db)
	if err != nil {
		t.Fatalf("构造绑定登记册：%v", err)
	}
	return register, db.Transactor(), pool
}

func bindingSpec(t *testing.T, tenant, seriesID, version string, exemption domain.ReviewExemption) domain.SourceConnectorBindingSpec {
	t.Helper()
	policy, err := domain.NewVersionReference(domain.ArtifactCommercialPolicy, "SYN-PRC-FX-POLICY", "v1", "sha256:syn-fx-policy")
	if err != nil {
		t.Fatalf("构造口径引用：%v", err)
	}
	return domain.SourceConnectorBindingSpec{
		Tenant:           evaluationValue(t, domain.NewTenantID, tenant),
		SeriesID:         seriesID,
		Version:          version,
		ConnectorKind:    "FILE",
		SourceIdentifier: "SYN-SOURCE/usd-cny-daily",
		SourceLocator:    "rates/usd-cny/latest.json",
		SeriesKind:       domain.ReferenceSeriesExchangeRate,
		QuoteBasis:       policy,
		Registrant:       "SYN-PRC-SERIES-REGISTRAR",
		Cadence:          "DAILY",
		ReviewExemption:  exemption,
	}
}

func binding(t *testing.T, spec domain.SourceConnectorBindingSpec) domain.SourceConnectorBinding {
	t.Helper()
	built, err := domain.NewSourceConnectorBinding(spec)
	if err != nil {
		t.Fatalf("构造绑定：%v", err)
	}
	return built
}

func registerBinding(t *testing.T, register *adapter.SourceConnectorBindings, transactor bentoapp.Transactor, ctx context.Context, bound domain.SourceConnectorBinding) ports.SourceConnectorBindingOutcome {
	t.Helper()
	var outcome ports.SourceConnectorBindingOutcome
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		saved, err := register.Register(txCtx, bound)
		outcome = saved
		return err
	}); err != nil {
		t.Fatalf("事务内登记绑定失败：%v", err)
	}
	return outcome
}

// TestSourceConnectorBindingRoundTripsAndTheLatestVersionIsCurrent 证登记后原样读回（含口径三件与
// 节律），再登一版后当前绑定切到新版本而旧版本仍在册；别的租户、别的序列看不见。
func TestSourceConnectorBindingRoundTripsAndTheLatestVersionIsCurrent(t *testing.T) {
	register, transactor, _ := newBindingRegister(t)
	ctx := t.Context()
	tenant := evaluationValue(t, domain.NewTenantID, "tenant-a")

	if _, found, err := register.LoadCurrentBinding(ctx, tenant, "SYN-PRC-USD-CNY"); err != nil || found {
		t.Fatalf("出厂就有绑定：found=%v err=%v", found, err)
	}

	first := binding(t, bindingSpec(t, "tenant-a", "SYN-PRC-USD-CNY", "b1", domain.ReviewExemptionGranted))
	if outcome := registerBinding(t, register, transactor, ctx, first); outcome != ports.SourceConnectorBindingRegistered {
		t.Fatalf("首登 outcome = %d，想要 Registered", outcome)
	}
	current, found, err := register.LoadCurrentBinding(ctx, tenant, "SYN-PRC-USD-CNY")
	if err != nil || !found {
		t.Fatalf("读回失败：found=%v err=%v", found, err)
	}
	if current.Spec() != first.Spec() {
		t.Fatalf("读回的绑定与登记的不同：\n%+v\n%+v", current.Spec(), first.Spec())
	}
	if basis, declared := current.QuoteBasis(); !declared || basis.Digest() != "sha256:syn-fx-policy" {
		t.Fatalf("口径引用的 digest 丢了：declared=%v basis=%v", declared, basis)
	}

	secondSpec := bindingSpec(t, "tenant-a", "SYN-PRC-USD-CNY", "b2", domain.ReviewExemptionUndeclared)
	secondSpec.SourceLocator = "rates/usd-cny/2026-09-05.json"
	secondSpec.Cadence = ""
	second := binding(t, secondSpec)
	if outcome := registerBinding(t, register, transactor, ctx, second); outcome != ports.SourceConnectorBindingRegistered {
		t.Fatalf("第二版 outcome = %d", outcome)
	}
	current, found, err = register.LoadCurrentBinding(ctx, tenant, "SYN-PRC-USD-CNY")
	if err != nil || !found || current.Version() != "b2" || current.ReviewExemption() != domain.ReviewExemptionUndeclared {
		t.Fatalf("当前绑定没切到最近登记的一版：found=%v version=%s err=%v", found, current.Version(), err)
	}
	if _, scheduled := current.Cadence(); scheduled {
		t.Fatal("没声明节律却读回了节律")
	}

	if _, found, err := register.LoadCurrentBinding(ctx, evaluationValue(t, domain.NewTenantID, "tenant-b"), "SYN-PRC-USD-CNY"); err != nil || found {
		t.Fatalf("别的租户看见了该绑定：found=%v err=%v", found, err)
	}
	if _, found, err := register.LoadCurrentBinding(ctx, tenant, "SYN-PRC-EUR-CNY"); err != nil || found {
		t.Fatalf("别的序列看见了该绑定：found=%v err=%v", found, err)
	}
}

// TestSourceConnectorBindingRegisterAlgebra 证结果代数：同键同声明是幂等重放；同键异声明是冲突且
// 原行不顶替——改声明请登新版本。
func TestSourceConnectorBindingRegisterAlgebra(t *testing.T) {
	register, transactor, _ := newBindingRegister(t)
	ctx := t.Context()
	tenant := evaluationValue(t, domain.NewTenantID, "tenant-a")

	original := binding(t, bindingSpec(t, "tenant-a", "SYN-PRC-USD-CNY-ALG", "b1", domain.ReviewExemptionGranted))
	registerBinding(t, register, transactor, ctx, original)
	if outcome := registerBinding(t, register, transactor, ctx, original); outcome != ports.SourceConnectorBindingAlreadyRegistered {
		t.Fatalf("重登 outcome = %d，想要 AlreadyRegistered", outcome)
	}

	impostorSpec := bindingSpec(t, "tenant-a", "SYN-PRC-USD-CNY-ALG", "b1", domain.ReviewExemptionWithheld)
	impostor := binding(t, impostorSpec)
	if outcome := registerBinding(t, register, transactor, ctx, impostor); outcome != ports.SourceConnectorBindingConflict {
		t.Fatalf("同键异声明 outcome = %d，想要 Conflict", outcome)
	}
	current, _, err := register.LoadCurrentBinding(ctx, tenant, "SYN-PRC-USD-CNY-ALG")
	if err != nil || current.ReviewExemption() != domain.ReviewExemptionGranted {
		t.Fatalf("冲突写入顶替了原行：%v err=%v", current.ReviewExemption(), err)
	}
}

// TestSourceConnectorBindingWritesRefuseToRunOutsideATransaction 是 PBC-08 行为面负向证据：写口在
// 无事务上下文必须被 RequireExecutor 拒绝。拒绝先于任何入参解读，所以传零值就够；若有人把入参
// 校验挪到守卫之前，断言会以「错误不是 ErrTransactionRequired」如实变红。
func TestSourceConnectorBindingWritesRefuseToRunOutsideATransaction(t *testing.T) {
	register, _, _ := newBindingRegister(t)
	if _, err := register.Register(t.Context(), domain.SourceConnectorBinding{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记绑定应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// TestSourceConnectorBindingCheckConstraintsRejectImpossibleRows 证库内再守一遍：序列种类封闭、
// 免复核三格封闭、裸汇率进不去、口径三列缺一不成对、空序列标识进不去。
func TestSourceConnectorBindingCheckConstraintsRejectImpossibleRows(t *testing.T) {
	_, _, pool := newBindingRegister(t)
	ctx := t.Context()

	const insert = `INSERT INTO parcel_pricing.source_connector_binding
		(tenant_id, series_id, binding_version, connector_kind, source_identifier, source_locator, series_kind,
		 quote_basis_id, quote_basis_version, quote_basis_digest, registrant, review_exemption)
	 VALUES ($1, $2, 'b1', 'FILE', 'src', 'loc', $3, $4, $5, $6, 'reg', $7)`

	cases := map[string][]any{
		"未知序列种类":      {"tenant-a", "s-bad-1", "MOON_PHASE", "p", "v1", "d", "EXEMPT"},
		"未知免复核取值":     {"tenant-a", "s-bad-2", "FUEL_RATE", nil, nil, nil, "MAYBE"},
		"裸汇率":         {"tenant-a", "s-bad-3", "EXCHANGE_RATE", nil, nil, nil, "EXEMPT"},
		"口径缺 digest":  {"tenant-a", "s-bad-4", "EXCHANGE_RATE", "p", "v1", nil, "EXEMPT"},
		"空序列标识":       {"tenant-a", " ", "FUEL_RATE", nil, nil, nil, "UNDECLARED"},
		"空免复核（无默认可退）": {"tenant-a", "s-bad-6", "FUEL_RATE", nil, nil, nil, ""},
	}
	for label, args := range cases {
		if _, err := pool.Exec(ctx, insert, args...); err == nil {
			t.Fatalf("一行「%s」溜进了绑定登记册", label)
		}
	}
	if _, err := pool.Exec(ctx, insert, "tenant-a", "s-ok", "FUEL_RATE", nil, nil, nil, "UNDECLARED"); err != nil {
		t.Fatalf("合法的燃油绑定行被拒：%v", err)
	}
}

// TestLoadLatestVersionPicksTheMostRecentlyRegistered 证「前一版」读口：取最近登记的一版而不看
// 复核；没有版本答 false；跨租户不可见。
func TestLoadLatestVersionPicksTheMostRecentlyRegistered(t *testing.T) {
	register, transactor, _ := newSeriesRegister(t)
	ctx := t.Context()
	tenant := evaluationValue(t, domain.NewTenantID, "tenant-a")

	if _, found, err := register.LoadLatestVersion(ctx, tenant, "SYN-PRC-FUEL-LATEST"); err != nil || found {
		t.Fatalf("没登记过却有最新版：found=%v err=%v", found, err)
	}
	registerSeries(t, register, transactor, ctx, fuelRegistration(t, "tenant-a", "SYN-PRC-FUEL-LATEST", "v1"))
	registerSeries(t, register, transactor, ctx, fuelRegistration(t, "tenant-a", "SYN-PRC-FUEL-LATEST", "v2",
		registerPeriod(t, registerWeekOne, registerWeekTwo, "0.22", "SYN-EVIDENCE/fuel-2026-W32"),
		registerPeriod(t, registerWeekTwo, time.Time{}, "0.24", "SYN-EVIDENCE/fuel-2026-W33")))

	latest, found, err := register.LoadLatestVersion(ctx, tenant, "SYN-PRC-FUEL-LATEST")
	if err != nil || !found || latest.Reference().Version() != "v2" {
		t.Fatalf("最新版 = %s found=%v err=%v，想要 v2", latest.Reference().Version(), found, err)
	}
	if len(latest.Periods()) != 2 {
		t.Fatalf("读回的最新版期次数 = %d", len(latest.Periods()))
	}
	if _, found, err := register.LoadLatestVersion(ctx, evaluationValue(t, domain.NewTenantID, "tenant-b"), "SYN-PRC-FUEL-LATEST"); err != nil || found {
		t.Fatalf("别的租户看见了该序列：found=%v err=%v", found, err)
	}
	if _, _, err := register.LoadLatestVersion(ctx, tenant, ""); err == nil {
		t.Fatal("空序列标识被接受了")
	}
}
