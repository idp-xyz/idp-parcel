package application_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 本文件证编排对金额序列（ADR-0110）的那一格：在用版本在计价基准时点没有期次时，编排把「查过这一版、无期次」
// 作缺席读数冻结进输入，纯函数按卡上声明的窗外行为分流——不计收的卡评价完成、不多收一行；费率序列照旧只留说明。

func amountSeriesReference(t *testing.T, version string) domain.VersionReference {
	t.Helper()
	reference, err := domain.NewVersionReferenceIdentity(domain.ArtifactReferenceSeries, "SYN-PSS", version)
	if err != nil {
		t.Fatalf("series reference: %v", err)
	}
	return reference
}

// pssRegistration 是一版只在某个窗口内有期次的金额序列：窗口外解析不到期次。
func pssRegistration(t *testing.T, version string) domain.ReferenceSeriesRegistration {
	t.Helper()
	period, err := domain.NewSeriesPeriodValue(
		time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 15, 0, 0, 0, 0, time.UTC),
		mustValue(t, domain.ParseDecimal, "3.5"), "SYN-EVIDENCE/pss")
	if err != nil {
		t.Fatalf("period: %v", err)
	}
	registration, err := domain.NewReferenceSeriesRegistration(domain.ReferenceSeriesRegistrationSpec{
		Tenant:           mustValue(t, domain.NewTenantID, "tenant-1"),
		Kind:             domain.ReferenceSeriesPublishedAmount,
		Reference:        amountSeriesReference(t, version),
		SourceIdentifier: "SYN-CARRIER/peak-bulletin",
		Registrant:       "SYN-OPS",
		Currency:         mustValue(t, domain.NewCurrency, "USD"),
		Periods:          []domain.SeriesPeriodValue{period},
	})
	if err != nil {
		t.Fatalf("registration: %v", err)
	}
	return registration
}

type amountSeriesReadingDouble struct {
	registration domain.ReferenceSeriesRegistration
}

func (double *amountSeriesReadingDouble) Register(context.Context, domain.ReferenceSeriesRegistration) (ports.ReferenceSeriesRegistrationOutcome, error) {
	return ports.ReferenceSeriesRegistrationOutcomeInvalid, nil
}

func (double *amountSeriesReadingDouble) ResolveAt(_ context.Context, _ domain.TenantID, _ domain.VersionReference, asOf time.Time) (domain.ResolvedSeriesReading, bool, error) {
	reading, found := double.registration.ResolveAt(asOf)
	return reading, found, nil
}

