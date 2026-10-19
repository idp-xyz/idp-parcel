package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件钉票 tf-face-algebra/14 的判零一半：tf-face-algebra/01 之外其余各口的时刻非零门按登记册精度判
// （规则见 AtRegistryPrecision）。外部跟踪事实那几道门在包内测试 external_tracking_registry_zero_test.go，
// 那里的夹具只在包内可见。夹具全为合成值（S 级）。

// beforeRegistryZero 是登记册上不为零点、又早于零点的时刻：转换门的「不早于前一时刻」那道门以它为下界时放行，
// 只剩非零门在判。
var beforeRegistryZero = zeroInstant.Add(-time.Nanosecond)

// pinRegistryZeroGate 钉一道时刻非零门：恰为零点与零点之后不足一微秒的各值都答 want，零点后一微秒照常。
func pinRegistryZeroGate(t *testing.T, want error, gate func(at time.Time) error) {
	t.Helper()
	cases := map[string]time.Time{"恰为零点": zeroInstant}
	for name, at := range subMicroAfterZero {
		cases[name] = at
	}
	for name, at := range cases {
		if err := gate(at); !errors.Is(err, want) {
			t.Errorf("%s：err = %v，期望 %v", name, err, want)
		}
	}
	if err := gate(zeroInstant.Add(time.Microsecond)); err != nil {
		t.Errorf("零点后 1µs：err = %v，期望照常", err)
	}
}

// Covers: 票 tf-face-algebra/14 判零——场外揽收的发生时刻与更正时刻。
func TestOffsitePickupGatesJudgeZeroOnTheRegistry(t *testing.T) {
	t.Run("FormOffsitePickup 发生时刻", func(t *testing.T) {
		pinRegistryZeroGate(t, domain.ErrInvalidOffsitePickup, func(at time.Time) error {
			spec := pickupSpec(t)
			spec.OccurredAt = at
			_, err := domain.FormOffsitePickup(spec)
			return err
		})
	})
	t.Run("Correct 更正时刻", func(t *testing.T) {
		pickup := formedPickup(t, "SYN-PARCEL-1", "SYN-ATTEMPT-1")
		pinRegistryZeroGate(t, domain.ErrInvalidOffsitePickup, func(at time.Time) error {
			_, err := pickup.Correct(pickupCorrection(t, "SYN-PICKUP-V2", at))
			return err
		})
	})
	t.Run("Correct 更正后的发生时刻", func(t *testing.T) {
		pickup := formedPickup(t, "SYN-PARCEL-1", "SYN-ATTEMPT-1")
		pinRegistryZeroGate(t, domain.ErrInvalidOffsitePickup, func(at time.Time) error {
			correction := pickupCorrection(t, "SYN-PICKUP-V2", pickedUpAt)
			correction.OccurredAt = at
			_, err := pickup.Correct(correction)
			return err
		})
	})
}

// Covers: 票 tf-face-algebra/14 判零——履约段的离场时刻与关段时刻。入场时刻取自控制事实，各控制事实的构造门
// 已先判过，这里不单钉。
func TestFulfillmentSegmentGatesJudgeZeroOnTheRegistry(t *testing.T) {
	spec := pickupSpec(t)
	spec.OccurredAt = beforeRegistryZero
	pickup, err := domain.FormOffsitePickup(spec)
	if err != nil {
		t.Fatalf("形成揽收：%v", err)
	}
	segment, err := domain.EstablishSegmentWithPickup(
		mustValue(t, domain.NewFulfillmentSegmentReference, "SYN-SEGMENT-1"), pickup, domain.PlannedSegmentReference{})
	if err != nil {
		t.Fatalf("成段：%v", err)
	}
	basis := mustValue(t, domain.NewParticipationBasisReference, "SYN-TERMINATION-1")

	t.Run("离场时刻", func(t *testing.T) {
		pinRegistryZeroGate(t, domain.ErrInvalidFulfillmentSegment, func(at time.Time) error {
			_, err := segment.EndParticipationWithTermination(pickup.Object(), basis, at)
			return err
		})
	})
	t.Run("关段时刻", func(t *testing.T) {
		ended, err := segment.EndParticipationWithTermination(pickup.Object(), basis, beforeRegistryZero)
		if err != nil {
			t.Fatalf("结束参与：%v", err)
		}
		pinRegistryZeroGate(t, domain.ErrInvalidFulfillmentSegment, func(at time.Time) error {
			_, err := ended.CloseSegment(at)
			return err
		})
	})
}

