package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件证体积重与自身计价重量进入判定特征（ADR-0347 决定一至三、五）：评价器在判定任何条件之前一次算好这两个量，
// 超重一类的条款因此能按「计费口径的重量」触发，而不是只读实重；不读它们的卡一步不多走。

// ownWeightSpec 描述一张只为这组用例造的卡：价表 0 至 100 千克一档 10 美元，按 0.5 千克向上进位；MAX 时体积系数
// 5000（立方厘米折千克）、体积重按 0.1 千克向上进位。
type ownWeightSpec struct {
	method      domain.PricingWeightMethod
	aggregation domain.AggregationMode
	tableUnit   domain.WeightUnit
	structures  domain.PricingPlanStructures
}

func ownWeightPlan(t testing.TB, spec ownWeightSpec) (domain.PricingPlanVersion, error) {
	t.Helper()
	if spec.tableUnit == "" {
		spec.tableUnit = domain.WeightUnitKilogram
	}
	if spec.aggregation == "" {
		spec.aggregation = domain.AggregationPerPackage
	}
	currency := mustValue(t, domain.NewCurrency, "USD")
	entry, err := domain.NewRateEntry(mustValue(t, domain.NewRateEntryID, "entry-own-weight"), "Z1",
		weight(t, "0", spec.tableUnit), weight(t, "100", spec.tableUnit), money(t, "10", currency))
	if err != nil {
		t.Fatalf("rate entry: %v", err)
	}
	table, err := domain.NewRateTableVersion(versionReference(t, domain.ArtifactRateTable, "table-own-weight", "v1"),
		domain.RateTableFamilyWeightZone, currency, spec.tableUnit, effectivePeriod(t), []domain.RateEntry{entry})
	if err != nil {
		t.Fatalf("rate table: %v", err)
	}
	rounding, err := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, weight(t, "0.5", spec.tableUnit))
	if err != nil {
		t.Fatalf("rounding: %v", err)
	}
	var factor *domain.VolumetricFactor
	if spec.method == domain.PricingWeightMax {
		factorRounding, roundingErr := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, weight(t, "0.1", spec.tableUnit))
		if roundingErr != nil {
			t.Fatalf("factor rounding: %v", roundingErr)
		}
		declared, factorErr := domain.NewVolumetricFactor(decimal(t, "5000"), domain.LengthUnitCentimeter, factorRounding)
		if factorErr != nil {
			t.Fatalf("factor: %v", factorErr)
		}
		factor = &declared
	}
	policy, err := domain.NewPricingWeightPolicy(versionReference(t, domain.ArtifactWeightPolicy, "weight-own-weight", "v1"),
		spec.method, rounding, factor)
	if err != nil {
		t.Fatalf("weight policy: %v", err)
	}
	return domain.NewAggregatePricingPlanVersion(spec.aggregation,
		versionReference(t, domain.ArtifactPricingPlan, "plan-own-weight", "v1"),
		mustValue(t, domain.NewPricingScopeID, "scope-1"),
		domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge,
		mustValue(t, domain.NewChargeCode, "BASE_FREIGHT"), effectivePeriod(t), table, policy, nil, spec.structures)
}

func mustOwnWeightPlan(t testing.TB, spec ownWeightSpec) domain.PricingPlanVersion {
	t.Helper()
	plan, err := ownWeightPlan(t, spec)
	if err != nil {
		t.Fatalf("own-weight plan: %v", err)
	}
	return plan
}

func ownWeightTrigger(t testing.TB, source domain.FeatureSource, operator domain.ComparisonOperator, threshold string, unit domain.WeightUnit) domain.TriggerCondition {
	t.Helper()
	condition, err := domain.NewWeightFeatureCondition(source, operator, weight(t, threshold, unit))
	if err != nil {
		t.Fatalf("%s condition: %v", source, err)
	}
	return leafTrigger(t, condition)
}

