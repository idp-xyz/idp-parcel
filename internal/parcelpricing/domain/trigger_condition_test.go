package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// `R40` on the authoritative card is one clause with three alternatives: actual
// weight over 67.5 KG, longest side over 274 CM, or length plus girth over 419
// CM. A single predicate cannot say that, and splitting it into three rules
// would charge three times for what the card charges once.
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

// A band needs both ends to hold at once. DHL's Non-Conveyable Piece is the
// carrier's own example: it applies to a piece weighing between 56 and 150 lbs,
// and a piece over 150 lbs is charged Overweight instead.
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

// A band's ends are inclusive or exclusive by the carrier's wording, so the
// comparison set has to carry both readings rather than force every threshold
// to be restated as a strict one.
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

// An empty combination has no truth value the card could have meant, so it is
// refused rather than silently reading as always-true or always-false.
func TestTriggerCombinationRefusesAnEmptyOperandList(t *testing.T) {
	if _, err := domain.NewAnyOfTrigger(); !errors.Is(err, domain.ErrInvalidFeatureCondition) {
		t.Fatalf("empty any-of error = %v", err)
	}
	if _, err := domain.NewAllOfTrigger(); !errors.Is(err, domain.ErrInvalidFeatureCondition) {
		t.Fatalf("empty all-of error = %v", err)
	}
}

// Two rules that trigger on different alternatives are different rules, so the
// combination has to reach the content digest rather than only its first leaf.
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

// oversizeLimitTrigger is `R40`: over 67.5 KG, or a longest side over 274 CM,
// or length plus girth over 419 CM. The thresholds are card content and are
// supplied here the way a card version would supply them.
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