// Covers: 票 tf-face-algebra/14 判零——承运商判断的依据业务时间、段成立时刻与各版形成时间。
func TestActualCarrierJudgmentGatesJudgeZeroOnTheRegistry(t *testing.T) {
	subject := mustCarrierSubject(t, domain.ExternalCarrierParty, "SYN-CARRIER-1")
	t.Run("NewCarrierEvidence 业务时间", func(t *testing.T) {
		pinRegistryZeroGate(t, domain.ErrInvalidCarrierEvidence, func(at time.Time) error {
			_, err := domain.NewCarrierEvidence(domain.CarrierEvidenceSpec{
				Source:     domain.CarrierDirectPickupScan,
				Reference:  mustRef(t, domain.NewCarrierEvidenceReference, "SYN-EVIDENCE-1"),
				OccurredAt: at,
				Subject:    subject,
			})
			return err
		})
	})
	t.Run("OpenActualCarrierJudgment 段成立时刻", func(t *testing.T) {
		pinRegistryZeroGate(t, domain.ErrInvalidActualCarrierJudgment, func(at time.Time) error {
			spec := openJudgmentSpec(t)
			spec.EstablishedAt = at
			_, err := domain.OpenActualCarrierJudgment(spec)
			return err
		})
	})
	t.Run("OpenActualCarrierJudgment 形成时间", func(t *testing.T) {
		pinRegistryZeroGate(t, domain.ErrInvalidActualCarrierJudgment, func(at time.Time) error {
			spec := openJudgmentSpec(t)
			spec.FormedAt = at
			_, err := domain.OpenActualCarrierJudgment(spec)
			return err
		})
	})

	judgment := mustOpenJudgment(t)
	t.Run("Consider 形成时间", func(t *testing.T) {
		evidence := registeredEvidence(t, domain.CarrierDirectPickupScan, "SYN-EVIDENCE-1", judgmentSegmentEstablishedAt, subject)
		pinRegistryZeroGate(t, domain.ErrInvalidActualCarrierJudgment, func(at time.Time) error {
			_, err := judgment.Consider(evidence, at)
			return err
		})
	})

	evidence := unregisteredEvidence(t, domain.CarrierDirectPickupScan, "SYN-EVIDENCE-2", judgmentSegmentEstablishedAt, "SYN-CARRIER-NAME")
	considered, err := judgment.Consider(evidence, judgmentFormedAt)
	if err != nil {
		t.Fatalf("收依据：%v", err)
	}
	t.Run("RecogniseCarrierIdentity 形成时间", func(t *testing.T) {
		pinRegistryZeroGate(t, domain.ErrInvalidActualCarrierJudgment, func(at time.Time) error {
			_, err := considered.RecogniseCarrierIdentity(evidence.Reference(), subject, at)
			return err
		})
	})
	t.Run("WithdrawEvidence 形成时间", func(t *testing.T) {
		pinRegistryZeroGate(t, domain.ErrInvalidActualCarrierJudgment, func(at time.Time) error {
			_, err := considered.WithdrawEvidence(evidence.Reference(), at)
			return err
		})
	})
}