// pssPlan 是 minimalPlan 加一条对任何包裹都成立的「取当期序列定额」规则，窗外不计收。
func pssPlan(t *testing.T) domain.PricingPlanVersion {
	t.Helper()
	base := minimalPlan(t)
	condition, err := domain.NewWeightFeatureCondition(domain.FeatureActualWeight, domain.ComparisonGreaterThanOrEqual,
		mustWeight(t, "0"))
	if err != nil {
		t.Fatalf("condition: %v", err)
	}
	trigger, err := domain.NewTrigger(condition)
	if err != nil {
		t.Fatalf("trigger: %v", err)
	}
	calculation, err := domain.NewSeriesAmountSurcharge("SYN-PSS", domain.OutOfWindowNotCharged)
	if err != nil {
		t.Fatalf("calculation: %v", err)
	}
	rule, err := domain.NewSurchargeRule("peak", mustValue(t, domain.NewChargeCode, "PEAK"), "peak", domain.ChargeEffectAdd, trigger, calculation)
	if err != nil {
		t.Fatalf("rule: %v", err)
	}
	rule, err = rule.Standalone()
	if err != nil {
		t.Fatalf("standalone: %v", err)
	}
	binding, err := domain.NewReferenceSeriesBinding(domain.ReferenceSeriesPublishedAmount, "SYN-PSS")
	if err != nil {
		t.Fatalf("binding: %v", err)
	}
	structures, err := domain.NewPricingPlanStructures([]domain.SurchargeRule{rule}, nil, []domain.ReferenceSeriesBinding{binding})
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

func mustWeight(t *testing.T, value string) domain.Weight {
	t.Helper()
	weight, err := domain.NewWeight(mustValue(t, domain.ParseDecimal, value), domain.WeightUnitKilogram)
	if err != nil {
		t.Fatalf("weight: %v", err)
	}
	return weight
}

func pssCommand(t *testing.T, id string, basisAt time.Time) application.EvaluatePricingCommand {
	t.Helper()
	subject, err := domain.NewAcceptedPackageSubject(mustValue(t, domain.NewPackageID, "package-1"))
	if err != nil {
		t.Fatalf("subject: %v", err)
	}
	sides, err := domain.NewDimensions(mustValue(t, domain.ParseDecimal, "10"), mustValue(t, domain.ParseDecimal, "10"), mustValue(t, domain.ParseDecimal, "10"), domain.LengthUnitCentimeter)
	if err != nil {
		t.Fatalf("dimensions: %v", err)
	}
	input, err := domain.NewPricingInputSnapshot(
		mustValue(t, domain.NewTenantID, "tenant-1"),
		mustValue(t, domain.NewPricingScopeID, "scope-1"),
		subject, "Z1", mustWeight(t, "5"), &sides, basisAt,
	)
	if err != nil {
		t.Fatalf("input: %v", err)
	}
	request, err := domain.NewEvaluationRequest(mustValue(t, domain.NewEvaluationID, id), pssPlan(t), input, domain.EvidenceSynthetic)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	return application.EvaluatePricingCommand{Request: request}
}

// Covers: ADR-0110 Decision 三——窗内取到定额并计收；窗外「查过这一版、无期次」冻结进输入，不计收的卡评价完成、
// 只有基础运费一行，且在用版本仍冻进清单。
func TestOutOfWindowAmountSeriesIsFrozenAsAnAbsentReading(t *testing.T) {
	newFixture := func() (*application.EvaluatePricingHandler, *evaluationStoreDouble) {
		store := &evaluationStoreDouble{byID: map[string]domain.PricingEvaluation{}}
		handler := application.NewEvaluatePricingHandler(application.EvaluatePricingDeps{
			Store:          store,
			Downstream:     &evaluationDownstreamDouble{},
			Clock:          fixedClock{at: formedAt},
			InForce:        &inForceDouble{reference: amountSeriesReference(t, "v1"), outcome: ports.SeriesVersionInForce},
			SeriesVersions: &amountSeriesReadingDouble{registration: pssRegistration(t, "v1")},
		})
		return handler, store
	}

	inWindow, _ := newFixture()
	result, err := inWindow.Handle(context.Background(), pssCommand(t, "eval-pss-in", time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)))
	if err != nil || result.Outcome() != application.EvaluationRecorded {
		t.Fatalf("outcome = %q err = %v", result.Outcome(), err)
	}
	evaluation, _ := result.Evaluation()
	if total, _ := evaluation.Total(); evaluation.Status() != domain.EvaluationCompleted || total.Amount().String() != "13.5" {
		t.Fatalf("in window: status = %q total = %v issues = %#v", evaluation.Status(), total, evaluation.Issues())
	}

	outOfWindow, _ := newFixture()
	result, err = outOfWindow.Handle(context.Background(), pssCommand(t, "eval-pss-out", time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)))
	if err != nil || result.Outcome() != application.EvaluationRecorded {
		t.Fatalf("outcome = %q err = %v", result.Outcome(), err)
	}
	evaluation, _ = result.Evaluation()
	if total, _ := evaluation.Total(); evaluation.Status() != domain.EvaluationCompleted || total.Amount().String() != "10" || len(evaluation.ChargeLines()) != 1 {
		t.Fatalf("out of window: status = %q total = %v lines = %d issues = %#v", evaluation.Status(), total, len(evaluation.ChargeLines()), evaluation.Issues())
	}
	readings := evaluation.Input().ReferenceSeriesValues()
	if len(readings) != 1 || !readings[0].Absent() || readings[0].Reference().Version() != "v1" {
		t.Fatalf("absent reading not frozen: %#v", readings)
	}
	if len(result.SeriesResolutionNotes()) != 1 {
		t.Fatalf("notes = %#v, want the no-period note kept for the reader", result.SeriesResolutionNotes())
	}
}
