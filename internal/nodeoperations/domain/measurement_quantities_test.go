package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// 本文件钉住称重量方里两个值对象的取法：数量按十进制正数的写法收、单位要在场，两头的空白不算内容，不换算、不改写数字，
// 零值不是一条测量。夹具为隔离合成，只记 `S`。

// malformedQuantities 是任何一格数量都不收的写法。
var malformedQuantities = map[string]string{
	"空":        "",
	"只有空白":     " \t ",
	"不是数":      "abc",
	"零":        "0",
	"零带小数":     "0.00",
	"负数":       "-2",
	"带正号":      "+2",
	"小数点后没有数":  "2.",
	"小数点前没有数":  ".5",
	"指数写法":     "1e2",
	"千分位":      "1,000",
	"数字之间夹着空白": "2 5",
	"全角数字":     "２５",
	"两个小数点":    "1.2.3",
}

func TestAGrossWeightKeepsItsQuantityAndUnitTrimmedAndUnconverted(t *testing.T) {
	weight, err := domain.NewGrossWeight(" 2.100 ", "\tKG ")
	if err != nil {
		t.Fatalf("形成毛重：%v", err)
	}
	if weight.Value() != "2.100" || weight.Unit() != "KG" {
		t.Fatalf("毛重 = %q %q，want 去掉两头空白、数字原样的 2.100 KG", weight.Value(), weight.Unit())
	}
	same, err := domain.NewGrossWeight("2.100", "KG")
	if err != nil || same != weight {
		t.Fatalf("同一份毛重的两种写法 = %+v, err %v，want 相等", same, err)
	}
	other, err := domain.NewGrossWeight("2.10", "KG")
	if err != nil || other == weight {
		t.Fatalf("2.10 与 2.100 = %+v, err %v，want 不相等：不换算、不归一", other, err)
	}
}

func TestAGrossWeightRefusesAMalformedQuantityOrAMissingUnit(t *testing.T) {
	for name, raw := range malformedQuantities {
		t.Run("数量/"+name, func(t *testing.T) {
			weight, err := domain.NewGrossWeight(raw, "KG")
			if !errors.Is(err, domain.ErrInvalidActualMeasurement) {
				t.Fatalf("实得 %v", err)
			}
			if weight != (domain.GrossWeight{}) {
				t.Fatalf("拒收时交回 %+v，want 零值", weight)
			}
		})
	}
	for name, unit := range map[string]string{"空": "", "只有空白": " \t "} {
		t.Run("单位/"+name, func(t *testing.T) {
			weight, err := domain.NewGrossWeight("2.10", unit)
			if !errors.Is(err, domain.ErrInvalidActualMeasurement) {
				t.Fatalf("实得 %v", err)
			}
			if weight != (domain.GrossWeight{}) {
				t.Fatalf("拒收时交回 %+v，want 零值", weight)
			}
		})
	}
}

func TestOuterDimensionsKeepTheirThreeSidesUnderOneUnit(t *testing.T) {
	dimensions, err := domain.NewOuterDimensions(" 20", "15.5\n", "10 ", " CM ")
	if err != nil {
		t.Fatalf("形成外廓尺寸：%v", err)
	}
	if dimensions.Length() != "20" || dimensions.Width() != "15.5" || dimensions.Height() != "10" || dimensions.Unit() != "CM" {
		t.Fatalf("外廓尺寸 = %q×%q×%q %q", dimensions.Length(), dimensions.Width(), dimensions.Height(), dimensions.Unit())
	}
	same, err := domain.NewOuterDimensions("20", "15.5", "10", "CM")
	if err != nil || same != dimensions {
		t.Fatalf("同一份尺寸的两种写法 = %+v, err %v，want 相等", same, err)
	}
	swapped, err := domain.NewOuterDimensions("15.5", "20", "10", "CM")
	if err != nil || swapped == dimensions {
		t.Fatalf("长宽对调 = %+v, err %v，want 不相等：三边各有各的位置", swapped, err)
	}
}

func TestOuterDimensionsRefuseAnySideThatIsMalformedOrAMissingUnit(t *testing.T) {
	sides := map[string]func(raw string) (domain.OuterDimensions, error){
		"长": func(raw string) (domain.OuterDimensions, error) {
			return domain.NewOuterDimensions(raw, "15", "10", "CM")
		},
		"宽": func(raw string) (domain.OuterDimensions, error) {
			return domain.NewOuterDimensions("20", raw, "10", "CM")
		},
		"高": func(raw string) (domain.OuterDimensions, error) {
			return domain.NewOuterDimensions("20", "15", raw, "CM")
		},
	}
	for side, form := range sides {
		for name, raw := range malformedQuantities {
			t.Run(side+"/"+name, func(t *testing.T) {
				dimensions, err := form(raw)
				if !errors.Is(err, domain.ErrInvalidActualMeasurement) {
					t.Fatalf("实得 %v", err)
				}
				if dimensions != (domain.OuterDimensions{}) {
					t.Fatalf("拒收时交回 %+v，want 零值", dimensions)
				}
			})
		}
	}
	for name, unit := range map[string]string{"空": "", "只有空白": " \t "} {
		t.Run("单位/"+name, func(t *testing.T) {
			dimensions, err := domain.NewOuterDimensions("20", "15", "10", unit)
			if !errors.Is(err, domain.ErrInvalidActualMeasurement) {
				t.Fatalf("实得 %v", err)
			}
			if dimensions != (domain.OuterDimensions{}) {
				t.Fatalf("拒收时交回 %+v，want 零值", dimensions)
			}
		})
	}
}

// Covers: 两个值对象的零值不是一条测量的毛重或尺寸。绕过构造器、直接拿零值填进规格，形成测量时被拒，与「只称了重、没量方」
// 和「只量了方、没称重」同一个结果；零值上的取值器交回空串，不 panic。
func TestTheZeroValueOfAMeasurementQuantityIsNotAMeasurement(t *testing.T) {
	var weight domain.GrossWeight
	var dimensions domain.OuterDimensions
	if weight.Value() != "" || weight.Unit() != "" {
		t.Fatalf("毛重零值 = %q %q", weight.Value(), weight.Unit())
	}
	if dimensions.Length() != "" || dimensions.Width() != "" || dimensions.Height() != "" || dimensions.Unit() != "" {
		t.Fatalf("尺寸零值 = %q×%q×%q %q", dimensions.Length(), dimensions.Width(), dimensions.Height(), dimensions.Unit())
	}

	withoutWeight := measurementSpec(t)
	withoutWeight.Weight = domain.GrossWeight{}
	if _, err := domain.NewActualMeasurement(withoutWeight); !errors.Is(err, domain.ErrInvalidActualMeasurement) {
		t.Fatalf("只量了方、没称重 = %v", err)
	}
	withoutDimensions := measurementSpec(t)
	withoutDimensions.Dimensions = domain.OuterDimensions{}
	if _, err := domain.NewActualMeasurement(withoutDimensions); !errors.Is(err, domain.ErrInvalidActualMeasurement) {
		t.Fatalf("只称了重、没量方 = %v", err)
	}
}