// Covers: 票 tf-face-algebra/14 判零——承运商首次有效收寄的业务发生时间与判断形成时间（三道构造门共用的形状判据）。
func TestCarrierFirstEffectivePickupGatesJudgeZeroOnTheRegistry(t *testing.T) {
	t.Run("已形成 业务发生时间", func(t *testing.T) {
		pinRegistryZeroGate(t, domain.ErrInvalidCarrierFirstEffectivePickup, func(at time.Time) error {
			spec := formedCarrierPickupSpec(t)
			spec.OccurredAt = at
			_, err := domain.FormCarrierFirstEffectivePickup(spec)
			return err
		})
	})
	t.Run("已形成 判断形成时间", func(t *testing.T) {
		pinRegistryZeroGate(t, domain.ErrInvalidCarrierFirstEffectivePickup, func(at time.Time) error {
			spec := formedCarrierPickupSpec(t)
			spec.JudgedAt = at
			_, err := domain.FormCarrierFirstEffectivePickup(spec)
			return err
		})
	})
	t.Run("待确认 判断形成时间", func(t *testing.T) {
		pinRegistryZeroGate(t, domain.ErrInvalidCarrierFirstEffectivePickup, func(at time.Time) error {
			_, err := domain.HoldCarrierFirstEffectivePickupPending(pendingCarrierPickupSpec(t, "SYN-CARRIER-NAME", at))
			return err
		})
	})
}

// Covers: 票 tf-face-algebra/14 判零——揽派任务的时间窗两端、建立时刻、改约后的时间窗与关闭时刻。改约时刻不入册，不在此列。
func TestDispatchTaskGatesJudgeZeroOnTheRegistry(t *testing.T) {
	underZero := zeroInstant.Add(-time.Microsecond)
	t.Run("OpenDispatchTask 时间窗起点", func(t *testing.T) {
		pinRegistryZeroGate(t, domain.ErrInvalidDispatchTask, func(at time.Time) error {
			spec := dispatchTaskSpec(t)
			spec.WindowFrom = at
			_, err := domain.OpenDispatchTask(spec)
			return err
		})
	})
	t.Run("OpenDispatchTask 时间窗终点", func(t *testing.T) {
		pinRegistryZeroGate(t, domain.ErrInvalidDispatchTask, func(at time.Time) error {
			spec := dispatchTaskSpec(t)
			spec.WindowFrom, spec.WindowTo = underZero, at
			_, err := domain.OpenDispatchTask(spec)
			return err
		})
	})
	t.Run("OpenDispatchTask 建立时刻", func(t *testing.T) {
		pinRegistryZeroGate(t, domain.ErrInvalidDispatchTask, func(at time.Time) error {
			spec := dispatchTaskSpec(t)
			spec.OpenedAt = at
			_, err := domain.OpenDispatchTask(spec)
			return err
		})
	})
	task := openedTask(t)
	t.Run("Reschedule 时间窗起点", func(t *testing.T) {
		pinRegistryZeroGate(t, domain.ErrInvalidDispatchTask, func(at time.Time) error {
			_, err := task.Reschedule(at, taskWindowTo, task.OpenedAt())
			return err
		})
	})
	t.Run("Reschedule 时间窗终点", func(t *testing.T) {
		pinRegistryZeroGate(t, domain.ErrInvalidDispatchTask, func(at time.Time) error {
			_, err := task.Reschedule(underZero, at, task.OpenedAt())
			return err
		})
	})
	t.Run("关闭时刻", func(t *testing.T) {
		spec := dispatchTaskSpec(t)
		spec.OpenedAt = beforeRegistryZero
		early, err := domain.OpenDispatchTask(spec)
		if err != nil {
			t.Fatalf("开立任务：%v", err)
		}
		basis := mustValue(t, domain.NewTaskClosureBasisReference, "SYN-CLOSURE-1")
		pinRegistryZeroGate(t, domain.ErrInvalidDispatchTask, func(at time.Time) error {
			_, err := early.Terminate(basis, at)
			return err
		})
	})
}

