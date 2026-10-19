package application

import (
	"context"
	"errors"
	"strings"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 失败尝试费发生项的事实一半（ADR-0098；ADR-0207 决定一）：登记方指名一次揽收尝试（按尝试来源键）或一次派送尝试
// （按尝试身份），外加那一次到场里的一个对象。
//
// 两侧各读各的册，不建新存储：揽收尝试经 FailedAttemptSource，派送尝试经交付生效已在用的 DeliveryAttemptView。
// 一次只登一个对象——一次到场里各对象的成败各自成立（CONTEXT「任一对象的成功、失败或取消不能被压缩成其他对象的
// 状态」），同一次到场里两个对象各失败一次，就是两项。

func (handler *RegisterChargeOccurrenceHandler) failedAttemptFact(
	ctx context.Context,
	command RegisterChargeOccurrenceCommand,
) (chargeOccurrenceFact, RegisterChargeOccurrenceResult, bool) {
	pickup := strings.TrimSpace(command.PickupAttemptSource) != ""
	delivery := strings.TrimSpace(command.DeliveryAttempt) != ""
	if pickup == delivery || strings.TrimSpace(command.Segment) != "" || strings.TrimSpace(command.Booking) != "" ||
		len(command.Objects) != 1 {
		return chargeOccurrenceFact{}, chargeNotAccepted(ChargeRequestMalformed), true
	}
	object, err := domain.NewCarriedObjectReference(command.Objects[0])
	if err != nil {
		return chargeOccurrenceFact{}, chargeNotAccepted(ChargeRequestMalformed), true
	}
	if pickup {
		return handler.failedPickupFact(ctx, command, object)
	}
	return handler.failedDeliveryFact(ctx, command, object)
}

func (handler *RegisterChargeOccurrenceHandler) failedPickupFact(
	ctx context.Context,
	command RegisterChargeOccurrenceCommand,
	object domain.CarriedObjectReference,
) (chargeOccurrenceFact, RegisterChargeOccurrenceResult, bool) {
	record, found, err := handler.deps.PickupAttempts.FindByKey(ctx, ports.PickupAttemptKey{
		TenantID: command.TenantID,
		SourceID: command.PickupAttemptSource,
	})
	if err != nil {
		return chargeOccurrenceFact{}, chargeUndecided(FactSourceUnavailable,
			chargeOccurrenceContinuation(command.TenantID.String(), command.PickupAttemptSource, object.String())), true
	}
	if !found {
		return chargeOccurrenceFact{}, chargeNotAccepted(ChargeFactNotFound), true
	}
	// 「这个对象不在这次尝试里」与「读不回来」分开：前者回去查是不是问错了对象，后者重试同一次调用。
	result, present := attemptResultFor(record, object)
	if !present {
		return chargeOccurrenceFact{}, chargeNotAccepted(ChargeFactNotFound), true
	}
	basis, err := domain.FailedPickupAttemptBasis(result)
	if errors.Is(err, domain.ErrNotAFailedAttempt) {
		return chargeOccurrenceFact{}, chargeNotAccepted(AttemptDidNotFail), true
	}
	if err != nil {
		return chargeOccurrenceFact{}, chargeNotAccepted(ChargeRequestMalformed), true
	}
	scope, err := domain.AttemptOccurrenceScope(result.Attempt())
	if err != nil {
		return chargeOccurrenceFact{}, chargeNotAccepted(ChargeRequestMalformed), true
	}
	return chargeOccurrenceFact{
		basis:      basis,
		scope:      scope,
		members:    []domain.CarriedObjectReference{object},
		occurredAt: result.OccurredAt(),
		form: func(spec domain.TransportChargeOccurrenceSpec) (domain.TransportChargeOccurrence, error) {
			return domain.ChargeOccurrenceForFailedAttempt(spec, result)
		},
	}, RegisterChargeOccurrenceResult{}, false
}

func (handler *RegisterChargeOccurrenceHandler) failedDeliveryFact(
	ctx context.Context,
	command RegisterChargeOccurrenceCommand,
	object domain.CarriedObjectReference,
) (chargeOccurrenceFact, RegisterChargeOccurrenceResult, bool) {
	attempt, err := domain.NewAttemptReference(command.DeliveryAttempt)
	if err != nil {
		return chargeOccurrenceFact{}, chargeNotAccepted(ChargeRequestMalformed), true
	}
	_, result, found, err := handler.deps.DeliveryAttempts.LoadDeliveryResult(ctx, command.TenantID, attempt, object)
	if err != nil {
		return chargeOccurrenceFact{}, chargeUndecided(FactSourceUnavailable,
			chargeOccurrenceContinuation(command.TenantID.String(), attempt.String(), object.String())), true
	}
	if !found {
		return chargeOccurrenceFact{}, chargeNotAccepted(ChargeFactNotFound), true
	}
	basis, err := domain.FailedDeliveryAttemptBasis(result)
	if errors.Is(err, domain.ErrNotAFailedAttempt) {
		return chargeOccurrenceFact{}, chargeNotAccepted(AttemptDidNotFail), true
	}
	if err != nil {
		return chargeOccurrenceFact{}, chargeNotAccepted(ChargeRequestMalformed), true
	}
	scope, err := domain.AttemptOccurrenceScope(result.Attempt())
	if err != nil {
		return chargeOccurrenceFact{}, chargeNotAccepted(ChargeRequestMalformed), true
	}
	return chargeOccurrenceFact{
		basis:      basis,
		scope:      scope,
		members:    []domain.CarriedObjectReference{object},
		occurredAt: result.OccurredAt(),
		form: func(spec domain.TransportChargeOccurrenceSpec) (domain.TransportChargeOccurrence, error) {
			return domain.ChargeOccurrenceForDeliveryAttempt(spec, result)
		},
	}, RegisterChargeOccurrenceResult{}, false
}

func attemptResultFor(
	record ports.PickupAttemptRecord,
	object domain.CarriedObjectReference,
) (domain.AttemptObjectResult, bool) {
	for _, result := range record.Results {
		if result.Object() == object {
			return result, true
		}
	}
	return domain.AttemptObjectResult{}, false
}
