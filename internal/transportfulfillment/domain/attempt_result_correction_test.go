package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件钉待确认的逐对象结果凭同一次上门的后补来源更正成确定结果（票 docs-consistency/code/90）。夹具全为合成值（S 级）。

func pickupCorrectionSource(t *testing.T) domain.AttemptResultSourceReference {
	t.Helper()
	return mustValue(t, domain.NewAttemptResultSourceReference, "source/partner-late-receipt")
}

func determinateBasis(t *testing.T, value string) domain.AttemptResultBasisReference {
	t.Helper()
	return mustValue(t, domain.NewAttemptResultBasisReference, value)
}

// Covers: `AT-TF-357`（领域半边）——TF CONTEXT「揽收、派送与交付」「之后同一对象的确定结果，凭同一次上门的后补来源以更正形成新判断版本；待确认那一版
// 留在册上不改写」的领域半边（揽收）——更正成揽收到手：新判断版本是确定结果，所凭的后补来源留在更正上、不进确定结果；
// 待确认那一版原样回指，不被改写。
func TestAPendingPickupResultIsCorrectedToAPickupWithoutRewritingThePendingVersion(t *testing.T) {
	attempt := formedAttempt(t, "attempt-1")
	pending := pendingPickupResult(t, attempt, "parcel-1")
	occurredAt := attemptArrivedAt.Add(2 * time.Minute)
	correctedAt := attemptArrivedAt.Add(3 * time.Hour)

	correction, err := domain.CorrectPendingAttemptObjectResult(
		attempt, pending, domain.ObjectPickedUp, domain.AttemptResultBasisReference{},
		pickupCorrectionSource(t), occurredAt, correctedAt)
	if err != nil {
		t.Fatalf("correct pending pickup result: %v", err)
	}

	effective := correction.Effective()
	if effective.Outcome() != domain.ObjectPickedUp || !effective.Outcome().Succeeded() {
		t.Fatalf("effective outcome = %q, want PICKED_UP", effective.Outcome())
	}
	if _, present := effective.Basis(); present {
		t.Fatal("揽收到手带了依据：它的证据在场外揽收的控制依据上，两处会打架")
	}
	if _, present := effective.Source(); present {
		t.Fatal("确定结果带了来源：来源只属待确认，更正所凭的来源留在更正上")
	}
	if !effective.OccurredAt().Equal(occurredAt) {
		t.Fatalf("effective occurredAt = %v, want %v；确定结果的业务时间是后补来源说的那一刻", effective.OccurredAt(), occurredAt)
	}
	if source := correction.Source(); source.String() != "source/partner-late-receipt" {
		t.Fatalf("correction source = %q", source)
	}
	if !correction.CorrectedAt().Equal(correctedAt) {
		t.Fatalf("correctedAt = %v, want %v", correction.CorrectedAt(), correctedAt)
	}
	kept := correction.Pending()
	if !kept.Outcome().PendingConfirmation() {
		t.Fatal("待确认那一版被改写成了确定结果：原版必须留在册上")
	}
	if kept.Object() != pending.Object() || kept.Attempt() != pending.Attempt() {
		t.Fatal("更正没有回指同一对象在同一次尝试里的待确认那一版")
	}
	if pending.Outcome() != domain.PickupPendingConfirmation {
		t.Fatal("传入的待确认值被更正改写了")
	}
}

// Covers: `AT-TF-357`（领域半边）——同上——后补来源说的业务时间可以早于待确认那一版登记的时刻（伙伴迟到的签收：业务上先发生、后到达），只要不早于
// 到场（UC-TF-002「外部伙伴状态迟到时按业务发生时间采用」）；更正本身晚于它所更正的待确认与它说的那件事。
func TestACorrectionMayCarryABusinessTimeEarlierThanThePendingVersion(t *testing.T) {
	attempt := formedAttempt(t, "attempt-1")
	pending := pendingPickupResult(t, attempt, "parcel-1") // 到场后 4 分钟

	if _, err := domain.CorrectPendingAttemptObjectResult(
		attempt, pending, domain.ObjectPickedUp, domain.AttemptResultBasisReference{},
		pickupCorrectionSource(t), attemptArrivedAt.Add(2*time.Minute), attemptArrivedAt.Add(time.Hour)); err != nil {
		t.Fatalf("a late source whose business time precedes the pending version = %v, want accepted", err)
	}
}

