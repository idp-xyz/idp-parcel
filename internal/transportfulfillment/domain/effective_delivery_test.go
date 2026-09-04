package domain_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

func deliverySpec(t *testing.T, version string) domain.EffectiveDeliverySpec {
	t.Helper()
	return domain.EffectiveDeliverySpec{
		Method:    mustValue(t, domain.NewDeliveryMethodReference, "method-signature"),
		Recipient: mustValue(t, domain.NewReceivingPartyReference, "recipient-1"),
		Proof:     mustValue(t, domain.NewDeliveryProofReference, "pod-1"),
		Version:   mustValue(t, domain.NewDeliveryResultVersion, version),
	}
}

func deliveredResult(t *testing.T, attempt domain.FulfillmentAttempt, object string) domain.DeliveryAttemptResult {
	t.Helper()
	result, err := domain.FormDeliveryAttemptResult(
		attempt,
		mustValue(t, domain.NewCarriedObjectReference, object),
		domain.ObjectDelivered,
		domain.AttemptResultBasisReference{},
		attemptArrivedAt.Add(10*time.Minute),
	)
	if err != nil {
		t.Fatalf("form delivered result: %v", err)
	}
	return result
}

// Covers: `AT-TF-067`「安全投放方式被合同允许且证据完整 → 形成有效交付和收件方控制转移」
// 与 CONTEXT「有效交付结果必须关联明确载运对象、履约尝试、实际承运商或待确认依据、业务
// 时间、地点、交付方式、接收对象和符合当时规则的交付证明」——地点取自尝试、时间取自对象
// 结果，各只有一个权威来源；实际承运商轴由 `ActualCarrierJudgment` 按段作答而不在本类型上
// （见类型注释），本用例不假装覆盖它。
func TestADeliveredObjectFormsAnEffectiveDeliveryWithItsPOD(t *testing.T) {
	attempt := formedAttempt(t, "attempt-1")
	result := deliveredResult(t, attempt, "parcel-1")

	delivery, err := domain.FormEffectiveDelivery(attempt, result, deliverySpec(t, "delivery-result/v1"))
	if err != nil {
		t.Fatalf("form effective delivery: %v", err)
	}
	if delivery.Object().String() != "parcel-1" || delivery.Attempt() != attempt.Attempt() {
		t.Fatal("有效交付没有锚在对象与尝试上")
	}
	if delivery.Place() != attempt.Place() {
		t.Fatal("地点没有取自尝试")
	}
	if !delivery.OccurredAt().Equal(result.OccurredAt()) {
		t.Fatal("业务时间没有取自对象结果")
	}
	if delivery.Proof().String() != "pod-1" || delivery.Method().String() != "method-signature" {
		t.Fatal("POD 或交付方式没有随结果保全")
	}
	if delivery.Recipient().String() != "recipient-1" {
		t.Fatal("接收对象没有随结果保全")
	}
	if _, corrected := delivery.Corrects(); corrected {
		t.Fatal("首个版本凭空带上了更正前身")
	}
}

