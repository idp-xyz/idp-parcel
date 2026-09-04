package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件证第三种计价参考序列——按期公布的金额（ADR-0110）：取值带币种、附加费计算「取当期序列定额」、
// 窗口由期次表达、窗外行为由卡声明。夹具全为 SYN 合成序列（S 级），金额与窗口都是编出来的形状。

func usd(t testing.TB) domain.Currency {
	t.Helper()
	return mustValue(t, domain.NewCurrency, "USD")
}

func amountSeriesValue(t testing.TB, id, version, amount, currency string) domain.ReferenceSeriesValue {
	t.Helper()
	value, err := domain.NewPublishedAmountSeriesValue(
		versionReference(t, domain.ArtifactReferenceSeries, id, version),
		money(t, amount, mustValue(t, domain.NewCurrency, currency)))
	if err != nil {
		t.Fatalf("published amount value %s: %v", id, err)
	}
	return value
}

func absentSeriesReading(t testing.TB, id, version string) domain.ReferenceSeriesValue {
	t.Helper()
	value, err := domain.NewAbsentSeriesReading(domain.ReferenceSeriesPublishedAmount, versionReference(t, domain.ArtifactReferenceSeries, id, version))
	if err != nil {
		t.Fatalf("absent reading %s: %v", id, err)
	}
	return value
}

// alwaysTrigger 是一条对任何包裹都成立的触发条件（实重 ≥ 0 KG）：本文件测的是金额从哪来，不是条件。
func alwaysTrigger(t testing.TB) domain.TriggerCondition {
	t.Helper()
	condition, err := domain.NewWeightFeatureCondition(domain.FeatureActualWeight, domain.ComparisonGreaterThanOrEqual, weight(t, "0", domain.WeightUnitKilogram))
	if err != nil {
		t.Fatalf("weight condition: %v", err)
	}
	return leafTrigger(t, condition)
}

func seriesAmountRule(t testing.TB, id, code, seriesID string, behaviour domain.OutOfWindowBehaviour, trigger domain.TriggerCondition) domain.SurchargeRule {
	t.Helper()
	calculation, err := domain.NewSeriesAmountSurcharge(seriesID, behaviour)
	if err != nil {
		t.Fatalf("series amount surcharge: %v", err)
	}
	rule, err := domain.NewSurchargeRule(id, mustValue(t, domain.NewChargeCode, code), id, domain.ChargeEffectAdd, trigger, calculation)
	if err != nil {
		t.Fatalf("surcharge rule %s: %v", id, err)
	}
	return standaloneRule(t, rule)
}

func amountBinding(t testing.TB, seriesID string) domain.ReferenceSeriesBinding {
	t.Helper()
	binding, err := domain.NewReferenceSeriesBinding(domain.ReferenceSeriesPublishedAmount, seriesID)
	if err != nil {
		t.Fatalf("binding %s: %v", seriesID, err)
	}
	return binding
}

func peakPlan(t testing.TB, behaviour domain.OutOfWindowBehaviour) domain.PricingPlanVersion {
	t.Helper()
	structures, err := domain.NewPricingPlanStructures(
		[]domain.SurchargeRule{seriesAmountRule(t, "peak", "PEAK", "pss-weekly", behaviour, alwaysTrigger(t))},
		nil,
		[]domain.ReferenceSeriesBinding{amountBinding(t, "pss-weekly")},
	)
	if err != nil {
		t.Fatalf("structures: %v", err)
	}
	return planWithStructures(t, structures)
}

func evaluateWithReadings(t testing.TB, id string, plan domain.PricingPlanVersion, zone string, readings ...domain.ReferenceSeriesValue) domain.PricingEvaluation {
	t.Helper()
	input := syntheticInputWithDimensions(t, "5", zone, dimensions(t, "10", "10", "10", domain.LengthUnitInch))
	if len(readings) > 0 {
		attached, err := input.WithReferenceSeries(readings...)
		if err != nil {
			t.Fatalf("attach readings: %v", err)
		}
		input = attached
	}
	return evaluate(t, id, plan, input)
}

