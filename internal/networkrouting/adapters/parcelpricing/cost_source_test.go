package parcelpricing_test

import (
	"context"
	"testing"
	"time"

	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"

	adapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/parcelpricing"
)

// 本文件钉票 routing-first-cut/10 的完成判据三格与 ADR-0148 决定四的合成纪律：
// 两候选成本不同各自出价、任一段不可计价即该候选出局（不可计价格）、缺依据即待判断、
// 缺口不折零，以及「先不舍入相加、合计后取整一次」（逐段先取整会算出另一个并列答案）。

var planPeriod = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func TestTwoCandidatesPriceIndependentlyWithTheirOwnCards(t *testing.T) {
	source := newAdapter(t,
		map[string][]nrports.LineSegmentCostBasis{
			"line-cheap":  {{SegmentIndex: 0, Kind: nrports.SupplierBuyPlanBasis, Reference: "plan-cheap/v1"}},
			"line-costly": {{SegmentIndex: 0, Kind: nrports.SupplierBuyPlanBasis, Reference: "plan-costly/v1"}},
		},
		map[string]ppdomain.PricingPlanVersion{
			"plan-cheap/v1":  syntheticBuyPlan(t, "plan-cheap", "v1", "5"),
			"plan-costly/v1": syntheticBuyPlan(t, "plan-costly", "v1", "8"),
		},
	)

	costs, err := source.LoadCandidateCosts(context.Background(), judgmentKey(t), evidenceWithComparison(t, "USD",
		pathOf(t, "line-cheap", 1), pathOf(t, "line-costly", 1)))
	if err != nil {
		t.Fatalf("取候选成本：%v", err)
	}
	if len(costs.Facts) != 2 {
		t.Fatalf("事实 %d 条，want 2（逐候选恰一条）", len(costs.Facts))
	}
	cheap := factOf(t, costs.Facts, "line-cheap")
	costly := factOf(t, costs.Facts, "line-costly")
	if cheap.State().String() != "PRICED" || cheap.AmountMinor() != 500 {
		t.Fatalf("cheap = %s %d minor, want PRICED 500", cheap.State(), cheap.AmountMinor())
	}
	if costly.State().String() != "PRICED" || costly.AmountMinor() != 800 {
		t.Fatalf("costly = %s %d minor, want PRICED 800", costly.State(), costly.AmountMinor())
	}
	for _, citation := range costs.Citations {
		if citation.EvaluationID() == "" || citation.PlanReference() == "" ||
			citation.ComparisonCurrency() != "USD" {
			t.Fatalf("出处缺格：%+v", citation)
		}
		if _, stated := citation.PolicyReference(); stated {
			t.Fatalf("外包段不该带政策引用：%+v", citation)
		}
	}
}

// Covers: 决定四.3 的取整纪律——两段 5.005 相加若逐段先取整得 10.02，先不舍入相加、
// 合计后取整一次得 10.01。并列判断会被这两种算法改变答案。
func TestLegAmountsAreSummedBeforeTheSingleRounding(t *testing.T) {
	source := newAdapter(t,
		map[string][]nrports.LineSegmentCostBasis{
			"line-two": {
				{SegmentIndex: 0, Kind: nrports.SupplierBuyPlanBasis, Reference: "plan-half/v1"},
				{SegmentIndex: 1, Kind: nrports.SupplierBuyPlanBasis, Reference: "plan-half/v1"},
			},
		},
		map[string]ppdomain.PricingPlanVersion{
			"plan-half/v1": syntheticBuyPlan(t, "plan-half", "v1", "5.005"),
		},
	)

	costs, err := source.LoadCandidateCosts(context.Background(), judgmentKey(t), evidenceWithComparison(t, "USD",
		pathOf(t, "line-two", 2)))
	if err != nil {
		t.Fatalf("取候选成本：%v", err)
	}
	fact := factOf(t, costs.Facts, "line-two")
	if fact.State().String() != "PRICED" || fact.AmountMinor() != 1001 {
		t.Fatalf("fact = %s %d minor, want PRICED 1001（先求和后取整）", fact.State(), fact.AmountMinor())
	}
}

