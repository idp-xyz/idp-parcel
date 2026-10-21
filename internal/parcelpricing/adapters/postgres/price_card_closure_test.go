package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	adapter "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证价卡收尾（ADR-0234）：截止与退役落在两张只增的册上、版本行一字不动；按范围选卡的两口
// （ResolveInForce、LoadApplicable）照收尾之后的口径答，重放用的点读口不折；重放不多写一行；不成立的收尾一行不写；
// 同一旧版上并发的两次收尾排在旧版行锁上；收尾同笔交一封与两版各自登记那一封都不同的「参数已登记」。夹具全是合成价卡，
// 证据只记 S。

const (
	closurePlanID = "SYN-PLAN-CLOSE"
	closureScope  = "SYN-SCOPE-CLOSE"
)

var (
	closureJanuary   = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	closureJune      = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	closureSeptember = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	closureRegistry  = time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
)

// closedCardPlan 造同一方案下的一版 BUY 卡：起止由用例给（ends 为零即无上界），结构可绑序列。
func closedCardPlan(t *testing.T, version, scope string, starts, ends time.Time, structures domain.PricingPlanStructures) domain.PricingPlanVersion {
	t.Helper()
	currency := evaluationValue(t, domain.NewCurrency, "USD")
	entry, err := domain.NewRateEntry(evaluationValue(t, domain.NewRateEntryID, "entry-close-"+version), "Z1",
		catalogWeight(t, "0"), catalogWeight(t, "10"), catalogMoney(t, "5.00", currency))
	if err != nil {
		t.Fatalf("构造费率段：%v", err)
	}
	period, err := domain.NewEffectivePeriod(starts, ends)
	if err != nil {
		t.Fatalf("构造适用期：%v", err)
	}
	reference := func(kind domain.ArtifactKind, id string) domain.VersionReference {
		value, err := domain.NewVersionReferenceIdentity(kind, id, version)
		if err != nil {
			t.Fatalf("构造引用 %s：%v", id, err)
		}
		return value
	}
	table, err := domain.NewRateTableVersion(reference(domain.ArtifactRateTable, "table-close"), domain.RateTableFamilyWeightZone,
		currency, domain.WeightUnitKilogram, period, []domain.RateEntry{entry})
	if err != nil {
		t.Fatalf("构造价表：%v", err)
	}
	rounding, err := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, catalogWeight(t, "0.5"))
	if err != nil {
		t.Fatalf("构造取整策略：%v", err)
	}
	weightPolicy, err := domain.NewPricingWeightPolicy(reference(domain.ArtifactWeightPolicy, "weight-close"),
		domain.PricingWeightActualOnly, rounding, nil)
	if err != nil {
		t.Fatalf("构造计价重策略：%v", err)
	}
	plan, err := domain.NewPricingPlanVersion(reference(domain.ArtifactPricingPlan, closurePlanID),
		evaluationValue(t, domain.NewPricingScopeID, scope), domain.PricingDirectionBuy, domain.PricingPurposeSupplierCost,
		evaluationValue(t, domain.NewChargeCode, "BASE_FREIGHT"), period, table, weightPolicy, nil, structures)
	if err != nil {
		t.Fatalf("构造价卡 %s：%v", version, err)
	}
	return plan
}

type closureFixture struct {
	db         *bentopg.DB
	catalog    *adapter.PriceCards
	transactor bentoapp.Transactor
	handler    *application.ClosePriceCardVersionHandler
}

