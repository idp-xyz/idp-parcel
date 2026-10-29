package domain

import (
	"errors"
	"math/big"
	"testing"
)

// 本文件从包内钉 ADR-0346 里外部测试够不着的几处：冻结模式只认记下的系数、不查目录；
// 答不出的换算（目录外的单位、跨类别、冻结模式下没记过）一律是原来的单位不一致，错误文字与换算接入前逐字相同；
// 有理数取整与 DivRoundToIncrement 是同一个判法。

func mustDecimal(t testing.TB, text string) Decimal {
	t.Helper()
	value, err := ParseDecimal(text)
	if err != nil {
		t.Fatalf("decimal %q: %v", text, err)
	}
	return value
}

// Covers: ADR-0346 决定三——冻结模式（重放、同标识重复请求）用的是记下的系数而不是目录里的：
// 记下 LB→KG 为 0.5 时，2.5 LB 换成 1.25 KG，而目录的 0.45359237 会给出 1.133980925。
func TestFrozenConversionUsesTheRecordedFactorNotTheCatalogue(t *testing.T) {
	recorded := []UnitConversionStep{{
		quantity: ConvertedActualWeight, from: "LB", to: "KG", factor: "0.5",
		original: []Decimal{mustDecimal(t, "2.5")}, converted: []Decimal{mustDecimal(t, "1.25")}, exact: true,
	}}
	frozen := newUnitConverter(true, recorded)
	weight := Weight{value: mustDecimal(t, "2.5"), unit: WeightUnitPound}
	converted, err := frozen.convertWeight(ConvertedActualWeight, weight, WeightUnitKilogram, 0, true)
	if err != nil || converted.value.String() != "1.25" {
		t.Fatalf("冻结换算 = %v %v，want 1.25", converted.value, err)
	}
	fresh := newUnitConverter(false, nil)
	fromCatalogue, err := fresh.convertWeight(ConvertedActualWeight, weight, WeightUnitKilogram, 0, true)
	if err != nil || fromCatalogue.value.String() != "1.133980925" {
		t.Fatalf("目录换算 = %v %v，want 1.133980925", fromCatalogue.value, err)
	}
}

// Covers: ADR-0346 决定二、三——冻结且没有记录：不换算，照旧答单位不一致，文字逐字是 ErrWeightUnitMismatch 的——
// 换算接入之前答过单位不一致的评价，重放得到的仍是同一句、同一个摘要。
func TestFrozenWithoutARecordKeepsTheOriginalUnitMismatchAnswer(t *testing.T) {
	frozen := newUnitConverter(true, nil)
	weight := Weight{value: mustDecimal(t, "2.5"), unit: WeightUnitPound}
	_, err := frozen.convertWeight(ConvertedActualWeight, weight, WeightUnitKilogram, 0, true)
	if err != ErrWeightUnitMismatch || err.Error() != "parcel pricing: weight unit mismatch" {
		t.Fatalf("err = %v", err)
	}
	sides, _ := NewDimensions(mustDecimal(t, "30"), mustDecimal(t, "20"), mustDecimal(t, "10"), LengthUnitInch)
	factor := volumetricFactorForTest(t, LengthUnitCentimeter)
	if _, err := frozen.volumetricWeightFromDimensions(sides, factor); err != ErrLengthUnitMismatch {
		t.Fatalf("尺寸冻结无记录 err = %v", err)
	}
}

// Covers: ADR-0346 决定一——目录外的单位、跨类别的单位答不出换算，仍是原来的单位不一致：
// 领域的 WeightUnit / LengthUnit 是封闭小集合，输入走不到这两格，但记录与目录是外来的，答复格要守住。
func TestUnknownAndCrossCategoryUnitsStayUnitMismatch(t *testing.T) {
	converter := newUnitConverter(false, nil)
	for name, unit := range map[string]WeightUnit{"目录外": "STONE", "跨类别": "CM", "小写": "kg"} {
		weight := Weight{value: mustDecimal(t, "1"), unit: unit}
		if _, err := converter.convertWeight(ConvertedActualWeight, weight, WeightUnitKilogram, 0, true); err != ErrWeightUnitMismatch {
			t.Errorf("%s重量单位 %s：err = %v", name, unit, err)
		}
	}
	for name, unit := range map[string]LengthUnit{"目录外": "FURLONG", "跨类别": "KG"} {
		sides := Dimensions{longest: mustDecimal(t, "3"), second: mustDecimal(t, "2"), shortest: mustDecimal(t, "1"), unit: unit}
		if _, err := converter.volumetricWeightFromDimensions(sides, volumetricFactorForTest(t, LengthUnitCentimeter)); !errors.Is(err, ErrLengthUnitMismatch) {
			t.Errorf("%s长度单位 %s：err = %v", name, unit, err)
		}
	}
}

