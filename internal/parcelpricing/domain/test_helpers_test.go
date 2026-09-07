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

func dimensions(t testing.TB, length, width, height string, unit domain.LengthUnit) domain.Dimensions {
	t.Helper()
	result, err := domain.NewDimensions(decimal(t, length), decimal(t, width), decimal(t, height), unit)
	if err != nil {
		t.Fatalf("dimensions %sx%sx%s %s: %v", length, width, height, unit, err)
	}
	return result
}

func length(t testing.TB, value string, unit domain.LengthUnit) domain.Length {
	t.Helper()
	result, err := domain.NewLength(decimal(t, value), unit)
	if err != nil {
		t.Fatalf("length %s %s: %v", value, unit, err)
	}
	return result
}

func features(t testing.TB, sides domain.Dimensions) domain.PackageFeatures {
	t.Helper()
	result, err := domain.NewPackageFeatures(sides, weight(t, "1", domain.WeightUnitKilogram))
	if err != nil {
		t.Fatalf("package features: %v", err)
	}
	return result
}

func assertLength(t testing.TB, label string, actual domain.Length, expected string, unit domain.LengthUnit) {
	t.Helper()
	if actual.Value().String() != expected || actual.Unit() != unit {
		t.Fatalf("%s = %s %s, want %s %s", label, actual.Value().String(), actual.Unit(), expected, unit)
	}
}

func money(t testing.TB, value string, currency domain.Currency) domain.Money {
	t.Helper()
	result, err := domain.NewMoney(decimal(t, value), currency)
	if err != nil {
		t.Fatalf("money %s %s: %v", value, currency, err)
	}
	return result
}

// versionReference 只带三元（ADR-0108）：SYN 夹具从来没有真摘要，此前那个 `sha256:syn-` 占位
// 装在 digest 槽里假装是哈希，现在指纹如实留空。
func versionReference(t testing.TB, kind domain.ArtifactKind, id, version string) domain.VersionReference {
	t.Helper()
	reference, err := domain.NewVersionReferenceIdentity(kind, id, version)
	if err != nil {
		t.Fatalf("version reference %s: %v", id, err)
	}
	return reference
}

// fingerprintedReference 是同一条引用带上一枚声明时附带的指纹的版本，给「带不带指纹摘要相同」
// 那组用例用。
func fingerprintedReference(t testing.TB, kind domain.ArtifactKind, id, version, fingerprint string) domain.VersionReference {
	t.Helper()
	reference, err := domain.NewVersionReferenceWithFingerprint(kind, id, version, fingerprint)
	if err != nil {
		t.Fatalf("fingerprinted version reference %s: %v", id, err)
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
	method domain.PricingWeightMethod,
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
	method domain.PricingWeightMethod,
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
		domain.RateTableFamilyWeightZone,
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
	// MAX 必须够得到体积重，所以合成价卡按真卡的方式声明自己的体积系数：5000 把立方厘米
	// 换成千克。
	var factor *domain.VolumetricFactor
	if method == domain.PricingWeightMax {
		factorRounding, roundingErr := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, weight(t, "0.1", domain.WeightUnitKilogram))
		if roundingErr != nil {
			t.Fatalf("volumetric rounding: %v", roundingErr)
		}
		declared, factorErr := domain.NewVolumetricFactor(decimal(t, "5000"), domain.LengthUnitCentimeter, factorRounding)
		if factorErr != nil {
			t.Fatalf("volumetric factor: %v", factorErr)
		}
		factor = &declared
	}
	weightPolicy, err := domain.NewPricingWeightPolicy(
		versionReference(t, domain.ArtifactWeightPolicy, "weight-"+suffix, "v1"),
		method,
		rounding,
		factor,
	)
	if err != nil {
		t.Fatalf("pricing weight policy: %v", err)
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
		domain.PricingPlanStructures{},
	)
	if err != nil {
		t.Fatalf("pricing plan: %v", err)
	}
	return plan
}

