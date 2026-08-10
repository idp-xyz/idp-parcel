package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// Covers: CONTEXT「卡上一条含多个替代项的条款只计收一次，把它拆成多条规则会按替代项个数
// 重复计收」— 权威价卡的 `R40` 就是一条含三个替代项的条款：实重超 67.5 KG、最长边超
// 274 CM，或长加围超 419 CM。单个谓词说不出这件事。
func TestAnyOfTriggerHitsWhenOneAlternativeHolds(t *testing.T) {
	trigger := oversizeLimitTrigger(t)
	for _, testCase := range []struct {
		name    string
		sides   domain.Dimensions
		weight  string
		matched bool
	}{
		{"within every limit", dimensions(t, "100", "40", "40", domain.LengthUnitCentimeter), "10", false},
		{"only the longest side is over", dimensions(t, "280", "40", "40", domain.LengthUnitCentimeter), "10", true},
		{"only the weight is over", dimensions(t, "100", "40", "40", domain.LengthUnitCentimeter), "70", true},
		{"only length plus girth is over", dimensions(t, "100", "90", "90", domain.LengthUnitCentimeter), "10", true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			packageFeatures := featuresOf(t, testCase.sides, testCase.weight)
			matched, err := trigger.Matches(packageFeatures)
			if err != nil {
				t.Fatalf("matches: %v", err)
			}
			if matched != testCase.matched {
				t.Fatalf("matched = %v, want %v", matched, testCase.matched)
			}
		})
	}
}

// 区间型条款要求两端同时成立。DHL 的 Non-Conveyable Piece 就是承运商自己的例子：它适用于
// 56 至 150 磅之间的件，超过 150 磅的件改收 Overweight。
func TestAllOfTriggerNeedsEveryAlternativeToHold(t *testing.T) {
	trigger := weightBandTrigger(t, "56", "150")
	for _, testCase := range []struct {
		name    string
		weight  string
		matched bool
	}{
		{"below the band", "55.9", false},
		{"on the lower bound", "56", true},
		{"inside the band", "100", true},
		{"on the upper bound", "150", true},
		{"above the band", "150.1", false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			packageFeatures := featuresOf(t, dimensions(t, "10", "10", "10", domain.LengthUnitInch), testCase.weight)
			matched, err := trigger.Matches(packageFeatures)
			if err != nil {
				t.Fatalf("matches: %v", err)
			}
			if matched != testCase.matched {
				t.Fatalf("weight %s matched = %v, want %v", testCase.weight, matched, testCase.matched)
			}
		})
	}
}

// Covers: CONTEXT「把含端边界改写成严格边界会迫使转抄者自造下一个可表示值」— 区间两端是
// 含端还是严格，取决于承运商的写法，所以比较运算集合必须把两种读法都带上。
func TestComparisonSetCarriesBothInclusiveAndStrictBounds(t *testing.T) {
	for _, testCase := range []struct {
		operator domain.ComparisonOperator
		weight   string
		matched  bool
	}{
		{domain.ComparisonGreaterThan, "50", false},
		{domain.ComparisonGreaterThanOrEqual, "50", true},
		{domain.ComparisonLessThan, "50", false},
		{domain.ComparisonLessThanOrEqual, "50", true},
	} {
		t.Run(testCase.operator.String(), func(t *testing.T) {
			trigger := leafTrigger(t, boundedWeightCondition(t, testCase.operator, "50"))
			matched, err := trigger.Matches(featuresOf(t, dimensions(t, "10", "10", "10", domain.LengthUnitInch), testCase.weight))
			if err != nil {
				t.Fatalf("matches: %v", err)
			}
			if matched != testCase.matched {
				t.Fatalf("%s at the threshold matched = %v, want %v", testCase.operator, matched, testCase.matched)
			}
		})
	}
}

// Covers: CONTEXT「空组合不成立——它没有卡可能表达的读法，当作恒真或恒假都是在替卡发明
// 规则」— 所以它被拒绝，而不是静默读成恒真或恒假。
func TestTriggerCombinationRefusesAnEmptyOperandList(t *testing.T) {
	if _, err := domain.NewAnyOfTrigger(); !errors.Is(err, domain.ErrInvalidFeatureCondition) {
		t.Fatalf("empty any-of error = %v", err)
	}
	if _, err := domain.NewAllOfTrigger(); !errors.Is(err, domain.ErrInvalidFeatureCondition) {
		t.Fatalf("empty all-of error = %v", err)
	}
}

