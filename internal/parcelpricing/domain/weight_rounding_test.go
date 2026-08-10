package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// PA-PP-03（产品需求假设）：同一渠道可能在某个重量以下按克进位、以上按千克进位。只持有
// 单一进位单位的策略表达不了这件事，因而必须由重量落入的那个分段决定它怎么进位。
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

// 分段上界取开区间，与 RateEntry 对其最大值的既有读法一致。相邻两段绝不能同时认领同一
// 个重量。
func TestSegmentedRoundingTreatsTheSegmentBoundaryAsExclusive(t *testing.T) {
	policy := segmentedRoundingPolicy(t,
		boundedRoundingSegment(t, "0.001", "2"),
		openRoundingSegment(t, "1"),
	)
	if rounded := applyRounding(t, policy, "2"); rounded != "2" {
		t.Fatalf("the boundary weight rounded to %s, want 2 from the coarse segment", rounded)
	}
}

// 单分段策略是既有方案都在用的形状。引入分段不得改变这些方案算出来的结果。
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

// Covers: CONTEXT「区间空档、边界重叠」— 空档、重叠与缺开放段各自会让某个重量落到无规则
// 或两条规则上，两者都使进位不确定，因此必须在构造期拒绝，而不是等评价时才冒出来。
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

// 每个分段各带自己的进位单位，因而混单位的策略会拿一个边界去比一个不按同一单位计量的
// 重量。
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
