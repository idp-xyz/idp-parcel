package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

func TestChargeCodeRequiresStableCanonicalFormat(t *testing.T) {
	for _, value := range []string{"", "base_freight", "1BASE", "BASE-FREIGHT", " BASE_FREIGHT", "BASE_FREIGHT "} {
		if _, err := domain.NewChargeCode(value); !errors.Is(err, domain.ErrInvalidChargeCode) {
			t.Fatalf("charge code %q error = %v", value, err)
		}
	}
	code := mustValue(t, domain.NewChargeCode, "BASE_FREIGHT_2")
	if code.String() != "BASE_FREIGHT_2" {
		t.Fatalf("charge code string = %q", code.String())
	}
}

func TestPricingPlanRequiresChargeCodesAndRejectsDuplicates(t *testing.T) {
	currency := mustValue(t, domain.NewCurrency, "USD")
	if _, err := domain.NewFixedChargeRule("missing-code", domain.ChargeCode{}, "Missing code", domain.ChargeEffectAdd, money(t, "1", currency), 1); !errors.Is(err, domain.ErrInvalidChargeRule) {
		t.Fatalf("missing rule code error = %v", err)
	}
	baseCode := mustValue(t, domain.NewChargeCode, "BASE_FREIGHT")
	duplicateBase, err := domain.NewFixedChargeRule("duplicate-base", baseCode, "Duplicate base", domain.ChargeEffectAdd, money(t, "1", currency), 1)
	if err != nil {
		t.Fatalf("duplicate base rule: %v", err)
	}
	if _, err := newSyntheticPlanRules(t, "duplicate-base-code", baseCode, []domain.FixedChargeRule{duplicateBase}); !errors.Is(err, domain.ErrDuplicateChargeCode) {
		t.Fatalf("duplicate base code error = %v", err)
	}
	firstCode := mustValue(t, domain.NewChargeCode, "SURCHARGE")
	first, err := domain.NewFixedChargeRule("first", firstCode, "First", domain.ChargeEffectAdd, money(t, "1", currency), 1)
	if err != nil {
		t.Fatalf("first rule: %v", err)
	}
	second, err := domain.NewFixedChargeRule("second", firstCode, "Second", domain.ChargeEffectAdd, money(t, "1", currency), 2)
	if err != nil {
		t.Fatalf("second rule: %v", err)
	}
	if _, err := newSyntheticPlanRules(t, "duplicate-rule-code", baseCode, []domain.FixedChargeRule{first, second}); !errors.Is(err, domain.ErrDuplicateChargeCode) {
		t.Fatalf("duplicate rule code error = %v", err)
	}
	if _, err := newSyntheticPlanRules(t, "missing-base-code", domain.ChargeCode{}, nil); !errors.Is(err, domain.ErrInvalidPricingPlan) {
		t.Fatalf("missing base code error = %v", err)
	}
}

func TestChargeCodeChangesPlanContentDigest(t *testing.T) {
	firstRuleCode := mustValue(t, domain.NewChargeCode, "RESIDENTIAL")
	secondRuleCode := mustValue(t, domain.NewChargeCode, "HOME_DELIVERY")
	currency := mustValue(t, domain.NewCurrency, "USD")
	firstRule, err := domain.NewFixedChargeRule("same-rule", firstRuleCode, "Same rule", domain.ChargeEffectAdd, money(t, "1", currency), 1)
	if err != nil {
		t.Fatalf("first rule: %v", err)
	}
	secondRule, err := domain.NewFixedChargeRule("same-rule", secondRuleCode, "Same rule", domain.ChargeEffectAdd, money(t, "1", currency), 1)
	if err != nil {
		t.Fatalf("second rule: %v", err)
	}
	firstPlan := syntheticPlan(t, "code-digest", domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge, "10", domain.BillableWeightActualOnly, []domain.FixedChargeRule{firstRule})
	baseChangedPlan := syntheticPlanWithBaseCode(t, "code-digest", domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge, "10", "BASE_FREIGHT_V2", domain.BillableWeightActualOnly, []domain.FixedChargeRule{firstRule})
	ruleChangedPlan := syntheticPlanWithBaseCode(t, "code-digest", domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge, "10", "BASE_FREIGHT", domain.BillableWeightActualOnly, []domain.FixedChargeRule{secondRule})
	if !firstPlan.Manifest().Equal(baseChangedPlan.Manifest()) || !firstPlan.Manifest().Equal(ruleChangedPlan.Manifest()) {
		t.Fatal("test plans must share the same version manifest")
	}
	if firstPlan.ContentDigest() == baseChangedPlan.ContentDigest() {
		t.Fatal("base charge code change was omitted from the plan content digest")
	}
	if firstPlan.ContentDigest() == ruleChangedPlan.ContentDigest() {
		t.Fatal("rule charge code change was omitted from the plan content digest")
	}
}

