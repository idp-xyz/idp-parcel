package domain

import (
	"errors"
	"fmt"
)

// ErrInvalidSellEvaluationTrigger 说明触发时点不是 SELL 这套内置触发面认得的值。
var ErrInvalidSellEvaluationTrigger = errors.New("settlement accounting: invalid sell evaluation trigger")

// SellEvaluationTriggerMoment 是 SELL 评价的内置触发时点。只有发生项形成这一格。
// 它与 BUY 评价请求不是同一条身份。
type SellEvaluationTriggerMoment uint8

const (
	SellEvaluationTriggerMomentInvalid SellEvaluationTriggerMoment = iota
	SellEvaluationTriggerOnOccurrenceFormed
)

func (moment SellEvaluationTriggerMoment) String() string {
	switch moment {
	case SellEvaluationTriggerOnOccurrenceFormed:
		return "OCCURRENCE_FORMED"
	default:
		return ""
	}
}

func (moment SellEvaluationTriggerMoment) triggers() bool {
	return moment == SellEvaluationTriggerOnOccurrenceFormed
}

func SellEvaluationTriggerMomentFromName(name string) (SellEvaluationTriggerMoment, error) {
	switch name {
	case "OCCURRENCE_FORMED":
		return SellEvaluationTriggerOnOccurrenceFormed, nil
	default:
		return SellEvaluationTriggerMomentInvalid, fmt.Errorf("%w: moment", ErrInvalidSellEvaluationTrigger)
	}
}

// SellEvaluationTriggerRegistration 把一个发生项原因挂到 SELL 的内置时点上。
// 原因是租户的词。登记在 BUY 触发册上的原因不会让这里发起。
type SellEvaluationTriggerRegistration struct {
	reason OccurrenceReasonReference
	moment SellEvaluationTriggerMoment
}

func NewSellEvaluationTriggerRegistration(
	reason OccurrenceReasonReference,
	moment SellEvaluationTriggerMoment,
) (SellEvaluationTriggerRegistration, error) {
	if !reason.valid() || !moment.triggers() {
		return SellEvaluationTriggerRegistration{}, fmt.Errorf("%w: sell evaluation trigger", ErrBlankValue)
	}
	return SellEvaluationTriggerRegistration{reason: reason, moment: moment}, nil
}

func (registration SellEvaluationTriggerRegistration) Reason() OccurrenceReasonReference {
	return registration.reason
}

func (registration SellEvaluationTriggerRegistration) Moment() SellEvaluationTriggerMoment {
	return registration.moment
}

func (registration SellEvaluationTriggerRegistration) SameRegistration(other SellEvaluationTriggerRegistration) bool {
	return registration.reason == other.reason && registration.moment == other.moment && registration.moment.triggers()
}