// weightSurcharge 造一条按重量特征触发的定额附加费，独立计收。
func weightSurcharge(t testing.TB, id, code string, trigger domain.TriggerCondition, amount string) domain.SurchargeRule {
	t.Helper()
	calculation, err := domain.NewFixedAmountSurcharge(money(t, amount, mustValue(t, domain.NewCurrency, "USD")))
	if err != nil {
		t.Fatalf("fixed surcharge: %v", err)
	}
	rule, err := domain.NewSurchargeRule(id, mustValue(t, domain.NewChargeCode, code), id, domain.ChargeEffectAdd, trigger, calculation)
	if err != nil {
		t.Fatalf("surcharge rule %s: %v", id, err)
	}
	return standaloneRule(t, rule)
}

func structuresOf(t testing.TB, rules ...domain.SurchargeRule) domain.PricingPlanStructures {
	t.Helper()
	structures, err := domain.NewPricingPlanStructures(rules, nil, nil)
	if err != nil {
		t.Fatalf("structures: %v", err)
	}
	return structures
}

// evaluateParcel 用 tenant-1 / scope-1 的合成输入评价一件带尺寸（厘米）的包裹，实重按千克。
func evaluateParcel(t testing.TB, id string, plan domain.PricingPlanVersion, actual, longest, middle, shortest string) domain.PricingEvaluation {
	t.Helper()
	return evaluate(t, id, plan, syntheticInputWithDimensions(t, actual, "Z1", dimensions(t, longest, middle, shortest, domain.LengthUnitCentimeter)))
}

func ownWeightTotal(t testing.TB, evaluation domain.PricingEvaluation) string {
	t.Helper()
	total, ok := evaluation.Total()
	if !ok {
		t.Fatalf("no total: status=%s issues=%v", evaluation.Status(), issueCodes(evaluation))
	}
	return total.Amount().String()
}

// Covers: CONTEXT「特征……计价重量」「判定条件」+ ADR-0347 决定一——超重费按自身计价重量触发：读的是实重与体积重取较大、
// 再按卡的取整策略进位之后的值。边界：恰等于阈值不触发（GT 是严格大于），阈值之上的最小可表示值（进位单位 0.5）触发。
func TestOverweightSurchargeTriggersOnTheChargeableWeightAtTheBoundary(t *testing.T) {
	overweight := weightSurcharge(t, "overweight", "OVERWEIGHT_SURCHARGE",
		ownWeightTrigger(t, domain.FeatureChargeableWeight, domain.ComparisonGreaterThan, "20", domain.WeightUnitKilogram), "7")
	plan := mustOwnWeightPlan(t, ownWeightSpec{method: domain.PricingWeightMax, structures: structuresOf(t, overweight)})

	for name, test := range map[string]struct {
		longest, middle, shortest string
		charged                   bool
		total                     string
		chargeable                string
	}{
		// 100000 立方厘米 / 5000 = 20.0 千克，进位后仍是 20.0：等于阈值，不触发。
		"体积重恰等于阈值": {"50", "40", "50", false, "10", "20 kg"},
		// 100500 / 5000 = 20.1，按 0.5 进位得 20.5：阈值之上的最小可表示值，触发。
		"阈值加最小位": {"50.25", "40", "50", true, "17", "20.5 kg"},
		// 实重 10 千克远小于阈值，触发的只可能是体积重 24.0。
		"体积重远超实重": {"60", "50", "40", true, "17", "24 kg"},
	} {
		t.Run(name, func(t *testing.T) {
			evaluation := evaluateParcel(t, "eval-chargeable-"+strings.ReplaceAll(name, " ", "-"), plan, "10", test.longest, test.middle, test.shortest)
			if evaluation.Status() != domain.EvaluationCompleted {
				t.Fatalf("status=%s issues=%v", evaluation.Status(), issueCodes(evaluation))
			}
			if got := hasChargeCode(evaluation, "OVERWEIGHT_SURCHARGE"); got != test.charged {
				t.Fatalf("overweight charged = %v, want %v; explanation=%v", got, test.charged, evaluation.Explanation())
			}
			if got := ownWeightTotal(t, evaluation); got != test.total {
				t.Fatalf("total = %s, want %s", got, test.total)
			}
			if !explanationMentions(evaluation, "chargeable="+strings.ReplaceAll(test.chargeable, " kg", " KG")) {
				t.Fatalf("explanation = %v, want the chargeable weight %s recorded for the condition", evaluation.Explanation(), test.chargeable)
			}
		})
	}
}

