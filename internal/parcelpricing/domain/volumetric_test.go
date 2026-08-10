package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// Covers: CONTEXT「本上下文不内置任何系数常量」— 权威价卡的 L4 同时给出体积系数与它配对
// 的那对单位：体积重（磅）＝ 英寸长 × 宽 × 高 ÷ 250。12 英寸立方是 1728 立方英寸，得
// 6.912 磅，再由声明的进位抬到下一个整磅。系数是卡的内容，所以测试按一个价卡版本的方式
// 把它喂进来。
func TestVolumetricWeightIsDerivedFromDimensionsAndTheDeclaredDivisor(t *testing.T) {
	policy := maxWeightPolicy(t, "weight-volumetric", "250", domain.LengthUnitInch, "1")
	input := inputWithSides(t, "5", domain.WeightUnitPound, dimensions(t, "12", "12", "12", domain.LengthUnitInch))

	result, err := domain.CalculatePricingWeight(input, policy, domain.WeightUnitPound)
	if err != nil {
		t.Fatalf("pricing weight: %v", err)
	}
	volumetric, derived := result.VolumetricWeight()
	if !derived || volumetric.Value().String() != "7" {
		t.Fatalf("volumetric = %#v, derived=%v, want 7 LB", volumetric, derived)
	}
	if result.RawWeight().Value().String() != "7" {
		t.Fatalf("raw = %s, want the greater of 5 and 7", result.RawWeight().Value())
	}
}

// Covers: CONTEXT「系数只对它声明的那一对单位成立——把立方英寸换成磅的除数不适用于厘米，
// 跨单位换算须另有版本化声明」— 悄悄换算会算错价，而换算规则本身也得版本化。
func TestVolumetricFactorRefusesSidesMeasuredInAnotherUnit(t *testing.T) {
	policy := maxWeightPolicy(t, "weight-unit-clash", "250", domain.LengthUnitInch, "1")
	input := inputWithSides(t, "5", domain.WeightUnitPound, dimensions(t, "30", "30", "30", domain.LengthUnitCentimeter))
	if _, err := domain.CalculatePricingWeight(input, policy, domain.WeightUnitPound); !errors.Is(err, domain.ErrLengthUnitMismatch) {
		t.Fatalf("cross-unit divisor error = %v", err)
	}
}

// Covers: CONTEXT「缺尺寸而无法得出体积重时评价保持待判断，不退回实重」— 边长一直没到的
// 包裹是一项还可能补上的事实缺失，不是一个碰巧只按重量计价的包裹。
func TestPricingWeightStaysPendingWhenMaxHasNoSides(t *testing.T) {
	policy := maxWeightPolicy(t, "weight-no-sides", "250", domain.LengthUnitInch, "1")
	input := inputWithoutSides(t, "5", domain.WeightUnitPound)
	if _, err := domain.CalculatePricingWeight(input, policy, domain.WeightUnitPound); !errors.Is(err, domain.ErrMissingDimensions) {
		t.Fatalf("missing sides error = %v", err)
	}
}

// 改了体积系数的卡，对同一个包裹收的钱就不一样，所以两个版本不得共享内容摘要。
func TestPricingPlanContentDigestCoversTheVolumetricDivisor(t *testing.T) {
	first := planWithWeightPolicy(t, maxWeightPolicy(t, "weight-divisor", "250", domain.LengthUnitInch, "1"))
	second := planWithWeightPolicy(t, maxWeightPolicy(t, "weight-divisor", "139", domain.LengthUnitInch, "1"))
	if first.ContentDigest() == second.ContentDigest() {
		t.Fatal("volumetric divisor was omitted from the plan content digest")
	}
}

// Covers: CONTEXT「重量方法不需要体积重时不得声明系数，需要而未声明即为工件不合法」—
// MAX 没有系数就得不出体积重，ACTUAL_ONLY 则根本不读系数。两种错配都会留下一条没有任何
// 东西去执行的声明。
func TestPricingWeightPolicyPairsTheDivisorWithTheMethodThatNeedsIt(t *testing.T) {
	rounding, err := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, weight(t, "1", domain.WeightUnitPound))
	if err != nil {
		t.Fatalf("rounding: %v", err)
	}
	reference := versionReference(t, domain.ArtifactWeightPolicy, "weight-pairing", "v1")
	factor := volumetricFactor(t, "250", domain.LengthUnitInch, "1")

	if _, err := domain.NewPricingWeightPolicy(reference, domain.PricingWeightMax, rounding, nil); !errors.Is(err, domain.ErrInvalidRoundingPolicy) {
		t.Fatalf("MAX without a divisor error = %v", err)
	}
	if _, err := domain.NewPricingWeightPolicy(reference, domain.PricingWeightActualOnly, rounding, &factor); !errors.Is(err, domain.ErrInvalidRoundingPolicy) {
		t.Fatalf("ACTUAL_ONLY with a divisor error = %v", err)
	}
}