// Covers: ADR-0346 决定三——系数记录只收规范写法，同一个系数只有一种写法：0.450、4.5E-1、2/4 都被拒。
func TestFactorTextIsCanonicalOrRefused(t *testing.T) {
	for _, text := range []string{"0.45359237", "1000", "100000000/45359237", "50/127", "1"} {
		if _, ok := parseCanonicalFactor(text); !ok {
			t.Errorf("%s 是规范写法，应收", text)
		}
	}
	for _, text := range []string{"0.450", "4.5E-1", "2/4", "0", "-1", "", "abc", "1/0", " 1", "+1"} {
		if _, ok := parseCanonicalFactor(text); ok {
			t.Errorf("%q 不是规范写法，应拒", text)
		}
	}
}

// Covers: ADR-0346 决定四——有理数取整与 Decimal.DivRoundToIncrement 是同一个判法：被除数本来就是十进制时
// （系数为 1）两者逐一相同，三种取整模式下都含临界点。
func TestRationalRoundingAgreesWithDecimalDivRoundToIncrement(t *testing.T) {
	divisor, increment := mustDecimal(t, "250"), mustDecimal(t, "0.5")
	for _, text := range []string{"0", "1", "124.99", "125", "125.01", "250", "624.999", "1000", "123456.789"} {
		value := mustDecimal(t, text)
		for _, mode := range []RoundingMode{RoundingCeiling, RoundingHalfUp} {
			want, err := value.DivRoundToIncrement(divisor, increment, mode)
			if err != nil {
				t.Fatalf("DivRoundToIncrement(%s): %v", text, err)
			}
			got, err := roundRatToIncrement(decimalRat(value), divisor, increment, mode)
			if err != nil || got.String() != want.String() {
				t.Errorf("%s %s：有理数取整 %v %v，Decimal 取整 %s", mode, text, got, err, want)
			}
		}
	}
	if _, err := roundRatToIncrement(big.NewRat(1, 1), divisor, increment, RoundingNone); !errors.Is(err, ErrInvalidRoundingPolicy) {
		t.Errorf("NONE 应拒：%v", err)
	}
}

// Covers: ADR-0346 决定四——有限小数判定与截断：1/8 是 0.125，1/3 不是；截断向零、不进位。
func TestRationalToDecimalSeparatesFiniteFromNonTerminating(t *testing.T) {
	if value, exact := ratToDecimalExact(big.NewRat(1, 8)); !exact || value.String() != "0.125" {
		t.Errorf("1/8 = %v %v", value, exact)
	}
	if value, exact := ratToDecimalExact(big.NewRat(1000, 1)); !exact || value.String() != "1000" {
		t.Errorf("1000 = %v %v", value, exact)
	}
	if _, exact := ratToDecimalExact(big.NewRat(1, 3)); exact {
		t.Error("1/3 不是有限小数")
	}
	if value, ok := ratTruncate(big.NewRat(2, 3), 4); !ok || value.String() != "0.6666" {
		t.Errorf("2/3 截到 4 位 = %v %v，应为 0.6666 而不是 0.6667", value, ok)
	}
}

func volumetricFactorForTest(t testing.TB, length LengthUnit) VolumetricFactor {
	t.Helper()
	increment, err := NewWeight(mustDecimal(t, "0.1"), WeightUnitKilogram)
	if err != nil {
		t.Fatal(err)
	}
	rounding, err := NewWeightRoundingPolicy(RoundingCeiling, increment)
	if err != nil {
		t.Fatal(err)
	}
	factor, err := NewVolumetricFactor(mustDecimal(t, "5000"), length, rounding)
	if err != nil {
		t.Fatal(err)
	}
	return factor
}
