package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// The rate card states its dimension thresholds against the longest and the
// second longest side of a package, never against whichever side a shipper
// happened to declare first. Deriving the order from the values is what keeps
// a 8x70x9 package and a 70x8x9 package from being judged differently.
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

// Length plus girth is the card's own formula: longest side plus twice each of
// the two remaining sides. Computing it from the ordered sides rather than the
// declared ones is what makes the result independent of how a shipper wrote
// the measurements down.
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

// Cubic volume became a trigger in its own right for the 2026-01-26 additions,
// so it is a feature the card reads directly rather than a by-product of the
// side checks.
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

// A package with a zero or negative side is not a small package with an unusual
// shape, it is an unusable measurement. Accepting one would let it pass every
// "greater than" threshold check silently instead of holding the evaluation.
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