// Covers: 比较币种异于卡币种时，取数侧拿的是比较币种金额——换算由提供方按同一读数完成，
// 比较金额不过卡的取整点。
func TestAComparisonCurrencyReachesThroughTheLegs(t *testing.T) {
	plan := syntheticBuyPlanWithRate(t, "plan-cny", "v1", "10")
	source := newAdapter(t,
		map[string][]nrports.LineSegmentCostBasis{
			"line-cny": {{SegmentIndex: 0, Kind: nrports.SupplierBuyPlanBasis, Reference: "plan-cny/v1"}},
		},
		map[string]ppdomain.PricingPlanVersion{"plan-cny/v1": plan},
	)
	source.series = []ppdomain.ReferenceSeriesValue{cnyExchangeReading(t)}

	costs, err := source.LoadCandidateCosts(context.Background(), judgmentKey(t), evidenceWithComparison(t, "CNY",
		pathOf(t, "line-cny", 1)))
	if err != nil {
		t.Fatalf("取候选成本：%v", err)
	}
	fact := factOf(t, costs.Facts, "line-cny")
	if fact.State().String() != "PRICED" || fact.AmountMinor() != 7200 {
		t.Fatalf("fact = %s %d minor, want PRICED 7200（10 USD × 7.2 汇率）", fact.State(), fact.AmountMinor())
	}
	if len(costs.Citations) != 1 || costs.Citations[0].ComparisonCurrency() != "CNY" ||
		costs.Citations[0].ComparisonAmount() != "72" {
		t.Fatalf("出处应记全精度比较金额 72 CNY：%+v", costs.Citations)
	}
}

// Covers: 判据「一段不可计价 → 该候选出局」：卡在判断时点已不适用（适用期之外），评价落
// 内容冲突——单段候选整体不可计价，不以零金额顶替。
func TestAnUnpriceableLegLeavesTheCandidateUnpriceable(t *testing.T) {
	source := newAdapter(t,
		map[string][]nrports.LineSegmentCostBasis{
			"line-stale": {{SegmentIndex: 0, Kind: nrports.SupplierBuyPlanBasis, Reference: "plan-stale/v1"}},
		},
		map[string]ppdomain.PricingPlanVersion{
			"plan-stale/v1": syntheticBuyPlan(t, "plan-stale", "v1", "3"),
		},
	)
	source.now = planPeriod.AddDate(2, 0, 0)

	costs, err := source.LoadCandidateCosts(context.Background(), judgmentKey(t), evidenceWithComparison(t, "USD",
		pathOf(t, "line-stale", 1)))
	if err != nil {
		t.Fatalf("取候选成本：%v", err)
	}
	fact := factOf(t, costs.Facts, "line-stale")
	if fact.State().String() != "UNPRICEABLE" {
		t.Fatalf("fact = %s, want UNPRICEABLE（整条候选只此一段）", fact.State())
	}
}

// Covers: 缺依据不折零——没挂依据的段如实待判断；政策没登正文或方案不在册同样待判断。
func TestAMissingBasisLeavesTheCandidatePending(t *testing.T) {
	source := newAdapter(t,
		map[string][]nrports.LineSegmentCostBasis{},
		nil,
	)

	costs, err := source.LoadCandidateCosts(context.Background(), judgmentKey(t), evidenceWithComparison(t, "USD",
		pathOf(t, "line-bare", 2)))
	if err != nil {
		t.Fatalf("取候选成本：%v", err)
	}
	fact := factOf(t, costs.Facts, "line-bare")
	if fact.State().String() != "PENDING" {
		t.Fatalf("fact = %s, want PENDING", fact.State())
	}
	if len(costs.Citations) != 0 {
		t.Fatalf("缺依据的候选不该有出处：%+v", costs.Citations)
	}
}

// Covers: 没登比较币种的退路——退到各段评价总价、只在同币种下合成；异币种即停下
// （待判断），不换算不编汇率。
func TestWithoutAComparisonCurrencyMixedCurrenciesStop(t *testing.T) {
	source := newAdapter(t,
		map[string][]nrports.LineSegmentCostBasis{
			"line-mixed": {
				{SegmentIndex: 0, Kind: nrports.SupplierBuyPlanBasis, Reference: "plan-usd/v1"},
				{SegmentIndex: 1, Kind: nrports.SupplierBuyPlanBasis, Reference: "plan-eur/v1"},
			},
		},
		map[string]ppdomain.PricingPlanVersion{
			"plan-usd/v1": syntheticBuyPlanIn(t, "plan-usd", "v1", "5", "USD"),
			"plan-eur/v1": syntheticBuyPlanIn(t, "plan-eur", "v1", "4", "EUR"),
		},
	)

	costs, err := source.LoadCandidateCosts(context.Background(), judgmentKey(t), evidenceWithComparison(t, "",
		pathOf(t, "line-mixed", 2)))
	if err != nil {
		t.Fatalf("取候选成本：%v", err)
	}
	if fact := factOf(t, costs.Facts, "line-mixed"); fact.State().String() != "PENDING" {
		t.Fatalf("fact = %s, want PENDING（异币种停下不比）", fact.State())
	}
}

