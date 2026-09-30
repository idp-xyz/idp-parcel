package domain

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
)

var (
	// ErrInvalidPeriodicFee 说明周期费用形态或这次的实绩不是这套文法能算的输入。
	ErrInvalidPeriodicFee = errors.New("settlement accounting: invalid periodic fee")
)

// PeriodicFeeForm 是周期费用的封闭形态。价卡上的最低收费不在这里。
type PeriodicFeeForm uint8

const (
	PeriodicFeeFormInvalid PeriodicFeeForm = iota
	MinimumSpend
	VolumeFloor
	TieredRebate
	PeriodicFeeNotApplicable
)

func (form PeriodicFeeForm) valid() bool {
	return form == MinimumSpend || form == VolumeFloor || form == TieredRebate || form == PeriodicFeeNotApplicable
}

func (form PeriodicFeeForm) String() string {
	switch form {
	case MinimumSpend:
		return "MINIMUM_SPEND"
	case VolumeFloor:
		return "VOLUME_FLOOR"
	case TieredRebate:
		return "TIERED_REBATE"
	case PeriodicFeeNotApplicable:
		return "NOT_APPLICABLE"
	default:
		return ""
	}
}

func PeriodicFeeFormFromName(name string) (PeriodicFeeForm, error) {
	switch name {
	case "MINIMUM_SPEND":
		return MinimumSpend, nil
	case "VOLUME_FLOOR":
		return VolumeFloor, nil
	case "TIERED_REBATE":
		return TieredRebate, nil
	case "NOT_APPLICABLE":
		return PeriodicFeeNotApplicable, nil
	default:
		return PeriodicFeeFormInvalid, fmt.Errorf("%w: form", ErrInvalidPeriodicFee)
	}
}

// RebateTier 是阶梯返利的一档。有上界的档覆盖到该上界为止；开放档只许出现在最后，覆盖其上的全部基数。
type RebateTier struct {
	upperMinor      int64
	open            bool
	rateBasisPoints int64
	ok              bool
}

func NewRebateTier(upperMinor, rateBasisPoints int64) (RebateTier, error) {
	if upperMinor <= 0 || rateBasisPoints < 0 || rateBasisPoints > 10_000 {
		return RebateTier{}, fmt.Errorf("%w: tier", ErrInvalidPeriodicFee)
	}
	return RebateTier{upperMinor: upperMinor, rateBasisPoints: rateBasisPoints, ok: true}, nil
}

func NewOpenRebateTier(rateBasisPoints int64) (RebateTier, error) {
	if rateBasisPoints < 0 || rateBasisPoints > 10_000 {
		return RebateTier{}, fmt.Errorf("%w: open tier", ErrInvalidPeriodicFee)
	}
	return RebateTier{open: true, rateBasisPoints: rateBasisPoints, ok: true}, nil
}

func (tier RebateTier) Open() bool { return tier.open }

func (tier RebateTier) UpperMinor() int64 { return tier.upperMinor }

func (tier RebateTier) RateBasisPoints() int64 { return tier.rateBasisPoints }

// PeriodicFeeTerms 是一份已登记的周期费用形态。零值不是登记。
type PeriodicFeeTerms struct {
	form              PeriodicFeeForm
	minimumMinor      int64
	committedQuantity int64
	rateMinor         int64
	tiers             []RebateTier
	ok                bool
}

func NewMinimumSpendTerms(minimumMinor int64) (PeriodicFeeTerms, error) {
	if minimumMinor < 0 {
		return PeriodicFeeTerms{}, fmt.Errorf("%w: minimum", ErrInvalidPeriodicFee)
	}
	return PeriodicFeeTerms{form: MinimumSpend, minimumMinor: minimumMinor, ok: true}, nil
}

func NewVolumeFloorTerms(committedQuantity, rateMinor int64) (PeriodicFeeTerms, error) {
	if committedQuantity < 0 || rateMinor < 0 {
		return PeriodicFeeTerms{}, fmt.Errorf("%w: volume floor", ErrInvalidPeriodicFee)
	}
	return PeriodicFeeTerms{
		form: VolumeFloor, committedQuantity: committedQuantity, rateMinor: rateMinor, ok: true,
	}, nil
}

