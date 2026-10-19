package domain

import (
	"errors"
	"time"
)

// 实际履约发生项随参与结束自动形成时，领域给出的两件判断（ADR-0218）：这次控制事实结束的是哪一条参与，发生项属于
// 哪一趟旅程。登记本身仍走既有登记用例，这里不另立第二道形成门。

// ErrAmbiguousJourney：对象在参与入场那一刻落在两条同一时刻开始的替代 / 退运旅程里。旅程是发生项的一维（原旅程与
// 新旅程各自关联自己的发生项，CONTEXT「运输收费发生项与成本来源」），挑一条就是替旅程册裁决对象当时走的是哪一趟。
// 重试不会让它变，不挑，响亮报出。
var ErrAmbiguousJourney = errors.New("transport fulfillment: the object is in several alternate journeys started at the same instant")

// derivedJourneyPrefix 是按正式包裹身份派生旅程引用时的类别段。分隔用斜杠，与发生项依据串同一种拼法。
const derivedJourneyPrefix = "JRN"

// ActualFulfillmentJourney 定自动形成的实际履约发生项属于哪一趟旅程（ADR-0218，产品策略）。
//
// 先看对象在替代 / 退运旅程册上有没有一趟在这条参与入场时已经开始：有就取开始得最晚的那一趟——参与是在那一趟里进的
// 段。没有就按对象的正式包裹身份确定性派生 `JRN/<对象>`：本上下文没有原旅程的权威来源（ADR-0207 决定四），派生而
// 不签发，人工登记写同一个拼法就落回同一项（答`已存在`），重放也落回同一项。
//
// 按入场时刻而不按此刻判：退运在对象还在原段里时就可能已经启动，原段那一程属于原旅程（CONTEXT「退运……不得通过倒退
// 原实际履约段……表达」）；信封晚到或重投时按此刻判，会把原段的成本挂到新旅程上。同一刻开始的算入场时已开始。
//
// alternates 由调用方按成员读来；这里仍按成员筛一遍，传进来的不是这个对象的旅程也不会被误取。
func ActualFulfillmentJourney(
	object CarriedObjectReference,
	enteredAt time.Time,
	alternates []AlternateJourney,
) (JourneyReference, error) {
	if !object.valid() || AtRegistryPrecision(enteredAt).IsZero() {
		return JourneyReference{}, ErrInvalidChargeOccurrence
	}
	var current AlternateJourney
	found, tied := false, false
	for _, journey := range alternates {
		if !journey.covers(object) || journey.startedAt.After(enteredAt) {
			continue
		}
		switch {
		case !found || journey.startedAt.After(current.startedAt):
			current, found, tied = journey, true, false
		case journey.startedAt.Equal(current.startedAt) && journey.journey != current.journey:
			tied = true
		}
	}
	if tied {
		return JourneyReference{}, ErrAmbiguousJourney
	}
	if found {
		return current.journey, nil
	}
	return NewJourneyReference(derivedJourneyPrefix + "/" + object.String())
}

func (journey AlternateJourney) covers(object CarriedObjectReference) bool {
	for _, member := range journey.members {
		if member == object {
			return true
		}
	}
	return false
}

// EffectiveDeliveryEndBasis 是有效交付结束参与时记下的终点依据：那一版交付结果。结束参与与事后找「这次交付结束了哪一条
// 参与」用的是同一串，由这里一处给出。
func EffectiveDeliveryEndBasis(version DeliveryResultVersion) (ParticipationBasisReference, error) {
	if !version.valid() {
		return ParticipationBasisReference{}, ErrInvalidFulfillmentSegment
	}
	return NewParticipationBasisReference("EFFECTIVE-DELIVERY/" + version.String())
}

// ParticipationEndedBy 答这个对象在本段的当前参与是不是恰由指名的那件控制事实结束的：链尾、不是失效版本、终点的种类与
// 依据都对得上。替代版本继承前版的终点（rederive），所以按链尾判，更正入场不会让「被谁结束」读丢。
func (segment ActualFulfillmentSegment) ParticipationEndedBy(
	object CarriedObjectReference,
	kind ParticipationEndKind,
	basis ParticipationBasisReference,
) (FulfillmentParticipation, bool) {
	current, present := segment.ParticipationFor(object)
	if !present || current.Voided() {
		return FulfillmentParticipation{}, false
	}
	endKind, endBasis, _, ended := current.End()
	if !ended || endKind != kind || endBasis != basis {
		return FulfillmentParticipation{}, false
	}
	return current, true
}