// Covers: CONTEXT「无人签收、地址错误、收件人拒收或证据不足不构成有效交付」
// （AT-TF-065 的交付半边）——三个失败取值一律进不了 FormEffectiveDelivery；错锚尝试与
// 缺件是另一格形状错误。
func TestFailedDeliveryOutcomesCannotBecomeAnEffectiveDelivery(t *testing.T) {
	attempt := formedAttempt(t, "attempt-1")
	basis := mustValue(t, domain.NewAttemptResultBasisReference, "reason-1")

	for name, outcome := range map[string]domain.DeliveryObjectOutcome{
		"refused":           domain.DeliveryRefused,
		"no one to receive": domain.NoOneToReceive,
		"wrong address":     domain.WrongAddress,
	} {
		t.Run(name, func(t *testing.T) {
			result, err := domain.FormDeliveryAttemptResult(
				attempt,
				mustValue(t, domain.NewCarriedObjectReference, "parcel-1"),
				outcome,
				basis,
				attemptArrivedAt.Add(10*time.Minute),
			)
			if err != nil {
				t.Fatalf("form failed result: %v", err)
			}
			_, err = domain.FormEffectiveDelivery(attempt, result, deliverySpec(t, "delivery-result/v1"))
			if !errors.Is(err, domain.ErrNotAnEffectiveDelivery) {
				t.Fatalf("error = %v, want ErrNotAnEffectiveDelivery", err)
			}
			if errors.Is(err, domain.ErrInvalidEffectiveDelivery) {
				t.Fatal("「不构成有效交付」被压成了形状错误")
			}
		})
	}

	t.Run("a result anchored to another attempt is refused", func(t *testing.T) {
		other := formedAttempt(t, "attempt-2")
		result := deliveredResult(t, other, "parcel-1")
		if _, err := domain.FormEffectiveDelivery(attempt, result, deliverySpec(t, "delivery-result/v1")); !errors.Is(err, domain.ErrInvalidEffectiveDelivery) {
			t.Fatalf("error = %v; 别次尝试的结果建成了本次尝试的交付", err)
		}
	})

	t.Run("a blank proof cannot form a delivery", func(t *testing.T) {
		result := deliveredResult(t, attempt, "parcel-1")
		spec := deliverySpec(t, "delivery-result/v1")
		spec.Proof = domain.DeliveryProofReference{}
		if _, err := domain.FormEffectiveDelivery(attempt, result, spec); !errors.Is(err, domain.ErrInvalidEffectiveDelivery) {
			t.Fatalf("error = %v, want ErrInvalidEffectiveDelivery", err)
		}
	})
}

// Covers: `AT-TF-064`「一个任务覆盖三个包裹，只有一个有效交付 → 只结束该包裹控制」与
// `AT-TF-065`「收件人拒收 → 形成拒收尝试，不自动签收或退运」——拒收只结束本次尝试的
// 相应对象结果，不挡同尝试他对象的交付；结果类型上没有控制或履约段字段可以冒充转移。
func TestARefusalEndsOnlyItsOwnObject(t *testing.T) {
	resultType := reflect.TypeOf(domain.DeliveryAttemptResult{})
	for index := 0; index < resultType.NumField(); index++ {
		field := resultType.Field(index)
		name := strings.ToLower(field.Name)
		if strings.Contains(name, "control") || strings.Contains(name, "segment") || strings.Contains(name, "return") {
			t.Fatalf("DeliveryAttemptResult 携带 %q，拒收就有了冒充控制转移或退运的地方", field.Name)
		}
	}

	attempt := formedAttempt(t, "attempt-1")
	refused, err := domain.FormDeliveryAttemptResult(
		attempt,
		mustValue(t, domain.NewCarriedObjectReference, "parcel-1"),
		domain.DeliveryRefused,
		mustValue(t, domain.NewAttemptResultBasisReference, "reason-refused-1"),
		attemptArrivedAt.Add(8*time.Minute),
	)
	if err != nil {
		t.Fatalf("form refused result: %v", err)
	}
	if got, present := refused.Basis(); !present || got.String() != "reason-refused-1" {
		t.Fatal("拒收丢了原因依据")
	}

	delivered := deliveredResult(t, attempt, "parcel-2")
	delivery, err := domain.FormEffectiveDelivery(attempt, delivered, deliverySpec(t, "delivery-result/v1"))
	if err != nil {
		t.Fatalf("一个对象拒收挡住了另一个对象的交付: %v", err)
	}
	if delivery.Object().String() != "parcel-2" {
		t.Fatalf("delivery object = %q", delivery.Object())
	}
}

