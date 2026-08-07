package domain

import "fmt"

// WeightRoundingSegment carries the rounding rule that applies below its
// maximum. Real channel terms round in grams up to some weight and in whole
// kilograms above it, so the increment cannot be a property of the policy as a
// whole; see `PA-PP-03` in the first-release development baseline.
type WeightRoundingSegment struct {
	mode       RoundingMode
	increment  Weight
	maximum    Weight
	hasMaximum bool
}

func NewWeightRoundingSegment(mode RoundingMode, increment Weight, maximum Weight) (WeightRoundingSegment, error) {
	segment, err := NewOpenEndedWeightRoundingSegment(mode, increment)
	if err != nil {
		return WeightRoundingSegment{}, err
	}
	if !maximum.valid() || maximum.value.Sign() <= 0 {
		return WeightRoundingSegment{}, fmt.Errorf("%w: maximum must be positive", ErrInvalidRoundingPolicy)
	}
	if maximum.unit != increment.unit {
		return WeightRoundingSegment{}, ErrWeightUnitMismatch
	}
	segment.maximum = maximum
	segment.hasMaximum = true
	return segment, nil
}

func NewOpenEndedWeightRoundingSegment(mode RoundingMode, increment Weight) (WeightRoundingSegment, error) {
	if !mode.valid() || !increment.valid() || increment.value.Sign() <= 0 {
		return WeightRoundingSegment{}, ErrInvalidRoundingPolicy
	}
	if mode == RoundingNone && !increment.value.Equal(NewDecimalFromInt64(1)) {
		return WeightRoundingSegment{}, fmt.Errorf("%w: NONE requires increment 1", ErrInvalidRoundingPolicy)
	}
	return WeightRoundingSegment{mode: mode, increment: increment}, nil
}

func (segment WeightRoundingSegment) Mode() RoundingMode { return segment.mode }
func (segment WeightRoundingSegment) Increment() Weight  { return segment.increment }
func (segment WeightRoundingSegment) Maximum() (Weight, bool) {
	return segment.maximum, segment.hasMaximum
}

func (segment WeightRoundingSegment) valid() bool {
	if !segment.mode.valid() || !segment.increment.valid() || segment.increment.value.Sign() <= 0 {
		return false
	}
	if segment.mode == RoundingNone && !segment.increment.value.Equal(NewDecimalFromInt64(1)) {
		return false
	}
	if !segment.hasMaximum {
		return true
	}
	return segment.maximum.valid() && segment.maximum.value.Sign() > 0 && segment.maximum.unit == segment.increment.unit
}

// WeightRoundingPolicy is an ordered, gap-free cover of every weight: each
// segment claims the weights below its maximum, and the last segment is open
// so no weight is left without a rule.
type WeightRoundingPolicy struct {
	segments []WeightRoundingSegment
}

func NewWeightRoundingPolicy(mode RoundingMode, increment Weight) (WeightRoundingPolicy, error) {
	segment, err := NewOpenEndedWeightRoundingSegment(mode, increment)
	if err != nil {
		return WeightRoundingPolicy{}, err
	}
	return NewSegmentedWeightRoundingPolicy([]WeightRoundingSegment{segment})
}

func NewSegmentedWeightRoundingPolicy(segments []WeightRoundingSegment) (WeightRoundingPolicy, error) {
	if len(segments) == 0 {
		return WeightRoundingPolicy{}, fmt.Errorf("%w: at least one segment is required", ErrInvalidRoundingPolicy)
	}
	copyOfSegments := append([]WeightRoundingSegment(nil), segments...)
	unit := copyOfSegments[0].increment.unit
	for index, segment := range copyOfSegments {
		if !segment.valid() {
			return WeightRoundingPolicy{}, ErrInvalidRoundingPolicy
		}
		if segment.increment.unit != unit || (segment.hasMaximum && segment.maximum.unit != unit) {
			return WeightRoundingPolicy{}, ErrWeightUnitMismatch
		}
		isLast := index == len(copyOfSegments)-1
		if segment.hasMaximum == isLast {
			return WeightRoundingPolicy{}, fmt.Errorf("%w: only the last segment may be open-ended", ErrInvalidRoundingPolicy)
		}
		if index > 0 && segment.hasMaximum && segment.maximum.value.Cmp(copyOfSegments[index-1].maximum.value) <= 0 {
			return WeightRoundingPolicy{}, fmt.Errorf("%w: segment boundaries must strictly increase", ErrInvalidRoundingPolicy)
		}
	}
	return WeightRoundingPolicy{segments: copyOfSegments}, nil
}

func (policy WeightRoundingPolicy) Segments() []WeightRoundingSegment {
	return append([]WeightRoundingSegment(nil), policy.segments...)
}

// Apply rounds a raw weight and reports which segment decided it, so the
// evaluation explanation can name the rule that was used rather than the
// policy as a whole.
func (policy WeightRoundingPolicy) Apply(raw Weight) (Weight, WeightRoundingSegment, error) {
	segment, err := policy.segmentFor(raw)
	if err != nil {
		return Weight{}, WeightRoundingSegment{}, err
	}
	roundedValue, err := raw.value.RoundToIncrement(segment.increment.value, segment.mode)
	if err != nil {
		return Weight{}, WeightRoundingSegment{}, err
	}
	rounded, err := NewWeight(roundedValue, raw.unit)
	if err != nil {
		return Weight{}, WeightRoundingSegment{}, err
	}
	return rounded, segment, nil
}

func (policy WeightRoundingPolicy) segmentFor(raw Weight) (WeightRoundingSegment, error) {
	if !policy.valid() {
		return WeightRoundingSegment{}, ErrInvalidRoundingPolicy
	}
	if !raw.valid() || raw.unit != policy.unit() {
		return WeightRoundingSegment{}, ErrWeightUnitMismatch
	}
	for _, segment := range policy.segments {
		if !segment.hasMaximum || raw.value.Cmp(segment.maximum.value) < 0 {
			return segment, nil
		}
	}
	return WeightRoundingSegment{}, ErrInvalidRoundingPolicy
}

// roundingSegmentScope names the segment in an explanation only when a policy
// actually has more than one; a single-segment policy has nothing to
// disambiguate and naming its bound would just add noise.
func roundingSegmentScope(segment WeightRoundingSegment) string {
	if !segment.hasMaximum {
		return ""
	}
	return fmt.Sprintf(" (segment below %s %s)", segment.maximum.value.String(), segment.maximum.unit)
}

// sole returns the only segment of a single-segment policy. Some rules declare
// a precision rather than a weight-banded rounding — a volumetric divisor
// cannot band by weight, because the weight is what the division produces — and
// those rules must refuse a segmented policy instead of silently using its
// first segment.
func (policy WeightRoundingPolicy) sole() (WeightRoundingSegment, bool) {
	if len(policy.segments) != 1 {
		return WeightRoundingSegment{}, false
	}
	return policy.segments[0], true
}

func (policy WeightRoundingPolicy) unit() WeightUnit {
	if len(policy.segments) == 0 {
		return ""
	}
	return policy.segments[0].increment.unit
}

func (policy WeightRoundingPolicy) valid() bool {
	_, err := NewSegmentedWeightRoundingPolicy(policy.segments)
	return err == nil
}
