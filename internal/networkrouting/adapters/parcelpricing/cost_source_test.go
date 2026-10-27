package parcelpricing_test

import (
	"context"
	"errors"
	"testing"
	"time"

	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	ppapplication "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	ppports "go.idp.xyz/idp-parcel/internal/parcelpricing/ports"

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

// Covers: `AT-NR-073` 的取数侧半边（票 demo-chain-gaps/105）——几条候选都出价时，每条出处认得出属于哪条候选，段号按各自
// 的段链从 1 计；待判断的候选没有出处。取数侧不知道哪一条会被选中，选的那一份由编排按候选取。
func TestEachCitationNamesTheCandidateWhoseLegItPrices(t *testing.T) {
	source := newAdapter(t,
		map[string][]nrports.LineSegmentCostBasis{
			"line-cheap": {{SegmentIndex: 0, Kind: nrports.SupplierBuyPlanBasis, Reference: "plan-cheap/v1"}},
			"line-costly": {
				{SegmentIndex: 0, Kind: nrports.SupplierBuyPlanBasis, Reference: "plan-costly/v1"},
				{SegmentIndex: 1, Kind: nrports.SupplierBuyPlanBasis, Reference: "plan-costly/v1"},
			},
		},
		map[string]ppdomain.PricingPlanVersion{
			"plan-cheap/v1":  syntheticBuyPlan(t, "plan-cheap", "v1", "5"),
			"plan-costly/v1": syntheticBuyPlan(t, "plan-costly", "v1", "8"),
		},
	)

	costs, err := source.LoadCandidateCosts(context.Background(), judgmentKey(t), evidenceWithComparison(t, "USD",
		pathOf(t, "line-cheap", 1), pathOf(t, "line-costly", 2), pathOf(t, "line-unbased", 1)))
	if err != nil {
		t.Fatalf("取候选成本：%v", err)
	}
	for candidate, want := range map[string]struct {
		plan string
		legs int
	}{
		"line-cheap":   {"plan-cheap/v1", 1},
		"line-costly":  {"plan-costly/v1", 2},
		"line-unbased": {"", 0},
	} {
		citations := costs.CitationsOf(lineCandidate(t, candidate))
		if len(citations) != want.legs {
			t.Fatalf("%s 的出处 %d 条，want %d：%+v", candidate, len(citations), want.legs, citations)
		}
		for index, citation := range citations {
			if citation.Ordinal() != index+1 || citation.PlanReference() != want.plan {
				t.Fatalf("%s 的第 %d 条出处 = %+v，want 段号 %d、出自 %s", candidate, index+1, citation, index+1, want.plan)
			}
		}
	}
	if len(costs.Citations) != 3 {
		t.Fatalf("出处共 %d 条，want 两条出价候选的三段各一条：%+v", len(costs.Citations), costs.Citations)
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
// 比较金额不过卡的取整点。兼 `AT-NR-072`（票 routing-first-cut/20）：卡绑的汇率序列由 parcel-pricing 的补齐读数按在用
// 版本解出，取数侧不自带读数。
func TestAComparisonCurrencyReachesThroughTheLegs(t *testing.T) {
	plan := syntheticBuyPlanWithRate(t, "plan-cny", "v1", "10")
	source := newAdapter(t,
		map[string][]nrports.LineSegmentCostBasis{
			"line-cny": {{SegmentIndex: 0, Kind: nrports.SupplierBuyPlanBasis, Reference: "plan-cny/v1"}},
		},
		map[string]ppdomain.PricingPlanVersion{"plan-cny/v1": plan},
	)
	source.series.inForce["fx-daily"] = fxDailyRegistration(t, "7.2")

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

// Covers: 票 routing-first-cut/17 判据一的混合段——段 1 有区、出得了价，段 2 区域未配置：候选仍待判断、不带出处。
// 只合已评价的段，会拿段 1 的价充整条候选的成本，那正是「缺口不折零、不以他段顶替」要拦的。
func TestAPricedLegDoesNotStandInForALegWhoseZoneIsUnconfigured(t *testing.T) {
	source := newAdapter(t,
		map[string][]nrports.LineSegmentCostBasis{
			"line-half-zoned": {
				{SegmentIndex: 0, Kind: nrports.SupplierBuyPlanBasis, Reference: "plan-x/v1"},
				{SegmentIndex: 1, Kind: nrports.SupplierBuyPlanBasis, Reference: "plan-x/v1"},
			},
		},
		map[string]ppdomain.PricingPlanVersion{"plan-x/v1": syntheticBuyPlan(t, "plan-x", "v1", "5")},
	)
	source.unzonedOrdinals = map[int]bool{2: true}

	costs, err := source.LoadCandidateCosts(context.Background(), judgmentKey(t), evidenceWithComparison(t, "USD",
		pathOf(t, "line-half-zoned", 2)))
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(source.zoneQueries) != 2 {
		t.Fatalf("区域之问 = %+v，want 两段各一问", source.zoneQueries)
	}
	fact := factOf(t, costs.Facts, "line-half-zoned")
	if fact.State().String() != "PENDING" {
		t.Fatalf("fact = %s %d minor, want PENDING（段 2 缺成本依据，不以段 1 的价顶替）", fact.State(), fact.AmountMinor())
	}
	if len(costs.Citations) != 0 {
		t.Fatalf("待判断的候选不该带出处：%+v", costs.Citations)
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
	source.facts.Basis = nrdomain.ParcelFactsNone

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

// Covers: `AT-NR-072` 的反面——卡绑的汇率序列没有在用版本：补齐读数留说明、不编汇率，评价待判断，候选随之待判断。
func TestASeriesBoundCardWithoutAnInForceSeriesStaysPending(t *testing.T) {
	source := newAdapter(t,
		map[string][]nrports.LineSegmentCostBasis{
			"line-cny": {{SegmentIndex: 0, Kind: nrports.SupplierBuyPlanBasis, Reference: "plan-cny/v1"}},
		},
		map[string]ppdomain.PricingPlanVersion{"plan-cny/v1": syntheticBuyPlanWithRate(t, "plan-cny", "v1", "10")},
	)

	costs, err := source.LoadCandidateCosts(context.Background(), judgmentKey(t), evidenceWithComparison(t, "CNY",
		pathOf(t, "line-cny", 1)))
	if err != nil {
		t.Fatalf("取候选成本：%v", err)
	}
	if fact := factOf(t, costs.Facts, "line-cny"); fact.State().String() != "PENDING" {
		t.Fatalf("fact = %s %d minor, want PENDING（序列没有在用版本，不编汇率）", fact.State(), fact.AmountMinor())
	}
}

// Covers: `AT-NR-070`（票 routing-first-cut/20，ADR-0189 决定一第一条）——绑了分区目录的卡，区由随请求携带的邮编路线经
// 在用目录读出：目的邮编 018000 落目录 018 → Z1，卡在 Z1 有档，候选出价。这一段另登了 Z9（卡上没有 Z9 的档）：出价说明
// 评价没读登的区。目录读数按路线的目的邮编取，路线原样交到目录。
func TestABoundCardsZoneIsReadFromTheCatalogueThroughThePostalRoute(t *testing.T) {
	source := newAdapter(t,
		map[string][]nrports.LineSegmentCostBasis{
			"line-bound": {{SegmentIndex: 0, Kind: nrports.SupplierBuyPlanBasis, Reference: "plan-bound/v1"}},
		},
		map[string]ppdomain.PricingPlanVersion{"plan-bound/v1": zoneCatalogueBoundBuyPlan(t, "plan-bound", "v1", "5")},
	)
	source.catalogues.inForce["zones-plan-bound"] = zoneCatalogueRegistration(t, "zones-plan-bound")
	source.zoneByOrdinal = map[int]string{1: "Z9"}
	source.carried = carriedRoute("200000", "018000")

	costs, err := source.LoadCandidateCosts(context.Background(), judgmentKey(t), evidenceWithComparison(t, "USD",
		pathOf(t, "line-bound", 1)))
	if err != nil {
		t.Fatalf("取候选成本：%v", err)
	}
	fact := factOf(t, costs.Facts, "line-bound")
	if fact.State().String() != "PRICED" || fact.AmountMinor() != 500 {
		t.Fatalf("fact = %s %d minor, want PRICED 500（目录解出 Z1，不读登的 Z9）", fact.State(), fact.AmountMinor())
	}
	if source.catalogues.lastRoute.Origin() != "200000" || source.catalogues.lastRoute.Destination() != "018000" {
		t.Fatalf("交到目录的路线 = %+v，want 200000 → 018000", source.catalogues.lastRoute)
	}
	if len(costs.Citations) != 1 || costs.Citations[0].PlanReference() != "plan-bound/v1" {
		t.Fatalf("出处 = %+v，want 这一段一条、指那张绑了目录的卡", costs.Citations)
	}
}

// Covers: `AT-NR-071`（票 routing-first-cut/20，ADR-0189 决定一）——绑了分区目录的卡没有邮编路线可用：请求没带投影，或带了
// 却没报收件邮编。这一段登了 Z1、卡的档位也在 Z1，评价照样不读登的区，候选待判断；不从节点推邮编，不拿卡上唯一的区当默认。
func TestABoundCardWithoutAPostalRouteStaysPending(t *testing.T) {
	for name, carried := range map[string]nrports.RequestCarriedContent{
		"请求没带投影": {},
		"收件邮编没报": {Geo: nrdomain.NewGeoResolutionProjection(
			nrdomain.NewGeoResolutionSide("CN", true, "200000", true),
			nrdomain.NewGeoResolutionSide("SG", true, "", false))},
	} {
		t.Run(name, func(t *testing.T) {
			source := newAdapter(t,
				map[string][]nrports.LineSegmentCostBasis{
					"line-bound": {{SegmentIndex: 0, Kind: nrports.SupplierBuyPlanBasis, Reference: "plan-bound/v1"}},
				},
				map[string]ppdomain.PricingPlanVersion{"plan-bound/v1": zoneCatalogueBoundBuyPlan(t, "plan-bound", "v1", "5")},
			)
			source.catalogues.inForce["zones-plan-bound"] = zoneCatalogueRegistration(t, "zones-plan-bound")
			source.zoneByOrdinal = map[int]string{1: "Z1"}
			source.carried = carried

			costs, err := source.LoadCandidateCosts(context.Background(), judgmentKey(t), evidenceWithComparison(t, "USD",
				pathOf(t, "line-bound", 1)))
			if err != nil {
				t.Fatalf("取候选成本：%v", err)
			}
			if fact := factOf(t, costs.Facts, "line-bound"); fact.State().String() != "PENDING" {
				t.Fatalf("fact = %s %d minor, want PENDING（绑了目录的卡没有路线，不读登的区）", fact.State(), fact.AmountMinor())
			}
			if source.catalogues.calls != 0 {
				t.Fatalf("没有路线还去问目录 %d 次", source.catalogues.calls)
			}
		})
	}
}

// Covers: `AT-NR-071` 的另一半（判据「没绑目录、也没登段计价区域的卡仍待判断」）——请求带了邮编路线也一样：路线只给绑了目录的卡解区，没绑的卡
// 不拿它顶替调用方给的分区（ADR-0189 决定一第二、四条）。
func TestAnUnboundCardDoesNotReadItsZoneFromThePostalRoute(t *testing.T) {
	source := newAdapter(t,
		map[string][]nrports.LineSegmentCostBasis{
			"line-x": {{SegmentIndex: 0, Kind: nrports.SupplierBuyPlanBasis, Reference: "plan-x/v1"}},
		},
		map[string]ppdomain.PricingPlanVersion{"plan-x/v1": syntheticBuyPlan(t, "plan-x", "v1", "5")},
	)
	source.zonesConfigured = false
	source.carried = carriedRoute("200000", "018000")

	costs, err := source.LoadCandidateCosts(context.Background(), judgmentKey(t), evidenceWithComparison(t, "USD",
		pathOf(t, "line-x", 1)))
	if err != nil {
		t.Fatalf("取候选成本：%v", err)
	}
	if fact := factOf(t, costs.Facts, "line-x"); fact.State().String() != "PENDING" {
		t.Fatalf("fact = %s %d minor, want PENDING", fact.State(), fact.AmountMinor())
	}
}

// Covers: `AT-NR-071` 末句——补齐读数读不回（目录在用解析的依赖故障）不是这一段待判断：整次取数报错，编排据此落依赖不可用的未决，
// 不把一次故障记成缺成本依据。
func TestAnUnreadableCatalogueFailsTheWholeCostLoad(t *testing.T) {
	source := newAdapter(t,
		map[string][]nrports.LineSegmentCostBasis{
			"line-bound": {{SegmentIndex: 0, Kind: nrports.SupplierBuyPlanBasis, Reference: "plan-bound/v1"}},
		},
		map[string]ppdomain.PricingPlanVersion{"plan-bound/v1": zoneCatalogueBoundBuyPlan(t, "plan-bound", "v1", "5")},
	)
	source.catalogues.err = errors.New("catalogue register is down")
	source.carried = carriedRoute("200000", "018000")

	if _, err := source.LoadCandidateCosts(context.Background(), judgmentKey(t), evidenceWithComparison(t, "USD",
		pathOf(t, "line-bound", 1))); err == nil {
		t.Fatal("目录读不回该整次报错，而不是交回一条待判断")
	}
}

// testSource 是可调替身的容器：plan 表按「id/version」键取；包裹事实、逐段区域、随请求携带的内容、在用序列与目录、计价
// 时点可逐格替换。评价走真的 parcel-pricing「对给定价卡形成试算」入口，补齐读数那几口换成下面两本册。
type testSource struct {
	*adapter.RouteCandidateCostAdapter
	bases           map[string][]nrports.LineSegmentCostBasis
	plans           map[string]ppdomain.PricingPlanVersion
	internal        map[string]string
	facts           adapter.ParcelFacts
	hasFacts        bool
	askedPoint      nrdomain.ParcelFactsPoint
	seenPoints      []nrdomain.ParcelFactsPoint
	pointed         adapter.ParcelFactsSource
	zonesConfigured bool
	zoneByOrdinal   map[int]string
	unzonedOrdinals map[int]bool
	zoneQueries     []adapter.LegZoneQuery
	carried         nrports.RequestCarriedContent
	series          *seriesBook
	catalogues      *catalogueBook
	now             time.Time
}

func newAdapter(t *testing.T, bases map[string][]nrports.LineSegmentCostBasis, plans map[string]ppdomain.PricingPlanVersion) *testSource {
	t.Helper()
	source := &testSource{
		bases:           bases,
		plans:           plans,
		internal:        map[string]string{},
		facts:           adapter.ParcelFacts{Weight: weightOf(t, "1"), Basis: nrdomain.ParcelFactsDeclaration},
		hasFacts:        true,
		zonesConfigured: true,
		series:          &seriesBook{inForce: map[string]ppdomain.ReferenceSeriesRegistration{}},
		catalogues:      &catalogueBook{inForce: map[string]ppdomain.ReferenceCatalogueRegistration{}},
		now:             planPeriod.Add(6 * 30 * 24 * time.Hour),
	}
	estimates, err := ppapplication.NewFormPlanGivenEstimateEvaluationsHandler(ppapplication.FormPlanGivenEstimateEvaluationsDeps{
		Clock:            source,
		SeriesInForce:    source.series,
		SeriesVersions:   source.series,
		CatalogueInForce: source.catalogues,
		Catalogues:       source.catalogues,
	})
	if err != nil {
		t.Fatalf("试算入口：%v", err)
	}
	source.RouteCandidateCostAdapter = adapter.NewRouteCandidateCostAdapter(adapter.RouteCandidateCostDeps{
		Bases:     source,
		Internal:  source,
		Facts:     source,
		Zones:     source,
		Estimates: estimates,
		Plans:     source,
		Clock:     source,
	})
	return source
}

// LoadCandidateCosts 带上本替身记着的携带内容去取：多数用例不关心邮编路线，零值即请求什么都没带。
func (source *testSource) LoadCandidateCosts(
	ctx context.Context, key nrdomain.InitialRouteJudgmentKey, evidence nrports.InitialRouteEvidence,
) (nrports.RouteCandidateCosts, error) {
	return source.RouteCandidateCostAdapter.LoadCandidateCosts(ctx, key, source.point(), evidence, source.carried)
}

// point 是本替身代取成本的判断点：多数用例不关心判断点，按预路由取；要验判断点的用例自己设。
func (source *testSource) point() nrdomain.ParcelFactsPoint {
	if source.askedPoint.Valid() {
		return source.askedPoint
	}
	return nrdomain.NewPreRouteFactsPoint()
}

func (source *testSource) ParcelFactsFor(
	ctx context.Context, key nrdomain.InitialRouteJudgmentKey, point nrdomain.ParcelFactsPoint,
) (adapter.ParcelFacts, bool, error) {
	source.seenPoints = append(source.seenPoints, point)
	if source.pointed != nil {
		return source.pointed.ParcelFactsFor(ctx, key, point)
	}
	return source.facts, source.hasFacts, nil
}

// ZoneFor 未单列的段一律答 Z1——合成价卡的档位都在 Z1。
func (source *testSource) ZoneFor(_ context.Context, query adapter.LegZoneQuery) (adapter.LegZone, bool, error) {
	source.zoneQueries = append(source.zoneQueries, query)
	if !source.zonesConfigured || source.unzonedOrdinals[query.Ordinal] {
		return adapter.LegZone{}, false, nil
	}
	if zone, listed := source.zoneByOrdinal[query.Ordinal]; listed {
		return adapter.LegZone{Zone: zone}, true, nil
	}
	return adapter.LegZone{Zone: "Z1"}, true, nil
}

// seriesBook 是在用序列那一对补齐口的替身：按序列标识登一版，在用解析答那一版，取值按计价基准时点从那一版解。没登即没有
// 登过任何版本。
type seriesBook struct {
	inForce map[string]ppdomain.ReferenceSeriesRegistration
}

func (book *seriesBook) ResolveInForce(
	_ context.Context, _ ppdomain.TenantID, _ ppdomain.ReferenceSeriesKind, seriesID string, _ time.Time,
) (ppdomain.VersionReference, ppports.InForceOutcome, error) {
	registration, found := book.inForce[seriesID]
	if !found {
		return ppdomain.VersionReference{}, ppports.SeriesHasNoRegisteredVersion, nil
	}
	return registration.Reference(), ppports.SeriesVersionInForce, nil
}

func (book *seriesBook) Register(context.Context, ppdomain.ReferenceSeriesRegistration) (ppports.ReferenceSeriesRegistrationOutcome, error) {
	return ppports.ReferenceSeriesRegistrationOutcomeInvalid, errors.New("not used")
}

func (book *seriesBook) ResolveAt(
	_ context.Context, _ ppdomain.TenantID, reference ppdomain.VersionReference, asOf time.Time,
) (ppdomain.ResolvedSeriesReading, bool, error) {
	for _, registration := range book.inForce {
		if registration.Reference().SameIdentity(reference) {
			reading, found := registration.ResolveAt(asOf)
			return reading, found, nil
		}
	}
	return ppdomain.ResolvedSeriesReading{}, false, nil
}

// catalogueBook 是在用目录那一对补齐口的替身，形状同 seriesBook；另记下问了几次与最近一次交来的路线。err 非空即在用解析
// 读不回。
type catalogueBook struct {
	inForce   map[string]ppdomain.ReferenceCatalogueRegistration
	err       error
	calls     int
	lastRoute ppdomain.PostalRoute
}

func (book *catalogueBook) ResolveInForce(
	_ context.Context, _ ppdomain.TenantID, _ ppdomain.CatalogueKind, catalogueID string, _ time.Time,
) (ppdomain.VersionReference, ppports.CatalogueInForceOutcome, error) {
	book.calls++
	if book.err != nil {
		return ppdomain.VersionReference{}, ppports.CatalogueInForceOutcomeInvalid, book.err
	}
	registration, found := book.inForce[catalogueID]
	if !found {
		return ppdomain.VersionReference{}, ppports.CatalogueHasNoRegisteredVersion, nil
	}
	return registration.Reference(), ppports.CatalogueVersionInForce, nil
}

func (book *catalogueBook) Register(context.Context, ppdomain.ReferenceCatalogueRegistration) (ppports.ReferenceCatalogueRegistrationOutcome, error) {
	return ppports.ReferenceCatalogueRegistrationOutcomeInvalid, errors.New("not used")
}

func (book *catalogueBook) ResolveAt(
	_ context.Context, _ ppdomain.TenantID, reference ppdomain.VersionReference, asOf time.Time, route ppdomain.PostalRoute,
) (ppdomain.ResolvedCatalogueValue, bool, error) {
	book.lastRoute = route
	for _, registration := range book.inForce {
		if registration.Reference().SameIdentity(reference) {
			reading, applicable := registration.ResolveAt(asOf, route)
			return reading, applicable, nil
		}
	}
	return ppdomain.ResolvedCatalogueValue{}, false, nil
}

// carriedRoute 是随请求携带的一份投影：寄件 CN、收件 SG，两端邮编按给的报。
func carriedRoute(origin, destination string) nrports.RequestCarriedContent {
	return nrports.RequestCarriedContent{Geo: nrdomain.NewGeoResolutionProjection(
		nrdomain.NewGeoResolutionSide("CN", true, origin, true),
		nrdomain.NewGeoResolutionSide("SG", true, destination, true))}
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

// Covers: `AT-NR-092`、`AT-NR-093` 取数侧那一半——方案方向配错、方案引用立不成、候选没有线路引用、比较币种译不出都挂在 nrports.ErrRouteCostAnswerUnusable
// 名下：重投多少次都一样，编排据它响亮报错，不与依赖读不回同进重投。目录读不回那一格不在这里（见
// TestAnUnreadableCatalogueFailsTheWholeCostLoad）。
func TestDeterministicCostAnswersAreUnusableNotUnreadable(t *testing.T) {
	cases := map[string]func(t *testing.T) (*testSource, nrports.InitialRouteEvidence){
		"方案方向配错": func(t *testing.T) (*testSource, nrports.InitialRouteEvidence) {
			source := newAdapter(t,
				map[string][]nrports.LineSegmentCostBasis{
					"line-x": {{SegmentIndex: 0, Kind: nrports.SupplierBuyPlanBasis, Reference: "plan-internal/v1"}},
				},
				map[string]ppdomain.PricingPlanVersion{"plan-internal/v1": syntheticInternalPlan(t, "plan-internal", "v1", "5")},
			)
			return source, evidenceWithComparison(t, "USD", pathOf(t, "line-x", 1))
		},
		"候选没有线路引用": func(t *testing.T) (*testSource, nrports.InitialRouteEvidence) {
			source := newAdapter(t, map[string][]nrports.LineSegmentCostBasis{}, map[string]ppdomain.PricingPlanVersion{})
			return source, evidenceWithComparison(t, "USD", nrports.CandidatePath{
				Candidate: value(t, nrdomain.NewCandidateID, "not-a-line-candidate"),
				Legs:      make([]nrdomain.PlannedLeg, 1),
			})
		},
		"方案引用立不成": func(t *testing.T) (*testSource, nrports.InitialRouteEvidence) {
			source := newAdapter(t,
				map[string][]nrports.LineSegmentCostBasis{
					"line-x": {{SegmentIndex: 0, Kind: nrports.SupplierBuyPlanBasis, Reference: "plan-x /v1"}},
				},
				map[string]ppdomain.PricingPlanVersion{},
			)
			return source, evidenceWithComparison(t, "USD", pathOf(t, "line-x", 1))
		},
		"比较币种译不出": func(t *testing.T) (*testSource, nrports.InitialRouteEvidence) {
			source := newAdapter(t, map[string][]nrports.LineSegmentCostBasis{}, map[string]ppdomain.PricingPlanVersion{})
			return source, evidenceWithComparison(t, "not a currency", pathOf(t, "line-x", 1))
		},
	}
	for name, arrange := range cases {
		t.Run(name, func(t *testing.T) {
			source, evidence := arrange(t)
			_, err := source.LoadCandidateCosts(context.Background(), judgmentKey(t), evidence)
			if !errors.Is(err, nrports.ErrRouteCostAnswerUnusable) {
				t.Fatalf("err = %v, want ErrRouteCostAnswerUnusable", err)
			}
		})
	}
}