// Covers: 票 tf-face-algebra/14 判零——履约尝试的计划窗两端与到场时刻、两种对象结果的发生时刻、交付证明更正时刻。
func TestFulfillmentAttemptAndDeliveryGatesJudgeZeroOnTheRegistry(t *testing.T) {
	t.Run("FormFulfillmentAttempt 计划窗起点", func(t *testing.T) {
		pinRegistryZeroGate(t, domain.ErrInvalidFulfillmentAttempt, func(at time.Time) error {
			spec := attemptSpec(t, "SYN-ATTEMPT-1")
			spec.PlannedFrom = at
			_, err := domain.FormFulfillmentAttempt(spec)
			return err
		})
	})
	t.Run("FormFulfillmentAttempt 计划窗终点", func(t *testing.T) {
		pinRegistryZeroGate(t, domain.ErrInvalidFulfillmentAttempt, func(at time.Time) error {
			spec := attemptSpec(t, "SYN-ATTEMPT-1")
			spec.PlannedFrom, spec.PlannedTo = zeroInstant.Add(-time.Microsecond), at
			_, err := domain.FormFulfillmentAttempt(spec)
			return err
		})
	})
	t.Run("FormFulfillmentAttempt 到场时刻", func(t *testing.T) {
		pinRegistryZeroGate(t, domain.ErrInvalidFulfillmentAttempt, func(at time.Time) error {
			spec := attemptSpec(t, "SYN-ATTEMPT-1")
			spec.ArrivedAt = at
			_, err := domain.FormFulfillmentAttempt(spec)
			return err
		})
	})

	spec := attemptSpec(t, "SYN-ATTEMPT-1")
	spec.ArrivedAt = beforeRegistryZero
	attempt, err := domain.FormFulfillmentAttempt(spec)
	if err != nil {
		t.Fatalf("形成尝试：%v", err)
	}
	object := spec.Objects[0]
	t.Run("FormAttemptObjectResult 发生时刻", func(t *testing.T) {
		pinRegistryZeroGate(t, domain.ErrInvalidAttemptObjectResult, func(at time.Time) error {
			_, err := domain.FormAttemptObjectResult(attempt, object, domain.ObjectPickedUp, domain.AttemptResultBasisReference{}, at)
			return err
		})
	})
	t.Run("FormDeliveryAttemptResult 发生时刻", func(t *testing.T) {
		pinRegistryZeroGate(t, domain.ErrInvalidDeliveryAttemptResult, func(at time.Time) error {
			_, err := domain.FormDeliveryAttemptResult(attempt, object, domain.ObjectDelivered, domain.AttemptResultBasisReference{}, at)
			return err
		})
	})
	t.Run("CorrectProof 更正时刻", func(t *testing.T) {
		result, err := domain.FormDeliveryAttemptResult(attempt, object, domain.ObjectDelivered, domain.AttemptResultBasisReference{}, beforeRegistryZero)
		if err != nil {
			t.Fatalf("形成派送结果：%v", err)
		}
		delivery, err := domain.FormEffectiveDelivery(attempt, result, deliverySpec(t, "SYN-DELIVERY-V1"))
		if err != nil {
			t.Fatalf("形成交付：%v", err)
		}
		proof := mustValue(t, domain.NewDeliveryProofReference, "SYN-POD-2")
		version := mustValue(t, domain.NewDeliveryResultVersion, "SYN-DELIVERY-V2")
		pinRegistryZeroGate(t, domain.ErrInvalidEffectiveDelivery, func(at time.Time) error {
			_, err := delivery.CorrectProof(proof, version, at)
			return err
		})
	})
}

// Covers: 票 tf-face-algebra/14 判零——外部承运凭证适用范围的起点与收掉终点的改变时间。
func TestExternalCarrierCredentialGatesJudgeZeroOnTheRegistry(t *testing.T) {
	t.Run("NewCredentialApplicability 起点", func(t *testing.T) {
		pinRegistryZeroGate(t, domain.ErrInvalidExternalCarrierCredential, func(at time.Time) error {
			_, err := domain.NewCredentialApplicability(at, time.Time{})
			return err
		})
	})
	t.Run("Revoke 改变时间", func(t *testing.T) {
		spec := credentialSpec(t)
		spec.Applicability = mustApplicability(t, zeroInstant.Add(-time.Microsecond), time.Time{})
		credential, err := domain.RegisterExternalCarrierCredential(spec)
		if err != nil {
			t.Fatalf("登记凭证：%v", err)
		}
		version := mustRef(t, domain.NewExternalCarrierCredentialVersion, "SYN-CREDV-2")
		pinRegistryZeroGate(t, domain.ErrInvalidExternalCarrierCredential, func(at time.Time) error {
			_, err := credential.Revoke(at, version)
			return err
		})
	})
}

