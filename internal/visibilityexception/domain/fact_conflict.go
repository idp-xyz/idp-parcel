package domain

import (
	"errors"
	"sort"
	"time"
)

var (
	ErrInvalidConflictInput   = errors.New("visibility exception: invalid conflict input")
	ErrInvalidExceptionSignal = errors.New("visibility exception: invalid exception signal")
)

// ConflictResolutionBasis 是版本化投影判断的裁决依据封闭集合（CONTEXT 硬句列举的
// 可用维度）。刻意没有「来源排名」与「最后消息」两格——全局来源排名和最后写入覆盖
// 被硬句明禁，封闭集合让它们在类型上就进不来。
type ConflictResolutionBasis uint8

const (
	ConflictResolutionBasisInvalid ConflictResolutionBasis = iota
	ResolvedByBusinessTime
	ResolvedByCausalOrder
	ResolvedByAuthorityScope
)

func (basis ConflictResolutionBasis) String() string {
	switch basis {
	case ResolvedByBusinessTime:
		return "BUSINESS_TIME"
	case ResolvedByCausalOrder:
		return "CAUSAL_ORDER"
	case ResolvedByAuthorityScope:
		return "AUTHORITY_SCOPE"
	default:
		return ""
	}
}

// ConflictJudgment 是对一组冲突事实的版本化裁决结果：可排序时给出全序与依据，
// 无法裁决时保留全部事实并要求形成异常信号。两种走向都不使任何源事实失效——
// 判断里只有引用，源事实的有效性归其所有者。
type ConflictJudgment struct {
	resolved bool
	basis    ConflictResolutionBasis
	ordered  []AcceptedSourceFact
	retained []AcceptedSourceFact
}

// ResolveByBusinessTime 按业务发生时间对同一包裹的冲突事实排全序（CONTEXT 允许的
// 裁决维度之一）。任意两份事实同刻即无法裁决——并列时间上不存在「谁更新」，硬凑
// 顺序与最后消息覆盖没有区别；其余维度（因果顺序、权威范围）各有自己的裁决函数，
// 不在这里混判。少于两份事实构不成冲突。
func ResolveByBusinessTime(facts []AcceptedSourceFact) (ConflictJudgment, error) {
	if len(facts) < 2 {
		return ConflictJudgment{}, ErrInvalidConflictInput
	}
	parcel := facts[0].parcel
	for _, fact := range facts {
		if !fact.source.valid() || fact.parcel != parcel {
			return ConflictJudgment{}, ErrInvalidConflictInput
		}
	}

	ordered := append([]AcceptedSourceFact(nil), facts...)
	sort.SliceStable(ordered, func(left, right int) bool {
		return ordered[left].occurredAt.Before(ordered[right].occurredAt)
	})
	for index := 1; index < len(ordered); index++ {
		if ordered[index].occurredAt.Equal(ordered[index-1].occurredAt) {
			return ConflictJudgment{
				retained: append([]AcceptedSourceFact(nil), facts...),
			}, nil
		}
	}
	return ConflictJudgment{
		resolved: true,
		basis:    ResolvedByBusinessTime,
		ordered:  ordered,
	}, nil
}

func (judgment ConflictJudgment) Resolved() bool {
	return judgment.resolved
}

// Basis 只在裁决成立时给出。
func (judgment ConflictJudgment) Basis() (ConflictResolutionBasis, bool) {
	return judgment.basis, judgment.resolved
}

// Ordered 给出裁决后的全序（最早在前）。只在裁决成立时非空。
func (judgment ConflictJudgment) Ordered() []AcceptedSourceFact {
	return append([]AcceptedSourceFact(nil), judgment.ordered...)
}

// Retained 给出无法裁决时保留的全部事实——一份都不能少（「必须保留各项事实及冲突
// 关系」），投影据此保持信息待确认。
func (judgment ConflictJudgment) Retained() []AcceptedSourceFact {
	return append([]AcceptedSourceFact(nil), judgment.retained...)
}

// ExceptionSignalKindReference 指名异常信号的种类。信号目录属实例参数，这里是开放
// 引用。
type ExceptionSignalKindReference struct{ requiredValue }

func NewExceptionSignalKindReference(value string) (ExceptionSignalKindReference, error) {
	required, err := newRequiredValue("exception signal kind reference", value)
	return ExceptionSignalKindReference{required}, err
}

// ExceptionSignal 是一份异常信号：无法裁决的冲突、可见性缺口或其他需要关注的情况。
// 它是信号不是案件——分诊、建案与处置各有后续对象；它也不改变任何源事实或投影。
type ExceptionSignal struct {
	kind     ExceptionSignalKindReference
	parcel   TrackedParcelReference
	facts    []AcceptedSourceFact
	raisedAt time.Time
}

// RaiseConflictSignal 依据一份未裁决的冲突判断形成异常信号（CONTEXT：「冲突仍无法
// 裁决时……投影保持信息待确认并形成适用异常信号」）。已裁决的判断形不成冲突信号——
// 那不是异常；信号携带全部保留事实的引用，处置者不用回头再拼现场。
func RaiseConflictSignal(
	kind ExceptionSignalKindReference,
	judgment ConflictJudgment,
	raisedAt time.Time,
) (ExceptionSignal, error) {
	if !kind.valid() || raisedAt.IsZero() {
		return ExceptionSignal{}, ErrInvalidExceptionSignal
	}
	if judgment.resolved || len(judgment.retained) == 0 {
		return ExceptionSignal{}, ErrInvalidExceptionSignal
	}
	return ExceptionSignal{
		kind:     kind,
		parcel:   judgment.retained[0].parcel,
		facts:    append([]AcceptedSourceFact(nil), judgment.retained...),
		raisedAt: raisedAt.UTC(),
	}, nil
}

func (signal ExceptionSignal) Kind() ExceptionSignalKindReference {
	return signal.kind
}

func (signal ExceptionSignal) Parcel() TrackedParcelReference {
	return signal.parcel
}

func (signal ExceptionSignal) Facts() []AcceptedSourceFact {
	return append([]AcceptedSourceFact(nil), signal.facts...)
}

func (signal ExceptionSignal) RaisedAt() time.Time {
	return signal.raisedAt
}
