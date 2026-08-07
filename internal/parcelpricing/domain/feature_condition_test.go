package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// The card states its dimension triggers as strict "greater than": BND-003
// records a 48 inch longest side as a miss and BND-004 records 48.01 as a hit.
// The threshold is declared data in both cases, never a constant the code
// carries, so the test supplies it the same way a rate card version would.
func TestLengthFeatureConditionGreaterThanExcludesTheThresholdItself(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		longestSide string
		expected    bool
	}{
		{"below the threshold", "47.99", false},
		{"at the threshold", "48", false},
		{"just above the threshold", "48.01", true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			condition, err := domain.NewLengthFeatureCondition(
				domain.FeatureLongestSide,
				domain.ComparisonGreaterThan,
				length(t, "48", domain.LengthUnitInch),
			)
			if err != nil {
				t.Fatalf("condition: %v", err)
			}

			matched, err := condition.Matches(
				features(t, dimensions(t, testCase.longestSide, "30", "20", domain.LengthUnitInch)),
			)
			if err != nil {
				t.Fatalf("matches: %v", err)
			}
			if matched != testCase.expected {
				t.Fatalf("matched = %t, want %t", matched, testCase.expected)
			}
		})
	}
}

// R40 states its limits in centimetres while the rate table works in inches, so
// a condition and the feature it reads can genuinely disagree on unit. This
// package must not convert between them: the card would have to declare the
// conversion as a versioned rule first. The disagreement is not academic — a
// 108 inch side is 274.32 cm and clears a 274 cm limit, while comparing the
// bare numbers would report the opposite.
func TestLengthFeatureConditionRefusesToCompareAcrossUnits(t *testing.T) {
	condition, err := domain.NewLengthFeatureCondition(
		domain.FeatureLongestSide,
		domain.ComparisonGreaterThan,
		length(t, "274", domain.LengthUnitCentimeter),
	)
	if err != nil {
		t.Fatalf("condition: %v", err)
	}

	_, err = condition.Matches(features(t, dimensions(t, "108", "30", "20", domain.LengthUnitInch)))
	if !errors.Is(err, domain.ErrLengthUnitMismatch) {
		t.Fatalf("err = %v, want %v", err, domain.ErrLengthUnitMismatch)
	}
}
