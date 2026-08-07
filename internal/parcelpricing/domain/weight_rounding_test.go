package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// PA-PP-03: a channel can round in grams below some weight and in kilograms
// above it. A policy holding a single increment cannot express that, so the
// segment a weight falls into has to decide how that weight is rounded.
func TestSegmentedRoundingAppliesTheSegmentTheRawWeightFallsInto(t *testing.T) {
	policy := segmentedRoundingPolicy(t,
		boundedRoundingSegment(t, "0.001", "2"),
		openRoundingSegment(t, "1"),
	)

	if rounded := applyRounding(t, policy, "1.2345"); rounded != "1.235" {
		t.Fatalf("below the boundary rounded to %s, want 1.235 from the 0.001 segment", rounded)
	}
	if rounded := applyRounding(t, policy, "2.4"); rounded != "3" {
		t.Fatalf("above the boundary rounded to %s, want 3 from the 1 segment", rounded)
	}
}

// The boundary is exclusive, matching how RateEntry already treats its maximum.
// Two adjacent segments must never both claim the same weight.
func TestSegmentedRoundingTreatsTheSegmentBoundaryAsExclusive(t *testing.T) {
	policy := segmentedRoundingPolicy(t,
		boundedRoundingSegment(t, "0.001", "2"),
		openRoundingSegment(t, "1"),
	)
	if rounded := applyRounding(t, policy, "2"); rounded != "2" {
		t.Fatalf("the boundary weight rounded to %s, want 2 from the coarse segment", rounded)
	}
}

// A single-segment policy is the shape every existing plan uses. Introducing
// segments must not change what those plans compute.
func TestSingleSegmentPolicyRoundsExactlyAsBefore(t *testing.T) {
	policy, err := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, weight(t, "1", domain.WeightUnitKilogram))
	if err != nil {
		t.Fatalf("single-segment policy: %v", err)
	}
	if rounded := applyRounding(t, policy, "1.2"); rounded != "2" {
		t.Fatalf("single-segment rounding produced %s, want 2", rounded)
	}
	if segments := policy.Segments(); len(segments) != 1 {
		t.Fatalf("single-segment policy exposed %d segments, want 1", len(segments))
	}
}

// Gaps, overlaps and a missing open end each leave some weight with either no
// rule or two rules. Both make the rounding non-deterministic, so they must be
// rejected at construction rather than surfacing during an evaluation.
func TestSegmentedRoundingRejectsPoliciesThatDoNotCoverEveryWeightExactlyOnce(t *testing.T) {
	fine := boundedRoundingSegment(t, "0.001", "2")
	wider := boundedRoundingSegment(t, "0.01", "5")
	open := openRoundingSegment(t, "1")

	cases := map[string][]domain.WeightRoundingSegment{
		"no segments":             {},
		"no open-ended segment":   {fine, wider},
		"open-ended not last":     {open, fine},
		"descending boundaries":   {wider, fine, open},
		"duplicate boundaries":    {fine, fine, open},
		"two open-ended segments": {open, open},
	}
	for name, segments := range cases {
		if _, err := domain.NewSegmentedWeightRoundingPolicy(segments); !errors.Is(err, domain.ErrInvalidRoundingPolicy) {
			t.Fatalf("%s: error = %v, want ErrInvalidRoundingPolicy", name, err)
		}
	}
}

// Each segment carries its own increment, so a mixed-unit policy would compare
// a boundary against a weight that is not measured in the same unit.
func TestSegmentedRoundingRejectsMixedUnits(t *testing.T) {
	metric := boundedRoundingSegment(t, "0.001", "2")
	imperial, err := domain.NewOpenEndedWeightRoundingSegment(domain.RoundingCeiling, weight(t, "1", domain.WeightUnitPound))
	if err != nil {
		t.Fatalf("imperial segment: %v", err)
	}
	if _, err := domain.NewSegmentedWeightRoundingPolicy([]domain.WeightRoundingSegment{metric, imperial}); !errors.Is(err, domain.ErrWeightUnitMismatch) {
		t.Fatalf("mixed-unit policy error = %v, want ErrWeightUnitMismatch", err)
	}
}

func boundedRoundingSegment(t *testing.T, increment, maximum string) domain.WeightRoundingSegment {
	t.Helper()
	segment, err := domain.NewWeightRoundingSegment(
		domain.RoundingCeiling,
		weight(t, increment, domain.WeightUnitKilogram),
		weight(t, maximum, domain.WeightUnitKilogram),
	)
	if err != nil {
		t.Fatalf("bounded rounding segment (%s, %s): %v", increment, maximum, err)
	}
	return segment
}

func openRoundingSegment(t *testing.T, increment string) domain.WeightRoundingSegment {
	t.Helper()
	segment, err := domain.NewOpenEndedWeightRoundingSegment(
		domain.RoundingCeiling,
		weight(t, increment, domain.WeightUnitKilogram),
	)
	if err != nil {
		t.Fatalf("open-ended rounding segment (%s): %v", increment, err)
	}
	return segment
}

func segmentedRoundingPolicy(t *testing.T, segments ...domain.WeightRoundingSegment) domain.WeightRoundingPolicy {
	t.Helper()
	policy, err := domain.NewSegmentedWeightRoundingPolicy(segments)
	if err != nil {
		t.Fatalf("segmented rounding policy: %v", err)
	}
	return policy
}

func applyRounding(t *testing.T, policy domain.WeightRoundingPolicy, raw string) string {
	t.Helper()
	rounded, _, err := policy.Apply(weight(t, raw, domain.WeightUnitKilogram))
	if err != nil {
		t.Fatalf("apply rounding to %s: %v", raw, err)
	}
	return rounded.Value().String()
}