func newClosureFixture(t *testing.T) closureFixture {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	catalog, err := adapter.NewPriceCards(db)
	if err != nil {
		t.Fatalf("构造价卡仓储：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	clock := evaluationHandoffClock{at: closureRegistry}
	handoffs, err := adapter.NewOutboxEvaluationResumptionHandoffs(db, store, clock)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := application.NewClosePriceCardVersionHandler(application.ClosePriceCardVersionDeps{
		Registry: catalog, Registered: handoffs, Clock: clock,
	})
	if err != nil {
		t.Fatal(err)
	}
	return closureFixture{db: db, catalog: catalog, transactor: db.Transactor(), handler: handler}
}

func (fixture closureFixture) register(t *testing.T, tenant string, plan domain.PricingPlanVersion) {
	t.Helper()
	if outcome := registerCard(t, fixture.catalog, fixture.transactor, t.Context(), cardRegistration(t, tenant, plan)); outcome != ports.PriceCardRegistered {
		t.Fatalf("登记 %s = %d", plan.Reference().Version(), outcome)
	}
}

func closureTarget(t *testing.T, tenant, version, successor string) application.PriceCardClosureTarget {
	t.Helper()
	return application.PriceCardClosureTarget{
		Tenant: evaluationValue(t, domain.NewTenantID, tenant), PlanID: closurePlanID, Version: version, Successor: successor,
	}
}

func (fixture closureFixture) cut(t *testing.T, ctx context.Context, tenant, version, successor, reference string) application.PriceCardClosureResult {
	t.Helper()
	command := application.CutPriceCardVersionCommand{
		PriceCardClosureTarget: closureTarget(t, tenant, version, successor), Reference: reference,
	}
	var result application.PriceCardClosureResult
	inCatalogTx(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		var err error
		result, err = fixture.handler.Cut(txCtx, command)
		return err
	})
	return result
}

func (fixture closureFixture) retire(t *testing.T, tenant, version, successor, reference string) application.PriceCardClosureResult {
	t.Helper()
	command := application.RetirePriceCardVersionCommand{
		PriceCardClosureTarget: closureTarget(t, tenant, version, successor), Reference: reference,
	}
	var result application.PriceCardClosureResult
	inCatalogTx(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var err error
		result, err = fixture.handler.RetireAtCoveringSuccessor(txCtx, command)
		return err
	})
	return result
}

func (fixture closureFixture) resolve(t *testing.T, tenant string, at time.Time) ports.PriceCardInForceResolution {
	t.Helper()
	resolution, err := fixture.catalog.ResolveInForce(t.Context(), evaluationValue(t, domain.NewTenantID, tenant),
		evaluationValue(t, domain.NewPricingScopeID, closureScope), domain.PricingDirectionBuy, domain.PricingPurposeSupplierCost, at)
	if err != nil {
		t.Fatalf("解析在用价卡：%v", err)
	}
	return resolution
}

// resolvesTo 断言这一刻选中的恰是这一版：ResolveInForce 与 LoadApplicable 两口同一口径。
func (fixture closureFixture) resolvesTo(t *testing.T, at time.Time, version string) {
	t.Helper()
	resolution := fixture.resolve(t, "tenant-a", at)
	if resolution.Outcome != ports.PriceCardVersionInForce || resolution.Plan.Reference().Version() != version {
		t.Fatalf("%s 在用解析 = %s %v，want 恰选中 %s", at.Format(time.RFC3339), resolution.Outcome, resolution.Candidates, version)
	}
	plans, err := fixture.catalog.LoadApplicable(t.Context(), evaluationValue(t, domain.NewTenantID, "tenant-a"),
		domain.PricingDirectionBuy, evaluationValue(t, domain.NewPricingScopeID, closureScope), at)
	if err != nil || len(plans) != 1 || plans[0].Reference().Version() != version {
		t.Fatalf("%s 装载适用价卡 = %d 张 err=%v，want 恰一张 %s", at.Format(time.RFC3339), len(plans), err, version)
	}
}

