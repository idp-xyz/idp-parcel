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
//
// 费用项目是这一行的第三格，也是租户取值（ADR-0208）：发生项已形成那一路要从这里取，发生项与协议上都没有它。
// 它可以缺席——缺席的行对那一路答未配置，而 ADR-0164 那条由调用方交齐三件引用的触发面本来就不读它。
type BuyEvaluationTriggerRegistration struct {
	reason  OccurrenceReasonReference
	moment  BuyEvaluationTriggerMoment
	feeItem FeeItemReference
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

// WithFeeItem 给这一行登上费用项目。一个原因只映一个费用项目、不分协议（ADR-0208 越权风险点 1）。
func (registration BuyEvaluationTriggerRegistration) WithFeeItem(feeItem FeeItemReference) (BuyEvaluationTriggerRegistration, error) {
	if !registration.reason.valid() || !registration.moment.triggers() || !feeItem.valid() {
		return BuyEvaluationTriggerRegistration{}, fmt.Errorf("%w: buy evaluation trigger fee item", ErrBlankValue)
	}
	registration.feeItem = feeItem
	return registration, nil
}

// FeeItem 交回这一行登的费用项目；没登时第二个返回值为假，那一路据此答未配置，不拿原因或别的串顶上。
func (registration BuyEvaluationTriggerRegistration) FeeItem() (FeeItemReference, bool) {
	return registration.feeItem, registration.feeItem.valid()
}

// SameRegistration 三格都比：同原因同时点而费用项目不同（含一边没登）是另一行内容，不是重放。
func (registration BuyEvaluationTriggerRegistration) SameRegistration(other BuyEvaluationTriggerRegistration) bool {
	return registration.reason == other.reason && registration.moment == other.moment &&
		registration.feeItem == other.feeItem && registration.moment.triggers()
}