func TestPricingPlanRejectsDuplicateRuleOrderAndBuildsStableManifest(t *testing.T) {
	duplicateOrder := syntheticPlanRules(t, "duplicate", []domain.FixedChargeRule{
		fixedRule(t, "r-1", domain.ChargeEffectAdd, "1", 1),
		fixedRule(t, "r-2", domain.ChargeEffectAdd, "2", 1),
	})
	if duplicateOrder != nil {
		t.Fatal("helper unexpectedly accepted duplicate rule order")
	}

	plan := syntheticPlan(t, "manifest-order", domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge, "10", domain.BillableWeightActualOnly, nil)
	references := plan.Manifest().References()
	if len(references) != 4 {
		t.Fatalf("manifest length = %d, want 4", len(references))
	}
	for index := 1; index < len(references); index++ {
		left := string(references[index-1].Kind()) + "|" + references[index-1].ID()
		right := string(references[index].Kind()) + "|" + references[index].ID()
		if left > right {
			t.Fatalf("manifest is not stable-sorted: %q before %q", left, right)
		}
	}
}

func syntheticPlanRules(t testing.TB, suffix string, rules []domain.FixedChargeRule) *domain.PricingPlanVersion {
	t.Helper()
	plan, err := newSyntheticPlanRules(t, suffix, mustValue(t, domain.NewChargeCode, "BASE_FREIGHT"), rules)
	if errors.Is(err, domain.ErrDuplicateChargeRuleOrder) {
		return nil
	}
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	return &plan
}

func newSyntheticPlanRules(t testing.TB, suffix string, baseCode domain.ChargeCode, rules []domain.FixedChargeRule) (domain.PricingPlanVersion, error) {
	t.Helper()
	currency := mustValue(t, domain.NewCurrency, "USD")
	entry, err := domain.NewRateEntry(
		mustValue(t, domain.NewRateEntryID, "entry-"+suffix), "Z1",
		weight(t, "0", domain.WeightUnitKilogram), weight(t, "10", domain.WeightUnitKilogram), money(t, "10", currency),
	)
	if err != nil {
		t.Fatalf("entry: %v", err)
	}
	table, err := domain.NewRateTableVersion(versionReference(t, domain.ArtifactRateTable, "table-"+suffix, "v1"), domain.RateTableKindWeightZone, currency, domain.WeightUnitKilogram, effectivePeriod(t), []domain.RateEntry{entry})
	if err != nil {
		t.Fatalf("table: %v", err)
	}
	rounding, err := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, weight(t, "1", domain.WeightUnitKilogram))
	if err != nil {
		t.Fatalf("rounding: %v", err)
	}
	policy, err := domain.NewBillableWeightPolicy(versionReference(t, domain.ArtifactWeightPolicy, "weight-"+suffix, "v1"), domain.BillableWeightActualOnly, rounding)
	if err != nil {
		t.Fatalf("weight policy: %v", err)
	}
	return domain.NewPricingPlanVersion(versionReference(t, domain.ArtifactPricingPlan, "plan-"+suffix, "v1"), mustValue(t, domain.NewPricingScopeID, "scope-1"), domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge, baseCode, effectivePeriod(t), table, policy, rules)
}

func TestMoneyAndWeightRejectMismatchedDimensions(t *testing.T) {
	usd := mustValue(t, domain.NewCurrency, "USD")
	eur := mustValue(t, domain.NewCurrency, "EUR")
	left := money(t, "1", usd)
	right := money(t, "1", eur)
	if _, err := left.Add(right); !errors.Is(err, domain.ErrCurrencyMismatch) {
		t.Fatalf("currency mismatch error = %v", err)
	}
	kg := weight(t, "1", domain.WeightUnitKilogram)
	lb := weight(t, "1", domain.WeightUnitPound)
	if _, err := kg.Compare(lb); !errors.Is(err, domain.ErrWeightUnitMismatch) {
		t.Fatalf("weight mismatch error = %v", err)
	}
}