// Covers: ADR-0110 Decision 一「一期取值是带币种的金额」——金额序列的取值带币种，费率序列不许带；缺币种的
// 金额取值立不住。
func TestPublishedAmountSeriesValueCarriesACurrency(t *testing.T) {
	value := amountSeriesValue(t, "pss-weekly", "v1", "3.5", "USD")
	amount, ok := value.Amount()
	if !ok || amount.Amount().String() != "3.5" || amount.Currency().String() != "USD" {
		t.Fatalf("amount = %v %v", amount, ok)
	}
	if _, err := domain.NewReferenceSeriesValue(domain.ReferenceSeriesPublishedAmount, versionReference(t, domain.ArtifactReferenceSeries, "pss-weekly", "v1"), decimal(t, "3.5")); !errors.Is(err, domain.ErrInvalidReferenceSeries) {
		t.Fatalf("published amount without a currency accepted: %v", err)
	}
	if _, ok := seriesValue(t, "fuel-weekly", "v1", "0.2").Amount(); ok {
		t.Fatal("a fuel rate reading claims to carry an amount")
	}
	if _, err := domain.NewAbsentSeriesReading(domain.ReferenceSeriesFuelRate, versionReference(t, domain.ArtifactReferenceSeries, "fuel-weekly", "v1")); !errors.Is(err, domain.ErrInvalidReferenceSeries) {
		t.Fatalf("an absent reading for a rate series accepted: %v", err)
	}
}

// Covers: ADR-0110 Decision 一、二——金额序列在登记面照序列那一套：spec 带币种，缺币种或费率序列带币种都拒；
// 按基准时点解析得到带币种的金额取值。
func TestPublishedAmountSeriesRegistrationDeclaresItsCurrency(t *testing.T) {
	spec := fuelSeriesSpec(t)
	spec.Kind = domain.ReferenceSeriesPublishedAmount
	spec.Reference = versionReference(t, domain.ArtifactReferenceSeries, "SYN-PRC-PSS", "v1")
	if _, err := domain.NewReferenceSeriesRegistration(spec); !errors.Is(err, domain.ErrInvalidReferenceSeriesRegistration) {
		t.Fatalf("amount series without a currency accepted: %v", err)
	}
	spec.Currency = usd(t)
	registration, err := domain.NewReferenceSeriesRegistration(spec)
	if err != nil {
		t.Fatalf("amount series: %v", err)
	}
	reading, found := registration.ResolveAt(seriesWeekOne.Add(time.Hour))
	if !found {
		t.Fatal("first period not resolved")
	}
	if amount, ok := reading.Value().Amount(); !ok || amount.Amount().String() != "0.22" || amount.Currency().String() != "USD" {
		t.Fatalf("resolved amount = %v %v", amount, ok)
	}
	raw, err := domain.MarshalReferenceSeriesRegistration(registration)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	restored, err := domain.RehydrateReferenceSeriesRegistration(raw)
	if err != nil || restored.ContentDigest() != registration.ContentDigest() {
		t.Fatalf("round trip: %v", err)
	}
	if currency, ok := restored.Currency(); !ok || currency.String() != "USD" {
		t.Fatalf("currency lost across the snapshot: %v %v", currency, ok)
	}

	fuel := fuelSeriesSpec(t)
	fuel.Currency = usd(t)
	if _, err := domain.NewReferenceSeriesRegistration(fuel); !errors.Is(err, domain.ErrInvalidReferenceSeriesRegistration) {
		t.Fatalf("fuel rate series with a currency accepted: %v", err)
	}
}

// Covers: ADR-0110 Decision 二「取当期序列定额」——按评价基准时点解出的那一期金额就是附加费金额，定额不带
// 基数依赖；解释项记下取自哪条序列哪一版。
func TestSeriesAmountSurchargeTakesThePublishedAmountOfThePeriod(t *testing.T) {
	plan := peakPlan(t, domain.OutOfWindowNotCharged)
	evaluation := evaluateWithReadings(t, "eval-pss-in-window", plan, "Z1", amountSeriesValue(t, "pss-weekly", "v3", "3.5", "USD"))
	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s %v", evaluation.Status(), issueCodes(evaluation))
	}
	total, _ := evaluation.Total()
	if total.Amount().String() != "13.5" {
		t.Fatalf("total = %s, want 10 base + 3.5 published amount", total.Amount())
	}
	if len(evaluation.ChargeLines()) != 2 || evaluation.ChargeLines()[1].Code().String() != "PEAK" {
		t.Fatalf("lines = %#v", evaluation.ChargeLines())
	}
	if !explanationMentions(evaluation, "pss-weekly@v3") {
		t.Fatalf("explanation = %#v, want the series version the amount came from", evaluation.Explanation())
	}
	frozen := false
	for _, reference := range evaluation.Manifest().References() {
		if reference.Kind() == domain.ArtifactReferenceSeries && reference.ID() == "pss-weekly" && reference.Version() == "v3" {
			frozen = true
		}
	}
	if !frozen {
		t.Fatalf("manifest = %#v, want pss-weekly@v3 frozen in", evaluation.Manifest().References())
	}
}

