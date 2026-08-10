package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// Covers: CONTEXT「最长边、次长边按尺寸三边的实际大小排序得出，不按声明顺序」— 价卡的尺寸
// 阈值是对着最长边与次长边说的，从来不是对着发货人碰巧先填的那一边。按取值排序，才使
// 8x70x9 与 70x8x9 两个包裹不会被判成两样。
func TestDimensionsDeriveSideOrderFromValuesNotDeclarationOrder(t *testing.T) {
	for _, testCase := range []struct {
		name                  string
		length, width, height string
	}{
		{"longest declared first", "70", "8", "9"},
		{"longest declared last", "8", "9", "70"},
		{"longest declared in the middle", "9", "70", "8"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			sides := dimensions(t, testCase.length, testCase.width, testCase.height, domain.LengthUnitInch)

			assertLength(t, "longest side", sides.LongestSide(), "70", domain.LengthUnitInch)
			assertLength(t, "second longest side", sides.SecondLongestSide(), "9", domain.LengthUnitInch)
			assertLength(t, "shortest side", sides.ShortestSide(), "8", domain.LengthUnitInch)
		})
	}
}

// Covers: CONTEXT「长加围为最长边加其余两边各两倍」— 这是卡自己的公式。用排序后的边而不是
// 声明顺序的边去算，结果才与发货人怎么填写测量值无关。
func TestDimensionsComputeLengthPlusGirthFromOrderedSides(t *testing.T) {
	for _, testCase := range []struct {
		name                  string
		length, width, height string
		expected              string
	}{
		{"longest declared first", "70", "8", "9", "104"},
		{"longest declared last", "8", "9", "70", "104"},
		{"all sides equal", "10", "10", "10", "50"},
		{"fractional sides", "48.5", "12.25", "6.5", "86"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			sides := dimensions(t, testCase.length, testCase.width, testCase.height, domain.LengthUnitInch)

			lengthPlusGirth, err := sides.LengthPlusGirth()
			if err != nil {
				t.Fatalf("length plus girth: %v", err)
			}
			assertLength(t, "length plus girth", lengthPlusGirth, testCase.expected, domain.LengthUnitInch)
		})
	}
}

// 体积在 2026-01-26 新增条款里成了独立的触发量，所以它是卡直接读的一个特征，不是边长检查
// 的副产品。
func TestDimensionsComputeCubicVolume(t *testing.T) {
	for _, testCase := range []struct {
		name                  string
		length, width, height string
		expected              string
	}{
		{"whole sides", "70", "8", "9", "5040"},
		{"cube", "24", "24", "18", "10368"},
		{"fractional sides multiply exactly", "12.5", "10.2", "4", "510"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			sides := dimensions(t, testCase.length, testCase.width, testCase.height, domain.LengthUnitInch)

			volume, err := sides.Volume()
			if err != nil {
				t.Fatalf("volume: %v", err)
			}
			if volume.Value().String() != testCase.expected {
				t.Fatalf("volume = %s, want %s", volume.Value().String(), testCase.expected)
			}
			if volume.Unit() != domain.LengthUnitInch {
				t.Fatalf("volume unit = %s, want %s", volume.Unit(), domain.LengthUnitInch)
			}
		})
	}
}

// 某一边为零或为负的包裹不是形状特别的小包，而是一份不可用的测量。接受它会让它静默通过
// 每一次「大于」阈值检查，而不是把评价拦下来。
func TestDimensionsRejectUnusableMeasurements(t *testing.T) {
	for _, testCase := range []struct {
		name                  string
		length, width, height string
		unit                  domain.LengthUnit
	}{
		{"zero side", "70", "0", "9", domain.LengthUnitInch},
		{"negative side", "70", "-8", "9", domain.LengthUnitInch},
		{"unknown unit", "70", "8", "9", domain.LengthUnit("FT")},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := domain.NewDimensions(
				decimal(t, testCase.length),
				decimal(t, testCase.width),
				decimal(t, testCase.height),
				testCase.unit,
			)
			if !errors.Is(err, domain.ErrInvalidDimensions) {
				t.Fatalf("err = %v, want %v", err, domain.ErrInvalidDimensions)
			}
		})
	}
}