// Covers: 内部段经解析桥拿方案引用，出处带政策引用；方向配错的方案响亮拒译。
func TestAnInternalLegResolvesThroughThePolicy(t *testing.T) {
	source := newAdapter(t,
		map[string][]nrports.LineSegmentCostBasis{
			"line-internal": {{SegmentIndex: 0, Kind: nrports.InternalPolicyBasis, Reference: "policy-internal/v1"}},
		},
		map[string]ppdomain.PricingPlanVersion{
			"plan-internal/v1": syntheticInternalPlan(t, "plan-internal", "v1", "6"),
		},
	)
	source.internal = map[string]string{"policy-internal/v1": "plan-internal/v1"}

	costs, err := source.LoadCandidateCosts(context.Background(), judgmentKey(t), evidenceWithComparison(t, "USD",
		pathOf(t, "line-internal", 1)))
	if err != nil {
		t.Fatalf("取候选成本：%v", err)
	}
	fact := factOf(t, costs.Facts, "line-internal")
	if fact.State().String() != "PRICED" || fact.AmountMinor() != 600 {
		t.Fatalf("fact = %s %d, want PRICED 600", fact.State(), fact.AmountMinor())
	}
	if len(costs.Citations) != 1 {
		t.Fatalf("出处 %d 条，want 1", len(costs.Citations))
	}
	if policy, stated := costs.Citations[0].PolicyReference(); !stated || policy != "policy-internal/v1" {
		t.Fatalf("内部段出处应带政策引用：%+v", costs.Citations[0])
	}
}

// Covers: 票 routing-first-cut/17 判据一——逐段区域未配置时该段待判断，候选随之缺成本依据：取数侧照常交回事实，
// 不整判断答 ErrRouteCostSourceNotConfigured（那会让编排形成 COST_SOURCE_NOT_CONFIGURED），也不把缺的成本折成零。
func TestAnUnconfiguredLegZoneLeavesTheCandidatePendingWithoutStoppingTheJudgment(t *testing.T) {
	source := newAdapter(t,
		map[string][]nrports.LineSegmentCostBasis{
			"line-x": {{SegmentIndex: 0, Kind: nrports.SupplierBuyPlanBasis, Reference: "plan-x/v1"}},
		},
		map[string]ppdomain.PricingPlanVersion{"plan-x/v1": syntheticBuyPlan(t, "plan-x", "v1", "5")},
	)
	source.zonesConfigured = false

	costs, err := source.LoadCandidateCosts(context.Background(), judgmentKey(t), evidenceWithComparison(t, "USD",
		pathOf(t, "line-x", 1)))
	if err != nil {
		t.Fatalf("err = %v, want nil——区域未配置是这一段待判断，不是整条取数路径没接", err)
	}
	fact := factOf(t, costs.Facts, "line-x")
	if fact.State().String() != "PENDING" {
		t.Fatalf("fact = %s %d minor, want PENDING（缺成本依据不折零）", fact.State(), fact.AmountMinor())
	}
	if len(costs.Citations) != 0 {
		t.Fatalf("没评价的段不该有出处：%+v", costs.Citations)
	}
}

// Covers: 没有包裹事实时一段都不评价，候选待判断；同样不整判断停下。
func TestWithoutParcelFactsEveryLegStaysPending(t *testing.T) {
	source := newAdapter(t,
		map[string][]nrports.LineSegmentCostBasis{
			"line-x": {{SegmentIndex: 0, Kind: nrports.SupplierBuyPlanBasis, Reference: "plan-x/v1"}},
		},
		map[string]ppdomain.PricingPlanVersion{"plan-x/v1": syntheticBuyPlan(t, "plan-x", "v1", "5")},
	)
	source.hasFacts = false

	costs, err := source.LoadCandidateCosts(context.Background(), judgmentKey(t), evidenceWithComparison(t, "USD",
		pathOf(t, "line-x", 1)))
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if fact := factOf(t, costs.Facts, "line-x"); fact.State().String() != "PENDING" {
		t.Fatalf("fact = %s, want PENDING", fact.State())
	}
	if len(source.zoneQueries) != 0 {
		t.Fatalf("没有包裹事实还去折区域：%+v", source.zoneQueries)
	}
}