// Covers: ADR-0347 决定一「自身计价重量是进位之后的值」——含端条件读到的是进位后的 20.0，而不是进位前的 19.8：
// 体积重 19.8 千克按 0.5 进位得 20.0，`>= 20` 因此成立。这是「计费口径」与读原始体积重的区别，用例把它钉住。
func TestChargeableWeightIsReadAfterRoundingNotBefore(t *testing.T) {
	atLeast := weightSurcharge(t, "heavy", "HEAVY_SURCHARGE",
		ownWeightTrigger(t, domain.FeatureChargeableWeight, domain.ComparisonGreaterThanOrEqual, "20", domain.WeightUnitKilogram), "7")
	rawGreater := weightSurcharge(t, "raw-heavy", "RAW_HEAVY_SURCHARGE",
		ownWeightTrigger(t, domain.FeatureVolumetricWeight, domain.ComparisonGreaterThanOrEqual, "20", domain.WeightUnitKilogram), "3")
	plan := mustOwnWeightPlan(t, ownWeightSpec{method: domain.PricingWeightMax, structures: structuresOf(t, atLeast, rawGreater)})

	// 99000 / 5000 = 19.8：体积重 19.8（按 0.1 进位仍是 19.8），自身计价重量进位到 20.0。
	evaluation := evaluateParcel(t, "eval-rounding-order", plan, "10", "50", "40", "49.5")
	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status=%s issues=%v", evaluation.Status(), issueCodes(evaluation))
	}
	if !hasChargeCode(evaluation, "HEAVY_SURCHARGE") {
		t.Fatalf("chargeable weight 19.8 → 20.0 after rounding must satisfy >= 20; explanation=%v", evaluation.Explanation())
	}
	if hasChargeCode(evaluation, "RAW_HEAVY_SURCHARGE") {
		t.Fatal("the volumetric weight itself is 19.8 and must not satisfy >= 20")
	}
}

// Covers: ADR-0347 决定一——体积重特征是卡的体积系数得出的值，与实重无关：实重 25 而体积重只有 0.2，按体积重触发的条件不成立；
// 反过来实重 5 而体积重 24，成立。
func TestVolumetricWeightFeatureIsIndependentOfTheActualWeight(t *testing.T) {
	rule := weightSurcharge(t, "bulky", "BULKY_SURCHARGE",
		ownWeightTrigger(t, domain.FeatureVolumetricWeight, domain.ComparisonGreaterThan, "20", domain.WeightUnitKilogram), "7")
	plan := mustOwnWeightPlan(t, ownWeightSpec{method: domain.PricingWeightMax, structures: structuresOf(t, rule)})

	dense := evaluateParcel(t, "eval-dense", plan, "25", "10", "10", "10")
	if hasChargeCode(dense, "BULKY_SURCHARGE") {
		t.Fatalf("a dense 25 kg parcel with 0.2 kg volumetric weight was charged as bulky: %v", dense.Explanation())
	}
	bulky := evaluateParcel(t, "eval-bulky", plan, "5", "60", "50", "40")
	if !hasChargeCode(bulky, "BULKY_SURCHARGE") {
		t.Fatalf("a 5 kg parcel with 24 kg volumetric weight was not charged as bulky: %v", bulky.Explanation())
	}
}

