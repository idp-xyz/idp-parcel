package domain

import (
	"errors"
	"fmt"
)

var (
	// ErrInvalidBuyEvaluationTrigger 说明触发时点不是这套内置触发面认得的值。
	ErrInvalidBuyEvaluationTrigger = errors.New("settlement accounting: invalid buy evaluation trigger")
)

// BuyEvaluationTriggerMoment 是内置触发时点。只有发生项形成这一格。
// 结算周期批量不在这里。路由择优的 BUY 评价也不走这条请求。
type BuyEvaluationTriggerMoment uint8

const (
	BuyEvaluationTriggerMomentInvalid BuyEvaluationTriggerMoment = iota
	BuyEvaluationTriggerOnOccurrenceFormed
)

func (moment BuyEvaluationTriggerMoment) String() string {
	switch moment {
	case BuyEvaluationTriggerOnOccurrenceFormed:
		return "OCCURRENCE_FORMED"
	default:
		return ""
	}
}

func (moment BuyEvaluationTriggerMoment) triggers() bool {
	return moment == BuyEvaluationTriggerOnOccurrenceFormed
}

// BuyEvaluationTriggerMomentFromName 只认封闭的那一个时点。词表外拒，不夹成发生项形成。
func BuyEvaluationTriggerMomentFromName(name string) (BuyEvaluationTriggerMoment, error) {
	switch name {
	case "OCCURRENCE_FORMED":
		return BuyEvaluationTriggerOnOccurrenceFormed, nil
	default:
		return BuyEvaluationTriggerMomentInvalid, fmt.Errorf("%w: moment", ErrInvalidBuyEvaluationTrigger)
	}
}

// BuyEvaluationTriggerRegistration 把一个发生项原因挂到内置时点上。原因是租户的词，不在产品里预列。
type BuyEvaluationTriggerRegistration struct {
	reason OccurrenceReasonReference
	moment BuyEvaluationTriggerMoment
}

func NewBuyEvaluationTriggerRegistration(
	reason OccurrenceReasonReference,
	moment BuyEvaluationTriggerMoment,
) (BuyEvaluationTriggerRegistration, error) {
	if !reason.valid() || !moment.triggers() {
		return BuyEvaluationTriggerRegistration{}, fmt.Errorf("%w: buy evaluation trigger", ErrBlankValue)
	}
	return BuyEvaluationTriggerRegistration{reason: reason, moment: moment}, nil
}

func (registration BuyEvaluationTriggerRegistration) Reason() OccurrenceReasonReference {
	return registration.reason
}

func (registration BuyEvaluationTriggerRegistration) Moment() BuyEvaluationTriggerMoment {
	return registration.moment
}

func (registration BuyEvaluationTriggerRegistration) SameRegistration(other BuyEvaluationTriggerRegistration) bool {
	return registration.reason == other.reason && registration.moment == other.moment && registration.moment.triggers()
}
