package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证失败尝试费那一种原因（ADR-0098；ADR-0207 决定一）：揽收侧按尝试来源键、派送侧按尝试身份取回对象结果，
// 依据、范围与业务时间取自结果；成功的结果被拒；两侧读不动答未决、指错对象答未受理。夹具全是合成串，证据只记 S。

var attemptArrivedAt = time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC)

type pickupResultSourceDouble struct {
	record ports.PickupAttemptRecord
	found  bool
	err    error
}

func (double pickupResultSourceDouble) FindByKey(_ context.Context, _ ports.PickupAttemptKey) (ports.PickupAttemptRecord, bool, error) {
	return double.record, double.found, double.err
}

type deliveryResultViewDouble struct {
	attempt domain.FulfillmentAttempt
	result  domain.DeliveryAttemptResult
	found   bool
	err     error
}

func (double deliveryResultViewDouble) LoadDeliveryResult(
	_ context.Context, _ domain.TenantID, _ domain.AttemptReference, _ domain.CarriedObjectReference,
) (domain.FulfillmentAttempt, domain.DeliveryAttemptResult, bool, error) {
	return double.attempt, double.result, double.found, double.err
}

func attemptFixture(t *testing.T, attempt string) domain.FulfillmentAttempt {
	t.Helper()
	formed, err := domain.FormFulfillmentAttempt(domain.FulfillmentAttemptSpec{
		TenantID:    occurrenceValue(t, domain.NewTenantID, occurrenceTenant),
		Attempt:     occurrenceValue(t, domain.NewAttemptReference, attempt),
		Task:        occurrenceValue(t, domain.NewDispatchTaskReference, "SYN-TASK-01"),
		ExecutedBy:  occurrenceValue(t, domain.NewExecutingPartyReference, "SYN-COURIER-01"),
		Place:       occurrenceValue(t, domain.NewAttemptPlaceReference, "SYN-PLACE-01"),
		Evidence:    occurrenceValue(t, domain.NewAttemptEvidenceReference, "SYN-EVIDENCE-01"),
		Objects:     []domain.CarriedObjectReference{occurrenceValue(t, domain.NewCarriedObjectReference, trunkParcel)},
		PlannedFrom: attemptArrivedAt.Add(-time.Hour),
		PlannedTo:   attemptArrivedAt.Add(time.Hour),
		ArrivedAt:   attemptArrivedAt,
	})
	if err != nil {
		t.Fatalf("尝试夹具：%v", err)
	}
	return formed
}

func pickupWith(t *testing.T, outcome domain.AttemptObjectOutcome) pickupResultSourceDouble {
	t.Helper()
	attempt := attemptFixture(t, "SYN-PICKUP-ATTEMPT-01")
	var basis domain.AttemptResultBasisReference
	if outcome.Failed() {
		basis = occurrenceValue(t, domain.NewAttemptResultBasisReference, "SYN-REASON-NOT-READY")
	}
	result, err := domain.FormAttemptObjectResult(attempt,
		occurrenceValue(t, domain.NewCarriedObjectReference, trunkParcel), outcome, basis, attemptArrivedAt.Add(10*time.Minute))
	if err != nil {
		t.Fatalf("揽收结果夹具：%v", err)
	}
	return pickupResultSourceDouble{
		record: ports.PickupAttemptRecord{Attempt: attempt, Results: []domain.AttemptObjectResult{result}},
		found:  true,
	}
}

func deliveryWith(t *testing.T, outcome domain.DeliveryObjectOutcome) deliveryResultViewDouble {
	t.Helper()
	attempt := attemptFixture(t, "SYN-DELIVERY-ATTEMPT-01")
	var basis domain.AttemptResultBasisReference
	if outcome.Failed() {
		basis = occurrenceValue(t, domain.NewAttemptResultBasisReference, "SYN-REASON-NO-ONE-HOME")
	}
	result, err := domain.FormDeliveryAttemptResult(attempt,
		occurrenceValue(t, domain.NewCarriedObjectReference, trunkParcel), outcome, basis, attemptArrivedAt.Add(15*time.Minute))
	if err != nil {
		t.Fatalf("派送结果夹具：%v", err)
	}
	return deliveryResultViewDouble{attempt: attempt, result: result, found: true}
}

