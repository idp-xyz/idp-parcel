package domain

import (
	"sort"
	"strings"
	"time"
)

// MemberMeasuredWeight 是提交版本上某一件成员在成版时的实测毛重。数值、单位、称重量方的来源身份与发生时间成套，
// 拆开就不再是那一次称重。值类型没有回写入口。
type MemberMeasuredWeight struct {
	parcel     DeclaredParcelReference
	gross      string
	unit       string
	source     string
	measuredAt time.Time
}

// NewMemberMeasuredWeight 收下一条实测毛重。空白与零时刻在入口就拒：成版与读回还要再验一遍，免得绕过构造器的写入看起来合法。
func NewMemberMeasuredWeight(
	parcel DeclaredParcelReference,
	grossWeight, unit, source string,
	measuredAt time.Time,
) (MemberMeasuredWeight, error) {
	weight := MemberMeasuredWeight{
		parcel: parcel, gross: grossWeight, unit: unit, source: source, measuredAt: measuredAt.UTC(),
	}
	if !weight.valid() {
		return MemberMeasuredWeight{}, ErrInvalidSubmissionVersion
	}
	return weight, nil
}

func (weight MemberMeasuredWeight) Parcel() DeclaredParcelReference { return weight.parcel }

func (weight MemberMeasuredWeight) GrossWeight() string { return weight.gross }

func (weight MemberMeasuredWeight) GrossWeightUnit() string { return weight.unit }

func (weight MemberMeasuredWeight) MeasurementSource() string { return weight.source }

func (weight MemberMeasuredWeight) MeasuredAt() time.Time { return weight.measuredAt }

func (weight MemberMeasuredWeight) valid() bool {
	return weight.parcel.valid() &&
		strings.TrimSpace(weight.gross) != "" &&
		strings.TrimSpace(weight.unit) != "" &&
		strings.TrimSpace(weight.source) != "" &&
		!weight.measuredAt.IsZero()
}

// orderedMemberWeights 把一组毛重收成稳定顺序：按成员引用排序，使写入与读回不必靠调用方预先排好。
// 指向非成员、同一成员两条、空白字段或零时刻都不是一份可以固定的快照。
func orderedMemberWeights(members []DeclaredParcelReference, weights []MemberMeasuredWeight) ([]MemberMeasuredWeight, error) {
	allowed := make(map[DeclaredParcelReference]bool, len(members))
	for _, member := range members {
		allowed[member] = true
	}
	ordered := append([]MemberMeasuredWeight(nil), weights...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].parcel.String() < ordered[j].parcel.String()
	})
	seen := make(map[DeclaredParcelReference]bool, len(ordered))
	for _, weight := range ordered {
		if !weight.valid() || !allowed[weight.parcel] || seen[weight.parcel] {
			return nil, ErrInvalidSubmissionVersion
		}
		seen[weight.parcel] = true
	}
	return ordered, nil
}

// MemberMeasurementNow 是「此刻的实测与成版时记下的那一条是否还是同一条」的封闭取值。比较只看有没有、以及来源身份，不挑一条。
type MemberMeasurementNow uint8

const (
	MemberMeasurementNowInvalid MemberMeasurementNow = iota
	MemberMeasurementSame
	MemberMeasurementChanged
	MemberMeasurementNoneNow
	MemberMeasurementUndetermined
	MemberMeasurementUnavailable
)

func (state MemberMeasurementNow) String() string {
	switch state {
	case MemberMeasurementSame:
		return "SAME"
	case MemberMeasurementChanged:
		return "CHANGED"
	case MemberMeasurementNoneNow:
		return "NONE_NOW"
	case MemberMeasurementUndetermined:
		return "UNDETERMINED"
	case MemberMeasurementUnavailable:
		return "UNAVAILABLE"
	case MemberMeasurementNowInvalid:
		return ""
	default:
		return ""
	}
}

// MemberMeasurementNowFacts 是一次比较的输入。Failed 是读口报错，Undetermined 是定不下，Found 是此刻有一条。
// 三格互斥由调用方保证；这里按读不动、定不下、有、没有的次序取，免得两种失败被说成「没有」。
type MemberMeasurementNowFacts struct {
	Fixed        bool
	FixedSource  string
	Failed       bool
	Undetermined bool
	Found        bool
	Source       string
}

// CompareMemberMeasurementNow 比较成版时的那一条（或无）与此刻的答案。listed 为假是两边都没有，回答里不列这一件。
func CompareMemberMeasurementNow(facts MemberMeasurementNowFacts) (MemberMeasurementNow, bool) {
	if facts.Failed {
		return MemberMeasurementUnavailable, true
	}
	if facts.Undetermined {
		return MemberMeasurementUndetermined, true
	}
	if facts.Found {
		if facts.Fixed && facts.Source == facts.FixedSource {
			return MemberMeasurementSame, true
		}
		return MemberMeasurementChanged, true
	}
	if facts.Fixed {
		return MemberMeasurementNoneNow, true
	}
	return MemberMeasurementNowInvalid, false
}
