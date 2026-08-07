package domain_test

import (
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

func mustValue[T any](t testing.TB, constructor func(string) (T, error), value string) T {
	t.Helper()
	result, err := constructor(value)
	if err != nil {
		t.Fatalf("construct %q: %v", value, err)
	}
	return result
}

func decimal(t testing.TB, value string) domain.Decimal {
	t.Helper()
	return mustValue(t, domain.ParseDecimal, value)
}

func weight(t testing.TB, value string, unit domain.WeightUnit) domain.Weight {
	t.Helper()
	result, err := domain.NewWeight(decimal(t, value), unit)
	if err != nil {
		t.Fatalf("weight %s %s: %v", value, unit, err)
	}
	return result
}

func money(t testing.TB, value string, currency domain.Currency) domain.Money {
	t.Helper()
	result, err := domain.NewMoney(decimal(t, value), currency)
	if err != nil {
		t.Fatalf("money %s %s: %v", value, currency, err)
	}
	return result
}

func versionReference(t testing.TB, kind domain.ArtifactKind, id, version string) domain.VersionReference {
	t.Helper()
	reference, err := domain.NewVersionReference(kind, id, version, "sha256:syn-"+id+"-"+version)
	if err != nil {
		t.Fatalf("version reference %s: %v", id, err)
	}
	return reference
}

func effectivePeriod(t testing.TB) domain.EffectivePeriod {
	t.Helper()
	period, err := domain.NewEffectivePeriod(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("effective period: %v", err)
	}
	return period
}

func fixedRule(t testing.TB, id string, effect domain.ChargeEffect, value string, order int) domain.FixedChargeRule {
	t.Helper()
	currency := mustValue(t, domain.NewCurrency, "USD")
	codeValue := "RULE_" + strings.ToUpper(strings.NewReplacer("-", "_", ":", "_", "|", "_", " ", "_").Replace(id))
	code := mustValue(t, domain.NewChargeCode, codeValue)
	rule, err := domain.NewFixedChargeRule(id, code, id, effect, money(t, value, currency), order)
	if err != nil {
		t.Fatalf("fixed rule %s: %v", id, err)
	}
	return rule
}

func syntheticPlan(
	t testing.TB,
	suffix string,
	direction domain.PricingDirection,
	purpose domain.PricingPurpose,
	baseAmount string,
	method domain.BillableWeightMethod,
	rules []domain.FixedChargeRule,
) domain.PricingPlanVersion {
	return syntheticPlanWithBaseCode(t, suffix, direction, purpose, baseAmount, "BASE_FREIGHT", method, rules)
}

func syntheticPlanWithBaseCode(
	t testing.TB,
	suffix string,
	direction domain.PricingDirection,
	purpose domain.PricingPurpose,
	baseAmount string,
	baseCode string,
	method domain.BillableWeightMethod,
	rules []domain.FixedChargeRule,
) domain.PricingPlanVersion {
	t.Helper()
	currency := mustValue(t, domain.NewCurrency, "USD")
	entryID := mustValue(t, domain.NewRateEntryID, "entry-"+suffix)
	entry, err := domain.NewRateEntry(
		entryID,
		"Z1",
		weight(t, "0", domain.WeightUnitKilogram),
		weight(t, "10", domain.WeightUnitKilogram),
		money(t, baseAmount, currency),
	)
	if err != nil {
		t.Fatalf("rate entry: %v", err)
	}
	table, err := domain.NewRateTableVersion(
		versionReference(t, domain.ArtifactRateTable, "table-"+suffix, "v1"),
		domain.RateTableKindWeightZone,
		currency,
		domain.WeightUnitKilogram,
		effectivePeriod(t),
		[]domain.RateEntry{entry},
	)
	if err != nil {
		t.Fatalf("rate table: %v", err)
	}
	rounding, err := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, weight(t, "0.5", domain.WeightUnitKilogram))
	if err != nil {
		t.Fatalf("rounding policy: %v", err)
	}
	weightPolicy, err := domain.NewBillableWeightPolicy(
		versionReference(t, domain.ArtifactWeightPolicy, "weight-"+suffix, "v1"),
		method,
		rounding,
	)
	if err != nil {
		t.Fatalf("billable weight policy: %v", err)
	}
	plan, err := domain.NewPricingPlanVersion(
		versionReference(t, domain.ArtifactPricingPlan, "plan-"+suffix, "v1"),
		mustValue(t, domain.NewPricingScopeID, "scope-1"),
		direction,
		purpose,
		mustValue(t, domain.NewChargeCode, baseCode),
		effectivePeriod(t),
		table,
		weightPolicy,
		rules,
	)
	if err != nil {
		t.Fatalf("pricing plan: %v", err)
	}
	return plan
}

func syntheticInput(t testing.TB, actual string, volumetric *string, zone string) domain.PricingInputSnapshot {
	return syntheticInputAt(t, actual, volumetric, zone, time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC))
}

func syntheticInputAt(t testing.TB, actual string, volumetric *string, zone string, businessAt time.Time) domain.PricingInputSnapshot {
	t.Helper()
	var volumetricWeight *domain.Weight
	if volumetric != nil {
		value := weight(t, *volumetric, domain.WeightUnitKilogram)
		volumetricWeight = &value
	}
	input, err := domain.NewPricingInputSnapshot(
		mustValue(t, domain.NewTenantID, "tenant-1"),
		mustValue(t, domain.NewPricingScopeID, "scope-1"),
		mustValue(t, domain.NewPackageID, "package-1"),
		zone,
		weight(t, actual, domain.WeightUnitKilogram),
		volumetricWeight,
		businessAt,
	)
	if err != nil {
		t.Fatalf("pricing input: %v", err)
	}
	return input
}

func evaluate(t testing.TB, id string, plan domain.PricingPlanVersion, input domain.PricingInputSnapshot) domain.PricingEvaluation {
	t.Helper()
	request, err := domain.NewEvaluationRequest(
		mustValue(t, domain.NewEvaluationID, id),
		plan,
		input,
		domain.EvidenceSynthetic,
	)
	if err != nil {
		t.Fatalf("evaluation request: %v", err)
	}
	return domain.EvaluatePricing(request)
}