// Covers: 票 tf-face-algebra/14 判零——主单后续版本的改变时间。
func TestMasterDocumentChangeJudgesZeroOnTheRegistry(t *testing.T) {
	document := mustMasterDocument(t, masterDocumentSpec(t))
	version := mustRef(t, domain.NewMasterDocumentVersion, "SYN-MDV-2")
	pinRegistryZeroGate(t, domain.ErrInvalidMasterDocument, func(at time.Time) error {
		_, err := document.Revoke(at, version)
		return err
	})
}

// Covers: 票 tf-face-algebra/14 判零——运输委托提交、开始与取消，订舱申请提交与取消，承运接受决定时间。
func TestCommissionAndBookingGatesJudgeZeroOnTheRegistry(t *testing.T) {
	t.Run("DraftTransportCommission 提交时刻", func(t *testing.T) {
		pinRegistryZeroGate(t, domain.ErrInvalidTransportCommission, func(at time.Time) error {
			spec := commissionSpec(t)
			spec.SubmittedAt = at
			_, err := domain.DraftTransportCommission(spec)
			return err
		})
	})
	spec := commissionSpec(t)
	spec.SubmittedAt = beforeRegistryZero
	commission, err := submitCommission(t, spec)
	if err != nil {
		t.Fatalf("提交委托：%v", err)
	}
	t.Run("MarkTransportStarted 开始时刻", func(t *testing.T) {
		basis := mustValue(t, domain.NewParticipationBasisReference, "SYN-BASIS-1")
		pinRegistryZeroGate(t, domain.ErrInvalidTransportCommission, func(at time.Time) error {
			_, err := commission.MarkTransportStarted(basis, at)
			return err
		})
	})
	t.Run("TransportCommission.Cancel 取消时刻", func(t *testing.T) {
		pinRegistryZeroGate(t, domain.ErrInvalidTransportCommission, func(at time.Time) error {
			_, err := commission.Cancel(at)
			return err
		})
	})

	bookingSpec := func(at time.Time) domain.BookingRequestSpec {
		return domain.BookingRequestSpec{
			TenantID:    mustValue(t, domain.NewTenantID, "SYN-TENANT-01"),
			Booking:     mustValue(t, domain.NewBookingReference, "SYN-BOOKING-1"),
			Commission:  mustValue(t, domain.NewTransportCommissionReference, "SYN-COMMISSION-1"),
			Schedule:    mustValue(t, domain.NewScheduleReference, "SYN-SCHEDULE-1"),
			Quantity:    100,
			Unit:        mustValue(t, domain.NewQuantityUnitReference, "kg"),
			RequestedAt: at,
		}
	}
	t.Run("SubmitBookingRequest 申请时刻", func(t *testing.T) {
		pinRegistryZeroGate(t, domain.ErrInvalidBookingRequest, func(at time.Time) error {
			_, err := domain.SubmitBookingRequest(bookingSpec(at))
			return err
		})
	})
	booking, err := domain.SubmitBookingRequest(bookingSpec(beforeRegistryZero))
	if err != nil {
		t.Fatalf("提交订舱：%v", err)
	}
	t.Run("BookingRequest.Cancel 取消时刻", func(t *testing.T) {
		pinRegistryZeroGate(t, domain.ErrInvalidBookingRequest, func(at time.Time) error {
			_, err := booking.Cancel(at)
			return err
		})
	})
	t.Run("FormCarrierAcceptance 决定时间", func(t *testing.T) {
		acceptance := mustValue(t, domain.NewCarrierAcceptanceReference, "SYN-ACCEPTANCE-1")
		pinRegistryZeroGate(t, domain.ErrInvalidCarrierAcceptance, func(at time.Time) error {
			_, err := domain.FormCarrierAcceptance(booking, acceptance, domain.BookingAccepted, 10, domain.AcceptanceBasisReference{}, at)
			return err
		})
	})
}