// Covers: ADR-0347 决定一「自身计价重量不含条件最低计价重量的抬高」——尺寸条件把方案级计价重量抬到 30 千克（基础价按 30 千克
// 查表），但条件读到的自身计价重量仍是 5.0：超重条款不因另一条条款的抬高而成立。
func TestChargeableWeightFeatureDoesNotSeeTheConditionalMinimumRaise(t *testing.T) {
	raise := conditionalMinimumOnLongestSide(t, "oversize-minimum", "100", "30")
	overweight := weightSurcharge(t, "overweight", "OVERWEIGHT_SURCHARGE",
		ownWeightTrigger(t, domain.FeatureChargeableWeight, domain.ComparisonGreaterThan, "20", domain.WeightUnitKilogram), "7")
	plan := mustOwnWeightPlan(t, ownWeightSpec{method: domain.PricingWeightMax, structures: structuresOf(t, raise, overweight)})

	evaluation := evaluateParcel(t, "eval-raise-unseen", plan, "5", "120", "10", "10")
	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status=%s issues=%v", evaluation.Status(), issueCodes(evaluation))
	}
	pricingWeight, ok := evaluation.PricingWeight()
	if !ok || pricingWeight.RoundedWeight().Value().String() != "30" {
		t.Fatalf("pricing weight = %v, want the raised 30", pricingWeight.RoundedWeight().Value().String())
	}
	if hasChargeCode(evaluation, "OVERWEIGHT_SURCHARGE") {
		t.Fatalf("the 30 kg raise leaked into the chargeable weight feature; explanation=%v", evaluation.Explanation())
	}
	if !explanationMentions(evaluation, "chargeable=5 KG") {
		t.Fatalf("explanation = %v, want chargeable=5 KG", evaluation.Explanation())
	}
}

// Covers: ADR-0347 决定三——抬高条件可以读体积重（它不取决于任何抬高）：体积重 24 触发下限 30，基础价按 30 千克查表。
func TestAConditionalMinimumMayReadTheVolumetricWeight(t *testing.T) {
	trigger := ownWeightTrigger(t, domain.FeatureVolumetricWeight, domain.ComparisonGreaterThan, "20", domain.WeightUnitKilogram)
	minimum, err := domain.NewConditionalMinimumWeight("bulky-minimum", trigger, weight(t, "30", domain.WeightUnitKilogram))
	if err != nil {
		t.Fatalf("minimum reading the volumetric weight: %v", err)
	}
	carrier := weightSurcharge(t, "bulky-fee", "BULKY_FEE",
		ownWeightTrigger(t, domain.FeatureVolumetricWeight, domain.ComparisonGreaterThan, "20", domain.WeightUnitKilogram), "2")
	carrier, err = carrier.WithConditionalMinimumWeight(minimum)
	if err != nil {
		t.Fatalf("attach minimum: %v", err)
	}
	plan := mustOwnWeightPlan(t, ownWeightSpec{method: domain.PricingWeightMax, structures: structuresOf(t, carrier)})

	evaluation := evaluateParcel(t, "eval-volumetric-minimum", plan, "5", "60", "50", "40")
	pricingWeight, ok := evaluation.PricingWeight()
	if evaluation.Status() != domain.EvaluationCompleted || !ok || pricingWeight.RoundedWeight().Value().String() != "30" {
		t.Fatalf("status=%s pricing weight=%v issues=%v", evaluation.Status(), pricingWeight.RoundedWeight().Value().String(), issueCodes(evaluation))
	}
}

// Covers: ADR-0347 决定三——条件最低计价重量的触发条件不得读自身计价重量，装配时拒（沿用 ErrInvalidSurchargeRule 的拒绝形式）；
// 嵌在组合条件里的也拒；读别的特征照常。
func TestAConditionalMinimumCannotBeTriggeredByTheChargeableWeight(t *testing.T) {
	chargeable := ownWeightTrigger(t, domain.FeatureChargeableWeight, domain.ComparisonGreaterThan, "20", domain.WeightUnitKilogram)
	_, err := domain.NewConditionalMinimumWeight("self", chargeable, weight(t, "30", domain.WeightUnitKilogram))
	if !errors.Is(err, domain.ErrConditionalMinimumReadsChargeableWeight) || !errors.Is(err, domain.ErrInvalidSurchargeRule) {
		t.Fatalf("err = %v, want the chargeable-weight refusal wrapping ErrInvalidSurchargeRule", err)
	}

	nested, err := domain.NewAllOfTrigger(
		leafTrigger(t, mustLengthCondition(t, domain.FeatureLongestSide, "100")),
		chargeable,
	)
	if err != nil {
		t.Fatalf("all-of: %v", err)
	}
	if _, err := domain.NewConditionalMinimumWeight("nested", nested, weight(t, "30", domain.WeightUnitKilogram)); !errors.Is(err, domain.ErrConditionalMinimumReadsChargeableWeight) {
		t.Fatalf("err = %v, want the chargeable-weight refusal for a nested leaf too", err)
	}

	actual := ownWeightTrigger(t, domain.FeatureActualWeight, domain.ComparisonGreaterThan, "20", domain.WeightUnitKilogram)
	if _, err := domain.NewConditionalMinimumWeight("actual", actual, weight(t, "30", domain.WeightUnitKilogram)); err != nil {
		t.Fatalf("a minimum triggered by the actual weight must stay legal: %v", err)
	}
}