// Covers: ADR-0110 Decision 三——窗口由期次表达，窗外行为由卡声明：不计收即该规则不形成费用行且解释留痕；
// 待判断即 REFERENCE_SERIES_UNRESOLVED；根本没有读数（无在用版本）时不论声明什么都待判断——那是数据缺口。
func TestOutOfWindowBehaviourIsDeclaredByTheCard(t *testing.T) {
	notCharged := evaluateWithReadings(t, "eval-pss-not-charged", peakPlan(t, domain.OutOfWindowNotCharged), "Z1", absentSeriesReading(t, "pss-weekly", "v3"))
	if notCharged.Status() != domain.EvaluationCompleted || len(notCharged.ChargeLines()) != 1 {
		t.Fatalf("not charged: %s lines=%d %v", notCharged.Status(), len(notCharged.ChargeLines()), issueCodes(notCharged))
	}
	if !explanationMentions(notCharged, "NOT_CHARGED") {
		t.Fatalf("explanation = %#v, want the declared out-of-window behaviour", notCharged.Explanation())
	}

	pending := evaluateWithReadings(t, "eval-pss-pending", peakPlan(t, domain.OutOfWindowPending), "Z1", absentSeriesReading(t, "pss-weekly", "v3"))
	if pending.Status() != domain.EvaluationPending || issueCodes(pending)[0] != "REFERENCE_SERIES_UNRESOLVED" {
		t.Fatalf("pending: %s %v", pending.Status(), issueCodes(pending))
	}

	noReading := evaluateWithReadings(t, "eval-pss-no-reading", peakPlan(t, domain.OutOfWindowNotCharged), "Z1")
	if noReading.Status() != domain.EvaluationPending || issueCodes(noReading)[0] != "REFERENCE_SERIES_UNRESOLVED" {
		t.Fatalf("no reading: %s %v", noReading.Status(), issueCodes(noReading))
	}

	if _, err := domain.NewSeriesAmountSurcharge("pss-weekly", domain.OutOfWindowBehaviour("ZERO")); !errors.Is(err, domain.ErrInvalidSurchargeRule) {
		t.Fatalf("out-of-window behaviour outside the closed set accepted: %v", err)
	}
	if _, err := domain.NewSeriesAmountSurcharge("pss-weekly", ""); !errors.Is(err, domain.ErrInvalidSurchargeRule) {
		t.Fatalf("undeclared out-of-window behaviour accepted: %v", err)
	}
}

// Covers: ADR-0110 Decision 二「金额币种须与方案币种一致」——币种在评价解出时才可知，不一致是冲突不是缺口。
func TestPublishedAmountCurrencyMustMatchThePlan(t *testing.T) {
	evaluation := evaluateWithReadings(t, "eval-pss-currency", peakPlan(t, domain.OutOfWindowNotCharged), "Z1", amountSeriesValue(t, "pss-weekly", "v3", "3.5", "EUR"))
	if evaluation.Status() != domain.EvaluationConflict || issueCodes(evaluation)[0] != "REFERENCE_SERIES_CURRENCY_MISMATCH" {
		t.Fatalf("status = %s %v", evaluation.Status(), issueCodes(evaluation))
	}
}

