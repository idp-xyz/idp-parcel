package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 订舱与取消两种原因的事实一半（票 demo-chain-gaps/28；ADR-0207 决定一与文末补记）：登记方指名一次订舱，外加订舱所属
// 委托里的一个或多个成员。订舱费的依据是那次订舱已接受的承运答复，取消费的依据是那次订舱尚未开始范围的取消。
//
// 订舱自身不列对象，它申请的是委托范围里的舱位，所以成员取委托的成员、由登记方指名（与实际履约由登记方指名段内成员
// 同一做法）；委托之外的对象答未受理。订舱属于委托，采购上下文因此总从委托取（ADR-0207 决定二末条）。
//
// 各册各读各的、不建新存储：订舱、承运答复与委托都是承运编排已在写的那几册，经各自的窄只读口读。

// ErrBookingAnswerMismatch 说明承运答复读口交回了另一次订舱或另一租户的答复：所答非所问是读口缺陷，重试不会变。
var ErrBookingAnswerMismatch = errors.New("transport fulfillment: booking answer view answered for another booking")

func (handler *RegisterChargeOccurrenceHandler) bookingFact(
	ctx context.Context,
	command RegisterChargeOccurrenceCommand,
) (chargeOccurrenceFact, RegisterChargeOccurrenceResult, bool, error) {
	if strings.TrimSpace(command.PickupAttemptSource) != "" || strings.TrimSpace(command.DeliveryAttempt) != "" ||
		strings.TrimSpace(command.Segment) != "" || len(command.Objects) == 0 {
		return chargeOccurrenceFact{}, chargeNotAccepted(ChargeRequestMalformed), true, nil
	}
	bookingRef, err := domain.NewBookingReference(command.Booking)
	if err != nil {
		return chargeOccurrenceFact{}, chargeNotAccepted(ChargeRequestMalformed), true, nil
	}
	members, ok := distinctMembers(command.Objects)
	if !ok {
		return chargeOccurrenceFact{}, chargeNotAccepted(ChargeRequestMalformed), true, nil
	}

	continuation := chargeOccurrenceContinuation(command.TenantID.String(), command.Reason.String(), bookingRef.String())
	key := ports.BookingKey{TenantID: command.TenantID, Booking: bookingRef}
	booking, found, err := handler.deps.Bookings.FindByKey(ctx, key)
	if err != nil {
		return chargeOccurrenceFact{}, chargeUndecided(FactSourceUnavailable, continuation), true, nil
	}
	if !found {
		return chargeOccurrenceFact{}, chargeNotAccepted(ChargeFactNotFound), true, nil
	}
	commission, found, err := handler.deps.Commissions.FindByKey(ctx, ports.TransportCommissionKey{
		TenantID:   command.TenantID,
		Commission: booking.Booking.Commission(),
	})
	if err != nil {
		return chargeOccurrenceFact{}, chargeUndecided(ProcurementSourceUnavailable, continuation), true, nil
	}
	if !found {
		// 订舱只在委托在册时成立，委托又不删：读不回是数据违约，同承运编排的 ErrBookingWithoutCommission。
		return chargeOccurrenceFact{}, RegisterChargeOccurrenceResult{}, false, fmt.Errorf("%w: booking %s, commission %s",
			ErrBookingWithoutCommission, bookingRef, booking.Booking.Commission())
	}
	answer, answered, err := handler.deps.BookingAnswers.FindByKey(ctx, key)
	if err != nil {
		return chargeOccurrenceFact{}, chargeUndecided(FactSourceUnavailable, continuation), true, nil
	}
	if answered && (answer.Key != key || answer.Acceptance.Booking() != bookingRef) {
		return chargeOccurrenceFact{}, RegisterChargeOccurrenceResult{}, false, fmt.Errorf("%w: asked %s", ErrBookingAnswerMismatch, bookingRef)
	}

	fact := domain.BookingChargeFact{
		Booking:    booking.Booking,
		Commission: commission.Commission,
		Answer:     answer.Acceptance,
		Answered:   answered,
	}
	grounds := domain.BookingOccurrenceGrounds
	formGate := domain.ChargeOccurrenceForBooking
	if command.Reason == domain.CancellationOccurrence {
		grounds = domain.CancellationOccurrenceGrounds
		formGate = domain.ChargeOccurrenceForCancellation
	}
	basis, scope, occurredAt, err := grounds(fact, members)
	if answered, detail := bookingFactRefusal(err); answered {
		return chargeOccurrenceFact{}, chargeNotAccepted(detail), true, nil
	}
	if err != nil {
		// 三件事实各按身份读回却彼此对不上（订舱指名的委托不是读回的那一份）是读口缺陷，不是请求有错。
		return chargeOccurrenceFact{}, RegisterChargeOccurrenceResult{}, false, fmt.Errorf("register charge occurrence: booking fact: %w", err)
	}
	return chargeOccurrenceFact{
		basis:         basis,
		scope:         scope,
		members:       members,
		occurredAt:    occurredAt,
		commission:    commission,
		hasCommission: true,
		form: func(spec domain.TransportChargeOccurrenceSpec) (domain.TransportChargeOccurrence, error) {
			return formGate(spec, fact)
		},
	}, RegisterChargeOccurrenceResult{}, false, nil
}

// bookingFactRefusal 把订舱与取消事实不成立的各领域哨兵译成未受理的细格。
func bookingFactRefusal(err error) (bool, ChargeOccurrenceDetail) {
	switch {
	case errors.Is(err, domain.ErrBookingNotAnswered):
		return true, BookingNotAnswered
	case errors.Is(err, domain.ErrBookingNotAccepted):
		return true, BookingNotAccepted
	case errors.Is(err, domain.ErrBookingNotCancelled):
		return true, BookingNotCancelled
	case errors.Is(err, domain.ErrCancelledAfterTransportStart):
		return true, CancelledAfterTransportStart
	case errors.Is(err, domain.ErrNotACommissionMember):
		return true, ObjectNotInCommission
	default:
		return false, ChargeOccurrenceDetailNone
	}
}