func NewTieredRebateTerms(tiers []RebateTier) (PeriodicFeeTerms, error) {
	if len(tiers) == 0 {
		return PeriodicFeeTerms{}, fmt.Errorf("%w: tiers", ErrInvalidPeriodicFee)
	}
	var previous int64
	for index, tier := range tiers {
		if !tier.ok {
			return PeriodicFeeTerms{}, fmt.Errorf("%w: tier", ErrInvalidPeriodicFee)
		}
		if tier.open {
			if index != len(tiers)-1 {
				return PeriodicFeeTerms{}, fmt.Errorf("%w: open tier", ErrInvalidPeriodicFee)
			}
			continue
		}
		if tier.upperMinor <= previous {
			return PeriodicFeeTerms{}, fmt.Errorf("%w: tier order", ErrInvalidPeriodicFee)
		}
		previous = tier.upperMinor
	}
	copied := append([]RebateTier(nil), tiers...)
	return PeriodicFeeTerms{form: TieredRebate, tiers: copied, ok: true}, nil
}

func NewPeriodicFeeNotApplicable() PeriodicFeeTerms {
	return PeriodicFeeTerms{form: PeriodicFeeNotApplicable, ok: true}
}

func (terms PeriodicFeeTerms) Form() PeriodicFeeForm { return terms.form }

func (terms PeriodicFeeTerms) MinimumMinor() int64 { return terms.minimumMinor }

func (terms PeriodicFeeTerms) CommittedQuantity() int64 { return terms.committedQuantity }

func (terms PeriodicFeeTerms) RateMinor() int64 { return terms.rateMinor }

func (terms PeriodicFeeTerms) Tiers() []RebateTier {
	return append([]RebateTier(nil), terms.tiers...)
}

func (terms PeriodicFeeTerms) Same(other PeriodicFeeTerms) bool {
	if !terms.ok || !other.ok || terms.form != other.form {
		return false
	}
	if terms.minimumMinor != other.minimumMinor || terms.committedQuantity != other.committedQuantity ||
		terms.rateMinor != other.rateMinor || len(terms.tiers) != len(other.tiers) {
		return false
	}
	for index := range terms.tiers {
		left, right := terms.tiers[index], other.tiers[index]
		if left.open != right.open || left.upperMinor != right.upperMinor || left.rateBasisPoints != right.rateBasisPoints {
			return false
		}
	}
	return true
}

// PeriodicFeeAssessment 是一次周期实绩收出来的结果。金额为 0 时不形成周期费用。
// 不适用是一份登记，不是没登记。
type PeriodicFeeAssessment struct {
	form          PeriodicFeeForm
	amountMinor   int64
	notApplicable bool
}

func (assessment PeriodicFeeAssessment) Form() PeriodicFeeForm { return assessment.form }

func (assessment PeriodicFeeAssessment) AmountMinor() int64 { return assessment.amountMinor }

func (assessment PeriodicFeeAssessment) FormsAmount() bool {
	return !assessment.notApplicable && assessment.amountMinor > 0
}

func (assessment PeriodicFeeAssessment) NotApplicable() bool { return assessment.notApplicable }

// AssessMinimumSpend 用周期内已计金额对最低消费求补差。已计金额达到或超过最低消费时补差为 0。
func (terms PeriodicFeeTerms) AssessMinimumSpend(actualMinor int64) (PeriodicFeeAssessment, error) {
	if !terms.ok || terms.form != MinimumSpend || actualMinor < 0 {
		return PeriodicFeeAssessment{}, fmt.Errorf("%w: minimum spend", ErrInvalidPeriodicFee)
	}
	shortfall := terms.minimumMinor - actualMinor
	if shortfall < 0 {
		shortfall = 0
	}
	return PeriodicFeeAssessment{form: MinimumSpend, amountMinor: shortfall}, nil
}