// 派送结果与揽收结果同一条纪律：失败必须带原因、妥投不得携带失败依据、范围外与到场前
// 都进不来。
func TestDeliveryResultDisciplineMirrorsPickup(t *testing.T) {
	attempt := formedAttempt(t, "attempt-1")
	object := mustValue(t, domain.NewCarriedObjectReference, "parcel-1")
	basis := mustValue(t, domain.NewAttemptResultBasisReference, "reason-1")

	t.Run("a failure without a basis is refused", func(t *testing.T) {
		if _, err := domain.FormDeliveryAttemptResult(
			attempt, object, domain.NoOneToReceive, domain.AttemptResultBasisReference{}, attemptArrivedAt,
		); !errors.Is(err, domain.ErrInvalidDeliveryAttemptResult) {
			t.Fatalf("error = %v; 没有原因的失败与数据丢失无从分辨", err)
		}
	})

	t.Run("a delivered result carrying a failure basis is refused", func(t *testing.T) {
		if _, err := domain.FormDeliveryAttemptResult(
			attempt, object, domain.ObjectDelivered, basis, attemptArrivedAt,
		); !errors.Is(err, domain.ErrInvalidDeliveryAttemptResult) {
			t.Fatalf("error = %v; 妥投的证据走 POD，不走失败依据", err)
		}
	})

	t.Run("an object outside the scope is refused", func(t *testing.T) {
		if _, err := domain.FormDeliveryAttemptResult(
			attempt, mustValue(t, domain.NewCarriedObjectReference, "parcel-9"),
			domain.NoOneToReceive, basis, attemptArrivedAt,
		); !errors.Is(err, domain.ErrObjectOutsideAttempt) {
			t.Fatalf("error = %v, want ErrObjectOutsideAttempt", err)
		}
	})

	t.Run("a result before arrival is refused", func(t *testing.T) {
		if _, err := domain.FormDeliveryAttemptResult(
			attempt, object, domain.NoOneToReceive, basis, attemptArrivedAt.Add(-time.Minute),
		); !errors.Is(err, domain.ErrInvalidDeliveryAttemptResult) {
			t.Fatalf("error = %v; 到场之前没有可记的执行结果", err)
		}
	})

	t.Run("the delivery outcome set is closed", func(t *testing.T) {
		labels := map[string]struct{}{}
		for _, outcome := range []domain.DeliveryObjectOutcome{
			domain.ObjectDelivered, domain.DeliveryRefused, domain.NoOneToReceive, domain.WrongAddress,
		} {
			label := outcome.String()
			if label == "" {
				t.Fatalf("outcome %d has no label", outcome)
			}
			labels[label] = struct{}{}
		}
		if len(labels) != 4 {
			t.Fatalf("outcome labels collapsed into %d", len(labels))
		}
		if domain.DeliveryObjectOutcome(len(labels)+1).String() != "" {
			t.Fatal("第五个交付取值带了标签——封闭集合被悄悄放开")
		}
	})
}