// Covers: ADR-0347 决定二、三——读体积重与自身计价重量的条件在装配时校验：只按实重计价的卡没有体积系数，读体积重无从算起；
// 阈值须用价表的重量单位（特征判定不换算阈值）；读实重的条件阈值单位不受这一道管（既有行为不变）。
func TestPlanAssemblyRefusesConditionsThatCanNeverBeEvaluated(t *testing.T) {
	volumetricOnActualOnly := structuresOf(t, weightSurcharge(t, "bulky", "BULKY",
		ownWeightTrigger(t, domain.FeatureVolumetricWeight, domain.ComparisonGreaterThan, "20", domain.WeightUnitKilogram), "7"))
	if _, err := ownWeightPlan(t, ownWeightSpec{method: domain.PricingWeightActualOnly, structures: volumetricOnActualOnly}); !errors.Is(err, domain.ErrInvalidPricingPlan) {
		t.Fatalf("err = %v, want a card reading the volumetric weight without a volumetric factor refused", err)
	}

	poundThreshold := structuresOf(t, weightSurcharge(t, "heavy", "HEAVY",
		ownWeightTrigger(t, domain.FeatureChargeableWeight, domain.ComparisonGreaterThan, "44", domain.WeightUnitPound), "7"))
	if _, err := ownWeightPlan(t, ownWeightSpec{method: domain.PricingWeightMax, structures: poundThreshold}); !errors.Is(err, domain.ErrWeightUnitMismatch) {
		t.Fatalf("err = %v, want a pound threshold on a kilogram table refused", err)
	}

	chargeableOnActualOnly := structuresOf(t, weightSurcharge(t, "heavy", "HEAVY",
		ownWeightTrigger(t, domain.FeatureChargeableWeight, domain.ComparisonGreaterThan, "20", domain.WeightUnitKilogram), "7"))
	if _, err := ownWeightPlan(t, ownWeightSpec{method: domain.PricingWeightActualOnly, structures: chargeableOnActualOnly}); err != nil {
		t.Fatalf("the chargeable weight exists under ACTUAL_ONLY too: %v", err)
	}

	actualInPounds := structuresOf(t, weightSurcharge(t, "actual-pounds", "ACTUAL_POUNDS",
		ownWeightTrigger(t, domain.FeatureActualWeight, domain.ComparisonGreaterThan, "44", domain.WeightUnitPound), "7"))
	if _, err := ownWeightPlan(t, ownWeightSpec{method: domain.PricingWeightMax, structures: actualInPounds}); err != nil {
		t.Fatalf("a pound threshold on the actual weight was legal before and must stay legal: %v", err)
	}
}

// Covers: ADR-0347 决定一「缺输入答待判断，不是 0」——直接调判定条件：体积重没填进特征时报特征不可用，而不是当 0 命中
// 「小于 5」；评价一层：缺尺寸答待判断，没有合计、没有费用行。
func TestMissingDimensionsLeaveTheDerivedWeightsUnavailableNotZero(t *testing.T) {
	bare, err := domain.NewPackageFeatures(dimensions(t, "60", "40", "30", domain.LengthUnitCentimeter), weight(t, "12", domain.WeightUnitKilogram))
	if err != nil {
		t.Fatalf("features: %v", err)
	}
	lessThanFive, err := domain.NewWeightFeatureCondition(domain.FeatureVolumetricWeight, domain.ComparisonLessThan, weight(t, "5", domain.WeightUnitKilogram))
	if err != nil {
		t.Fatalf("condition: %v", err)
	}
	if matched, err := lessThanFive.Matches(bare); !errors.Is(err, domain.ErrFeatureUnavailable) || matched {
		t.Fatalf("matched=%v err=%v; an unfilled volumetric weight must be unavailable, never zero", matched, err)
	}

	rule := weightSurcharge(t, "overweight", "OVERWEIGHT_SURCHARGE",
		ownWeightTrigger(t, domain.FeatureChargeableWeight, domain.ComparisonGreaterThan, "20", domain.WeightUnitKilogram), "7")
	plan := mustOwnWeightPlan(t, ownWeightSpec{method: domain.PricingWeightMax, structures: structuresOf(t, rule)})
	evaluation := evaluate(t, "eval-no-dimensions", plan, syntheticInput(t, "25", "Z1"))
	if evaluation.Status() != domain.EvaluationPending || issueCodes(evaluation)[0] != "PACKAGE_FEATURES_UNAVAILABLE" {
		t.Fatalf("status=%s issues=%v, want PENDING PACKAGE_FEATURES_UNAVAILABLE", evaluation.Status(), issueCodes(evaluation))
	}
	if _, ok := evaluation.Total(); ok || len(evaluation.ChargeLines()) != 0 {
		t.Fatalf("a pending evaluation carries a total or charge lines: %v", evaluation.ChargeLines())
	}
}