func failedAttemptCommand(t *testing.T) application.RegisterChargeOccurrenceCommand {
	t.Helper()
	command := actualFulfillmentCommand(t)
	command.Reason = domain.FailedAttemptOccurrence
	command.Segment = ""
	command.Unit = "ATTEMPT"
	command.ConditionAssertion = "末端协议 v1：客户不在场的失败到场计一次"
	return command
}

// Covers: AT-TF-094——外包揽收第一次到场失败形成失败尝试费发生项：依据取「尝试结果/尝试/对象」、范围取那一次尝试、
// 业务时间取结果本身（不取登记时刻）；采购上下文照核对口取。
func TestAFailedPickupRegistersAFailedAttemptOccurrence(t *testing.T) {
	rig := newOccurrenceRig(t)
	rig.pickups = pickupWith(t, domain.GoodsNotReady)
	command := failedAttemptCommand(t)
	command.PickupAttemptSource = "SYN-PICKUP-SOURCE-01"

	result := rig.register(t, command)
	if result.Outcome() != application.ChargeOccurrenceRegistered {
		t.Fatalf("outcome = %q detail = %q", result.Outcome(), result.Detail())
	}
	record, _ := result.Record()
	occurrence := record.Occurrence
	if occurrence.Reason() != domain.FailedAttemptOccurrence ||
		occurrence.FactBasis().String() != "ATTEMPT-RESULT/SYN-PICKUP-ATTEMPT-01/"+trunkParcel ||
		occurrence.Scope().String() != "SYN-PICKUP-ATTEMPT-01" {
		t.Fatalf("原因 / 依据 / 范围 = %q / %q / %q", occurrence.Reason(), occurrence.FactBasis(), occurrence.Scope())
	}
	if !occurrence.OccurredAt().Equal(attemptArrivedAt.Add(10 * time.Minute)) {
		t.Fatalf("业务时间 = %v，应取对象结果", occurrence.OccurredAt())
	}
	if len(rig.handoff.intents) != 1 {
		t.Fatalf("意图 %d 份", len(rig.handoff.intents))
	}
}

// Covers: AT-TF-095——外包尾程第一次无人签收形成失败尝试费发生项，经交付生效已在用的派送尝试读口取结果，不建新存储；
// 依据前缀与揽收那一侧分开。
func TestAFailedDeliveryRegistersAFailedAttemptOccurrence(t *testing.T) {
	rig := newOccurrenceRig(t)
	rig.deliveries = deliveryWith(t, domain.NoOneToReceive)
	command := failedAttemptCommand(t)
	command.DeliveryAttempt = "SYN-DELIVERY-ATTEMPT-01"

	result := rig.register(t, command)
	if result.Outcome() != application.ChargeOccurrenceRegistered {
		t.Fatalf("outcome = %q detail = %q", result.Outcome(), result.Detail())
	}
	record, _ := result.Record()
	if got := record.Occurrence.FactBasis().String(); got != "DELIVERY-ATTEMPT-RESULT/SYN-DELIVERY-ATTEMPT-01/"+trunkParcel {
		t.Fatalf("依据 = %q", got)
	}
	if !record.Occurrence.OccurredAt().Equal(attemptArrivedAt.Add(15 * time.Minute)) {
		t.Fatalf("业务时间 = %v", record.Occurrence.OccurredAt())
	}
}