func syntheticInput(t testing.TB, actual, zone string) domain.PricingInputSnapshot {
	return syntheticInputAt(t, actual, zone, time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC))
}

func packageSubject(t testing.TB, id string) domain.EvaluationSubject {
	t.Helper()
	subject, err := domain.NewAcceptedPackageSubject(mustValue(t, domain.NewPackageID, id))
	if err != nil {
		t.Fatalf("package subject %s: %v", id, err)
	}
	return subject
}

func syntheticInputForSubject(t testing.TB, subject domain.EvaluationSubject, actual, zone string) domain.PricingInputSnapshot {
	t.Helper()
	input, err := domain.NewPricingInputSnapshot(
		mustValue(t, domain.NewTenantID, "tenant-1"),
		mustValue(t, domain.NewPricingScopeID, "scope-1"),
		subject,
		zone,
		weight(t, actual, domain.WeightUnitKilogram),
		nil,
		time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("pricing input: %v", err)
	}
	return input
}

func syntheticInputWithDimensions(t testing.TB, actual, zone string, sides domain.Dimensions) domain.PricingInputSnapshot {
	t.Helper()
	input, err := domain.NewPricingInputSnapshot(
		mustValue(t, domain.NewTenantID, "tenant-1"),
		mustValue(t, domain.NewPricingScopeID, "scope-1"),
		packageSubject(t, "package-1"),
		zone,
		weight(t, actual, domain.WeightUnitKilogram),
		&sides,
		time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("pricing input: %v", err)
	}
	return input
}

func syntheticInputAt(t testing.TB, actual, zone string, businessAt time.Time) domain.PricingInputSnapshot {
	t.Helper()
	input, err := domain.NewPricingInputSnapshot(
		mustValue(t, domain.NewTenantID, "tenant-1"),
		mustValue(t, domain.NewPricingScopeID, "scope-1"),
		packageSubject(t, "package-1"),
		zone,
		weight(t, actual, domain.WeightUnitKilogram),
		nil,
		businessAt,
	)
	if err != nil {
		t.Fatalf("pricing input: %v", err)
	}
	return input
}

// registrationOf 把一张价卡装进合成的价卡登记：源文件身份、方向授权引用与批准责任方全是 SYN
// 夹具常量。生产上方案只随登记一起持久化（见 plan_snapshot.go 头注），要把方案折装重建就得先
// 把它装进登记。
func registrationOf(t testing.TB, plan domain.PricingPlanVersion) domain.PriceCardRegistration {
	t.Helper()
	source, err := domain.NewSourceFileIdentity("SYN-PRC-CARD-260820.xlsx", synSourceSHA)
	if err != nil {
		t.Fatalf("构造源文件身份：%v", err)
	}
	registration, err := domain.NewPriceCardRegistration(
		mustValue(t, domain.NewTenantID, "tenant-1"),
		plan,
		source,
		versionReference(t, domain.ArtifactCommercialAuthorization, "SYN-PRC-BUY-GRANT", "v1"),
		"SYN-PRC-PRICING-GOVERNANCE",
	)
	if err != nil {
		t.Fatalf("构造价卡登记：%v", err)
	}
	return registration
}

// planSnapshotRoundTrip 把一张价卡经生产唯一的持久化门——价卡登记快照——折装再重建，交回登记
// 快照字节与重建后的方案。方案层曾有一对导出的快照函数，只有测试调它们而生产走登记层那一对；
// 测试改经同一道门，证的才是生产真走的那条路径。
func planSnapshotRoundTrip(t testing.TB, plan domain.PricingPlanVersion) ([]byte, domain.PricingPlanVersion) {
	t.Helper()
	raw, err := domain.MarshalPriceCardRegistration(registrationOf(t, plan))
	if err != nil {
		t.Fatalf("折装价卡登记快照：%v", err)
	}
	rebuilt, err := domain.RehydratePriceCardRegistration(raw)
	if err != nil {
		t.Fatalf("重建价卡登记：%v", err)
	}
	return raw, rebuilt.Plan()
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