// Covers: `AT-TF-358`（领域半边）——同上——更正出来的那一版走既有确定结果的构造门：失败与拒收必带依据，揽收到手不带依据，结果不早于到场、对象在尝试范围内。
func TestACorrectionGoesThroughTheDeterminateResultGates(t *testing.T) {
	attempt := formedAttempt(t, "attempt-1")
	pending := pendingPickupResult(t, attempt, "parcel-1")
	source := pickupCorrectionSource(t)
	at := attemptArrivedAt.Add(2 * time.Minute)
	corrected := attemptArrivedAt.Add(time.Hour)

	for name, outcome := range map[string]domain.AttemptObjectOutcome{
		"customer absent": domain.CustomerAbsent, "goods not ready": domain.GoodsNotReady,
		"packaging unacceptable": domain.PackagingUnacceptable, "executor refused": domain.ExecutorRefused,
	} {
		t.Run(name+" needs a basis", func(t *testing.T) {
			if _, err := domain.CorrectPendingAttemptObjectResult(
				attempt, pending, outcome, domain.AttemptResultBasisReference{}, source, at, corrected); !errors.Is(err, domain.ErrInvalidAttemptObjectResult) {
				t.Fatalf("no basis error = %v, want ErrInvalidAttemptObjectResult", err)
			}
			correction, err := domain.CorrectPendingAttemptObjectResult(
				attempt, pending, outcome, determinateBasis(t, "reason/confirmed-later"), source, at, corrected)
			if err != nil {
				t.Fatalf("correct to %s: %v", outcome, err)
			}
			if !correction.Effective().Outcome().Failed() {
				t.Fatalf("%s 更正出来的确定结果读不成失败：失败尝试费等不到事实依据", outcome)
			}
			if _, err := domain.FailedPickupAttemptBasis(correction.Effective()); err != nil {
				t.Fatalf("failed attempt basis of the corrected %s = %v, want formed", outcome, err)
			}
		})
	}

	t.Run("a pickup carries no basis", func(t *testing.T) {
		if _, err := domain.CorrectPendingAttemptObjectResult(
			attempt, pending, domain.ObjectPickedUp, determinateBasis(t, "reason/x"), source, at, corrected); !errors.Is(err, domain.ErrInvalidAttemptObjectResult) {
			t.Fatalf("error = %v, want ErrInvalidAttemptObjectResult", err)
		}
	})
	t.Run("before the arrival", func(t *testing.T) {
		if _, err := domain.CorrectPendingAttemptObjectResult(
			attempt, pending, domain.ObjectPickedUp, domain.AttemptResultBasisReference{}, source,
			attemptArrivedAt.Add(-time.Minute), corrected); !errors.Is(err, domain.ErrInvalidAttemptObjectResult) {
			t.Fatalf("error = %v, want ErrInvalidAttemptObjectResult", err)
		}
	})
}