// Covers: 每段按自己的区域计价——段 1 的卡只在 Z1 有档、段 2 的卡只在 Z9 有档，区域逐段问、逐段进输入，两段都出价；
// 若一次判断只折一个区，总有一段落空档而整条候选不可计价。问区域时带候选与段号（从 1 起）。
func TestEachLegIsPricedInItsOwnZone(t *testing.T) {
	source := newAdapter(t,
		map[string][]nrports.LineSegmentCostBasis{
			"line-two-zones": {
				{SegmentIndex: 0, Kind: nrports.SupplierBuyPlanBasis, Reference: "plan-z1/v1"},
				{SegmentIndex: 1, Kind: nrports.SupplierBuyPlanBasis, Reference: "plan-z9/v1"},
			},
		},
		map[string]ppdomain.PricingPlanVersion{
			"plan-z1/v1": syntheticBuyPlan(t, "plan-z1", "v1", "5"),
			"plan-z9/v1": zoneLessBuyPlan(t, "plan-z9", 1),
		},
	)
	source.zoneByOrdinal = map[int]string{1: "Z1", 2: "Z9"}

	costs, err := source.LoadCandidateCosts(context.Background(), judgmentKey(t), evidenceWithComparison(t, "USD",
		pathOf(t, "line-two-zones", 2)))
	if err != nil {
		t.Fatalf("取候选成本：%v", err)
	}
	fact := factOf(t, costs.Facts, "line-two-zones")
	if fact.State().String() != "PRICED" || fact.AmountMinor() != 800 {
		t.Fatalf("fact = %s %d minor, want PRICED 800（Z1 段 5 + Z9 段 3）", fact.State(), fact.AmountMinor())
	}
	want := lineCandidate(t, "line-two-zones")
	if len(source.zoneQueries) != 2 ||
		source.zoneQueries[0].Candidate != want || source.zoneQueries[0].Ordinal != 1 ||
		source.zoneQueries[1].Candidate != want || source.zoneQueries[1].Ordinal != 2 {
		t.Fatalf("区域之问 = %+v，want 同一候选的第 1、2 段各一问", source.zoneQueries)
	}
}

// Covers: 包裹事实进每一段的输入——卡的档位只到 10kg：声明 1kg 时出价；声明 12kg 时落在档外，评价答待判断，候选随之
// 待判断。两种重量答得不同，说明评价读的是这份声明。
func TestTheParcelFactsReachEveryLegsEvaluation(t *testing.T) {
	for name, want := range map[string]struct {
		weight string
		state  string
	}{
		"档内": {"1", "PRICED"},
		"超档": {"12", "PENDING"},
	} {
		t.Run(name, func(t *testing.T) {
			source := newAdapter(t,
				map[string][]nrports.LineSegmentCostBasis{
					"line-x": {{SegmentIndex: 0, Kind: nrports.SupplierBuyPlanBasis, Reference: "plan-x/v1"}},
				},
				map[string]ppdomain.PricingPlanVersion{"plan-x/v1": syntheticBuyPlan(t, "plan-x", "v1", "5")},
			)
			source.facts.Weight = weightOf(t, want.weight)

			costs, err := source.LoadCandidateCosts(context.Background(), judgmentKey(t), evidenceWithComparison(t, "USD",
				pathOf(t, "line-x", 1)))
			if err != nil {
				t.Fatalf("取候选成本：%v", err)
			}
			if fact := factOf(t, costs.Facts, "line-x"); fact.State().String() != want.state {
				t.Fatalf("声明 %skg：fact = %s, want %s", want.weight, fact.State(), want.state)
			}
		})
	}
}

// testSource 是可调替身的容器：plan 表按「id/version」键取；包裹事实、逐段区域、序列读数与计价时点可逐格替换。
type testSource struct {
	*adapter.RouteCandidateCostAdapter
	bases           map[string][]nrports.LineSegmentCostBasis
	plans           map[string]ppdomain.PricingPlanVersion
	internal        map[string]string
	facts           adapter.ParcelFacts
	hasFacts        bool
	zonesConfigured bool
	zoneByOrdinal   map[int]string
	zoneQueries     []adapter.LegZoneQuery
	series          []ppdomain.ReferenceSeriesValue
	now             time.Time
}