// Covers: ADR-0347 决定一——票级主体没有尺寸：自身计价重量由合计实重与（MAX 时）合计体积重得出；MAX 而成员清单没声明合计体积重
// 仍是缺事实（DIMENSIONS_REQUIRED），ACTUAL_ONLY 的卡只读实重一路。
func TestAggregateSubjectsReadTheTotalsAsTheirOwnWeights(t *testing.T) {
	overweight := weightSurcharge(t, "overweight", "OVERWEIGHT_SURCHARGE",
		ownWeightTrigger(t, domain.FeatureChargeableWeight, domain.ComparisonGreaterThan, "20", domain.WeightUnitKilogram), "7")

	actualOnly := mustOwnWeightPlan(t, ownWeightSpec{method: domain.PricingWeightActualOnly, aggregation: domain.AggregationPerShipment, structures: structuresOf(t, overweight)})
	withTotals := evaluate(t, "eval-aggregate-actual", actualOnly, aggregateInput(t, shipmentSubject(t, "shipment-1"), members(t, "20.2", nil, "p-1", "p-2")))
	if withTotals.Status() != domain.EvaluationCompleted || !hasChargeCode(withTotals, "OVERWEIGHT_SURCHARGE") {
		t.Fatalf("status=%s issues=%v; total actual 20.2 → 20.5 after rounding should trip > 20", withTotals.Status(), issueCodes(withTotals))
	}

	maxPlan := mustOwnWeightPlan(t, ownWeightSpec{method: domain.PricingWeightMax, aggregation: domain.AggregationPerShipment, structures: structuresOf(t, overweight)})
	missing := evaluate(t, "eval-aggregate-max-missing", maxPlan, aggregateInput(t, shipmentSubject(t, "shipment-1"), members(t, "5", nil, "p-1", "p-2")))
	if missing.Status() != domain.EvaluationPending || issueCodes(missing)[0] != "DIMENSIONS_REQUIRED" {
		t.Fatalf("status=%s issues=%v, want PENDING DIMENSIONS_REQUIRED", missing.Status(), issueCodes(missing))
	}
	volumetric := "30"
	declared := evaluate(t, "eval-aggregate-max-declared", maxPlan, aggregateInput(t, shipmentSubject(t, "shipment-1"), members(t, "5", &volumetric, "p-1", "p-2")))
	if declared.Status() != domain.EvaluationCompleted || !hasChargeCode(declared, "OVERWEIGHT_SURCHARGE") {
		t.Fatalf("status=%s issues=%v; declared total volumetric 30 should trip > 20", declared.Status(), issueCodes(declared))
	}
}

