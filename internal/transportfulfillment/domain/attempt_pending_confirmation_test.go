package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件钉履约尝试逐对象结果的「待确认」与揽收的「执行方拒收」两格（票 docs-consistency/code/01）。夹具全为合成值（S 级）。

func pendingPickupResult(t *testing.T, attempt domain.FulfillmentAttempt, object string) domain.AttemptObjectResult {
	t.Helper()
	result, err := domain.FormPendingAttemptObjectResult(
		attempt,
		mustValue(t, domain.NewCarriedObjectReference, object),
		mustValue(t, domain.NewAttemptResultSourceReference, "source/vehicle-arrival+driver-scan"),
		mustValue(t, domain.NewAttemptResultBasisReference, "gap/no-receipt-or-control-evidence"),
		attemptArrivedAt.Add(4*time.Minute),
	)
	if err != nil {
		t.Fatalf("form pending pickup result: %v", err)
	}
	return result
}

// Covers: `AT-TF-018`「只有车辆到场和司机扫描 → 保存来源，结果待确认，不推导实际接收」的领域半边——待确认一格带着
// 已有来源与缺口成立，既不是揽收到手、也不是失败：任务汇总把它读作未结束，失败尝试费也不能拿它作事实依据（不默认
// 成功、不默认失败）。结构上它与失败结果同一个类型，没有控制或履约段字段（TestAFailureCarriesItsReasonAndBuildsNoSegment）。
func TestAPendingPickupResultKeepsItsSourceAndGapAndConcludesNothing(t *testing.T) {
	attempt := formedAttempt(t, "attempt-1")
	result := pendingPickupResult(t, attempt, "parcel-1")

	if result.Outcome() != domain.PickupPendingConfirmation || result.Outcome().String() != "PENDING_CONFIRMATION" {
		t.Fatalf("outcome = %q, want PENDING_CONFIRMATION", result.Outcome())
	}
	if source, present := result.Source(); !present || source.String() != "source/vehicle-arrival+driver-scan" {
		t.Fatalf("source = %q present=%v；待确认丢了所凭的来源", source, present)
	}
	if gap, present := result.Basis(); !present || gap.String() != "gap/no-receipt-or-control-evidence" {
		t.Fatalf("basis = %q present=%v；待确认丢了缺口", gap, present)
	}
	if !result.Outcome().PendingConfirmation() {
		t.Fatal("待确认一格答不出自己是待确认")
	}
	if result.Outcome().Succeeded() {
		t.Fatal("待确认被读成揽收到手：来源不足以证明接收与控制")
	}
	if result.Outcome().Failed() {
		t.Fatal("待确认被读成已结束的失败：汇总把它当成确定结果，失败尝试费就有了不存在的事实依据")
	}
	if _, err := domain.FailedPickupAttemptBasis(result); !errors.Is(err, domain.ErrAttemptResultPendingConfirmation) {
		t.Fatalf("failed attempt basis error = %v, want ErrAttemptResultPendingConfirmation", err)
	}

	t.Run("the general constructor does not take a pending outcome", func(t *testing.T) {
		if _, err := domain.FormAttemptObjectResult(
			attempt,
			mustValue(t, domain.NewCarriedObjectReference, "parcel-1"),
			domain.PickupPendingConfirmation,
			mustValue(t, domain.NewAttemptResultBasisReference, "gap/1"),
			attemptArrivedAt,
		); !errors.Is(err, domain.ErrInvalidAttemptObjectResult) {
			t.Fatalf("error = %v；那一条构造器不收来源，从它进来的待确认就是一格没有来源的待确认", err)
		}
	})

	broken := map[string]struct {
		source domain.AttemptResultSourceReference
		gap    domain.AttemptResultBasisReference
		object string
		at     time.Time
		want   error
	}{
		"no source": {
			gap: mustValue(t, domain.NewAttemptResultBasisReference, "gap/1"), object: "parcel-1",
			at: attemptArrivedAt, want: domain.ErrInvalidAttemptObjectResult,
		},
		"no gap": {
			source: mustValue(t, domain.NewAttemptResultSourceReference, "source/1"), object: "parcel-1",
			at: attemptArrivedAt, want: domain.ErrInvalidAttemptObjectResult,
		},
		"before arrival": {
			source: mustValue(t, domain.NewAttemptResultSourceReference, "source/1"),
			gap:    mustValue(t, domain.NewAttemptResultBasisReference, "gap/1"), object: "parcel-1",
			at: attemptArrivedAt.Add(-time.Minute), want: domain.ErrInvalidAttemptObjectResult,
		},
		"outside the attempt": {
			source: mustValue(t, domain.NewAttemptResultSourceReference, "source/1"),
			gap:    mustValue(t, domain.NewAttemptResultBasisReference, "gap/1"), object: "parcel-9",
			at: attemptArrivedAt, want: domain.ErrObjectOutsideAttempt,
		},
	}
	for name, tc := range broken {
		t.Run(name, func(t *testing.T) {
			_, err := domain.FormPendingAttemptObjectResult(
				attempt, mustValue(t, domain.NewCarriedObjectReference, tc.object), tc.source, tc.gap, tc.at)
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}

	t.Run("determinate results carry no source", func(t *testing.T) {
		failed, err := domain.FormAttemptObjectResult(
			attempt,
			mustValue(t, domain.NewCarriedObjectReference, "parcel-2"),
			domain.CustomerAbsent,
			mustValue(t, domain.NewAttemptResultBasisReference, "reason-absent-1"),
			attemptArrivedAt,
		)
		if err != nil {
			t.Fatalf("form failed result: %v", err)
		}
		if _, present := failed.Source(); present {
			t.Fatal("确定结果长出了待确认的来源一格")
		}
	})
}

// Covers: `AT-TF-017`「包装不合格且执行方明确拒收 → 保存对象级拒收和证据，控制不转移」的领域半边——执行方拒收是
// 独立一格，不借包装不合格那一格；它带原因与证据依据，是本次尝试的确定结果（与派送一侧收件人拒收同一族：可作失败
// 尝试费的事实依据），结构上没有控制字段可以冒充转移。
func TestAnExecutorRefusalIsItsOwnResultAndCarriesItsReason(t *testing.T) {
	attempt := formedAttempt(t, "attempt-1")
	object := mustValue(t, domain.NewCarriedObjectReference, "parcel-1")
	reason := mustValue(t, domain.NewAttemptResultBasisReference, "reason/packaging-unacceptable+photo-1")

	refused, err := domain.FormAttemptObjectResult(attempt, object, domain.ExecutorRefused, reason, attemptArrivedAt.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("form refusal: %v", err)
	}
	if refused.Outcome().String() != "EXECUTOR_REFUSED" {
		t.Fatalf("word = %q, want EXECUTOR_REFUSED", refused.Outcome())
	}
	if refused.Outcome() == domain.PackagingUnacceptable {
		t.Fatal("执行方拒收并进了包装不合格一格")
	}
	if got, present := refused.Basis(); !present || got != reason {
		t.Fatal("拒收丢了原因与证据依据")
	}
	if refused.Outcome().Succeeded() || refused.Outcome().PendingConfirmation() {
		t.Fatalf("outcome = %q 被读成揽收到手或待确认", refused.Outcome())
	}
	if !refused.Outcome().Failed() {
		t.Fatal("拒收不是本次尝试的确定未成功——失败尝试费那一侧就认不出它")
	}
	if _, err := domain.FailedPickupAttemptBasis(refused); err != nil {
		t.Fatalf("failed attempt basis: %v；拒收与收件人拒收同一族，可作事实依据", err)
	}
	if _, err := domain.FormAttemptObjectResult(
		attempt, object, domain.ExecutorRefused, domain.AttemptResultBasisReference{}, attemptArrivedAt,
	); !errors.Is(err, domain.ErrInvalidAttemptObjectResult) {
		t.Fatalf("error = %v；没有原因的拒收与数据丢失无从分辨", err)
	}
}

func pendingDeliveryResult(t *testing.T, attempt domain.FulfillmentAttempt, object string) domain.DeliveryAttemptResult {
	t.Helper()
	result, err := domain.FormPendingDeliveryAttemptResult(
		attempt,
		mustValue(t, domain.NewCarriedObjectReference, object),
		mustValue(t, domain.NewAttemptResultSourceReference, "source/signature-scan+address-geofence"),
		mustValue(t, domain.NewAttemptResultBasisReference, "conflict/signature-says-recipient-address-says-elsewhere"),
		attemptArrivedAt.Add(9*time.Minute),
	)
	if err != nil {
		t.Fatalf("form pending delivery result: %v", err)
	}
	return result
}

// Covers: `AT-TF-070`「本人签收和地址来源冲突 → 形成冲突或待确认，不按最后消息覆盖」的领域半边——派送一侧的待确认
// 带全部来源与冲突成立；它登不出有效交付（控制不转移、参与不结束），也不是失败（不是失败尝试费的事实依据）。
func TestAPendingDeliveryResultIsNeitherADeliveryNorAFailure(t *testing.T) {
	attempt := formedAttempt(t, "attempt-1")
	result := pendingDeliveryResult(t, attempt, "parcel-1")

	if result.Outcome() != domain.DeliveryPendingConfirmation || result.Outcome().String() != "PENDING_CONFIRMATION" {
		t.Fatalf("outcome = %q, want PENDING_CONFIRMATION", result.Outcome())
	}
	if source, present := result.Source(); !present || source.String() != "source/signature-scan+address-geofence" {
		t.Fatalf("source = %q present=%v；待确认丢了全部来源", source, present)
	}
	if conflict, present := result.Basis(); !present || conflict.String() != "conflict/signature-says-recipient-address-says-elsewhere" {
		t.Fatalf("basis = %q present=%v；待确认丢了冲突", conflict, present)
	}
	if !result.Outcome().PendingConfirmation() || result.Outcome().Succeeded() || result.Outcome().Failed() {
		t.Fatalf("outcome = %q：待确认只能是待确认，不是妥投也不是已结束的失败", result.Outcome())
	}
	_, err := domain.FormEffectiveDelivery(attempt, result, deliverySpec(t, "delivery-result/v1"))
	if !errors.Is(err, domain.ErrNotAnEffectiveDelivery) {
		t.Fatalf("effective delivery error = %v, want ErrNotAnEffectiveDelivery——证据不足不构成有效交付，控制不转移", err)
	}
	if !errors.Is(err, domain.ErrDeliveryResultPendingConfirmation) {
		t.Fatalf("effective delivery error = %v, want 可认作 ErrDeliveryResultPendingConfirmation——续办与失败不同，要分得出", err)
	}
	failed, formErr := domain.FormDeliveryAttemptResult(attempt, mustValue(t, domain.NewCarriedObjectReference, "parcel-2"),
		domain.NoOneToReceive, mustValue(t, domain.NewAttemptResultBasisReference, "reason-no-one-1"), attemptArrivedAt.Add(time.Minute))
	if formErr != nil {
		t.Fatalf("form failed result: %v", formErr)
	}
	if _, err := domain.FormEffectiveDelivery(attempt, failed, deliverySpec(t, "delivery-result/v2")); errors.Is(err, domain.ErrDeliveryResultPendingConfirmation) {
		t.Fatal("无人签收被认作待确认")
	}
	if _, err := domain.FailedDeliveryAttemptBasis(result); !errors.Is(err, domain.ErrAttemptResultPendingConfirmation) {
		t.Fatalf("failed attempt basis error = %v, want ErrAttemptResultPendingConfirmation", err)
	}

	if _, err := domain.FormDeliveryAttemptResult(
		attempt,
		mustValue(t, domain.NewCarriedObjectReference, "parcel-1"),
		domain.DeliveryPendingConfirmation,
		mustValue(t, domain.NewAttemptResultBasisReference, "conflict/1"),
		attemptArrivedAt,
	); !errors.Is(err, domain.ErrInvalidDeliveryAttemptResult) {
		t.Fatalf("error = %v；那一条构造器不收来源", err)
	}
	if _, err := domain.FormPendingDeliveryAttemptResult(
		attempt,
		mustValue(t, domain.NewCarriedObjectReference, "parcel-1"),
		domain.AttemptResultSourceReference{},
		mustValue(t, domain.NewAttemptResultBasisReference, "conflict/1"),
		attemptArrivedAt,
	); !errors.Is(err, domain.ErrInvalidDeliveryAttemptResult) {
		t.Fatalf("error = %v；没有来源的待确认说不出凭什么待确认", err)
	}
	if _, err := domain.FormPendingDeliveryAttemptResult(
		attempt,
		mustValue(t, domain.NewCarriedObjectReference, "parcel-1"),
		mustValue(t, domain.NewAttemptResultSourceReference, "source/1"),
		domain.AttemptResultBasisReference{},
		attemptArrivedAt,
	); !errors.Is(err, domain.ErrInvalidDeliveryAttemptResult) {
		t.Fatalf("error = %v；没有冲突或缺口的待确认说不出还差什么", err)
	}
	if _, err := domain.FormPendingDeliveryAttemptResult(
		attempt,
		mustValue(t, domain.NewCarriedObjectReference, "parcel-9"),
		mustValue(t, domain.NewAttemptResultSourceReference, "source/1"),
		mustValue(t, domain.NewAttemptResultBasisReference, "conflict/1"),
		attemptArrivedAt,
	); !errors.Is(err, domain.ErrObjectOutsideAttempt) {
		t.Fatalf("error = %v, want ErrObjectOutsideAttempt", err)
	}
	if delivered := deliveredResult(t, attempt, "parcel-2"); func() bool { _, present := delivered.Source(); return present }() {
		t.Fatal("妥投长出了待确认的来源一格")
	}
}

// 来源引用与其余必备值同一道门：空白与含 NUL 都拒（登记册 text 列存不下 NUL）。
func TestAnAttemptResultSourceIsARequiredValue(t *testing.T) {
	for _, raw := range []string{"", "   ", "SYN-SOURCE\x00"} {
		if _, err := domain.NewAttemptResultSourceReference(raw); err == nil {
			t.Fatalf("source %q accepted", raw)
		}
	}
}

// 揽收尝试口的比对在 TFC-1 与无版本两形之上认 TFC-3（带待确认来源那一形，ADR-0267 决定一、ADR-0209 决定三）：同形同串
// 是重放；在册那一版带来源而这一份没有，或这一份带来源而在册的是没有来源一格的旧形，都是冲突——旧形之下来源没有写法，
// 不按丢掉来源认作重放。认不出的前缀照旧答认不出。
func TestStoredPickupAttemptDigestComparesThePendingSourceShape(t *testing.T) {
	const unversioned = "bb"
	cases := []struct {
		name            string
		stored, current string
		want            domain.StoredDigestComparison
	}{
		{"TFC-3 same content", "TFC-3:aa", "TFC-3:aa", domain.SamePayload},
		{"TFC-3 different content", "TFC-3:aa", "TFC-3:cc", domain.DifferentPayload},
		{"stored carries a source, this one none", "TFC-3:aa", "TFC-1:aa", domain.DifferentPayload},
		{"TFC-1 row, this one carries a source", "TFC-1:aa", "TFC-3:aa", domain.DifferentPayload},
		{"unversioned row, this one carries a source", "bb", "TFC-3:aa", domain.DifferentPayload},
		{"TFC-1 row replayed without a source", "TFC-1:aa", "TFC-1:aa", domain.SamePayload},
		{"unversioned row replayed without a source", "bb", "TFC-1:aa", domain.SamePayload},
		{"the handover node shape is not a pickup shape", "TFC-2:aa", "TFC-1:aa", domain.UnknownPayloadShape},
		{"unknown shape stays unknown", "TFC-9:aa", "TFC-3:aa", domain.UnknownPayloadShape},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := domain.CompareStoredPickupAttemptDigest(testCase.stored, testCase.current, unversioned); got != testCase.want {
				t.Fatalf("CompareStoredPickupAttemptDigest(%q, %q) = %d, want %d", testCase.stored, testCase.current, got, testCase.want)
			}
		})
	}
}
