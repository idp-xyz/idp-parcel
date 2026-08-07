package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// The card refuses a parcel past its last size tier rather than charging for
// it. CONTEXT gives that its own outcome: 明确排除形成不可计价. Reporting it as
// 已完成 would need an amount, and the only amount available is zero — which
// the zero-amount prohibition exists to stop, because a zero-amount completed
// evaluation reaches settlement as "we priced it and charged nothing".
func TestExcludedParcelIsUnratableRatherThanPricedAtZero(t *testing.T) {
	plan := planWithStructures(t, exclusionStructures(t, "48"))
	evaluation := evaluateWithSides(t, plan, "eval-excluded", "50")

	if evaluation.Status() != domain.EvaluationUnratable {
		t.Fatalf("status = %s, issues = %#v, want UNRATABLE", evaluation.Status(), evaluation.Issues())
	}
	if _, formed := evaluation.Total(); formed {
		t.Fatal("an excluded evaluation produced a total")
	}
	if lines := evaluation.ChargeLines(); len(lines) != 0 {
		t.Fatalf("charge lines = %#v, want none", lines)
	}
}

// CONTEXT: 结果携带排除依据. A status alone cannot be checked against the card,
// so the clause that refused the parcel has to reach the explanation.
func TestUnratableEvaluationNamesTheClauseThatRefusedIt(t *testing.T) {
	plan := planWithStructures(t, exclusionStructures(t, "48"))
	evaluation := evaluateWithSides(t, plan, "eval-excluded-explained", "50")

	if !explanationMentions(evaluation, "oversize-refusal") {
		t.Fatalf("explanation = %#v, want the refusing clause named", evaluation.Explanation())
	}
}

// A clause that does not hold must leave pricing alone; otherwise declaring any
// refusal would refuse everything.
func TestExclusionRuleThatMissesLeavesPricingAlone(t *testing.T) {
	plan := planWithStructures(t, exclusionStructures(t, "96"))
	evaluation := evaluateWithSides(t, plan, "eval-not-excluded", "50")

	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s, issues = %#v", evaluation.Status(), evaluation.Issues())
	}
	if total, _ := evaluation.Total(); total.Amount().String() != "10" {
		t.Fatalf("total = %s, want the ordinary 10", total.Amount().String())
	}
}

// Refusal is a settled conclusion, so it has to be reached before anything that
// would report a shortfall. A missing series reading is 待判断, which promises
// the caller that supplying it can produce a price — on an excluded parcel that
// promise is false and the caller retries forever.
func TestExclusionIsDecidedBeforeAMissingSeriesReadingCanDeferIt(t *testing.T) {
	structures, err := seriesPlan(t, "0.8").Structures().WithExclusionRules(oversizeRefusal(t, "48"))
	if err != nil {
		t.Fatalf("attach exclusion: %v", err)
	}
	plan := planWithStructures(t, structures)
	evaluation := evaluateWithSides(t, plan, "eval-excluded-without-reading", "50")

	if evaluation.Status() != domain.EvaluationUnratable {
		t.Fatalf("status = %s, issues = %#v, want UNRATABLE rather than a deferral", evaluation.Status(), evaluation.Issues())
	}
}

// Two plans that refuse different parcels are different plans, so the clause
// has to reach the content digest rather than ride along uncounted.
func TestPricingPlanContentDigestCoversExclusionRules(t *testing.T) {
	without := planWithStructures(t, declaredSurcharges(t))
	with := planWithStructures(t, exclusionStructures(t, "48"))
	other := planWithStructures(t, exclusionStructures(t, "96"))

	if without.ContentDigest() == with.ContentDigest() {
		t.Fatal("declaring an exclusion rule left the content digest unchanged")
	}
	if with.ContentDigest() == other.ContentDigest() {
		t.Fatal("changing the exclusion threshold left the content digest unchanged")
	}
}

func exclusionStructures(t testing.TB, threshold string) domain.PricingPlanStructures {
	t.Helper()
	structures, err := declaredSurcharges(t).WithExclusionRules(oversizeRefusal(t, threshold))
	if err != nil {
		t.Fatalf("exclusion structures: %v", err)
	}
	return structures
}

func oversizeRefusal(t testing.TB, threshold string) domain.ExclusionRule {
	t.Helper()
	condition, err := domain.NewLengthFeatureCondition(
		domain.FeatureLongestSide,
		domain.ComparisonGreaterThan,
		length(t, threshold, domain.LengthUnitInch),
	)
	if err != nil {
		t.Fatalf("condition: %v", err)
	}
	rule, err := domain.NewExclusionRule("oversize-refusal", "over the last size tier the card carries", leafTrigger(t, condition))
	if err != nil {
		t.Fatalf("exclusion rule: %v", err)
	}
	return rule
}
