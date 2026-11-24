package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
)

// 本文件钉 ADR-0380 决定三在「按评价请求形成评价」上的落法：有序范围列表逐个问，只有没有在用卡和基础运费表
// 没有这一格才问下一个；只带一个范围的请求不走这条路。

type scopedPriceCards struct {
	byScope map[string]ports.PriceCardInForceResolution
	asked   []string
}

func (double *scopedPriceCards) ResolveInForce(
	_ context.Context,
	_ domain.TenantID,
	scope domain.PricingScopeID,
	_ domain.PricingDirection,
	_ domain.PricingPurpose,
	_ time.Time,
	_ domain.ServiceProductReference,
) (ports.PriceCardInForceResolution, error) {
	double.asked = append(double.asked, scope.String())
	resolution, found := double.byScope[scope.String()]
	if !found {
		return ports.PriceCardInForceResolution{Outcome: ports.PriceCardNotConfigured}, nil
	}
	return resolution, nil
}

type scopedInputs struct {
	byScope map[string]domain.PricingInputSnapshot
}

func (double *scopedInputs) ResolvePricingInput(_ context.Context, query ports.PricingInputQuery) (ports.PricingInputResolution, error) {
	input, found := double.byScope[query.Scope.String()]
	if !found {
		return ports.PricingInputResolution{Outcome: ports.PricingInputUnavailable, Missing: []string{"scope"}}, nil
	}
	return ports.PricingInputResolution{Outcome: ports.PricingInputResolved, Input: input}, nil
}

