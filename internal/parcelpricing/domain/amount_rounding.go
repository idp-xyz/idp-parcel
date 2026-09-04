package domain

import (
	"errors"
	"fmt"
	"sort"
)

// ErrInvalidAmountRoundingPolicy 表示一条金额取整策略立不住：模式不在封闭集或为 NONE、进位单位非正、
// 应用点缺合计或不在封闭集。
var ErrInvalidAmountRoundingPolicy = errors.New("parcel pricing: invalid amount rounding policy")

// AmountRoundingPoint 是金额取整的应用点（ADR-0107 Decision 二）。封闭集合：合计必声明，逐行与换算后
// 可选。取整顺序不由卡声明——它就是这三格在评价里的固定先后（逐行 → 换算后 → 合计），顺序属机制，
// 点属实例。
type AmountRoundingPoint string

const (
	AmountRoundingPerLine         AmountRoundingPoint = "PER_LINE"
	AmountRoundingAfterConversion AmountRoundingPoint = "AFTER_CONVERSION"
	AmountRoundingTotal           AmountRoundingPoint = "TOTAL"
)

func (point AmountRoundingPoint) valid() bool {
	switch point {
	case AmountRoundingPerLine, AmountRoundingAfterConversion, AmountRoundingTotal:
		return true
	default:
		return false
	}
}

func (point AmountRoundingPoint) String() string { return string(point) }

// amountRoundingOrder 是三个应用点在评价里的固定先后，也是策略里点集合的规范化排序。
func amountRoundingOrder(point AmountRoundingPoint) int {
	switch point {
	case AmountRoundingPerLine:
		return 0
	case AmountRoundingAfterConversion:
		return 1
	case AmountRoundingTotal:
		return 2
	default:
		return 3
	}
}

// AmountRoundingPolicy 是价卡声明的金额取整策略（CONTEXT「金额取整策略」；ADR-0107 Decision 一、二）：
// 模式、进位单位、应用点集合三件，随卡版本化、进内容摘要。进位单位以该卡币种的金额表示，是卡上声明
// 的实例值——本上下文不内置任何金额精度常量，也不按币种查任何表；未声明策略的卡评价照旧不取整并记
// 问题项（Decision 四），这里没有默认值可给。
type AmountRoundingPolicy struct {
	mode      RoundingMode
	increment Money
	points    []AmountRoundingPoint
}

// NewAmountRoundingPolicy 立一条策略。模式不许 NONE：「不取整」由不声明策略表达，声明一条 NONE 会让
// 「声明了但等于没声明」与「没声明」在摘要里长两张脸而行为一样。进位单位必须为正；应用点去重排序，
// 必含合计。
func NewAmountRoundingPolicy(mode RoundingMode, increment Money, points []AmountRoundingPoint) (AmountRoundingPolicy, error) {
	if !mode.valid() || mode == RoundingNone {
		return AmountRoundingPolicy{}, fmt.Errorf("%w: mode %q", ErrInvalidAmountRoundingPolicy, mode)
	}
	if !increment.valid() || increment.amount.Sign() <= 0 {
		return AmountRoundingPolicy{}, fmt.Errorf("%w: increment must be a positive amount", ErrInvalidAmountRoundingPolicy)
	}
	seen := make(map[AmountRoundingPoint]struct{}, len(points))
	ordered := make([]AmountRoundingPoint, 0, len(points))
	for _, point := range points {
		if !point.valid() {
			return AmountRoundingPolicy{}, fmt.Errorf("%w: application point %q", ErrInvalidAmountRoundingPolicy, point)
		}
		if _, exists := seen[point]; exists {
			continue
		}
		seen[point] = struct{}{}
		ordered = append(ordered, point)
	}
	if _, declared := seen[AmountRoundingTotal]; !declared {
		return AmountRoundingPolicy{}, fmt.Errorf("%w: the TOTAL application point must be declared", ErrInvalidAmountRoundingPolicy)
	}
	sort.Slice(ordered, func(left, right int) bool {
		return amountRoundingOrder(ordered[left]) < amountRoundingOrder(ordered[right])
	})
	return AmountRoundingPolicy{mode: mode, increment: increment, points: ordered}, nil
}

