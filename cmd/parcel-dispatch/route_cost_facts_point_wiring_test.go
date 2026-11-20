package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	nrpricing "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/parcelpricing"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件对真库证生产装配里判断点怎样改变候选出价（CONTEXT「预路由使用客户声明快照；首次收寄复核使用当时已经取得的物理事实，
// 到站复核使用当前有效物理实测」）：同一张演示成本卡 Z1 三档（[0,1) 30、[1,5) 85、[5,30) 260），客户声明 2.5 KG 落 85 档，
// 实测 7 KG 落 260 档。生产装配读的是节点登记的当前有效实测：没登实测退回声明并标明，登了按实测出价。夹具为隔离合成，只记 S。

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

// measuredParcel 造一份取得了的物理事实：一条实测同时带毛重与长宽高（外廓小到体积重远不及实重，价只由实重定），事实引用
// 标明出自实际测量。
func measuredParcel(t *testing.T, kilograms string) nrpricing.PhysicalParcelFacts {
	t.Helper()
	weight, err := ppdomain.NewWeightFromString(kilograms, ppdomain.WeightUnitKilogram)
	if err != nil {
		t.Fatalf("实测重量：%v", err)
	}
	dimensions, err := ppdomain.NewDimensionsFromStrings("30", "20", "10", ppdomain.LengthUnitCentimeter)
	if err != nil {
		t.Fatalf("实测外廓：%v", err)
	}
	reference, err := ppdomain.NewVersionReferenceIdentity(ppdomain.ArtifactActualMeasurement, "SYN-PARCEL-01", "SYN-MEASUREMENT-01")
	if err != nil {
		t.Fatalf("事实引用：%v", err)
	}
	fact, err := ppdomain.NewVersionedFactReference(reference)
	if err != nil {
		t.Fatalf("事实引用：%v", err)
	}
	facts, err := nrpricing.ObtainedPhysicalFacts(weight, dimensions, []ppdomain.VersionedFactReference{fact})
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
// 实测出价 78000，出处标 PHYSICAL（重量与外廓整份出自实测，声明那一格不补）；首次收寄复核的物理事实截到收寄的业务时间，到站复核
// 取判断此刻。
func TestTheJudgmentPointDecidesWhetherTheDeclarationOrTheMeasurementPricesTheCandidate(t *testing.T) {
	f := newFactsPointFixture(t)
	physical := &physicalFactsDouble{answer: measuredParcel(t, "7")}
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

	for _, test := range []struct {
		name  string
		point nrdomain.ParcelFactsPoint
		// 物理事实截至何时：首次收寄是收寄的业务时间，其余是判断此刻（nil）。
		asOf *time.Time
	}{
		{"首次收寄", intake, &intakeAt},
		{"到站", nrdomain.NewArrivalFactsPoint(), nil},
		{"授权决定", nrdomain.NewDecisionFactsPoint(), nil},
	} {
		costs := f.load(t, physical, test.point)
		if fact := demoCandidateCost(t, costs); fact.State() != nrdomain.CandidateCostPriced || fact.AmountMinor() != 78000 {
			t.Fatalf("%s = %s %d minor，want 按实测 7 KG 的 78000", test.name, fact.State(), fact.AmountMinor())
		}
		if len(costs.Citations) != 3 {
			t.Fatalf("%s：出处 %d 条，want 三段各一条", test.name, len(costs.Citations))
		}
		assertBasis(t, costs, nrdomain.ParcelFactsPhysical)
		asked := physical.asOf[len(physical.asOf)-1]
		if test.asOf != nil && !asked.Equal(*test.asOf) {
			t.Fatalf("%s的物理事实截至 %s，want 收寄的业务时间 %s", test.name, asked, *test.asOf)
		}
		if test.asOf == nil && (asked.Equal(intakeAt) || time.Since(asked) > time.Minute) {
			t.Fatalf("%s的物理事实截至 %s，want 判断此刻", test.name, asked)
		}
	}
}

// Covers: 生产装配读得到节点登记的实测；这一件没登实测，到站复核退回客户声明出价，价与预路由同（25500）、排序照常，出处如实标退回。
func TestWithoutARegisteredMeasurementTheProductionWiringFallsBackToTheDeclarationAndSaysSo(t *testing.T) {
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

// Covers: 生产装配（routeCosts，不经替身）在库里登一条实测后，到站判断点的候选按实测出价、出处 PHYSICAL。
func TestARegisteredMeasurementPricesTheArrivalCandidateInProductionWiring(t *testing.T) {
	f := newFactsPointFixture(t)
	occurred := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	associateDemoParcel(t, f.fixture, "SYN-HU-RPF-01", "SYN-RPF-RECEPTION-01", "v1")
	recordDemoMeasurement(t, f.fixture, "SYN-HU-RPF-01", "SYN-RPF-MEASURE-01", "7", occurred)

	costs := loadProduction(t, f, nrdomain.NewArrivalFactsPoint())
	if fact := demoCandidateCost(t, costs); fact.State() != nrdomain.CandidateCostPriced || fact.AmountMinor() != 78000 {
		t.Fatalf("到站复核 = %s %d minor，want 按实测 7 KG 的 78000", fact.State(), fact.AmountMinor())
	}
	assertBasis(t, costs, nrdomain.ParcelFactsPhysical)
}

// Covers: 登两条仍有效实测且没有采用形态时，到站判断点不退回客户声明，候选待判断、依据 PHYSICAL_UNRESOLVED。
func TestTwoMeasurementsWithoutAnAdoptionDoNotPriceTheArrivalCandidateOnTheDeclaration(t *testing.T) {
	f := newFactsPointFixture(t)
	occurred := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	associateDemoParcel(t, f.fixture, "SYN-HU-RPF-01", "SYN-RPF-RECEPTION-01", "v1")
	recordDemoMeasurement(t, f.fixture, "SYN-HU-RPF-01", "SYN-RPF-MEASURE-01", "2.10", occurred)
	recordDemoMeasurement(t, f.fixture, "SYN-HU-RPF-01", "SYN-RPF-MEASURE-02", "7", occurred.Add(time.Hour))

	costs := loadProduction(t, f, nrdomain.NewArrivalFactsPoint())
	if fact := demoCandidateCost(t, costs); fact.State() != nrdomain.CandidateCostPending || len(costs.Citations) != 0 {
		t.Fatalf("成本 = %s、出处 %d 条，want 待判断、没有出处", fact.State(), len(costs.Citations))
	}
	if costs.FactsBasis != nrdomain.ParcelFactsPhysicalUnresolved {
		t.Fatalf("依据 = %s，want PHYSICAL_UNRESOLVED", costs.FactsBasis)
	}
}

// Covers: 首次收寄判断点上，发生时间晚于收寄业务时间的实测不被采用，退回客户声明并标明；同一条到站判断点仍按实测出价。
func TestAMeasurementAfterTheIntakeTimeIsNotUsedAtFirstIntake(t *testing.T) {
	f := newFactsPointFixture(t)
	occurred := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	associateDemoParcel(t, f.fixture, "SYN-HU-RPF-01", "SYN-RPF-RECEPTION-01", "v1")
	recordDemoMeasurement(t, f.fixture, "SYN-HU-RPF-01", "SYN-RPF-MEASURE-01", "7", occurred)
	intake, err := nrdomain.NewFirstIntakeFactsPoint(occurred.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	first := loadProduction(t, f, intake)
	if fact := demoCandidateCost(t, first); fact.State() != nrdomain.CandidateCostPriced || fact.AmountMinor() != 25500 {
		t.Fatalf("首次收寄 = %s %d minor，want 退回声明的 25500", fact.State(), fact.AmountMinor())
	}
	assertBasis(t, first, nrdomain.ParcelFactsDeclarationFallback)

	arrival := loadProduction(t, f, nrdomain.NewArrivalFactsPoint())
	if fact := demoCandidateCost(t, arrival); fact.State() != nrdomain.CandidateCostPriced || fact.AmountMinor() != 78000 {
		t.Fatalf("到站复核 = %s %d minor，want 按同一条实测的 78000", fact.State(), fact.AmountMinor())
	}
	assertBasis(t, arrival, nrdomain.ParcelFactsPhysical)
}

func loadProduction(t *testing.T, f factsPointFixture, point nrdomain.ParcelFactsPoint) nrports.RouteCandidateCosts {
	t.Helper()
	costs, err := routeCosts(f.fixture.db, systemClock{}).
		LoadCandidateCosts(t.Context(), f.key, point, f.evidence, carriedPostalRoute())
	if err != nil {
		t.Fatalf("候选成本取数侧：%v", err)
	}
	return costs
}

func associateDemoParcel(t *testing.T, fixture *synVerticalFixture, unit, sourceID, judgment string) {
	t.Helper()
	intake := fmt.Sprintf(`{"unit":%q,"association":"SYN-PARCEL-01","version":%q}`, unit, judgment)
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO node_operations.reception
			(tenant_id, source_id, content_digest, kind, intake, control, candidates, identity_conflict,
			 service_markers, recorded_at, judgment_version, predecessor_version)
		 VALUES ($1, $2, 'digest', 'INTAKE_FORMED', $3::jsonb, '{}'::jsonb, '[]'::jsonb, false,
		         '[]'::jsonb, now(), $4, NULL)`,
		demoTenant, sourceID, intake, judgment); err != nil {
		t.Fatalf("登记包裹关联：%v", err)
	}
}

func recordDemoMeasurement(t *testing.T, fixture *synVerticalFixture, unit, sourceID, kilograms string, occurred time.Time) {
	t.Helper()
	source, err := nodomain.NewWorkFactSource(
		sourceID,
		rpfRef(t, nodomain.NewPerformingPartyReference, "SYN-SCALE-RPF"),
		rpfRef(t, nodomain.NewExecutionEvidenceReference, "WEIGH/"+sourceID),
		occurred,
	)
	if err != nil {
		t.Fatalf("来源：%v", err)
	}
	gross, err := nodomain.NewGrossWeight(kilograms, "KG")
	if err != nil {
		t.Fatalf("毛重：%v", err)
	}
	outer, err := nodomain.NewOuterDimensions("30", "20", "10", "CM")
	if err != nil {
		t.Fatalf("外廓：%v", err)
	}
	measurement, err := nodomain.NewActualMeasurement(nodomain.ActualMeasurementSpec{
		Source: source, ObjectKind: nodomain.HandlingUnitObject, ObjectID: unit,
		Node: rpfRef(t, nodomain.NewNodeReference, "node-origin"), Location: rpfRef(t, nodomain.NewWorkLocationReference, "bench-2"),
		Basis:  rpfRef(t, nodomain.NewWorkBasisReference, "intake-1"),
		Weight: gross, Dimensions: outer,
	})
	if err != nil {
		t.Fatalf("测量：%v", err)
	}
	measurements, err := nopostgres.NewActualMeasurements(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	record := noports.ActualMeasurementRecord{
		Key:           noports.ActualMeasurementKey{TenantID: rpfRef(t, nodomain.NewTenantID, demoTenant), SourceID: sourceID},
		Measurement:   measurement,
		ContentDigest: measurement.ContentDigest(),
		RecordedAt:    occurred.Add(time.Minute),
	}
	mustWithinTX(t, fixture.transactor, t.Context(), func(ctx context.Context) error {
		outcome, err := measurements.SaveMeasurement(ctx, record)
		if err != nil {
			return err
		}
		if outcome != noports.ActualMeasurementSaved {
			return fmt.Errorf("登记实测 %s", outcome)
		}
		return nil
	})
}

func rpfRef[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