func TestVersionManifestDistinguishesSeparatorBearingReferenceFields(t *testing.T) {
	first, err := domain.NewVersionReference(domain.ArtifactKind("custom"), "alpha|beta", "gamma", "digest-1")
	if err != nil {
		t.Fatalf("first reference: %v", err)
	}
	second, err := domain.NewVersionReference(domain.ArtifactKind("custom"), "alpha", "beta|gamma", "digest-2")
	if err != nil {
		t.Fatalf("second reference: %v", err)
	}
	manifest, err := domain.NewVersionManifest([]domain.VersionReference{first, second})
	if err != nil {
		t.Fatalf("manifest rejected distinct structured identities: %v", err)
	}
	if len(manifest.References()) != 2 {
		t.Fatalf("manifest reference count = %d, want 2", len(manifest.References()))
	}
}

func TestPricingPlanContentDigestUsesStructuredRuleEncoding(t *testing.T) {
	currency := mustValue(t, domain.NewCurrency, "USD")
	code := mustValue(t, domain.NewChargeCode, "STRUCTURED_RULE")
	firstRule, err := domain.NewFixedChargeRule("alpha|beta", code, "gamma\nline", domain.ChargeEffectAdd, money(t, "1", currency), 1)
	if err != nil {
		t.Fatalf("first rule: %v", err)
	}
	secondRule, err := domain.NewFixedChargeRule("alpha", code, "beta|gamma\nline", domain.ChargeEffectAdd, money(t, "1", currency), 1)
	if err != nil {
		t.Fatalf("second rule: %v", err)
	}
	firstPlan := syntheticPlan(t, "structured-rule-digest", domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge, "10", domain.BillableWeightActualOnly, []domain.FixedChargeRule{firstRule})
	secondPlan := syntheticPlan(t, "structured-rule-digest", domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge, "10", domain.BillableWeightActualOnly, []domain.FixedChargeRule{secondRule})
	if !firstPlan.Manifest().Equal(secondPlan.Manifest()) {
		t.Fatal("test plans must share the same version manifest")
	}
	if firstPlan.ContentDigest() == secondPlan.ContentDigest() {
		t.Fatal("structured rule fields produced a content digest collision")
	}
}

func TestPricingPlanContentDigestIsStableAcrossEntryAndRuleInputOrder(t *testing.T) {
	forward := planWithInputOrder(t, false)
	reverse := planWithInputOrder(t, true)
	if forward.ContentDigest() != reverse.ContentDigest() {
		t.Fatalf("content digests differ by caller input order: %s != %s", forward.ContentDigest(), reverse.ContentDigest())
	}
}

func planWithInputOrder(t testing.TB, reverse bool) domain.PricingPlanVersion {
	t.Helper()
	currency := mustValue(t, domain.NewCurrency, "USD")
	firstEntry, err := domain.NewRateEntry(mustValue(t, domain.NewRateEntryID, "ordered-entry-1"), "Z1", weight(t, "0", domain.WeightUnitKilogram), weight(t, "1", domain.WeightUnitKilogram), money(t, "5", currency))
	if err != nil {
		t.Fatalf("first entry: %v", err)
	}
	secondEntry, err := domain.NewRateEntry(mustValue(t, domain.NewRateEntryID, "ordered-entry-2"), "Z1", weight(t, "1", domain.WeightUnitKilogram), weight(t, "10", domain.WeightUnitKilogram), money(t, "10", currency))
	if err != nil {
		t.Fatalf("second entry: %v", err)
	}
	entries := []domain.RateEntry{firstEntry, secondEntry}
	rules := []domain.FixedChargeRule{fixedRule(t, "ordered-rule-1", domain.ChargeEffectAdd, "1", 1), fixedRule(t, "ordered-rule-2", domain.ChargeEffectDeduct, "0.5", 2)}
	if reverse {
		entries[0], entries[1] = entries[1], entries[0]
		rules[0], rules[1] = rules[1], rules[0]
	}
	table, err := domain.NewRateTableVersion(versionReference(t, domain.ArtifactRateTable, "ordered-table", "v1"), domain.RateTableKindWeightZone, currency, domain.WeightUnitKilogram, effectivePeriod(t), entries)
	if err != nil {
		t.Fatalf("rate table: %v", err)
	}
	rounding, err := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, weight(t, "0.5", domain.WeightUnitKilogram))
	if err != nil {
		t.Fatalf("rounding: %v", err)
	}
	weightPolicy, err := domain.NewBillableWeightPolicy(versionReference(t, domain.ArtifactWeightPolicy, "ordered-weight", "v1"), domain.BillableWeightActualOnly, rounding)
	if err != nil {
		t.Fatalf("weight policy: %v", err)
	}
	plan, err := domain.NewPricingPlanVersion(versionReference(t, domain.ArtifactPricingPlan, "ordered-plan", "v1"), mustValue(t, domain.NewPricingScopeID, "scope-1"), domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge, mustValue(t, domain.NewChargeCode, "BASE_FREIGHT"), effectivePeriod(t), table, weightPolicy, rules)
	if err != nil {
		t.Fatalf("pricing plan: %v", err)
	}
	return plan
}

