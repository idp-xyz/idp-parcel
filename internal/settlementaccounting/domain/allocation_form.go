package domain

import (
	"errors"
	"fmt"
	"math/big"
	"sort"
)

var (
	// ErrInvalidAllocationForm 说明分法或这次的权重不是三套内置分法能算的输入。
	ErrInvalidAllocationForm = errors.New("settlement accounting: invalid allocation form")
	// ErrAllocationFormOverflow 说明权重合计装不进一个分母。不绕回，也不改成未配置。
	ErrAllocationFormOverflow = errors.New("settlement accounting: allocation form overflow")
)

// AllocationForm 是一套内置分法。三套共用同一条展开，差别只在权重的含义。
// NOT_APPLICABLE 是租户登记的「本期不适用」，不是没登记，也不能拿来分摊。
type AllocationForm uint8

const (
	AllocationFormInvalid AllocationForm = iota
	AllocationByWeight
	AllocationByPiece
	AllocationByRevenue
	AllocationNotApplicable
)

func (form AllocationForm) String() string {
	switch form {
	case AllocationByWeight:
		return "BY_WEIGHT"
	case AllocationByPiece:
		return "BY_PIECE"
	case AllocationByRevenue:
		return "BY_REVENUE"
	case AllocationNotApplicable:
		return "NOT_APPLICABLE"
	default:
		return ""
	}
}

func (form AllocationForm) apportions() bool {
	return form == AllocationByWeight || form == AllocationByPiece || form == AllocationByRevenue
}

// AllocationFormFromName 只认封闭四值。词表外拒，不夹成按重。
func AllocationFormFromName(name string) (AllocationForm, error) {
	switch name {
	case "BY_WEIGHT":
		return AllocationByWeight, nil
	case "BY_PIECE":
		return AllocationByPiece, nil
	case "BY_REVENUE":
		return AllocationByRevenue, nil
	case "NOT_APPLICABLE":
		return AllocationNotApplicable, nil
	default:
		return AllocationFormInvalid, fmt.Errorf("%w: allocation form", ErrInvalidAllocationForm)
	}
}

// AllocationBasis 是一个对象在这次分摊上的权重。按重是重量，按件是件数，按收入是收入的最小货币单位。
// 数值由这次分摊交入，不写在分法登记上。
type AllocationBasis struct {
	Target AllocationTargetReference
	Basis  int64
}

// AllocationShare 是一个对象的展开：理论分数、向下取整、余数与实际份额。实际为 0 的对象仍留在展开里，
// 但不进入份额——份额必须是正数（成本分摊的构造门）。
type AllocationShare struct {
	Target                 AllocationTargetReference
	Basis                  int64
	TheoreticalNumerator   int64
	TheoreticalDenominator int64
	FloorMinor             int64
	RemainderNumerator     int64
	ActualMinor            int64
	Carried                bool
}

// AllocationApportionment 是一次分法展开。未分摊余额与各实际份额之和等于来源金额。
type AllocationApportionment struct {
	form        AllocationForm
	sourceMinor int64
	denominator int64
	shares      []AllocationShare
	unallocated int64
}

func (apportionment AllocationApportionment) Form() AllocationForm { return apportionment.form }

func (apportionment AllocationApportionment) SourceMinor() int64 { return apportionment.sourceMinor }

func (apportionment AllocationApportionment) Denominator() int64 { return apportionment.denominator }

func (apportionment AllocationApportionment) UnallocatedMinor() int64 {
	return apportionment.unallocated
}

func (apportionment AllocationApportionment) Shares() []AllocationShare {
	return append([]AllocationShare(nil), apportionment.shares...)
}

// Portions 只交实际为正的份额，顺序是目标标识升序。实际为 0 的展开行不在这里。
func (apportionment AllocationApportionment) Portions() []AllocationPortion {
	portions := make([]AllocationPortion, 0, len(apportionment.shares))
	for _, share := range apportionment.shares {
		if share.ActualMinor <= 0 {
			continue
		}
		portions = append(portions, AllocationPortion{Target: share.Target, AmountMinor: share.ActualMinor})
	}
	return portions
}