// Covers: 票 tf-face-algebra/14 判零——权威运输交接的判断时间与更正时间。
func TestTransportHandoverGatesJudgeZeroOnTheRegistry(t *testing.T) {
	t.Run("FormTransportHandover 判断时间", func(t *testing.T) {
		pinRegistryZeroGate(t, domain.ErrInvalidTransportHandover, func(at time.Time) error {
			spec := handoverSpec(t, "SYN-PARCEL-1", domain.ObjectHandedOver)
			spec.JudgedAt = at
			_, err := domain.FormTransportHandover(spec)
			return err
		})
	})
	t.Run("Correct 更正时间", func(t *testing.T) {
		spec := handoverSpec(t, "SYN-PARCEL-1", domain.ObjectHandedOver)
		spec.JudgedAt = beforeRegistryZero
		handover, err := domain.FormTransportHandover(spec)
		if err != nil {
			t.Fatalf("形成交接：%v", err)
		}
		pinRegistryZeroGate(t, domain.ErrInvalidTransportHandover, func(at time.Time) error {
			_, err := handover.Correct(domain.HandoverCorrection{
				Verdict:           domain.ObjectHandedOver,
				ReleasingEvidence: spec.ReleasingEvidence,
				ReceivingEvidence: spec.ReceivingEvidence,
				Rule:              spec.Rule,
				Version:           mustValue(t, domain.NewHandoverResultVersion, "SYN-HANDOVER-V2"),
				CorrectedAt:       at,
			})
			return err
		})
	})
}

// Covers: 票 tf-face-algebra/14 判零——运输收费发生项的业务时间与有效性修订时间。
func TestTransportChargeOccurrenceGatesJudgeZeroOnTheRegistry(t *testing.T) {
	t.Run("FormTransportChargeOccurrence 业务时间", func(t *testing.T) {
		pinRegistryZeroGate(t, domain.ErrInvalidChargeOccurrence, func(at time.Time) error {
			spec := occurrenceSpec(t, "SYN-OCCURRENCE-1", "SYN-JOURNEY-1")
			spec.OccurredAt = at
			_, err := domain.FormTransportChargeOccurrence(spec)
			return err
		})
	})
	t.Run("ReviseValidity 修订时间", func(t *testing.T) {
		spec := occurrenceSpec(t, "SYN-OCCURRENCE-1", "SYN-JOURNEY-1")
		spec.OccurredAt = beforeRegistryZero
		occurrence, err := domain.FormTransportChargeOccurrence(spec)
		if err != nil {
			t.Fatalf("形成发生项：%v", err)
		}
		version := mustValue(t, domain.NewOccurrenceValidityVersion, "SYN-OCCURRENCE-V2")
		basis := mustValue(t, domain.NewOccurrenceBasisReference, "SYN-BASIS-1")
		pinRegistryZeroGate(t, domain.ErrInvalidChargeOccurrence, func(at time.Time) error {
			_, err := occurrence.ReviseValidity(domain.OccurrenceInvalidated, version, basis, at)
			return err
		})
	})
}

// Covers: 票 tf-face-algebra/14 判零——班次发运时间与预占有效期；预占的判断时点与释放的时点随 ADR-0203 入册，一并判零。
// 消耗的调用时刻不入册，不在此列。
func TestScheduleAndCapacityGatesJudgeZeroOnTheRegistry(t *testing.T) {
	t.Run("FormTransportSchedule 发运时间", func(t *testing.T) {
		pinRegistryZeroGate(t, domain.ErrInvalidTransportSchedule, func(at time.Time) error {
			_, err := domain.FormTransportSchedule(domain.TransportScheduleSpec{
				TenantID:     mustValue(t, domain.NewTenantID, "SYN-TENANT-01"),
				Schedule:     mustValue(t, domain.NewScheduleReference, "SYN-SCHEDULE-1"),
				Direction:    directionFor(t),
				DepartsAt:    at,
				ArrivesAt:    at.Add(time.Hour),
				ServiceScope: mustValue(t, domain.NewScheduleServiceScopeReference, "SYN-SCOPE-01"),
			})
			return err
		})
	})
	t.Run("Reserve 有效期", func(t *testing.T) {
		pool := establishedPool(t, 100)
		reference := reservationRef(t, "SYN-RESERVATION-1")
		pinRegistryZeroGate(t, domain.ErrInvalidCapacityPool, func(at time.Time) error {
			_, err := pool.Reserve(reference, declaredDemand(t, 10), at, zeroInstant.Add(-time.Microsecond))
			return err
		})
	})
	t.Run("Reserve 判断时点", func(t *testing.T) {
		pool := establishedPool(t, 100)
		reference := reservationRef(t, "SYN-RESERVATION-1")
		pinRegistryZeroGate(t, domain.ErrInvalidCapacityPool, func(at time.Time) error {
			_, err := pool.Reserve(reference, declaredDemand(t, 10), reservedUntil, at)
			return err
		})
	})
	t.Run("Release 时点", func(t *testing.T) {
		pool := establishedPool(t, 100)
		reference := reservationRef(t, "SYN-RESERVATION-1")
		// 预占声明在零点前，这样零点之后的释放才落在预占的时段里，判零是唯一会拒它的那道门。
		pool, err := pool.Reserve(reference, declaredDemand(t, 10), reservedUntil, zeroInstant.Add(-time.Microsecond))
		if err != nil {
			t.Fatal(err)
		}
		pinRegistryZeroGate(t, domain.ErrInvalidCapacityPool, func(at time.Time) error {
			_, err := pool.Release(releaseRef(t, "SYN-RELEASE-1"), reference, declaredDemand(t, 1), at)
			return err
		})
	})
}