// Covers: ADR-0346 决定三 + ADR-0347 决定一——输入按磅给、价卡按千克查表：自身计价重量读换算后的值（50 磅折 22.6796185 千克，
// 进位 23.0），评价记下的换算记录只有一条、解释里的换算行只有一行（算条件读到的重量那一遍不重复记）；重放得同一个摘要。
func TestChargeableWeightReadsTheConvertedWeightAndDoesNotRecordTheConversionTwice(t *testing.T) {
	overweight := weightSurcharge(t, "overweight", "OVERWEIGHT_SURCHARGE",
		ownWeightTrigger(t, domain.FeatureChargeableWeight, domain.ComparisonGreaterThan, "20", domain.WeightUnitKilogram), "7")
	plan := mustOwnWeightPlan(t, ownWeightSpec{method: domain.PricingWeightMax, structures: structuresOf(t, overweight)})
	sides := dimensions(t, "10", "10", "10", domain.LengthUnitCentimeter)
	input, err := domain.NewPricingInputSnapshot(
		mustValue(t, domain.NewTenantID, "tenant-1"), mustValue(t, domain.NewPricingScopeID, "scope-1"),
		packageSubject(t, "package-1"), "Z1", weight(t, "50", domain.WeightUnitPound), &sides,
		time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("input: %v", err)
	}

	evaluation := evaluate(t, "eval-converted-chargeable", plan, input)
	if evaluation.Status() != domain.EvaluationCompleted || !hasChargeCode(evaluation, "OVERWEIGHT_SURCHARGE") {
		t.Fatalf("status=%s issues=%v; 50 lb is 22.68 kg and should trip > 20 kg", evaluation.Status(), issueCodes(evaluation))
	}
	if steps := evaluation.UnitConversions(); len(steps) != 1 {
		t.Fatalf("conversion records = %d, want exactly one", len(steps))
	}
	conversions := 0
	for _, line := range evaluation.Explanation() {
		if strings.HasPrefix(line, "unit conversion") {
			conversions++
		}
	}
	if conversions != 1 {
		t.Fatalf("conversion explanation lines = %d, want 1; explanation=%v", conversions, evaluation.Explanation())
	}
	if !explanationMentions(evaluation, "chargeable=23 KG") {
		t.Fatalf("explanation = %v, want chargeable=23 KG", evaluation.Explanation())
	}

	replay, err := domain.ReplayPricingEvaluation(mustValue(t, domain.NewEvaluationID, "eval-converted-chargeable-2"), evaluation, plan, domain.EvidenceSynthetic)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.SemanticDigest() != evaluation.SemanticDigest() || replay.Status() != domain.EvaluationCompleted {
		t.Fatalf("replay digest/status differ: %s/%s", replay.SemanticDigest(), replay.Status())
	}
}

// Covers: ADR-0347 决定五——票级主体的合计体积重以另一个单位声明时会被换算并可能截断，读体积重的条件的阈值是拿来跟它比较的
// 有限小数，必须并入截断位数的边界（ADR-0346 越权风险点 2）。1 千克折磅是无限小数 2.20462262184877…，阈值
// 2.204622621848 比不带阈值时的保留位数还细：不并入，截断值落到阈值之下，条件错判为不成立。
func TestAThresholdFinerThanTheTruncationWidensTheBoundary(t *testing.T) {
	trigger := ownWeightTrigger(t, domain.FeatureVolumetricWeight, domain.ComparisonGreaterThan, "2.204622621848", domain.WeightUnitPound)
	rule := weightSurcharge(t, "bulky", "BULKY_SURCHARGE", trigger, "7")
	plan := mustOwnWeightPlan(t, ownWeightSpec{
		method: domain.PricingWeightMax, aggregation: domain.AggregationPerShipment, tableUnit: domain.WeightUnitPound,
		structures: structuresOf(t, rule),
	})
	volumetric := "1"
	evaluation := evaluate(t, "eval-boundary-widened", plan, aggregateInput(t, shipmentSubject(t, "shipment-1"), members(t, "0.5", &volumetric, "p-1", "p-2")))
	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status=%s issues=%v", evaluation.Status(), issueCodes(evaluation))
	}
	if !hasChargeCode(evaluation, "BULKY_SURCHARGE") {
		t.Fatalf("1 kg = 2.20462262184877… lb is above 2.204622621848 lb but the surcharge did not apply; explanation=%v", evaluation.Explanation())
	}
}