// Covers: 领域 ErrNotAFailedAttempt——揽收到手与妥投都不形成失败尝试费，且那是改请求的未受理，不是不适用：
// 不适用只留给自营（ADR-0098 决定三）。
func TestASucceededAttemptDoesNotFormAFailedAttemptOccurrence(t *testing.T) {
	pickedUp := newOccurrenceRig(t)
	pickedUp.pickups = pickupWith(t, domain.ObjectPickedUp)
	command := failedAttemptCommand(t)
	command.PickupAttemptSource = "SYN-PICKUP-SOURCE-01"
	if result := pickedUp.register(t, command); result.Outcome() != application.ChargeOccurrenceInputNotAccepted ||
		result.Detail() != application.AttemptDidNotFail {
		t.Fatalf("揽收到手 outcome/detail = %q/%q", result.Outcome(), result.Detail())
	}
	assertNothingWritten(t, pickedUp)

	delivered := newOccurrenceRig(t)
	delivered.deliveries = deliveryWith(t, domain.ObjectDelivered)
	command = failedAttemptCommand(t)
	command.DeliveryAttempt = "SYN-DELIVERY-ATTEMPT-01"
	if result := delivered.register(t, command); result.Outcome() != application.ChargeOccurrenceInputNotAccepted ||
		result.Detail() != application.AttemptDidNotFail {
		t.Fatalf("妥投 outcome/detail = %q/%q", result.Outcome(), result.Detail())
	}
	assertNothingWritten(t, delivered)
}

// Covers: 依赖读不动答未决并带续办引用；指名的尝试或对象不存在答未受理（回去查是不是问错了）；揽收与派送两把键
// 恰给其一，失败尝试一次恰一个对象。
func TestFailedAttemptFactsSeparateUnreadableFromMissingAndMalformed(t *testing.T) {
	unreadable := newOccurrenceRig(t)
	unreadable.pickups = pickupResultSourceDouble{err: errors.New("pickups down")}
	command := failedAttemptCommand(t)
	command.PickupAttemptSource = "SYN-PICKUP-SOURCE-01"
	if result := unreadable.register(t, command); result.Outcome() != application.ChargeOccurrenceUndecided ||
		result.Detail() != application.FactSourceUnavailable || result.Continuation() == "" {
		t.Fatalf("读不动 outcome/detail = %q/%q", result.Outcome(), result.Detail())
	}

	wrongObject := newOccurrenceRig(t)
	wrongObject.pickups = pickupWith(t, domain.GoodsNotReady)
	command = failedAttemptCommand(t)
	command.PickupAttemptSource = "SYN-PICKUP-SOURCE-01"
	command.Objects = []string{trunkSecondParcel}
	if result := wrongObject.register(t, command); result.Outcome() != application.ChargeOccurrenceInputNotAccepted ||
		result.Detail() != application.ChargeFactNotFound {
		t.Fatalf("对象不在尝试里 outcome/detail = %q/%q", result.Outcome(), result.Detail())
	}

	missingDelivery := newOccurrenceRig(t)
	missingDelivery.deliveries = deliveryResultViewDouble{}
	command = failedAttemptCommand(t)
	command.DeliveryAttempt = "SYN-DELIVERY-ATTEMPT-09"
	if result := missingDelivery.register(t, command); result.Detail() != application.ChargeFactNotFound {
		t.Fatalf("派送尝试不存在 detail = %q", result.Detail())
	}

	for name, mutate := range map[string]func(*application.RegisterChargeOccurrenceCommand){
		"both attempt keys": func(command *application.RegisterChargeOccurrenceCommand) {
			command.PickupAttemptSource, command.DeliveryAttempt = "SYN-PICKUP-SOURCE-01", "SYN-DELIVERY-ATTEMPT-01"
		},
		"no attempt key": func(*application.RegisterChargeOccurrenceCommand) {},
		"two objects": func(command *application.RegisterChargeOccurrenceCommand) {
			command.PickupAttemptSource = "SYN-PICKUP-SOURCE-01"
			command.Objects = []string{trunkParcel, trunkSecondParcel}
		},
	} {
		t.Run(name, func(t *testing.T) {
			rig := newOccurrenceRig(t)
			command := failedAttemptCommand(t)
			mutate(&command)
			if result := rig.register(t, command); result.Outcome() != application.ChargeOccurrenceInputNotAccepted ||
				result.Detail() != application.ChargeRequestMalformed {
				t.Fatalf("outcome/detail = %q/%q", result.Outcome(), result.Detail())
			}
		})
	}
}