func newAdapter(t *testing.T, bases map[string][]nrports.LineSegmentCostBasis, plans map[string]ppdomain.PricingPlanVersion) *testSource {
	t.Helper()
	source := &testSource{
		bases:           bases,
		plans:           plans,
		internal:        map[string]string{},
		facts:           adapter.ParcelFacts{Weight: weightOf(t, "1")},
		hasFacts:        true,
		zonesConfigured: true,
		now:             planPeriod.Add(6 * 30 * 24 * time.Hour),
	}
	source.RouteCandidateCostAdapter = adapter.NewRouteCandidateCostAdapter(adapter.RouteCandidateCostDeps{
		Bases:    source,
		Internal: source,
		Facts:    source,
		Zones:    source,
		Series:   source,
		Plans:    source,
		Clock:    source,
	})
	return source
}

func (source *testSource) ParcelFactsFor(
	_ context.Context, _ nrdomain.InitialRouteJudgmentKey,
) (adapter.ParcelFacts, bool, error) {
	return source.facts, source.hasFacts, nil
}

// ZoneFor 未单列的段一律答 Z1——合成价卡的档位都在 Z1。
func (source *testSource) ZoneFor(_ context.Context, query adapter.LegZoneQuery) (adapter.LegZone, bool, error) {
	source.zoneQueries = append(source.zoneQueries, query)
	if !source.zonesConfigured {
		return adapter.LegZone{}, false, nil
	}
	if zone, listed := source.zoneByOrdinal[query.Ordinal]; listed {
		return adapter.LegZone{Zone: zone}, true, nil
	}
	return adapter.LegZone{Zone: "Z1"}, true, nil
}

func (source *testSource) ReferenceSeriesFor(
	context.Context, nrdomain.InitialRouteJudgmentKey, time.Time,
) ([]ppdomain.ReferenceSeriesValue, error) {
	return source.series, nil
}

func (source *testSource) Now() time.Time { return source.now }

func (source *testSource) LoadLineCostBases(
	_ context.Context, _ nrdomain.TenantID, lineCode string, _ int32,
) ([]nrports.LineSegmentCostBasis, error) {
	return source.bases[lineCode], nil
}

func (source *testSource) PlanReferenceForInternalPolicy(
	_ context.Context, _ nrdomain.TenantID, policyReference string,
) (string, bool, error) {
	plan, found := source.internal[policyReference]
	return plan, found, nil
}

func (source *testSource) FindByReference(
	_ context.Context, _ ppdomain.TenantID, reference ppdomain.VersionReference,
) (ppdomain.PricingPlanVersion, bool, error) {
	plan, found := source.plans[reference.ID()+"/"+reference.Version()]
	return plan, found, nil
}

func evidenceWithComparison(t *testing.T, currency string, paths ...nrports.CandidatePath) nrports.InitialRouteEvidence {
	t.Helper()
	evidence := nrports.InitialRouteEvidence{Paths: paths}
	if currency != "" {
		evidence.HasComparisonCurrency = true
		evidence.ComparisonCurrency = currency
	}
	return evidence
}

func pathOf(t *testing.T, line string, legs int) nrports.CandidatePath {
	t.Helper()
	candidate := lineCandidate(t, line)
	return nrports.CandidatePath{Candidate: candidate, Legs: make([]nrdomain.PlannedLeg, legs)}
}

// lineCandidate 按目录折叠同一个铸法给线路的版本 1 铸候选标识。
func lineCandidate(t *testing.T, line string) nrdomain.CandidateID {
	t.Helper()
	candidate, err := nrdomain.NewLineCandidateID(line, 1)
	if err != nil {
		t.Fatal(err)
	}
	return candidate
}

func judgmentKey(t *testing.T) nrdomain.InitialRouteJudgmentKey {
	t.Helper()
	return nrdomain.InitialRouteJudgmentKey{TenantID: value(t, nrdomain.NewTenantID, "tenant-1")}
}

func factOf(t *testing.T, facts []nrdomain.CandidateCostFact, candidate string) nrdomain.CandidateCostFact {
	t.Helper()
	for _, fact := range facts {
		if fact.Candidate() == lineCandidate(t, candidate) {
			return fact
		}
	}
	t.Fatalf("候选 %s 没有事实：%+v", candidate, facts)
	return nrdomain.CandidateCostFact{}
}
