package application_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// excludingPlan 是 minimalPlan 加一条拒收条款：自身计价重量超过 2 千克即拒（价表按千克查，进位 0.5 千克）。
func excludingPlan(t *testing.T) domain.PricingPlanVersion {
	t.Helper()
	threshold, err := domain.NewWeight(mustValue(t, domain.ParseDecimal, "2"), domain.WeightUnitKilogram)
	if err != nil {
		t.Fatalf("threshold: %v", err)
	}
	condition, err := domain.NewWeightFeatureCondition(domain.FeatureChargeableWeight, domain.ComparisonGreaterThan, threshold)
	if err != nil {
		t.Fatalf("condition: %v", err)
	}
	trigger, err := domain.NewTrigger(condition)
	if err != nil {
		t.Fatalf("trigger: %v", err)
	}
	refusal, err := domain.NewExclusionRule("overlimit", "over 2 kg chargeable", trigger)
	if err != nil {
		t.Fatalf("exclusion: %v", err)
	}
	structures, err := (domain.PricingPlanStructures{}).WithExclusionRules(refusal)
	if err != nil {
		t.Fatalf("structures: %v", err)
	}
	base := minimalPlan(t)
	plan, err := domain.NewPricingPlanVersion(base.Reference(), base.Scope(), base.Direction(), base.Purpose(), base.BaseChargeCode(),
		base.EffectivePeriod(), base.RateTable(), base.WeightPolicy(), nil, structures)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	return plan
}

func excludedPoundCommand(t *testing.T, id string) application.EvaluatePricingCommand {
	t.Helper()
	subject, err := domain.NewAcceptedPackageSubject(mustValue(t, domain.NewPackageID, "package-1"))
	if err != nil {
		t.Fatalf("subject: %v", err)
	}
	actual, err := domain.NewWeight(mustValue(t, domain.ParseDecimal, "5"), domain.WeightUnitPound)
	if err != nil {
		t.Fatalf("actual: %v", err)
	}
	sides, err := domain.NewDimensions(mustValue(t, domain.ParseDecimal, "10"), mustValue(t, domain.ParseDecimal, "10"), mustValue(t, domain.ParseDecimal, "10"), domain.LengthUnitCentimeter)
	if err != nil {
		t.Fatalf("sides: %v", err)
	}
	input, err := domain.NewPricingInputSnapshot(mustValue(t, domain.NewTenantID, "tenant-1"), mustValue(t, domain.NewPricingScopeID, "scope-1"),
		subject, "Z1", actual, &sides, time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("input: %v", err)
	}
	request, err := domain.NewEvaluationRequest(mustValue(t, domain.NewEvaluationID, id), excludingPlan(t), input, domain.EvidenceSynthetic)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	return application.EvaluatePricingCommand{Request: request}
}

// Covers: ADR-0346 决定三 + ADR-0347 决定一——输入按磅给、卡按千克查、拒收条款读自身计价重量：评价在拒收那一步不可计价，没走到正式计价重量，
// 换算记录仍随评价入册；同标识重复请求借这条记录比对，认作同一次计算（已有结果），不判成冲突、不重复入册。
func TestAnExcludedPoundParcelIsRecognisedAgainByTheSameRequest(t *testing.T) {
	fixture := newEvaluateFixture(t)

	first, err := fixture.handler.Handle(context.Background(), excludedPoundCommand(t, "eval-excluded-pound"))
	if err != nil || first.Outcome() != application.EvaluationRecorded {
		t.Fatalf("first = %q %v", first.Outcome(), err)
	}
	evaluation, _ := first.Evaluation()
	if evaluation.Status() != domain.EvaluationUnratable || len(evaluation.UnitConversions()) != 1 {
		t.Fatalf("status %s conversions %d issues %v; the early exit must keep the conversion", evaluation.Status(), len(evaluation.UnitConversions()), evaluation.Issues())
	}

	again, err := fixture.handler.Handle(context.Background(), excludedPoundCommand(t, "eval-excluded-pound"))
	if err != nil || again.Outcome() != application.EvaluationExistingResult || fixture.store.saved != 1 {
		t.Fatalf("again = %q %v saved %d；同标识重复请求被判成了冲突或重复入册", again.Outcome(), err, fixture.store.saved)
	}
}