// Covers: CONTEXT「同时必须声明其商的精度，因为精确商不一定是有限小数」— 不取整就等于让
// 代码碰巧怎么做就是什么精度，而那个精度从未被声明。系数必须讲清它的商落在哪里。
func TestVolumetricFactorRequiresADeclaredQuotientPrecision(t *testing.T) {
	rounding, err := domain.NewWeightRoundingPolicy(domain.RoundingNone, weight(t, "1", domain.WeightUnitPound))
	if err != nil {
		t.Fatalf("rounding: %v", err)
	}
	if _, err := domain.NewVolumetricFactor(decimal(t, "250"), domain.LengthUnitInch, rounding); !errors.Is(err, domain.ErrInvalidVolumetricFactor) {
		t.Fatalf("unrounded quotient error = %v", err)
	}
}

func volumetricFactor(t testing.TB, divisor string, lengthUnit domain.LengthUnit, increment string) domain.VolumetricFactor {
	t.Helper()
	rounding, err := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, weight(t, increment, domain.WeightUnitPound))
	if err != nil {
		t.Fatalf("volumetric rounding: %v", err)
	}
	factor, err := domain.NewVolumetricFactor(decimal(t, divisor), lengthUnit, rounding)
	if err != nil {
		t.Fatalf("volumetric factor: %v", err)
	}
	return factor
}

func maxWeightPolicy(t testing.TB, id, divisor string, lengthUnit domain.LengthUnit, increment string) domain.PricingWeightPolicy {
	t.Helper()
	factor := volumetricFactor(t, divisor, lengthUnit, increment)
	rounding, err := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, weight(t, "1", domain.WeightUnitPound))
	if err != nil {
		t.Fatalf("rounding: %v", err)
	}
	policy, err := domain.NewPricingWeightPolicy(
		versionReference(t, domain.ArtifactWeightPolicy, id, "v1"),
		domain.PricingWeightMax,
		rounding,
		&factor,
	)
	if err != nil {
		t.Fatalf("weight policy: %v", err)
	}
	return policy
}

func inputWithSides(t testing.TB, actual string, unit domain.WeightUnit, sides domain.Dimensions) domain.PricingInputSnapshot {
	t.Helper()
	return newSnapshot(t, actual, unit, &sides)
}

func inputWithoutSides(t testing.TB, actual string, unit domain.WeightUnit) domain.PricingInputSnapshot {
	t.Helper()
	return newSnapshot(t, actual, unit, nil)
}

func newSnapshot(t testing.TB, actual string, unit domain.WeightUnit, sides *domain.Dimensions) domain.PricingInputSnapshot {
	t.Helper()
	input, err := domain.NewPricingInputSnapshot(
		mustValue(t, domain.NewTenantID, "tenant-1"),
		mustValue(t, domain.NewPricingScopeID, "scope-1"),
		packageSubject(t, "package-1"),
		"Z1",
		weight(t, actual, unit),
		sides,
		time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("pricing input: %v", err)
	}
	return input
}

func planWithWeightPolicy(t testing.TB, policy domain.PricingWeightPolicy) domain.PricingPlanVersion {
	t.Helper()
	currency := mustValue(t, domain.NewCurrency, "USD")
	entry, err := domain.NewRateEntry(
		mustValue(t, domain.NewRateEntryID, "entry-divisor"),
		"Z1",
		weight(t, "0", domain.WeightUnitPound),
		weight(t, "100", domain.WeightUnitPound),
		money(t, "10", currency),
	)
	if err != nil {
		t.Fatalf("rate entry: %v", err)
	}
	table, err := domain.NewRateTableVersion(
		versionReference(t, domain.ArtifactRateTable, "table-divisor", "v1"),
		domain.RateTableFamilyWeightZone,
		currency,
		domain.WeightUnitPound,
		effectivePeriod(t),
		[]domain.RateEntry{entry},
	)
	if err != nil {
		t.Fatalf("rate table: %v", err)
	}
	plan, err := domain.NewPricingPlanVersion(
		versionReference(t, domain.ArtifactPricingPlan, "plan-divisor", "v1"),
		mustValue(t, domain.NewPricingScopeID, "scope-1"),
		domain.PricingDirectionSell,
		domain.PricingPurposeCustomerCharge,
		mustValue(t, domain.NewChargeCode, "BASE_FREIGHT"),
		effectivePeriod(t),
		table,
		policy,
		nil,
		domain.PricingPlanStructures{},
	)
	if err != nil {
		t.Fatalf("pricing plan: %v", err)
	}
	return plan
}