// Covers: `AT-TF-360`（领域半边）——同上——更正只回指待确认那一版：确定结果不接更正成待确认，也不接另一版确定结果的更正（那是另一种更正）；更正必带所凭的后补
// 来源；更正不早于它所更正的待确认、也不早于它说的那件事；待确认属于另一次尝试或另一个对象的不接。
func TestACorrectionOnlyPointsBackAtAPendingVersion(t *testing.T) {
	attempt := formedAttempt(t, "attempt-1")
	pending := pendingPickupResult(t, attempt, "parcel-1")
	source := pickupCorrectionSource(t)
	at := attemptArrivedAt.Add(2 * time.Minute)
	corrected := attemptArrivedAt.Add(time.Hour)

	determinate, err := domain.FormAttemptObjectResult(
		attempt, mustValue(t, domain.NewCarriedObjectReference, "parcel-1"), domain.CustomerAbsent,
		determinateBasis(t, "reason/absent"), attemptArrivedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("form determinate result: %v", err)
	}

	cases := map[string]struct {
		pending     domain.AttemptObjectResult
		outcome     domain.AttemptObjectOutcome
		source      domain.AttemptResultSourceReference
		occurredAt  time.Time
		correctedAt time.Time
		attempt     domain.FulfillmentAttempt
		want        error
	}{
		"a determinate result is not corrected here": {
			pending: determinate, outcome: domain.ObjectPickedUp, source: source, occurredAt: at, correctedAt: corrected,
			attempt: attempt, want: domain.ErrAttemptResultNotPending,
		},
		"a pending result is not corrected into pending": {
			pending: pending, outcome: domain.PickupPendingConfirmation, source: source, occurredAt: at, correctedAt: corrected,
			attempt: attempt, want: domain.ErrInvalidAttemptResultCorrection,
		},
		"an unknown outcome": {
			pending: pending, outcome: domain.AttemptObjectOutcomeInvalid, source: source, occurredAt: at, correctedAt: corrected,
			attempt: attempt, want: domain.ErrInvalidAttemptResultCorrection,
		},
		"no correcting source": {
			pending: pending, outcome: domain.ObjectPickedUp, occurredAt: at, correctedAt: corrected,
			attempt: attempt, want: domain.ErrInvalidAttemptResultCorrection,
		},
		"corrected before the pending version": {
			pending: pending, outcome: domain.ObjectPickedUp, source: source, occurredAt: at,
			correctedAt: attemptArrivedAt.Add(3 * time.Minute), attempt: attempt, want: domain.ErrInvalidAttemptResultCorrection,
		},
		"corrected before the thing it says happened": {
			pending: pending, outcome: domain.ObjectPickedUp, source: source, occurredAt: attemptArrivedAt.Add(2 * time.Hour),
			correctedAt: attemptArrivedAt.Add(time.Hour), attempt: attempt, want: domain.ErrInvalidAttemptResultCorrection,
		},
		"the pending version of another attempt": {
			pending: pending, outcome: domain.ObjectPickedUp, source: source, occurredAt: at, correctedAt: corrected,
			attempt: formedAttempt(t, "attempt-2"), want: domain.ErrInvalidAttemptResultCorrection,
		},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := domain.CorrectPendingAttemptObjectResult(
				test.attempt, test.pending, test.outcome, domain.AttemptResultBasisReference{}, test.source, test.occurredAt, test.correctedAt)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

// Covers: `AT-TF-359`（领域半边）——`AT-TF-070`「本人签收和地址来源冲突 → 形成冲突或待确认」之后的确定——派送一侧更正成妥投：确定结果不带依据、不带来源，
// 同一次尝试里的那件对象于是登得出有效交付（结果变了，交付登记的构造门不变）；待确认那一版回指、不被改写。
func TestAPendingDeliveryResultCorrectedToDeliveredFormsAnEffectiveDelivery(t *testing.T) {
	attempt := formedAttempt(t, "delivery-attempt-1")
	pending := pendingDeliveryResult(t, attempt, "parcel-1")
	if _, err := domain.FormEffectiveDelivery(attempt, pending, deliverySpec(t, "delivery-v1")); !errors.Is(err, domain.ErrDeliveryResultPendingConfirmation) {
		t.Fatalf("before the correction the delivery error = %v, want ErrDeliveryResultPendingConfirmation", err)
	}

	correction, err := domain.CorrectPendingDeliveryAttemptResult(
		attempt, pending, domain.ObjectDelivered, domain.AttemptResultBasisReference{},
		mustValue(t, domain.NewAttemptResultSourceReference, "source/address-review-confirmed"),
		attemptArrivedAt.Add(8*time.Minute), attemptArrivedAt.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("correct pending delivery result: %v", err)
	}
	effective := correction.Effective()
	if effective.Outcome() != domain.ObjectDelivered {
		t.Fatalf("effective outcome = %q, want DELIVERED", effective.Outcome())
	}
	if _, present := effective.Source(); present {
		t.Fatal("确定结果带了来源")
	}
	if !correction.Pending().Outcome().PendingConfirmation() {
		t.Fatal("待确认那一版被改写")
	}
	delivery, err := domain.FormEffectiveDelivery(attempt, effective, deliverySpec(t, "delivery-v1"))
	if err != nil {
		t.Fatalf("form effective delivery from the corrected result: %v", err)
	}
	if !delivery.OccurredAt().Equal(effective.OccurredAt()) {
		t.Fatalf("delivery occurredAt = %v, want the corrected business time %v", delivery.OccurredAt(), effective.OccurredAt())
	}
}

// Covers: `AT-TF-359`、`AT-TF-360`（领域半边）——同上——派送一侧更正成失败（无人签收、地址错误、收件人拒收）必带依据，更正出来的失败结果是失败尝试费的事实依据；
// 其余各道门同揽收一侧。
func TestADeliveryCorrectionGoesThroughTheDeterminateGatesAndPointsBackAtPending(t *testing.T) {
	attempt := formedAttempt(t, "delivery-attempt-1")
	pending := pendingDeliveryResult(t, attempt, "parcel-1")
	source := mustValue(t, domain.NewAttemptResultSourceReference, "source/review")
	at := attemptArrivedAt.Add(8 * time.Minute)
	corrected := attemptArrivedAt.Add(time.Hour)

	for name, outcome := range map[string]domain.DeliveryObjectOutcome{
		"refused": domain.DeliveryRefused, "no one": domain.NoOneToReceive, "wrong address": domain.WrongAddress,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := domain.CorrectPendingDeliveryAttemptResult(
				attempt, pending, outcome, domain.AttemptResultBasisReference{}, source, at, corrected); !errors.Is(err, domain.ErrInvalidDeliveryAttemptResult) {
				t.Fatalf("no basis error = %v, want ErrInvalidDeliveryAttemptResult", err)
			}
			correction, err := domain.CorrectPendingDeliveryAttemptResult(
				attempt, pending, outcome, determinateBasis(t, "reason/review"), source, at, corrected)
			if err != nil {
				t.Fatalf("correct to %s: %v", outcome, err)
			}
			if _, err := domain.FailedDeliveryAttemptBasis(correction.Effective()); err != nil {
				t.Fatalf("failed attempt basis of the corrected result = %v, want formed", err)
			}
		})
	}

	determinate, err := domain.FormDeliveryAttemptResult(
		attempt, mustValue(t, domain.NewCarriedObjectReference, "parcel-1"), domain.NoOneToReceive,
		determinateBasis(t, "reason/nobody"), attemptArrivedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("form determinate delivery result: %v", err)
	}
	// 更正这一判断不早于它所更正的待确认那一版（待确认记在到场后 9 分钟）、也不早于它说的那件事；两道各守一格。
	for name, test := range map[string]struct {
		occurredAt  time.Time
		correctedAt time.Time
	}{
		"corrected before the pending version": {
			occurredAt: attemptArrivedAt.Add(8 * time.Minute), correctedAt: attemptArrivedAt.Add(8*time.Minute + 30*time.Second),
		},
		"corrected before the thing it says happened": {
			occurredAt: attemptArrivedAt.Add(2 * time.Hour), correctedAt: attemptArrivedAt.Add(time.Hour),
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := domain.CorrectPendingDeliveryAttemptResult(
				attempt, pending, domain.ObjectDelivered, domain.AttemptResultBasisReference{}, source, test.occurredAt, test.correctedAt)
			if !errors.Is(err, domain.ErrInvalidAttemptResultCorrection) {
				t.Fatalf("error = %v, want ErrInvalidAttemptResultCorrection", err)
			}
		})
	}

	for name, test := range map[string]struct {
		pending domain.DeliveryAttemptResult
		outcome domain.DeliveryObjectOutcome
		source  domain.AttemptResultSourceReference
		attempt domain.FulfillmentAttempt
		want    error
	}{
		"a determinate result is not corrected here": {determinate, domain.ObjectDelivered, source, attempt, domain.ErrAttemptResultNotPending},
		"not into pending":                           {pending, domain.DeliveryPendingConfirmation, source, attempt, domain.ErrInvalidAttemptResultCorrection},
		"no correcting source":                       {pending, domain.ObjectDelivered, domain.AttemptResultSourceReference{}, attempt, domain.ErrInvalidAttemptResultCorrection},
		"another attempt":                            {pending, domain.ObjectDelivered, source, formedAttempt(t, "delivery-attempt-2"), domain.ErrInvalidAttemptResultCorrection},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := domain.CorrectPendingDeliveryAttemptResult(
				test.attempt, test.pending, test.outcome, domain.AttemptResultBasisReference{}, test.source, at, corrected)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}