// Apportion 按已选定的分法把来源金额展开成份额。顺序固定：先按权重取整，余下的最小货币单位按余数从大到小
// 各加一；余数相同按目标标识升序。没有正权重时整笔未分摊，不虚构对象，也不改成平均分。
//
// 乘法走 big.Int。权重合计装不进 int64 时报溢出，不静默绕回。
func Apportion(form AllocationForm, sourceMinor int64, bases []AllocationBasis) (AllocationApportionment, error) {
	if !form.apportions() || sourceMinor <= 0 {
		return AllocationApportionment{}, fmt.Errorf("%w: form or source", ErrInvalidAllocationForm)
	}
	seen := make(map[AllocationTargetReference]struct{}, len(bases))
	positive := make([]AllocationShare, 0, len(bases))
	total := new(big.Int)
	for _, basis := range bases {
		if !basis.Target.valid() || basis.Basis < 0 {
			return AllocationApportionment{}, fmt.Errorf("%w: basis", ErrInvalidAllocationForm)
		}
		if _, exists := seen[basis.Target]; exists {
			return AllocationApportionment{}, fmt.Errorf("%w: duplicate target", ErrInvalidAllocationForm)
		}
		seen[basis.Target] = struct{}{}
		if basis.Basis == 0 {
			continue
		}
		positive = append(positive, AllocationShare{Target: basis.Target, Basis: basis.Basis})
		total.Add(total, big.NewInt(basis.Basis))
	}
	if len(positive) == 0 {
		return AllocationApportionment{form: form, sourceMinor: sourceMinor, unallocated: sourceMinor}, nil
	}
	if !total.IsInt64() {
		return AllocationApportionment{}, ErrAllocationFormOverflow
	}
	denominator := total.Int64()
	source := big.NewInt(sourceMinor)
	floorSum := int64(0)
	for index := range positive {
		product := new(big.Int).Mul(source, big.NewInt(positive[index].Basis))
		floor := new(big.Int).Quo(product, total)
		remainder := new(big.Int).Mod(product, total)
		if !product.IsInt64() || !floor.IsInt64() || !remainder.IsInt64() {
			return AllocationApportionment{}, ErrAllocationFormOverflow
		}
		positive[index].TheoreticalNumerator = product.Int64()
		positive[index].TheoreticalDenominator = denominator
		positive[index].FloorMinor = floor.Int64()
		positive[index].RemainderNumerator = remainder.Int64()
		positive[index].ActualMinor = positive[index].FloorMinor
		floorSum += positive[index].FloorMinor
	}
	leftover := sourceMinor - floorSum
	if leftover < 0 || leftover > int64(len(positive)) {
		return AllocationApportionment{}, ErrAllocationFormOverflow
	}
	order := make([]int, len(positive))
	for index := range order {
		order[index] = index
	}
	sort.SliceStable(order, func(left, right int) bool {
		if positive[order[left]].RemainderNumerator != positive[order[right]].RemainderNumerator {
			return positive[order[left]].RemainderNumerator > positive[order[right]].RemainderNumerator
		}
		return positive[order[left]].Target.String() < positive[order[right]].Target.String()
	})
	for index := int64(0); index < leftover; index++ {
		share := &positive[order[index]]
		share.ActualMinor++
		share.Carried = true
	}
	sort.SliceStable(positive, func(left, right int) bool {
		return positive[left].Target.String() < positive[right].Target.String()
	})
	return AllocationApportionment{
		form:        form,
		sourceMinor: sourceMinor,
		denominator: denominator,
		shares:      positive,
		unallocated: sourceMinor - (floorSum + leftover),
	}, nil
}

// AllocationFormRegistration 把选用的分法挂到一条分摊规则版本上。版本引用仍由适用表持有，这里不另造版本。
type AllocationFormRegistration struct {
	rule AllocationRuleVersionReference
	form AllocationForm
}

func NewAllocationFormRegistration(rule AllocationRuleVersionReference, form AllocationForm) (AllocationFormRegistration, error) {
	if !rule.valid() || form == AllocationFormInvalid || form.String() == "" {
		return AllocationFormRegistration{}, fmt.Errorf("%w: allocation form registration", ErrBlankValue)
	}
	return AllocationFormRegistration{rule: rule, form: form}, nil
}

func (registration AllocationFormRegistration) Rule() AllocationRuleVersionReference {
	return registration.rule
}

func (registration AllocationFormRegistration) Form() AllocationForm { return registration.form }

func (registration AllocationFormRegistration) SameRegistration(other AllocationFormRegistration) bool {
	return registration.rule == other.rule && registration.form == other.form && registration.form != AllocationFormInvalid
}