// The first release pairs each calculation purpose with exactly one direction,
// so a plan cannot price a customer sale while calling itself a supplier cost.
// Widening the purpose axis means revisiting the pairing deliberately; until
// then a mismatch is a modelling error rather than a configuration choice.
func TestPricingPlanRequiresPurposePairedWithDirection(t *testing.T) {
	paired := map[domain.PricingDirection]domain.PricingPurpose{
		domain.PricingDirectionSell:     domain.PricingPurposeCustomerCharge,
		domain.PricingDirectionBuy:      domain.PricingPurposeSupplierCost,
		domain.PricingDirectionInternal: domain.PricingPurposeInternalPrice,
	}
	for direction, purpose := range paired {
		if _, err := pairedPlan(t, "paired-"+string(direction), direction, purpose); err != nil {
			t.Fatalf("%s with %s rejected: %v", direction, purpose, err)
		}
	}

	mismatched := []struct {
		direction domain.PricingDirection
		purpose   domain.PricingPurpose
	}{
		{domain.PricingDirectionSell, domain.PricingPurposeSupplierCost},
		{domain.PricingDirectionBuy, domain.PricingPurposeCustomerCharge},
		{domain.PricingDirectionInternal, domain.PricingPurposeCustomerCharge},
		{domain.PricingDirectionSell, domain.PricingPurposeInternalPrice},
	}
	for _, test := range mismatched {
		_, err := pairedPlan(t, "mismatch", test.direction, test.purpose)
		if !errors.Is(err, domain.ErrDirectionPurposeMismatch) {
			t.Fatalf("%s with %s accepted: err = %v", test.direction, test.purpose, err)
		}
	}
}

// The purpose enum is closed. It used to accept anything shaped like an
// identifier, which let the reference design's wider CalculationPurpose values
// through without the language ever deciding to adopt them.
func TestPricingPurposeRejectsValuesOutsideTheClosedSet(t *testing.T) {
	for _, value := range []string{"QUOTE", "ESTIMATED_COST", "ACTUAL_COST", "CUSTOMER_BILLING", "customer_charge", ""} {
		if _, err := domain.NewPricingPurpose(value); !errors.Is(err, domain.ErrInvalidPurpose) {
			t.Fatalf("purpose %q accepted: err = %v", value, err)
		}
	}
	for _, value := range []string{"CUSTOMER_CHARGE", "SUPPLIER_COST", "INTERNAL_PRICE"} {
		if _, err := domain.NewPricingPurpose(value); err != nil {
			t.Fatalf("purpose %q rejected: %v", value, err)
		}
	}
}

func pairedPlan(
	t testing.TB,
	suffix string,
	direction domain.PricingDirection,
	purpose domain.PricingPurpose,
) (domain.PricingPlanVersion, error) {
	t.Helper()
	currency := mustValue(t, domain.NewCurrency, "USD")
	entry, err := domain.NewRateEntry(
		mustValue(t, domain.NewRateEntryID, "entry-"+suffix), "Z1",
		weight(t, "0", domain.WeightUnitKilogram), weight(t, "10", domain.WeightUnitKilogram), money(t, "10", currency),
	)
	if err != nil {
		t.Fatalf("rate entry: %v", err)
	}
	table, err := domain.NewRateTableVersion(versionReference(t, domain.ArtifactRateTable, "table-"+suffix, "v1"), domain.RateTableKindWeightZone, currency, domain.WeightUnitKilogram, effectivePeriod(t), []domain.RateEntry{entry})
	if err != nil {
		t.Fatalf("rate table: %v", err)
	}
	rounding, err := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, weight(t, "1", domain.WeightUnitKilogram))
	if err != nil {
		t.Fatalf("rounding: %v", err)
	}
	weightPolicy, err := domain.NewBillableWeightPolicy(versionReference(t, domain.ArtifactWeightPolicy, "weight-"+suffix, "v1"), domain.BillableWeightActualOnly, rounding)
	if err != nil {
		t.Fatalf("weight policy: %v", err)
	}
	return domain.NewPricingPlanVersion(
		versionReference(t, domain.ArtifactPricingPlan, "plan-"+suffix, "v1"),
		mustValue(t, domain.NewPricingScopeID, "scope-1"),
		direction,
		purpose,
		mustValue(t, domain.NewChargeCode, "BASE_FREIGHT"),
		effectivePeriod(t),
		table,
		weightPolicy,
		nil,
	)
}
