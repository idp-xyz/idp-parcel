package main

import (
	"context"
	"testing"
	"time"

	nrpricing "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/parcelpricing"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件对真库证生产装配里判断点怎样改变候选出价（CONTEXT「预路由使用客户声明快照；首次收寄复核使用当时已经取得的物理事实，
// 到站复核使用当前有效物理实测」）：同一张演示成本卡 Z1 三档（[0,1) 30、[1,5) 85、[5,30) 260），客户声明 2.5 KG 落 85 档，
// 实测 7 KG 落 260 档。

// physicalFactsDouble 是物理事实来源的替身：交回预置的答复，记下被问的时点。
type physicalFactsDouble struct {
	answer nrpricing.PhysicalParcelFacts
	asOf   []time.Time
}

func (double *physicalFactsDouble) PhysicalParcelFactsFor(
	_ context.Context, _ nrdomain.InitialRouteJudgmentKey, asOf time.Time,
) (nrpricing.PhysicalParcelFacts, error) {
	double.asOf = append(double.asOf, asOf)
	return double.answer, nil
}

// measuredWeight 造一份只称了重的物理事实，事实引用标明出自实际测量。
func measuredWeight(t *testing.T, kilograms string) nrpricing.PhysicalParcelFacts {
	t.Helper()
	weight, err := ppdomain.NewWeightFromString(kilograms, ppdomain.WeightUnitKilogram)
	if err != nil {
		t.Fatalf("实测重量：%v", err)
	}
	reference, err := ppdomain.NewVersionReferenceIdentity(ppdomain.ArtifactActualMeasurement, "SYN-PARCEL-01", "SYN-MEASUREMENT-01")
	if err != nil {
		t.Fatalf("事实引用：%v", err)
	}
	fact, err := ppdomain.NewVersionedFactReference(reference)
	if err != nil {
		t.Fatalf("事实引用：%v", err)
	}
	facts, err := nrpricing.ObtainedPhysicalFacts(&weight, nil, []ppdomain.VersionedFactReference{fact})
	if err != nil {
		t.Fatalf("物理事实：%v", err)
	}
	return facts
}

type factsPointFixture struct {
	fixture  *synVerticalFixture
	evidence nrports.InitialRouteEvidence
	key      nrdomain.InitialRouteJudgmentKey
}

// newFactsPointFixture 让 SYN-PARCEL-01 带客户声明 2.5 KG 受理，演示网络与绑了分区目录的成本卡照种子登好。
func newFactsPointFixture(t *testing.T) factsPointFixture {
	t.Helper()
	fixture := newSYNVerticalFixture(t)
	acceptDeclaredDemoParcel(t, fixture)
	registerDemoNetworkSeeds(t, fixture.db)
	registerCostCard(t, fixture.db, demoShapedCostCard(t, "CNY", zoneCatalogueBound(t)))
	registerDemoZoneCatalogue(t, fixture.db)
	seedWiringCustomsCatalog(t, fixture.db, demoTenant, true)
	return factsPointFixture{
		fixture:  fixture,
		evidence: loadWiringEvidence(t, fixture.db, demoTenant),
		key:      wiringRouteKey(t, demoTenant),
	}
}

func (f factsPointFixture) load(
	t *testing.T, physical nrpricing.PhysicalParcelFactsSource, point nrdomain.ParcelFactsPoint,
) nrports.RouteCandidateCosts {
	t.Helper()
	costs, err := routeCostsOver(f.fixture.db, systemClock{}, physical).
		LoadCandidateCosts(t.Context(), f.key, point, f.evidence, carriedPostalRoute())
	if err != nil {
		t.Fatalf("候选成本取数侧：%v", err)
	}
	return costs
}

func assertBasis(t *testing.T, costs nrports.RouteCandidateCosts, want nrdomain.ParcelFactsBasis) {
	t.Helper()
	if costs.FactsBasis != want {
		t.Fatalf("成本族依据 = %s，want %s", costs.FactsBasis, want)
	}
	for _, citation := range costs.Citations {
		if citation.FactsBasis() != want {
			t.Fatalf("出处依据 = %s，want %s", citation.FactsBasis(), want)
		}
	}
}

// Covers: 判据 4 真库那一格——声明 2.5 KG 与实测 7 KG 不符：预路由按声明出价 25500，首次收寄复核、到站复核、授权改路决定都按
// 实测出价 78000，出处标物理事实；首次收寄复核的物理事实截到收寄的业务时间，到站复核取判断此刻。
func TestTheJudgmentPointDecidesWhetherTheDeclarationOrTheMeasurementPricesTheCandidate(t *testing.T) {
	f := newFactsPointFixture(t)
	physical := &physicalFactsDouble{answer: measuredWeight(t, "7")}
	intakeAt := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	intake, err := nrdomain.NewFirstIntakeFactsPoint(intakeAt)
	if err != nil {
		t.Fatal(err)
	}

	preRoute := f.load(t, physical, nrdomain.NewPreRouteFactsPoint())
	if fact := demoCandidateCost(t, preRoute); fact.State() != nrdomain.CandidateCostPriced || fact.AmountMinor() != 25500 {
		t.Fatalf("预路由 = %s %d minor，want 按声明 2.5 KG 的 25500", fact.State(), fact.AmountMinor())
	}
	assertBasis(t, preRoute, nrdomain.ParcelFactsDeclaration)
	if len(physical.asOf) != 0 {
		t.Fatalf("预路由问了 %d 次物理事实", len(physical.asOf))
	}

	for name, point := range map[string]nrdomain.ParcelFactsPoint{
		"首次收寄": intake, "到站": nrdomain.NewArrivalFactsPoint(), "授权决定": nrdomain.NewDecisionFactsPoint(),
	} {
		costs := f.load(t, physical, point)
		if fact := demoCandidateCost(t, costs); fact.State() != nrdomain.CandidateCostPriced || fact.AmountMinor() != 78000 {
			t.Fatalf("%s = %s %d minor，want 按实测 7 KG 的 78000", name, fact.State(), fact.AmountMinor())
		}
		if len(costs.Citations) != 3 {
			t.Fatalf("%s：出处 %d 条，want 三段各一条", name, len(costs.Citations))
		}
		assertBasis(t, costs, nrdomain.ParcelFactsPhysical)
	}
	if got := physical.asOf[0]; !got.Equal(intakeAt) {
		t.Fatalf("首次收寄的物理事实截至 %s，want 收寄的业务时间 %s", got, intakeAt)
	}
	if got := physical.asOf[1]; got.Equal(intakeAt) || time.Since(got) > time.Minute {
		t.Fatalf("到站的物理事实截至 %s，want 判断此刻", got)
	}
}

// Covers: 生产装配没有物理事实来源——复核退回客户声明出价，价与预路由同（25500）、排序照常，出处如实标退回，而不是标成声明。
func TestWithoutAPhysicalFactSourceTheProductionWiringFallsBackToTheDeclarationAndSaysSo(t *testing.T) {
	f := newFactsPointFixture(t)

	costs, err := routeCosts(f.fixture.db, systemClock{}).
		LoadCandidateCosts(t.Context(), f.key, nrdomain.NewArrivalFactsPoint(), f.evidence, carriedPostalRoute())
	if err != nil {
		t.Fatalf("候选成本取数侧：%v", err)
	}
	if fact := demoCandidateCost(t, costs); fact.State() != nrdomain.CandidateCostPriced || fact.AmountMinor() != 25500 {
		t.Fatalf("到站复核 = %s %d minor，want 退回声明的 25500", fact.State(), fact.AmountMinor())
	}
	assertBasis(t, costs, nrdomain.ParcelFactsDeclarationFallback)
}

// Covers: 物理事实在而定不下用哪条——声明 2.5 KG 在库里，复核仍不退回它：演示候选待判断、没有出处，依据说明是定不下。
func TestUnresolvedPhysicalFactsAreNotPricedOnTheDeclarationInProductionWiring(t *testing.T) {
	f := newFactsPointFixture(t)

	costs := f.load(t, &physicalFactsDouble{answer: nrpricing.UnresolvedPhysicalFacts()}, nrdomain.NewArrivalFactsPoint())
	if fact := demoCandidateCost(t, costs); fact.State() != nrdomain.CandidateCostPending || len(costs.Citations) != 0 {
		t.Fatalf("成本 = %s、出处 %d 条，want 待判断、没有出处", fact.State(), len(costs.Citations))
	}
	if costs.FactsBasis != nrdomain.ParcelFactsPhysicalUnresolved {
		t.Fatalf("依据 = %s，want PHYSICAL_UNRESOLVED", costs.FactsBasis)
	}
}