func (fixture closureFixture) count(t *testing.T, sql string) int {
	t.Helper()
	querier, err := fixture.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := querier.QueryRow(t.Context(), sql).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (fixture closureFixture) closureEnvelopes(t *testing.T) int {
	t.Helper()
	return fixture.count(t, `SELECT count(*) FROM `+migrate.SchemaBento+`.outbox
		WHERE event_type = 'parcel-pricing.pricing-parameter.registered' AND event_id LIKE 'price-card-closure/%'`)
}

// Covers: AT-PP-007——同一方案两版重叠时按范围选卡答适用冲突；把旧版截止在后继起点之后，截止点之前选旧版、起选后继，
// 两口同答；版本行与重放用的点读口一字不动；截止同笔交一封收尾的「参数已登记」。
func TestASuccessorCutEndsTheApplicabilityConflictInTheStore(t *testing.T) {
	fixture := newClosureFixture(t)
	fixture.register(t, "tenant-a", closedCardPlan(t, "v1", closureScope, closureJanuary, time.Time{}, domain.PricingPlanStructures{}))
	fixture.register(t, "tenant-a", closedCardPlan(t, "v2", closureScope, closureJune, time.Time{}, domain.PricingPlanStructures{}))

	if got := fixture.resolve(t, "tenant-a", closureSeptember); got.Outcome != ports.PriceCardApplicabilityConflict || len(got.Candidates) != 2 {
		t.Fatalf("截止之前 = %s %v，want 两版适用冲突", got.Outcome, got.Candidates)
	}

	result := fixture.cut(t, t.Context(), "tenant-a", "v1", "v2", "SYN-CUT-V1-FOR-V2")
	cut, echoed := result.Cut()
	if result.Outcome() != application.PriceCardClosureRecorded || !echoed || !cut.SelectionEndsAt().Equal(closureJune) {
		t.Fatalf("截止 = %s（%v），截到 %s", result.Outcome(), result.Cause(), cut.SelectionEndsAt())
	}
	fixture.resolvesTo(t, closureJune.Add(-time.Second), "v1")
	fixture.resolvesTo(t, closureJune, "v2")
	fixture.resolvesTo(t, closureSeptember, "v2")

	if n := fixture.count(t, `SELECT count(*) FROM parcel_pricing.price_card_version
		WHERE plan_id = 'SYN-PLAN-CLOSE' AND plan_version = 'v1' AND effective_to IS NULL`); n != 1 {
		t.Fatalf("版本行被改写：发布时无上界的 v1 剩 %d 行", n)
	}
	original, found, err := fixture.catalog.FindByReference(t.Context(), evaluationValue(t, domain.NewTenantID, "tenant-a"),
		evaluationValue(t, func(version string) (domain.VersionReference, error) {
			return domain.NewVersionReferenceIdentity(domain.ArtifactPricingPlan, closurePlanID, version)
		}, "v1"))
	if err != nil || !found || !original.EffectivePeriod().EndsAt().IsZero() {
		t.Fatalf("重放读回 v1 found=%v err=%v，want 原样读回发布时的那一版", found, err)
	}
	if n := fixture.closureEnvelopes(t); n != 1 {
		t.Fatalf("收尾的参数已登记 %d 封，want 1", n)
	}
}

// Covers: AT-PP-008——同起点重发：退役旧版之后任何时点都只选后继；退役不改版本行；已退役的那一版不再算作绑着序列的卡。
func TestARetirementLeavesOnlyTheCoveringSuccessorInTheStore(t *testing.T) {
	fixture := newClosureFixture(t)
	fixture.register(t, "tenant-a", closedCardPlan(t, "v1", closureScope, closureJanuary, time.Time{}, seriesBound(t, "SYN-SERIES-CLOSE")))
	fixture.register(t, "tenant-a", closedCardPlan(t, "v2", closureScope, closureJanuary, time.Time{}, domain.PricingPlanStructures{}))
	tenant := evaluationValue(t, domain.NewTenantID, "tenant-a")
	if scopes, err := fixture.catalog.FindPricedScopesBinding(t.Context(), tenant, ports.PricingParameterReferenceSeries, "SYN-SERIES-CLOSE"); err != nil || len(scopes) != 1 {
		t.Fatalf("退役之前绑序列的卡 = %v err=%v", scopes, err)
	}

	result := fixture.retire(t, "tenant-a", "v1", "v2", "SYN-RETIRE-V1")
	retirement, echoed := result.Retirement()
	if result.Outcome() != application.PriceCardClosureRecorded || !echoed || !retirement.RetiredAt().Equal(closureRegistry) {
		t.Fatalf("退役 = %s（%v），时刻 %s", result.Outcome(), result.Cause(), retirement.RetiredAt())
	}
	fixture.resolvesTo(t, closureJanuary, "v2")
	fixture.resolvesTo(t, closureSeptember, "v2")
	if scopes, err := fixture.catalog.FindPricedScopesBinding(t.Context(), tenant, ports.PricingParameterReferenceSeries, "SYN-SERIES-CLOSE"); err != nil || len(scopes) != 0 {
		t.Fatalf("退役之后绑序列的卡 = %v err=%v，want 已退役的那一版不算", scopes, err)
	}

	again := fixture.retire(t, "tenant-a", "v1", "v2", "SYN-RETIRE-V1")
	if replayed, _ := again.Retirement(); again.Outcome() != application.PriceCardClosureAlreadyOnRegister || !replayed.RetiredAt().Equal(closureRegistry) {
		t.Fatalf("退役重放 = %s，时刻 %s", again.Outcome(), replayed.RetiredAt())
	}
	if other := fixture.retire(t, "tenant-a", "v1", "v2", "SYN-RETIRE-V1-OTHER"); other.Outcome() != application.PriceCardClosureNotAccepted ||
		!errors.Is(other.Cause(), domain.ErrPriceCardRetiredOtherwise) {
		t.Fatalf("换依据再退 = %s（%v）", other.Outcome(), other.Cause())
	}
	if n := fixture.count(t, `SELECT count(*) FROM parcel_pricing.price_card_version_retirement`); n != 1 {
		t.Fatalf("退役册 %d 行，want 1", n)
	}
	if n := fixture.closureEnvelopes(t); n != 1 {
		t.Fatalf("收尾的参数已登记 %d 封，want 1（重放同一封）", n)
	}
}

// Covers: AT-PP-009——同一次截止重放认出册上那一条，一行不多写、信封不多一封；换一份依据截同一处不成立。
func TestReplayingASuccessorCutWritesNothingMore(t *testing.T) {
	fixture := newClosureFixture(t)
	fixture.register(t, "tenant-a", closedCardPlan(t, "v1", closureScope, closureJanuary, time.Time{}, domain.PricingPlanStructures{}))
	fixture.register(t, "tenant-a", closedCardPlan(t, "v2", closureScope, closureJune, time.Time{}, domain.PricingPlanStructures{}))
	if first := fixture.cut(t, t.Context(), "tenant-a", "v1", "v2", "SYN-CUT"); first.Outcome() != application.PriceCardClosureRecorded {
		t.Fatalf("首次截止 = %s（%v）", first.Outcome(), first.Cause())
	}
	again := fixture.cut(t, t.Context(), "tenant-a", "v1", "v2", "SYN-CUT")
	if cut, _ := again.Cut(); again.Outcome() != application.PriceCardClosureAlreadyOnRegister || !cut.SelectionEndsAt().Equal(closureJune) {
		t.Fatalf("截止重放 = %s，截到 %s", again.Outcome(), cut.SelectionEndsAt())
	}
	if other := fixture.cut(t, t.Context(), "tenant-a", "v1", "v2", "SYN-CUT-OTHER"); other.Outcome() != application.PriceCardClosureNotAccepted ||
		!errors.Is(other.Cause(), domain.ErrPriceCardSuccessorCutOutsideSelection) {
		t.Fatalf("换依据截同一处 = %s（%v）", other.Outcome(), other.Cause())
	}
	if n := fixture.count(t, `SELECT count(*) FROM parcel_pricing.price_card_version_cut`); n != 1 {
		t.Fatalf("截止册 %d 行，want 1", n)
	}
	if n := fixture.closureEnvelopes(t); n != 1 {
		t.Fatalf("收尾的参数已登记 %d 封，want 1", n)
	}
}

// Covers: AT-PP-010——不成立的收尾答未受理，一行不写、一封不交：旧版或后继不在册、他租户、后继换了计价范围、截止落在旧版
// 起点上。
func TestAPriceCardClosureThatDoesNotHoldWritesNothing(t *testing.T) {
	fixture := newClosureFixture(t)
	fixture.register(t, "tenant-a", closedCardPlan(t, "v1", closureScope, closureJanuary, time.Time{}, domain.PricingPlanStructures{}))
	fixture.register(t, "tenant-a", closedCardPlan(t, "v2", "SYN-SCOPE-ELSEWHERE", closureJune, time.Time{}, domain.PricingPlanStructures{}))
	fixture.register(t, "tenant-a", closedCardPlan(t, "v3", closureScope, closureJanuary, time.Time{}, domain.PricingPlanStructures{}))
	for _, refused := range []struct {
		name, tenant, version, successor string
		want                             error
	}{
		{"旧版不在册", "tenant-a", "v9", "v3", domain.ErrPriceCardClosureVersionNotRegistered},
		{"后继不在册", "tenant-a", "v1", "v9", domain.ErrPriceCardClosureVersionNotRegistered},
		{"他租户", "tenant-b", "v1", "v3", domain.ErrPriceCardClosureVersionNotRegistered},
		{"后继换了计价范围", "tenant-a", "v1", "v2", domain.ErrPriceCardClosureAcrossScopes},
		{"同起点用截止", "tenant-a", "v1", "v3", domain.ErrPriceCardSuccessorCutOutsideSelection},
	} {
		t.Run(refused.name, func(t *testing.T) {
			result := fixture.cut(t, t.Context(), refused.tenant, refused.version, refused.successor, "SYN-CUT")
			if result.Outcome() != application.PriceCardClosureNotAccepted || !errors.Is(result.Cause(), refused.want) {
				t.Fatalf("截止 = %s（%v），want 未受理 %v", result.Outcome(), result.Cause(), refused.want)
			}
		})
	}
	if n := fixture.count(t, `SELECT count(*) FROM parcel_pricing.price_card_version_cut`) +
		fixture.count(t, `SELECT count(*) FROM parcel_pricing.price_card_version_retirement`); n != 0 {
		t.Fatalf("不成立的收尾写了 %d 行", n)
	}
	if n := fixture.closureEnvelopes(t); n != 0 {
		t.Fatalf("不成立的收尾交了 %d 封", n)
	}
}

// Covers: AT-PP-011——同一旧版上并发的两次截止：后到者停在旧版行锁上，先到者提交之后它读到已截止的旧版，后继起点已不在旧版
// 此刻的选用之内，答未受理；册上只一行。拆掉编排里先锁一步，后到者照着它读到的未截止的旧版判过、写下第二条，用例转红。
func TestConcurrentCutsOnOnePredecessorQueueOnItsRow(t *testing.T) {
	fixture := newClosureFixture(t)
	fixture.register(t, "tenant-a", closedCardPlan(t, "v1", closureScope, closureJanuary, time.Time{}, domain.PricingPlanStructures{}))
	fixture.register(t, "tenant-a", closedCardPlan(t, "v2", closureScope, closureJune, time.Time{}, domain.PricingPlanStructures{}))
	fixture.register(t, "tenant-a", closedCardPlan(t, "v3", closureScope, closureSeptember, time.Time{}, domain.PricingPlanStructures{}))
	ctx := t.Context()

	toV2 := application.CutPriceCardVersionCommand{PriceCardClosureTarget: closureTarget(t, "tenant-a", "v1", "v2"), Reference: "SYN-CUT-V2"}
	toV3 := application.CutPriceCardVersionCommand{PriceCardClosureTarget: closureTarget(t, "tenant-a", "v1", "v3"), Reference: "SYN-CUT-V3"}
	firstCut := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)
	var first, second application.PriceCardClosureResult
	go func() {
		firstDone <- fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			var err error
			first, err = fixture.handler.Cut(txCtx, toV2)
			close(firstCut)
			<-release
			return err
		})
	}()
	<-firstCut
	go func() {
		secondDone <- fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			var err error
			second, err = fixture.handler.Cut(txCtx, toV3)
			return err
		})
	}()

	select {
	case err := <-secondDone:
		close(release)
		<-firstDone
		t.Fatalf("后到者在先到者提交前就返回了（err=%v，outcome=%s）——没有停在旧版的版本行锁上", err, second.Outcome())
	case <-time.After(150 * time.Millisecond):
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("先到者事务失败：%v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("后到者事务失败：%v", err)
	}
	if first.Outcome() != application.PriceCardClosureRecorded {
		t.Fatalf("先到者 = %s（%v），want REGISTERED", first.Outcome(), first.Cause())
	}
	if second.Outcome() != application.PriceCardClosureNotAccepted || !errors.Is(second.Cause(), domain.ErrPriceCardSuccessorCutOutsideSelection) {
		t.Fatalf("后到者 = %s（%v），want NOT_ACCEPTED / ErrPriceCardSuccessorCutOutsideSelection", second.Outcome(), second.Cause())
	}
	if n := fixture.count(t, `SELECT count(*) FROM parcel_pricing.price_card_version_cut`); n != 1 {
		t.Fatalf("截止册 %d 行，want 1", n)
	}
}