func (policy AmountRoundingPolicy) Mode() RoundingMode { return policy.mode }
func (policy AmountRoundingPolicy) Increment() Money   { return policy.increment }

// Points 按评价里的固定先后交回声明的应用点。
func (policy AmountRoundingPolicy) Points() []AmountRoundingPoint {
	return append([]AmountRoundingPoint(nil), policy.points...)
}

// AppliesAt 说明策略是否在某一点取整。
func (policy AmountRoundingPolicy) AppliesAt(point AmountRoundingPoint) bool {
	for _, declared := range policy.points {
		if declared == point {
			return true
		}
	}
	return false
}

func (policy AmountRoundingPolicy) valid() bool {
	_, err := NewAmountRoundingPolicy(policy.mode, policy.increment, policy.points)
	return err == nil
}

// round 把一笔金额落到进位单位的整数倍上。只用进位单位的**数值**：换算后那一点上金额已是结算币种，
// 卡币种声明的 `0.01` 说的是精度不是币种。负数按绝对值取整再还原符号——HALF_UP 于是是「半数远离
// 零」，与商业取整的惯常读法一致。
func (policy AmountRoundingPolicy) round(amount Money) (Money, error) {
	magnitude := amount.amount.Abs()
	rounded, err := magnitude.RoundToIncrement(policy.increment.amount, policy.mode)
	if err != nil {
		return Money{}, err
	}
	if amount.amount.IsNegative() && rounded.Sign() != 0 {
		rounded = Decimal{coefficient: "-" + rounded.coefficient, scale: rounded.scale}
	}
	return NewMoney(rounded, amount.currency)
}

// AmountRoundingStep 是评价里的一次取整留痕（ADR-0107 Decision 二「可复算」对金额成立的那一格）：
// 在哪一点、对谁、按什么模式与进位单位、取整前后各是多少。逐行那一点 subject 是费用行标识，其余两点
// subject 留空。
type AmountRoundingStep struct {
	point     AmountRoundingPoint
	subject   string
	mode      RoundingMode
	increment Money
	before    Money
	after     Money
}

func (step AmountRoundingStep) Point() AmountRoundingPoint { return step.point }
func (step AmountRoundingStep) Subject() string            { return step.subject }
func (step AmountRoundingStep) Mode() RoundingMode         { return step.mode }
func (step AmountRoundingStep) Increment() Money           { return step.increment }
func (step AmountRoundingStep) Before() Money              { return step.before }
func (step AmountRoundingStep) After() Money               { return step.after }

func (step AmountRoundingStep) valid() bool {
	return step.point.valid() && step.mode.valid() && step.mode != RoundingNone &&
		step.increment.valid() && step.increment.amount.Sign() > 0 &&
		step.before.valid() && step.after.valid() && step.before.currency == step.after.currency
}

func (step AmountRoundingStep) explain() string {
	subject := ""
	if step.subject != "" {
		subject = " " + step.subject
	}
	return fmt.Sprintf("rounded %s%s %s %s to %s %s (%s to %s)",
		step.point, subject,
		step.before.amount.String(), step.before.currency,
		step.after.amount.String(), step.after.currency,
		step.mode, step.increment.amount.String())
}

// applyAmountRounding 在某一点对一笔金额取整并交回留痕。策略缺席或该点未声明时金额原样交回、不留痕。
func applyAmountRounding(policy *AmountRoundingPolicy, point AmountRoundingPoint, subject string, amount Money) (Money, *AmountRoundingStep, error) {
	if policy == nil || !policy.AppliesAt(point) {
		return amount, nil, nil
	}
	rounded, err := policy.round(amount)
	if err != nil {
		return Money{}, nil, err
	}
	step := AmountRoundingStep{
		point:     point,
		subject:   subject,
		mode:      policy.mode,
		increment: policy.increment,
		before:    amount,
		after:     rounded,
	}
	return rounded, &step, nil
}