// Covers: ADR-0347 决定五 + 回放——评价快照往返之后，自身计价重量那一遍不改变任何东西：重建后摘要自校通过，重放得同一个摘要。
func TestAnEvaluationReadingTheOwnWeightsSurvivesTheSnapshotAndReplays(t *testing.T) {
	overweight := weightSurcharge(t, "overweight", "OVERWEIGHT_SURCHARGE",
		ownWeightTrigger(t, domain.FeatureChargeableWeight, domain.ComparisonGreaterThan, "20", domain.WeightUnitKilogram), "7")
	plan := mustOwnWeightPlan(t, ownWeightSpec{method: domain.PricingWeightMax, structures: structuresOf(t, overweight)})
	evaluation := evaluateParcel(t, "eval-own-weights-snapshot", plan, "10", "50.25", "40", "50")

	raw, err := domain.MarshalEvaluationSnapshot(evaluation)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	restored, err := domain.RehydrateEvaluationSnapshot(raw)
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	if restored.SemanticDigest() != evaluation.SemanticDigest() {
		t.Fatal("semantic digest changed across the snapshot")
	}
	_, rebuiltPlan := planSnapshotRoundTrip(t, plan)
	replay, err := domain.ReplayPricingEvaluation(mustValue(t, domain.NewEvaluationID, "eval-own-weights-snapshot-2"), restored, rebuiltPlan, domain.EvidenceSynthetic)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.SemanticDigest() != evaluation.SemanticDigest() || !hasChargeCode(replay, "OVERWEIGHT_SURCHARGE") {
		t.Fatalf("replay differs: digest %s vs %s", replay.SemanticDigest(), evaluation.SemanticDigest())
	}
}

// Covers: ADR-0347 决定六「不改变任何今天已算对的评价」——不读这两个特征的卡，评价的解释里不出现「条件读到的重量」一行，
// 内容摘要与不读它们的同形卡一致（新特征只在新装配的规则用到它们时才生效）。
func TestAPlanThatDoesNotReadTheDerivedWeightsPaysNothingForThem(t *testing.T) {
	actualOnly := weightSurcharge(t, "actual-heavy", "ACTUAL_HEAVY",
		ownWeightTrigger(t, domain.FeatureActualWeight, domain.ComparisonGreaterThan, "20", domain.WeightUnitKilogram), "7")
	plan := mustOwnWeightPlan(t, ownWeightSpec{method: domain.PricingWeightMax, structures: structuresOf(t, actualOnly)})
	evaluation := evaluateParcel(t, "eval-no-derived-read", plan, "25", "10", "10", "10")
	if evaluation.Status() != domain.EvaluationCompleted || !hasChargeCode(evaluation, "ACTUAL_HEAVY") {
		t.Fatalf("status=%s issues=%v", evaluation.Status(), issueCodes(evaluation))
	}
	if explanationMentions(evaluation, "package weights read by conditions") {
		t.Fatalf("a plan that reads neither derived weight got the extra explanation line: %v", evaluation.Explanation())
	}
}

func mustLengthCondition(t testing.TB, source domain.FeatureSource, threshold string) domain.FeatureCondition {
	t.Helper()
	condition, err := domain.NewLengthFeatureCondition(source, domain.ComparisonGreaterThan, length(t, threshold, domain.LengthUnitCentimeter))
	if err != nil {
		t.Fatalf("length condition: %v", err)
	}
	return condition
}

// conditionalMinimumOnLongestSide 造一条「最长边超过 threshold 厘米则计价重量不低于 minimum 千克」的抬高条款，
// 挂在一条同条件的定额附加费上（附加费本身收 1 美元）。
func conditionalMinimumOnLongestSide(t testing.TB, id, threshold, minimum string) domain.SurchargeRule {
	t.Helper()
	trigger := leafTrigger(t, mustLengthCondition(t, domain.FeatureLongestSide, threshold))
	floor, err := domain.NewConditionalMinimumWeight(id, trigger, weight(t, minimum, domain.WeightUnitKilogram))
	if err != nil {
		t.Fatalf("conditional minimum: %v", err)
	}
	rule := weightSurcharge(t, id+"-fee", strings.ToUpper(strings.ReplaceAll(id, "-", "_"))+"_FEE", trigger, "1")
	rule, err = rule.WithConditionalMinimumWeight(floor)
	if err != nil {
		t.Fatalf("attach minimum: %v", err)
	}
	return rule
}