// Covers: 票 tf-face-algebra/14 判零——替代路程的启动时间与受监管处置的决定时间。
func TestAlternateJourneyAndRegulatoryDispositionGatesJudgeZeroOnTheRegistry(t *testing.T) {
	t.Run("FormAlternateJourney 启动时间", func(t *testing.T) {
		pinRegistryZeroGate(t, domain.ErrInvalidAlternateJourney, func(at time.Time) error {
			spec := alternateJourneySpec(t, domain.ReturnJourneyPurpose, domain.ServiceDispositionDecision)
			spec.StartedAt = at
			_, err := domain.FormAlternateJourney(spec)
			return err
		})
	})
	t.Run("FormRegulatoryTransportDisposition 决定时间", func(t *testing.T) {
		pinRegistryZeroGate(t, domain.ErrInvalidRegulatoryDisposition, func(at time.Time) error {
			spec := declinedDispositionSpec(t, "SYN-DECLINE-1")
			spec.DecidedAt = at
			_, err := domain.FormRegulatoryTransportDisposition(spec)
			return err
		})
	})
}

// pendingCarrierPickupSpec 是一条待确认（承运主体身份未登记）首登版本的输入。
func pendingCarrierPickupSpec(t *testing.T, material string, judgedAt time.Time) domain.PendingCarrierFirstEffectivePickupSpec {
	t.Helper()
	return domain.PendingCarrierFirstEffectivePickupSpec{
		TenantID: mustValue(t, domain.NewTenantID, "SYN-TENANT-01"),
		Object:   mustValue(t, domain.NewCarriedObjectReference, "SYN-PARCEL-1"),
		Fact:     mustValue(t, domain.NewCarrierFirstEffectivePickupReference, "SYN-CFEP-1"),
		Version:  mustValue(t, domain.NewCarrierFirstEffectivePickupVersion, "SYN-CFEV-1"),
		Reason:   domain.PickupCarrierIdentityNotRegistered,
		Material: material,
		JudgedAt: judgedAt,
		Bases:    []domain.CarrierPickupBasis{carrierPickupBasis(t, "SYN-EXTF-1", "SYN-EXTV-1")},
	}
}

// declinedDispositionSpec 是一份拒接的承接决定输入：拒因必带，不带对象也不需要移动授权。
func declinedDispositionSpec(t *testing.T, declineBasis string) domain.RegulatoryTransportDispositionSpec {
	t.Helper()
	return domain.RegulatoryTransportDispositionSpec{
		Tenant:       mustValue(t, domain.NewTenantID, "SYN-TENANT-01"),
		Item:         mustValue(t, domain.NewCollaborationItemReference, "SYN-ITEM-1"),
		Basis:        mustValue(t, domain.NewDispositionBasisReference, "SYN-DISPOSITION-1"),
		Kind:         domain.DispositionDeclined,
		DeclineBasis: declineBasis,
		DecidedAt:    time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC),
	}
}