// AssessVolumeFloor 用周期内实绩数量对保底量求不足部分，再乘已登记的单位金额，向下取整。
// 实绩达到或超过保底量时金额为 0。数量的单位由登记该行的租户决定，这里不换算。
func (terms PeriodicFeeTerms) AssessVolumeFloor(actualQuantity int64) (PeriodicFeeAssessment, error) {
	if !terms.ok || terms.form != VolumeFloor || actualQuantity < 0 {
		return PeriodicFeeAssessment{}, fmt.Errorf("%w: volume floor", ErrInvalidPeriodicFee)
	}
	shortfall := terms.committedQuantity - actualQuantity
	if shortfall < 0 {
		shortfall = 0
	}
	product := new(big.Int).Mul(big.NewInt(shortfall), big.NewInt(terms.rateMinor))
	if !product.IsInt64() {
		return PeriodicFeeAssessment{}, fmt.Errorf("%w: volume floor overflow", ErrInvalidPeriodicFee)
	}
	return PeriodicFeeAssessment{form: VolumeFloor, amountMinor: product.Int64()}, nil
}

// AssessTieredRebate 按档把周期已计金额切成互不重叠的一段，每段乘该档万分比后向下取整，再相加。
// 最后一档上界之上若没有开放档，超出部分不返利。
func (terms PeriodicFeeTerms) AssessTieredRebate(baseMinor int64) (PeriodicFeeAssessment, error) {
	if !terms.ok || terms.form != TieredRebate || baseMinor < 0 {
		return PeriodicFeeAssessment{}, fmt.Errorf("%w: rebate", ErrInvalidPeriodicFee)
	}
	var cursor int64
	var rebate int64
	for _, tier := range terms.tiers {
		if baseMinor <= cursor {
			break
		}
		end := baseMinor
		if !tier.open && tier.upperMinor < end {
			end = tier.upperMinor
		}
		slice := end - cursor
		product := new(big.Int).Mul(big.NewInt(slice), big.NewInt(tier.rateBasisPoints))
		product.Quo(product, big.NewInt(10_000))
		if !product.IsInt64() {
			return PeriodicFeeAssessment{}, fmt.Errorf("%w: rebate overflow", ErrInvalidPeriodicFee)
		}
		next := rebate + product.Int64()
		if next < rebate {
			return PeriodicFeeAssessment{}, fmt.Errorf("%w: rebate overflow", ErrInvalidPeriodicFee)
		}
		rebate = next
		cursor = end
		if tier.open {
			break
		}
	}
	return PeriodicFeeAssessment{form: TieredRebate, amountMinor: rebate}, nil
}

func (terms PeriodicFeeTerms) AssessNotApplicable() (PeriodicFeeAssessment, error) {
	if !terms.ok || terms.form != PeriodicFeeNotApplicable {
		return PeriodicFeeAssessment{}, fmt.Errorf("%w: not applicable", ErrInvalidPeriodicFee)
	}
	return PeriodicFeeAssessment{form: PeriodicFeeNotApplicable, notApplicable: true}, nil
}

// PeriodicFeeRuleReference 指名租户采用的周期费用规则。形态与数值挂在这一个引用上。
type PeriodicFeeRuleReference struct{ requiredValue }

func NewPeriodicFeeRuleReference(value string) (PeriodicFeeRuleReference, error) {
	required, err := newRequiredValue("periodic fee rule reference", value)
	return PeriodicFeeRuleReference{required}, err
}

// PeriodicFeeRegistration 把一种形态挂到一条规则引用上。
type PeriodicFeeRegistration struct {
	rule  PeriodicFeeRuleReference
	terms PeriodicFeeTerms
}

func NewPeriodicFeeRegistration(rule PeriodicFeeRuleReference, terms PeriodicFeeTerms) (PeriodicFeeRegistration, error) {
	if !rule.valid() || !terms.ok || strings.TrimSpace(rule.String()) == "" {
		return PeriodicFeeRegistration{}, fmt.Errorf("%w: periodic fee", ErrBlankValue)
	}
	return PeriodicFeeRegistration{rule: rule, terms: terms}, nil
}

func (registration PeriodicFeeRegistration) Rule() PeriodicFeeRuleReference { return registration.rule }

func (registration PeriodicFeeRegistration) Terms() PeriodicFeeTerms { return registration.terms }

func (registration PeriodicFeeRegistration) SameRegistration(other PeriodicFeeRegistration) bool {
	return registration.rule.String() == other.rule.String() && registration.terms.Same(other.terms)
}