// Covers: AT-PP-010 的交接口半边——收尾那一封与两版各自登记那一封标识不同（同一个标识会被当成重发吞掉）；截止与退役各一封；
// 缺收尾格就拒。
func TestAClosureIsHandedOffApartFromTheCardRegistrations(t *testing.T) {
	handoffs, db := newResumptionHandoffs(t)
	tenant := evaluationValue(t, domain.NewTenantID, "tenant-a")
	scope := evaluationValue(t, domain.NewPricingScopeID, closureScope)
	v1, err := domain.NewVersionReferenceIdentity(domain.ArtifactPricingPlan, closurePlanID, "v1")
	if err != nil {
		t.Fatal(err)
	}
	registration := ports.PricingParameterRegisteredIntent{
		Tenant: tenant, Kind: ports.PricingParameterPriceCard, Registration: v1, Scope: scope, Direction: domain.PricingDirectionBuy,
	}
	cut, retirement := registration, registration
	cut.Closure = ports.PriceCardClosureOccasion{Kind: ports.PriceCardCutClosure, Successor: "v2", Reference: "SYN-CUT"}
	retirement.Closure = ports.PriceCardClosureOccasion{Kind: ports.PriceCardRetirementClosure, Successor: "v2", Reference: "SYN-CUT"}
	if err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		for _, intent := range []ports.PricingParameterRegisteredIntent{registration, cut, retirement, cut} {
			if err := handoffs.HandOffPricingParameterRegistered(ctx, intent); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	rows, err := querier.Query(t.Context(), `SELECT event_id FROM `+migrate.SchemaBento+`.outbox
		WHERE event_type = 'parcel-pricing.pricing-parameter.registered' ORDER BY event_id`)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	closures := 0
	for _, id := range ids {
		if strings.HasPrefix(id, "price-card-closure/") {
			closures++
		}
	}
	if len(ids) != 3 || closures != 2 {
		t.Fatalf("信封 = %v，want 登记一封、截止与退役各一封（重放吞掉）", ids)
	}
	for _, payload := range outboxOfType(t, db, "parcel-pricing.pricing-parameter.registered") {
		if payload.payload["registrationKind"] != "PRICE_CARD" || payload.payload["pricingScope"] != closureScope ||
			payload.payload["direction"] != "BUY" || len(payload.payload) != 4 {
			t.Fatalf("收尾那一封的载荷与登记那一封不同形：%v", payload.payload)
		}
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		blank := cut
		blank.Closure.Reference = ""
		if err := handoffs.HandOffPricingParameterRegistered(ctx, blank); err == nil {
			return errors.New("缺依据的收尾交了出去")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
