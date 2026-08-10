package domain

import "fmt"

// WeightRoundingSegment 携带在其上界以下适用的取整规则。真实渠道条款在某个重量以下按
// 克进位、以上按整千克进位，所以进位单位不能是整个取整策略的属性；见首发开发主线中的
// `PA-PP-03`。
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

// WeightRoundingPolicy 是对所有重量的一次有序、无空档的覆盖：每一段认领其上界以下的
// 重量，最后一段开口，因此没有任何重量落得下没有规则可用。
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

// Apply 对原始重量进位，并报出是哪一段作的决定，使评价解释能指名实际用到的那条规则，
// 而不是整个取整策略。
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

// roundingSegmentScope 只有在取整策略确实不止一段时，才在解释里点出是哪一段；单段策略
// 没有歧义可消，点出它的界限只会添噪音。
func roundingSegmentScope(segment WeightRoundingSegment) string {
	if !segment.hasMaximum {
		return ""
	}
	return fmt.Sprintf(" (segment below %s %s)", segment.maximum.value.String(), segment.maximum.unit)
}

// sole 返回单段取整策略的那唯一一段。有些规则声明的是精度而不是按重量分段的取整——
// 体积系数没法按重量分段，因为重量正是这次相除的产物——这类规则必须拒绝分段策略，
// 而不是静默取用它的第一段。
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