// 在不同替代项上触发的两条规则是两条不同的规则，所以进内容摘要的必须是整个组合，而不是
// 只有它的第一个叶子。
func TestPricingPlanContentDigestCoversTheTriggerCombination(t *testing.T) {
	single := planWithStructures(t, structuresWithTrigger(t, leafTrigger(t, boundedWeightCondition(t, domain.ComparisonGreaterThan, "50"))))
	combined := planWithStructures(t, structuresWithTrigger(t, weightBandTrigger(t, "50", "150")))
	if single.ContentDigest() == combined.ContentDigest() {
		t.Fatal("the trigger combination was omitted from the plan content digest")
	}
	anyOf := planWithStructures(t, structuresWithTrigger(t, anyOfWeightTrigger(t, "50", "150")))
	if combined.ContentDigest() == anyOf.ContentDigest() {
		t.Fatal("and-versus-or was omitted from the plan content digest")
	}
}

func structuresWithTrigger(t testing.TB, trigger domain.TriggerCondition) domain.PricingPlanStructures {
	t.Helper()
	currency := mustValue(t, domain.NewCurrency, "USD")
	calculation, err := domain.NewFixedAmountSurcharge(money(t, "10", currency))
	if err != nil {
		t.Fatalf("calculation: %v", err)
	}
	rule, err := domain.NewSurchargeRule(
		"ahs-dimension",
		mustValue(t, domain.NewChargeCode, "AHS_DIMENSION"),
		"ahs-dimension",
		domain.ChargeEffectAdd,
		trigger,
		calculation,
	)
	if err != nil {
		t.Fatalf("surcharge rule: %v", err)
	}
	structures, err := domain.NewPricingPlanStructures([]domain.SurchargeRule{standaloneRule(t, rule)}, nil, nil)
	if err != nil {
		t.Fatalf("plan structures: %v", err)
	}
	return structures
}

func featuresOf(t testing.TB, sides domain.Dimensions, actual string) domain.PackageFeatures {
	t.Helper()
	packageFeatures, err := domain.NewPackageFeatures(sides, weight(t, actual, domain.WeightUnitPound))
	if err != nil {
		t.Fatalf("package features: %v", err)
	}
	return packageFeatures
}

func boundedWeightCondition(t testing.TB, operator domain.ComparisonOperator, threshold string) domain.FeatureCondition {
	t.Helper()
	condition, err := domain.NewWeightFeatureCondition(domain.FeatureActualWeight, operator, weight(t, threshold, domain.WeightUnitPound))
	if err != nil {
		t.Fatalf("weight condition: %v", err)
	}
	return condition
}

func leafTrigger(t testing.TB, condition domain.FeatureCondition) domain.TriggerCondition {
	t.Helper()
	trigger, err := domain.NewTrigger(condition)
	if err != nil {
		t.Fatalf("leaf trigger: %v", err)
	}
	return trigger
}

func weightBandTrigger(t testing.TB, lower, upper string) domain.TriggerCondition {
	t.Helper()
	trigger, err := domain.NewAllOfTrigger(
		leafTrigger(t, boundedWeightCondition(t, domain.ComparisonGreaterThanOrEqual, lower)),
		leafTrigger(t, boundedWeightCondition(t, domain.ComparisonLessThanOrEqual, upper)),
	)
	if err != nil {
		t.Fatalf("weight band trigger: %v", err)
	}
	return trigger
}

func anyOfWeightTrigger(t testing.TB, lower, upper string) domain.TriggerCondition {
	t.Helper()
	trigger, err := domain.NewAnyOfTrigger(
		leafTrigger(t, boundedWeightCondition(t, domain.ComparisonGreaterThanOrEqual, lower)),
		leafTrigger(t, boundedWeightCondition(t, domain.ComparisonLessThanOrEqual, upper)),
	)
	if err != nil {
		t.Fatalf("any-of weight trigger: %v", err)
	}
	return trigger
}

// oversizeLimitTrigger 就是 `R40`：超 67.5 KG，或最长边超 274 CM，或长加围超 419 CM。
// 阈值是卡的内容，这里按一个价卡版本会提供它们的方式供给。
func oversizeLimitTrigger(t testing.TB) domain.TriggerCondition {
	t.Helper()
	longest, err := domain.NewLengthFeatureCondition(
		domain.FeatureLongestSide,
		domain.ComparisonGreaterThan,
		length(t, "274", domain.LengthUnitCentimeter),
	)
	if err != nil {
		t.Fatalf("longest side condition: %v", err)
	}
	girth, err := domain.NewLengthFeatureCondition(
		domain.FeatureLengthAndGirth,
		domain.ComparisonGreaterThan,
		length(t, "419", domain.LengthUnitCentimeter),
	)
	if err != nil {
		t.Fatalf("length and girth condition: %v", err)
	}
	heavy, err := domain.NewWeightFeatureCondition(
		domain.FeatureActualWeight,
		domain.ComparisonGreaterThan,
		weight(t, "67.5", domain.WeightUnitPound),
	)
	if err != nil {
		t.Fatalf("weight condition: %v", err)
	}
	trigger, err := domain.NewAnyOfTrigger(leafTrigger(t, heavy), leafTrigger(t, longest), leafTrigger(t, girth))
	if err != nil {
		t.Fatalf("oversize limit trigger: %v", err)
	}
	return trigger
}