func sellScopePlan(t *testing.T, scope, zone, planID string) domain.PricingPlanVersion {
	t.Helper()
	return pptest.Plan(t, pptest.PlanSpec{
		Reference:             pptest.IdentityReference(t, domain.ArtifactPricingPlan, planID, "v1"),
		TableReference:        pptest.IdentityReference(t, domain.ArtifactRateTable, planID+"-table", "v1"),
		WeightPolicyReference: pptest.IdentityReference(t, domain.ArtifactWeightPolicy, planID+"-weight", "v1"),
		Scope:                 scope,
		Direction:             domain.PricingDirectionSell,
		Purpose:               domain.PricingPurposeCustomerCharge,
		BaseChargeCode:        "BASE_FREIGHT",
		Currency:              "USD",
		Period: pptest.Period{
			StartsAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			EndsAt:   time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		RateEntryID:         planID + "-entry",
		RateZone:            zone,
		MinimumKilograms:    "0",
		MaximumKilograms:    "10",
		RateAmount:          "12",
		WeightRounding:      domain.RoundingCeiling,
		WeightStepKilograms: "0.5",
	})
}

func sellScopeInput(t *testing.T, scope string) domain.PricingInputSnapshot {
	t.Helper()
	return pptest.Input(t, pptest.InputSpec{
		Tenant:     "tenant-1",
		Scope:      scope,
		PackageID:  "SYN-PKG-1",
		Zone:       "Z1",
		Kilograms:  "5",
		BusinessAt: requestBasisAt,
	})
}

func sellListCommand(t *testing.T, scopes ...string) application.FormEvaluationFromRequestCommand {
	t.Helper()
	command := requestCommand(t)
	command.Direction = domain.PricingDirectionSell
	command.Purpose = domain.PricingPurposeCustomerCharge
	command.Sources = ports.EligibleSourceReferences{Parcel: "SYN-PARCEL-1"}
	command.Scope = mustValue(t, domain.NewPricingScopeID, scopes[0])
	command.Scopes = make([]domain.PricingScopeID, 0, len(scopes))
	for _, scope := range scopes {
		command.Scopes = append(command.Scopes, mustValue(t, domain.NewPricingScopeID, scope))
	}
	return command
}

func inForce(plan domain.PricingPlanVersion) ports.PriceCardInForceResolution {
	return ports.PriceCardInForceResolution{Outcome: ports.PriceCardVersionInForce, Plan: plan}
}

func sellListFixture(t *testing.T, cards map[string]ports.PriceCardInForceResolution, inputs map[string]domain.PricingInputSnapshot) (*formFixture, *scopedPriceCards) {
	t.Helper()
	fixture := newFormFixture(t, nil)
	scoped := &scopedPriceCards{byScope: cards}
	fixture.priceCards = nil
	fixture.handler = nil
	deps := application.FormEvaluationFromRequestDeps{
		Evaluations:     fixture.byRequest,
		PriceCards:      scoped,
		Identity:        fixture.identity,
		Evaluate:        application.NewEvaluatePricingHandler(application.EvaluatePricingDeps{Store: fixture.store, Downstream: fixture.downstream, Clock: fixedClock{at: requestBasisAt}}),
		Inputs:          &scopedInputs{byScope: inputs},
		ServiceProducts: fixture.products,
	}
	handler, err := application.NewFormEvaluationFromRequestHandler(deps)
	if err != nil {
		t.Fatalf("construct handler: %v", err)
	}
	fixture.handler = handler
	return fixture, scoped
}

func TestTheSecondScopeIsUsedWhenTheFirstHasNoCard(t *testing.T) {
	second := sellScopePlan(t, "SYN-SCOPE-B", "Z1", "SYN-PLAN-B")
	fixture, cards := sellListFixture(t, map[string]ports.PriceCardInForceResolution{
		"SYN-SCOPE-B": inForce(second),
	}, map[string]domain.PricingInputSnapshot{
		"SYN-SCOPE-B": sellScopeInput(t, "SYN-SCOPE-B"),
	})

	result, err := fixture.handler.Handle(context.Background(), sellListCommand(t, "SYN-SCOPE-A", "SYN-SCOPE-B"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome != application.RequestEvaluationFormed || result.Evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("outcome = %s status = %s issues = %v", result.Outcome, result.Evaluation.Status(), result.Evaluation.Issues())
	}
	if got := trailKinds(result.ScopeTrail); got != "SYN-SCOPE-A:NO_CARD,SYN-SCOPE-B:SELECTED" {
		t.Fatalf("trail = %s", got)
	}
	if cards.asked[0] != "SYN-SCOPE-A" || cards.asked[1] != "SYN-SCOPE-B" {
		t.Fatalf("asked = %v", cards.asked)
	}
	if !explanationHas(result.Evaluation, "sell price source SYN-SCOPE-A: NO_CARD") {
		t.Fatalf("explanation = %v", result.Evaluation.Explanation())
	}
}

func TestABaseRateGapFallsThroughAndAConflictStops(t *testing.T) {
	gap := sellScopePlan(t, "SYN-SCOPE-A", "Z9", "SYN-PLAN-A")
	hit := sellScopePlan(t, "SYN-SCOPE-B", "Z1", "SYN-PLAN-B")
	fixture, _ := sellListFixture(t, map[string]ports.PriceCardInForceResolution{
		"SYN-SCOPE-A": inForce(gap),
		"SYN-SCOPE-B": inForce(hit),
	}, map[string]domain.PricingInputSnapshot{
		"SYN-SCOPE-A": sellScopeInput(t, "SYN-SCOPE-A"),
		"SYN-SCOPE-B": sellScopeInput(t, "SYN-SCOPE-B"),
	})

	result, err := fixture.handler.Handle(context.Background(), sellListCommand(t, "SYN-SCOPE-A", "SYN-SCOPE-B"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome != application.RequestEvaluationFormed || result.Evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("outcome = %s status = %s issues = %v", result.Outcome, result.Evaluation.Status(), result.Evaluation.Issues())
	}
	if got := trailKinds(result.ScopeTrail); got != "SYN-SCOPE-A:BASE_RATE_NOT_IN_TABLE,SYN-SCOPE-B:SELECTED" {
		t.Fatalf("trail = %s", got)
	}
	if fixture.identity.minted != 1 || fixture.store.saved != 1 {
		t.Fatalf("minted = %d saved = %d，落空的那一格不该入册", fixture.identity.minted, fixture.store.saved)
	}

	conflict := sellListFixtureOnly(t, map[string]ports.PriceCardInForceResolution{
		"SYN-SCOPE-A": {Outcome: ports.PriceCardApplicabilityConflict, Candidates: []domain.VersionReference{hit.Reference()}},
		"SYN-SCOPE-B": inForce(hit),
	})
	stopped, cards := conflict.handler, conflict.cards
	result, err = stopped.Handle(context.Background(), sellListCommand(t, "SYN-SCOPE-A", "SYN-SCOPE-B"))
	if err != nil {
		t.Fatalf("conflict: %v", err)
	}
	if result.Outcome != application.RequestPriceCardApplicabilityConflict || len(cards.asked) != 1 {
		t.Fatalf("outcome = %s asked = %v，适用冲突不该去问后一个范围", result.Outcome, cards.asked)
	}
}

func TestEveryScopeMissingAnswersTheLastScope(t *testing.T) {
	gapA := sellScopePlan(t, "SYN-SCOPE-A", "Z9", "SYN-PLAN-A")
	gapB := sellScopePlan(t, "SYN-SCOPE-B", "Z9", "SYN-PLAN-B")
	fixture, _ := sellListFixture(t, map[string]ports.PriceCardInForceResolution{
		"SYN-SCOPE-A": inForce(gapA),
		"SYN-SCOPE-B": inForce(gapB),
	}, map[string]domain.PricingInputSnapshot{
		"SYN-SCOPE-A": sellScopeInput(t, "SYN-SCOPE-A"),
		"SYN-SCOPE-B": sellScopeInput(t, "SYN-SCOPE-B"),
	})
	result, err := fixture.handler.Handle(context.Background(), sellListCommand(t, "SYN-SCOPE-A", "SYN-SCOPE-B"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome != application.RequestEvaluationFormed || !result.Evaluation.BaseRateTableMiss() {
		t.Fatalf("outcome = %s baseMiss = %v，最后一格是基础运费表缺格，应形成待判断评价", result.Outcome, result.HasEvaluation && result.Evaluation.BaseRateTableMiss())
	}
	if got := trailKinds(result.ScopeTrail); got != "SYN-SCOPE-A:BASE_RATE_NOT_IN_TABLE,SYN-SCOPE-B:BASE_RATE_NOT_IN_TABLE" {
		t.Fatalf("trail = %s", got)
	}

	none, _ := sellListFixture(t, map[string]ports.PriceCardInForceResolution{}, map[string]domain.PricingInputSnapshot{})
	result, err = none.handler.Handle(context.Background(), sellListCommand(t, "SYN-SCOPE-A", "SYN-SCOPE-B"))
	if err != nil {
		t.Fatalf("no card: %v", err)
	}
	if result.Outcome != application.RequestPriceCardNotConfigured || result.HasEvaluation {
		t.Fatalf("outcome = %s has = %v，最后一格没有卡时不形成评价", result.Outcome, result.HasEvaluation)
	}
	if got := trailKinds(result.ScopeTrail); got != "SYN-SCOPE-A:NO_CARD,SYN-SCOPE-B:NO_CARD" {
		t.Fatalf("trail = %s", got)
	}
}

func TestOneScopeStaysOnTheExistingPath(t *testing.T) {
	fixture := newFormFixture(t, resolvedInputs(t))
	alone := requestCommand(t)
	listed := alone
	listed.Scopes = []domain.PricingScopeID{alone.Scope}

	first, err := fixture.handler.Handle(context.Background(), alone)
	if err != nil || first.Outcome != application.RequestEvaluationFormed {
		t.Fatalf("alone: %v %s", err, first.Outcome)
	}
	fixture.byRequest.byReference = map[string]domain.PricingEvaluation{}
	fixture.identity.next = "EVAL-SYN-1"
	fixture.identity.minted = 0
	fixture.store.saved = 0
	fixture.store.byID = map[string]domain.PricingEvaluation{}
	second, err := fixture.handler.Handle(context.Background(), listed)
	if err != nil || second.Outcome != application.RequestEvaluationFormed {
		t.Fatalf("listed: %v %s", err, second.Outcome)
	}
	if first.Evaluation.SemanticDigest() != second.Evaluation.SemanticDigest() {
		t.Fatal("只带一个范围时，列成一项与不列的语义摘要不同")
	}
	if len(second.ScopeTrail) != 0 {
		t.Fatalf("trail = %v，单范围不该有", second.ScopeTrail)
	}
}

// 以下三条钉绑了分区目录的卡。分区不由调用方给，要等目录读数补齐才有：探针不先补读数，这类卡在看运费表之前就落
// 待判断，运费表缺格永远看不到。上面几条的夹具不带目录，补不补读数它们都过，所以另立。

// catalogueBoundSellPlan 是 sellScopePlan 加一条分区目录绑定：卡不再读调用方给的分区。
func catalogueBoundSellPlan(t *testing.T, scope, zone, planID string) domain.PricingPlanVersion {
	t.Helper()
	base := sellScopePlan(t, scope, zone, planID)
	link, err := domain.NewReferenceCatalogueLink(domain.CatalogueKindZone, "SYN-CAT-ZONE")
	if err != nil {
		t.Fatalf("link: %v", err)
	}
	structures, err := (domain.PricingPlanStructures{}).WithReferenceCatalogues(link)
	if err != nil {
		t.Fatalf("structures: %v", err)
	}
	plan, err := domain.NewPricingPlanVersion(
		base.Reference(), base.Scope(), base.Direction(), base.Purpose(), base.BaseChargeCode(),
		base.EffectivePeriod(), base.RateTable(), base.WeightPolicy(), nil, structures,
	)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	return plan
}

// postalSellInput 是只带邮编路线、不带分区的输入，与生产上按包裹造的快照同形。
func postalSellInput(t *testing.T, scope, destination string) domain.PricingInputSnapshot {
	t.Helper()
	subject, err := domain.NewAcceptedPackageSubject(mustValue(t, domain.NewPackageID, "SYN-PKG-1"))
	if err != nil {
		t.Fatalf("subject: %v", err)
	}
	actual, err := domain.NewWeight(mustValue(t, domain.ParseDecimal, "5"), domain.WeightUnitKilogram)
	if err != nil {
		t.Fatalf("weight: %v", err)
	}
	route, err := domain.NewPostalRoute("", destination)
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	input, err := domain.NewPostalPricingInputSnapshot(
		mustValue(t, domain.NewTenantID, "tenant-1"), mustValue(t, domain.NewPricingScopeID, scope),
		subject, route, actual, nil, requestBasisAt)
	if err != nil {
		t.Fatalf("input: %v", err)
	}
	return input
}

// catalogueListFixture 给有序范围夹具的评价编排装上目录那一对口。目录取 zoneChartVersion：目的前缀 902 是 Z1。
func catalogueListFixture(
	t *testing.T,
	cards map[string]ports.PriceCardInForceResolution,
	destination string,
	inForce *catalogueInForceDouble,
) (*formFixture, *scopedPriceCards) {
	t.Helper()
	fixture := newFormFixture(t, nil)
	scoped := &scopedPriceCards{byScope: cards}
	fixture.priceCards = nil
	inputs := map[string]domain.PricingInputSnapshot{}
	for scope := range cards {
		inputs[scope] = postalSellInput(t, scope, destination)
	}
	handler, err := application.NewFormEvaluationFromRequestHandler(application.FormEvaluationFromRequestDeps{
		Evaluations: fixture.byRequest,
		PriceCards:  scoped,
		Identity:    fixture.identity,
		Evaluate: application.NewEvaluatePricingHandler(application.EvaluatePricingDeps{
			Store:            fixture.store,
			Downstream:       fixture.downstream,
			Clock:            fixedClock{at: requestBasisAt},
			CatalogueInForce: inForce,
			Catalogues: &catalogueReadingDouble{registrations: map[string]domain.ReferenceCatalogueRegistration{
				"v2": zoneChartVersion(t, "v2"),
			}},
		}),
		Inputs:          &scopedInputs{byScope: inputs},
		ServiceProducts: fixture.products,
	})
	if err != nil {
		t.Fatalf("construct handler: %v", err)
	}
	fixture.handler = handler
	return fixture, scoped
}

func zoneCatalogueInForce(t *testing.T) *catalogueInForceDouble {
	t.Helper()
	return &catalogueInForceDouble{reference: catalogueVersion(t, "v2"), outcome: ports.CatalogueVersionInForce}
}

// Covers: ADR-0380 决定三在绑了分区目录的卡上——分区由目录解出；靠前的卡运费表没有这一格，落到后一个范围，落空的
// 那一格不入册。靠前的卡有这一格时就用它，后一个范围不问。
func TestACatalogueBoundCardFallsThroughOnABaseRateGap(t *testing.T) {
	gap := catalogueBoundSellPlan(t, "SYN-SCOPE-A", "Z9", "SYN-PLAN-A")
	hit := catalogueBoundSellPlan(t, "SYN-SCOPE-B", "Z1", "SYN-PLAN-B")
	fixture, _ := catalogueListFixture(t, map[string]ports.PriceCardInForceResolution{
		"SYN-SCOPE-A": inForce(gap),
		"SYN-SCOPE-B": inForce(hit),
	}, "90210", zoneCatalogueInForce(t))

	result, err := fixture.handler.Handle(context.Background(), sellListCommand(t, "SYN-SCOPE-A", "SYN-SCOPE-B"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome != application.RequestEvaluationFormed || result.Evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("outcome = %s status = %s issues = %v", result.Outcome, result.Evaluation.Status(), result.Evaluation.Issues())
	}
	if got := trailKinds(result.ScopeTrail); got != "SYN-SCOPE-A:BASE_RATE_NOT_IN_TABLE,SYN-SCOPE-B:SELECTED" {
		t.Fatalf("trail = %s", got)
	}
	if result.Evaluation.PlanReference().ID() != "SYN-PLAN-B" {
		t.Fatalf("plan = %s，应出自后一个范围的卡", result.Evaluation.PlanReference().ID())
	}
	if fixture.identity.minted != 1 || fixture.store.saved != 1 {
		t.Fatalf("minted = %d saved = %d，落空的那一格不该入册", fixture.identity.minted, fixture.store.saved)
	}

	first := catalogueBoundSellPlan(t, "SYN-SCOPE-A", "Z1", "SYN-PLAN-A")
	negotiated, cards := catalogueListFixture(t, map[string]ports.PriceCardInForceResolution{
		"SYN-SCOPE-A": inForce(first),
		"SYN-SCOPE-B": inForce(hit),
	}, "90210", zoneCatalogueInForce(t))
	result, err = negotiated.handler.Handle(context.Background(), sellListCommand(t, "SYN-SCOPE-A", "SYN-SCOPE-B"))
	if err != nil {
		t.Fatalf("negotiated: %v", err)
	}
	if got := trailKinds(result.ScopeTrail); got != "SYN-SCOPE-A:SELECTED" || result.Evaluation.PlanReference().ID() != "SYN-PLAN-A" {
		t.Fatalf("trail = %s plan = %s，谈下来的线路应停在靠前的卡上", got, result.Evaluation.PlanReference().ID())
	}
	for _, asked := range cards.asked {
		if asked == "SYN-SCOPE-B" {
			t.Fatalf("asked = %v，靠前的卡有这一格时不该问后一个范围", cards.asked)
		}
	}
}

// Covers: ADR-0380 越权风险点 4——目录解不出分区是目录未决，停在问到的这一格，不回退到后一个范围。
func TestAnUnresolvedCatalogueStopsOnTheScopeAsked(t *testing.T) {
	first := catalogueBoundSellPlan(t, "SYN-SCOPE-A", "Z1", "SYN-PLAN-A")
	second := catalogueBoundSellPlan(t, "SYN-SCOPE-B", "Z1", "SYN-PLAN-B")
	fixture, cards := catalogueListFixture(t, map[string]ports.PriceCardInForceResolution{
		"SYN-SCOPE-A": inForce(first),
		"SYN-SCOPE-B": inForce(second),
	}, "10001", zoneCatalogueInForce(t))

	result, err := fixture.handler.Handle(context.Background(), sellListCommand(t, "SYN-SCOPE-A", "SYN-SCOPE-B"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome != application.RequestEvaluationFormed || result.Evaluation.Status() != domain.EvaluationPending ||
		result.Evaluation.BaseRateTableMiss() {
		t.Fatalf("outcome = %s status = %s baseMiss = %v，目录解不出应形成待判断评价，且不算运费表缺格",
			result.Outcome, result.Evaluation.Status(), result.Evaluation.BaseRateTableMiss())
	}
	if got := trailKinds(result.ScopeTrail); got != "SYN-SCOPE-A:STOPPED" {
		t.Fatalf("trail = %s", got)
	}
	for _, asked := range cards.asked {
		if asked == "SYN-SCOPE-B" {
			t.Fatalf("asked = %v，目录未决不该回退到后一个范围", cards.asked)
		}
	}
}

// Covers: 探针补读数时依赖故障答未决并点名这一格，不上抛成硬失败，也不入册、不铸标识。
type switchingInputs struct {
	parcel   func(scope string) domain.PricingInputSnapshot
	shipment func(scope string) domain.PricingInputSnapshot
}

func (double switchingInputs) ResolvePricingInput(_ context.Context, query ports.PricingInputQuery) (ports.PricingInputResolution, error) {
	var input domain.PricingInputSnapshot
	if query.Sources.ShipmentReferenced() {
		input = double.shipment(query.Scope.String())
	} else {
		input = double.parcel(query.Scope.String())
	}
	return ports.PricingInputResolution{Outcome: ports.PricingInputResolved, Input: input}, nil
}

type shipmentMemberDouble struct {
	picture ports.ShipmentMemberPicture
	found   bool
}

func (double shipmentMemberDouble) LoadShipmentMembers(context.Context, domain.TenantID, string) (ports.ShipmentMemberPicture, bool, error) {
	return double.picture, double.found, nil
}

func zonePackageInput(t *testing.T, scope, zone string) domain.PricingInputSnapshot {
	t.Helper()
	input := sellScopeInput(t, scope)
	zoned, err := input.WithCallerZone(zone)
	if err != nil {
		t.Fatalf("分区：%v", err)
	}
	return zoned
}

func zoneShipmentInput(t *testing.T, scope, zone string) domain.PricingInputSnapshot {
	t.Helper()
	ids := []domain.PackageID{
		mustValue(t, domain.NewPackageID, "SYN-PARCEL-1"),
		mustValue(t, domain.NewPackageID, "SYN-PARCEL-2"),
	}
	manifest, err := domain.NewMemberManifest(ids, mustValue(t, func(raw string) (domain.Weight, error) {
		return domain.NewWeight(mustValue(t, domain.ParseDecimal, raw), domain.WeightUnitKilogram)
	}, "5"), nil)
	if err != nil {
		t.Fatalf("清单：%v", err)
	}
	subject, err := domain.NewShipmentSubject("SYN-SHIP-1")
	if err != nil {
		t.Fatalf("委托：%v", err)
	}
	input, err := domain.NewAggregatePricingInputSnapshot(
		mustValue(t, domain.NewTenantID, "tenant-1"), mustValue(t, domain.NewPricingScopeID, scope),
		subject, zone, manifest, requestBasisAt,
	)
	if err != nil {
		t.Fatalf("输入：%v", err)
	}
	return input
}

func multiZonePlan(t *testing.T, scope, planID string, aggregate bool, zones []string) domain.PricingPlanVersion {
	t.Helper()
	base := sellScopePlan(t, scope, zones[0], planID)
	if len(zones) == 1 && !aggregate {
		return base
	}
	currency := mustValue(t, domain.NewCurrency, "USD")
	entries := make([]domain.RateEntry, 0, len(zones))
	for index, zone := range zones {
		entry, err := domain.NewRateEntry(mustValue(t, domain.NewRateEntryID, planID+"-"+zone), zone,
			mustValue(t, func(raw string) (domain.Weight, error) {
				return domain.NewWeight(mustValue(t, domain.ParseDecimal, raw), domain.WeightUnitKilogram)
			}, "0"),
			mustValue(t, func(raw string) (domain.Weight, error) {
				return domain.NewWeight(mustValue(t, domain.ParseDecimal, raw), domain.WeightUnitKilogram)
			}, "10"),
			mustValue(t, func(raw string) (domain.Money, error) {
				return domain.NewMoney(mustValue(t, domain.ParseDecimal, raw), currency)
			}, "12"),
		)
		if err != nil {
			t.Fatalf("价表行：%v", err)
		}
		entries = append(entries, entry)
		_ = index
	}
	table, err := domain.NewRateTableVersion(base.RateTable().Reference(), domain.RateTableFamilyWeightZone, currency,
		domain.WeightUnitKilogram, base.EffectivePeriod(), entries)
	if err != nil {
		t.Fatalf("价表：%v", err)
	}
	if !aggregate {
		plan, err := domain.NewPricingPlanVersion(base.Reference(), base.Scope(), base.Direction(), base.Purpose(),
			base.BaseChargeCode(), base.EffectivePeriod(), table, base.WeightPolicy(), nil, domain.PricingPlanStructures{})
		if err != nil {
			t.Fatalf("逐包裹卡：%v", err)
		}
		return plan
	}
	plan, err := domain.NewAggregatePricingPlanVersion(domain.AggregationPerShipment, base.Reference(), base.Scope(),
		base.Direction(), base.Purpose(), base.BaseChargeCode(), base.EffectivePeriod(), table, base.WeightPolicy(), nil, domain.PricingPlanStructures{})
	if err != nil {
		t.Fatalf("逐委托卡：%v", err)
	}
	return plan
}

func mismatchFixture(t *testing.T, cards map[string]ports.PriceCardInForceResolution, members *shipmentMemberDouble) *application.FormEvaluationFromRequestHandler {
	t.Helper()
	fixture := newFormFixture(t, nil)
	inputs := switchingInputs{
		parcel:   func(scope string) domain.PricingInputSnapshot { return zonePackageInput(t, scope, "Z2") },
		shipment: func(scope string) domain.PricingInputSnapshot { return zoneShipmentInput(t, scope, "Z2") },
	}
	deps := application.FormEvaluationFromRequestDeps{
		Evaluations:     fixture.byRequest,
		PriceCards:      &scopedPriceCards{byScope: cards},
		Identity:        fixture.identity,
		Evaluate:        application.NewEvaluatePricingHandler(application.EvaluatePricingDeps{Store: fixture.store, Downstream: fixture.downstream, Clock: fixedClock{at: requestBasisAt}}),
		Inputs:          inputs,
		ServiceProducts: fixture.products,
	}
	if members != nil {
		deps.ShipmentMembers = members
	}
	handler, err := application.NewFormEvaluationFromRequestHandler(deps)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	return handler
}

// Covers: 委托级请求撞上只写了 Z1 的逐包裹卡时，两件都缺格就继续问下一张逐委托卡。
func TestAShipmentRequestSkipsAPackageCardThatMissesEveryMember(t *testing.T) {
	perPackage := multiZonePlan(t, "SYN-SCOPE-A", "SYN-PLAN-PKG", false, []string{"Z1"})
	perShipment := multiZonePlan(t, "SYN-SCOPE-B", "SYN-PLAN-SHIP", true, []string{"Z1", "Z2"})
	handler := mismatchFixture(t, map[string]ports.PriceCardInForceResolution{
		"SYN-SCOPE-A": inForce(perPackage),
		"SYN-SCOPE-B": inForce(perShipment),
	}, nil)
	command := sellListCommand(t, "SYN-SCOPE-A", "SYN-SCOPE-B")
	command.Sources = ports.EligibleSourceReferences{Shipment: "SYN-SHIP-1", Members: []string{"SYN-PARCEL-1", "SYN-PARCEL-2"}}
	result, err := handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome != application.RequestEvaluationFormed || result.Evaluation.Status() != domain.EvaluationCompleted ||
		result.Evaluation.PlanReference().ID() != "SYN-PLAN-SHIP" {
		t.Fatalf("outcome = %s status = %s plan = %s issues = %v", result.Outcome, result.Evaluation.Status(), result.Evaluation.PlanReference().ID(), result.Evaluation.Issues())
	}
	if got := trailKinds(result.ScopeTrail); got != "SYN-SCOPE-A:BASE_RATE_NOT_IN_TABLE,SYN-SCOPE-B:SELECTED" {
		t.Fatalf("trail = %s", got)
	}
}

// Covers: 包裹级请求撞上只写了 Z1 的逐委托卡时，整票缺格就继续问下一张逐包裹卡。
func TestAParcelRequestSkipsAShipmentCardThatMissesTheMembers(t *testing.T) {
	perShipment := multiZonePlan(t, "SYN-SCOPE-A", "SYN-PLAN-SHIP", true, []string{"Z1"})
	perPackage := multiZonePlan(t, "SYN-SCOPE-B", "SYN-PLAN-PKG", false, []string{"Z1", "Z2"})
	handler := mismatchFixture(t, map[string]ports.PriceCardInForceResolution{
		"SYN-SCOPE-A": inForce(perShipment),
		"SYN-SCOPE-B": inForce(perPackage),
	}, &shipmentMemberDouble{found: true, picture: ports.ShipmentMemberPicture{
		Shipment: "SYN-SHIP-1", Members: []string{"SYN-PARCEL-1", "SYN-PARCEL-2"},
	}})
	result, err := handler.Handle(context.Background(), sellListCommand(t, "SYN-SCOPE-A", "SYN-SCOPE-B"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome != application.RequestEvaluationFormed || result.Evaluation.Status() != domain.EvaluationCompleted ||
		result.Evaluation.PlanReference().ID() != "SYN-PLAN-PKG" {
		t.Fatalf("outcome = %s status = %s plan = %s issues = %v", result.Outcome, result.Evaluation.Status(), result.Evaluation.PlanReference().ID(), result.Evaluation.Issues())
	}
	if got := trailKinds(result.ScopeTrail); got != "SYN-SCOPE-A:BASE_RATE_NOT_IN_TABLE,SYN-SCOPE-B:SELECTED" {
		t.Fatalf("trail = %s", got)
	}
}

func TestAReadingFailureDuringTheProbeAnswersUndecided(t *testing.T) {
	first := catalogueBoundSellPlan(t, "SYN-SCOPE-A", "Z1", "SYN-PLAN-A")
	second := catalogueBoundSellPlan(t, "SYN-SCOPE-B", "Z1", "SYN-PLAN-B")
	broken := zoneCatalogueInForce(t)
	broken.err = errors.New("catalogue register unreachable")
	fixture, _ := catalogueListFixture(t, map[string]ports.PriceCardInForceResolution{
		"SYN-SCOPE-A": inForce(first),
		"SYN-SCOPE-B": inForce(second),
	}, "90210", broken)

	result, err := fixture.handler.Handle(context.Background(), sellListCommand(t, "SYN-SCOPE-A", "SYN-SCOPE-B"))
	if err != nil {
		t.Fatalf("handle: %v，补读数读不回应答未决", err)
	}
	if result.Outcome != application.RequestEvaluationUndecided || result.Reason != application.RequestEvaluationFormationUndecided {
		t.Fatalf("outcome = %s reason = %s", result.Outcome, result.Reason)
	}
	if got := trailKinds(result.ScopeTrail); got != "SYN-SCOPE-A:UNDECIDED" {
		t.Fatalf("trail = %s", got)
	}
	if fixture.identity.minted != 0 || fixture.store.saved != 0 {
		t.Fatalf("minted = %d saved = %d，未决不该铸标识、不该入册", fixture.identity.minted, fixture.store.saved)
	}
}

type listFixture struct {
	handler *application.FormEvaluationFromRequestHandler
	cards   *scopedPriceCards
}

func sellListFixtureOnly(t *testing.T, cards map[string]ports.PriceCardInForceResolution) listFixture {
	t.Helper()
	fixture, scoped := sellListFixture(t, cards, map[string]domain.PricingInputSnapshot{})
	return listFixture{handler: fixture.handler, cards: scoped}
}

func trailKinds(trail []application.SellPriceSourceStep) string {
	parts := make([]string, 0, len(trail))
	for _, step := range trail {
		parts = append(parts, step.Scope.String()+":"+step.Kind)
	}
	return joinComma(parts)
}

func joinComma(parts []string) string {
	out := ""
	for index, part := range parts {
		if index > 0 {
			out += ","
		}
		out += part
	}
	return out
}

func explanationHas(evaluation domain.PricingEvaluation, fragment string) bool {
	for _, line := range evaluation.Explanation() {
		if line == fragment {
			return true
		}
	}
	return false
}