// Covers: `AT-TF-072`「POD 后来被证明属于错误地址 → 保留原 POD/判断，形成更正版本」与
// CONTEXT「POD 被更正或失效时保留原证据和判断，形成新版本」——更正回指前身、原值不动；
// 沿用原版本号就是覆盖，构造期拒绝。
func TestAPODCorrectionFormsANewVersionWithoutOverwriting(t *testing.T) {
	attempt := formedAttempt(t, "attempt-1")
	original, err := domain.FormEffectiveDelivery(attempt, deliveredResult(t, attempt, "parcel-1"), deliverySpec(t, "delivery-result/v1"))
	if err != nil {
		t.Fatalf("form original delivery: %v", err)
	}

	correctedAt := attemptArrivedAt.Add(48 * time.Hour)
	corrected, err := original.CorrectProof(
		mustValue(t, domain.NewDeliveryProofReference, "pod-2"),
		mustValue(t, domain.NewDeliveryResultVersion, "delivery-result/v2"),
		correctedAt,
	)
	if err != nil {
		t.Fatalf("correct proof: %v", err)
	}
	predecessor, present := corrected.Corrects()
	if !present || predecessor.String() != "delivery-result/v1" {
		t.Fatalf("corrects = %q present=%v, want v1", predecessor, present)
	}
	if corrected.Proof().String() != "pod-2" || corrected.Version().String() != "delivery-result/v2" {
		t.Fatal("更正没有换上新证据与新版本")
	}
	if at, present := corrected.CorrectedAt(); !present || !at.Equal(correctedAt) {
		t.Fatalf("corrected at = %v present=%v", at, present)
	}
	if original.Proof().String() != "pod-1" || original.Version().String() != "delivery-result/v1" {
		t.Fatal("更正改写了原 POD 或原版本")
	}
	if _, present := original.Corrects(); present {
		t.Fatal("原版本被更正动作反向打上了更正标记")
	}

	t.Run("reusing the original version is an overwrite and is refused", func(t *testing.T) {
		if _, err := original.CorrectProof(
			mustValue(t, domain.NewDeliveryProofReference, "pod-3"),
			mustValue(t, domain.NewDeliveryResultVersion, "delivery-result/v1"),
			correctedAt,
		); !errors.Is(err, domain.ErrInvalidEffectiveDelivery) {
			t.Fatalf("error = %v; 沿用原版本号就是覆盖", err)
		}
	})

	t.Run("a correction before the delivery time is refused", func(t *testing.T) {
		if _, err := original.CorrectProof(
			mustValue(t, domain.NewDeliveryProofReference, "pod-3"),
			mustValue(t, domain.NewDeliveryResultVersion, "delivery-result/v3"),
			original.OccurredAt().Add(-time.Hour),
		); !errors.Is(err, domain.ErrInvalidEffectiveDelivery) {
			t.Fatalf("error = %v; 更正不可能发生在交付之前", err)
		}
	})
}

// Covers: `AT-TF-066`「收件人不在场，后续改约 → 第一次失败保留，第二次形成新尝试」的
// 交付半边：有效交付只在第二次尝试上形成，第一次失败结果与尝试原样保留。
func TestASecondAttemptDeliversAfterAFirstFailure(t *testing.T) {
	first := formedAttempt(t, "attempt-1")
	failed, err := domain.FormDeliveryAttemptResult(
		first,
		mustValue(t, domain.NewCarriedObjectReference, "parcel-1"),
		domain.NoOneToReceive,
		mustValue(t, domain.NewAttemptResultBasisReference, "reason-no-one-1"),
		attemptArrivedAt.Add(5*time.Minute),
	)
	if err != nil {
		t.Fatalf("form first failure: %v", err)
	}

	spec := attemptSpec(t, "attempt-2")
	spec.RescheduledFrom = first.Attempt()
	spec.PlannedFrom = attemptPlannedFrom.Add(24 * time.Hour)
	spec.PlannedTo = attemptPlannedTo.Add(24 * time.Hour)
	spec.ArrivedAt = attemptArrivedAt.Add(24 * time.Hour)
	second, err := domain.FormFulfillmentAttempt(spec)
	if err != nil {
		t.Fatalf("form second attempt: %v", err)
	}
	delivered, err := domain.FormDeliveryAttemptResult(
		second,
		mustValue(t, domain.NewCarriedObjectReference, "parcel-1"),
		domain.ObjectDelivered,
		domain.AttemptResultBasisReference{},
		spec.ArrivedAt.Add(5*time.Minute),
	)
	if err != nil {
		t.Fatalf("form second delivered result: %v", err)
	}
	delivery, err := domain.FormEffectiveDelivery(second, delivered, deliverySpec(t, "delivery-result/v1"))
	if err != nil {
		t.Fatalf("form effective delivery: %v", err)
	}
	if delivery.Attempt() != second.Attempt() {
		t.Fatal("有效交付没有锚在第二次尝试上")
	}
	if failed.Outcome() != domain.NoOneToReceive || failed.Attempt() != first.Attempt() {
		t.Fatal("第二次成功改写了第一次失败结果")
	}
}