// Covers: ADR-0110 Decision 四「每分区一条附加费规则各绑一条金额序列」——一张卡可绑多条金额序列（费率序列
// 仍每种一条），规则按分区条件挑自己那条。
func TestEachZoneRuleBindsItsOwnAmountSeries(t *testing.T) {
	zoneCondition := func(zone string) domain.TriggerCondition {
		condition, err := domain.NewCategoryFeatureCondition(domain.FeatureZone, domain.CategoryValue(zone))
		if err != nil {
			t.Fatalf("zone condition: %v", err)
		}
		return leafTrigger(t, condition)
	}
	structures, err := domain.NewPricingPlanStructures(
		[]domain.SurchargeRule{
			seriesAmountRule(t, "peak-z1", "PEAK_Z1", "pss-z1", domain.OutOfWindowNotCharged, zoneCondition("Z1")),
			seriesAmountRule(t, "peak-z2", "PEAK_Z2", "pss-z2", domain.OutOfWindowNotCharged, zoneCondition("Z2")),
		},
		nil,
		[]domain.ReferenceSeriesBinding{amountBinding(t, "pss-z1"), amountBinding(t, "pss-z2")},
	)
	if err != nil {
		t.Fatalf("two amount bindings rejected: %v", err)
	}
	plan := planWithStructures(t, structures)
	evaluation := evaluateWithReadings(t, "eval-pss-zones", plan, "Z1",
		amountSeriesValue(t, "pss-z1", "v1", "2", "USD"), amountSeriesValue(t, "pss-z2", "v1", "9", "USD"))
	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s %v", evaluation.Status(), issueCodes(evaluation))
	}
	total, _ := evaluation.Total()
	if total.Amount().String() != "12" {
		t.Fatalf("total = %s, want 10 base + 2 for zone 1 only", total.Amount())
	}

	duplicateRate, _ := domain.NewReferenceSeriesBinding(domain.ReferenceSeriesFuelRate, "fuel-a")
	duplicateRateToo, _ := domain.NewReferenceSeriesBinding(domain.ReferenceSeriesFuelRate, "fuel-b")
	if _, err := domain.NewPricingPlanStructures(nil, nil, []domain.ReferenceSeriesBinding{duplicateRate, duplicateRateToo}); err == nil {
		t.Fatal("two fuel rate bindings accepted: a rate series is referenced by kind, so a card can bind only one")
	}
}

// Covers: 规范化与快照——金额序列计算、窗外行为与带币种 / 缺席的读数都进摘要与快照，往返后语义摘要自校仍过；
// PPC-5 不换号。
func TestSeriesAmountShapesSurviveSnapshotsWithinPPC5(t *testing.T) {
	plan := peakPlan(t, domain.OutOfWindowNotCharged)
	if plan.CanonicalizationVersion() != "PPC-5" {
		t.Fatalf("canonicalization = %q, want PPC-5", plan.CanonicalizationVersion())
	}
	other := peakPlan(t, domain.OutOfWindowPending)
	if plan.ContentDigest() == other.ContentDigest() {
		t.Fatal("two cards differing only in the out-of-window declaration share a digest")
	}
	raw, err := domain.MarshalPricingPlanSnapshot(plan)
	if err != nil {
		t.Fatalf("marshal plan: %v", err)
	}
	restored, err := domain.RehydratePricingPlanSnapshot(raw)
	if err != nil || restored.ContentDigest() != plan.ContentDigest() {
		t.Fatalf("plan round trip: %v", err)
	}
	calculation := restored.Structures().SurchargeRules()[0].Calculation()
	seriesID, behaviour, ok := calculation.SeriesAmount()
	if !ok || seriesID != "pss-weekly" || behaviour != domain.OutOfWindowNotCharged {
		t.Fatalf("series amount calculation lost: %q %q %v", seriesID, behaviour, ok)
	}

	for label, reading := range map[string]domain.ReferenceSeriesValue{
		"amount": amountSeriesValue(t, "pss-weekly", "v3", "3.5", "USD"),
		"absent": absentSeriesReading(t, "pss-weekly", "v3"),
	} {
		evaluation := evaluateWithReadings(t, "eval-pss-snapshot-"+label, plan, "Z1", reading)
		raw, err := domain.MarshalEvaluationSnapshot(evaluation)
		if err != nil {
			t.Fatalf("%s marshal: %v", label, err)
		}
		restored, err := domain.RehydrateEvaluationSnapshot(raw)
		if err != nil || restored.SemanticDigest() != evaluation.SemanticDigest() {
			t.Fatalf("%s evaluation round trip: %v", label, err)
		}
	}
}
