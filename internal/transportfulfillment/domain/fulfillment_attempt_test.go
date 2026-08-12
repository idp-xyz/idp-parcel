package domain_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

var (
	attemptPlannedFrom = time.Date(2026, 8, 9, 8, 0, 0, 0, time.UTC)
	attemptPlannedTo   = time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	attemptArrivedAt   = time.Date(2026, 8, 9, 8, 10, 0, 0, time.UTC)
)

func attemptSpec(t *testing.T, attemptID string) domain.FulfillmentAttemptSpec {
	t.Helper()
	return domain.FulfillmentAttemptSpec{
		TenantID:    mustValue(t, domain.NewTenantID, "tenant-1"),
		Attempt:     mustValue(t, domain.NewAttemptReference, attemptID),
		Task:        mustValue(t, domain.NewDispatchTaskReference, "pickup-task-1"),
		ExecutedBy:  mustValue(t, domain.NewExecutingPartyReference, "courier-1"),
		Place:       mustValue(t, domain.NewAttemptPlaceReference, "customer-warehouse-1"),
		PlannedFrom: attemptPlannedFrom,
		PlannedTo:   attemptPlannedTo,
		ArrivedAt:   attemptArrivedAt,
		Objects: []domain.CarriedObjectReference{
			mustValue(t, domain.NewCarriedObjectReference, "parcel-1"),
			mustValue(t, domain.NewCarriedObjectReference, "parcel-2"),
		},
		Evidence: mustValue(t, domain.NewAttemptEvidenceReference, "attempt-evidence-1"),
	}
}

func formedAttempt(t *testing.T, attemptID string) domain.FulfillmentAttempt {
	t.Helper()
	attempt, err := domain.FormFulfillmentAttempt(attemptSpec(t, attemptID))
	if err != nil {
		t.Fatalf("form fulfillment attempt: %v", err)
	}
	return attempt
}

// Covers: CONTEXT「履约尝试」词条与「尝试必须保存计划窗口、实际到场、执行方、地点、
// 对象范围、证据和失败原因」——缺任一必备件都形成不了尝试；对象范围不空不重。
func TestAnAttemptKeepsItsWindowScopeAndEvidence(t *testing.T) {
	attempt := formedAttempt(t, "attempt-1")
	if !attempt.PlannedFrom().Equal(attemptPlannedFrom) || !attempt.PlannedTo().Equal(attemptPlannedTo) {
		t.Fatalf("planned window = %s..%s", attempt.PlannedFrom(), attempt.PlannedTo())
	}
	if !attempt.ArrivedAt().Equal(attemptArrivedAt) {
		t.Fatalf("arrived at = %s", attempt.ArrivedAt())
	}
	if got := attempt.Objects(); len(got) != 2 {
		t.Fatalf("objects = %d, want 2（对象范围随尝试保全）", len(got))
	}
	if !attempt.Covers(mustValue(t, domain.NewCarriedObjectReference, "parcel-2")) {
		t.Fatal("范围内对象查不回来")
	}
	if attempt.Covers(mustValue(t, domain.NewCarriedObjectReference, "parcel-9")) {
		t.Fatal("范围外对象被读成在范围内")
	}
	if _, rescheduled := attempt.RescheduledFrom(); rescheduled {
		t.Fatal("首次尝试凭空带上了改约前身")
	}

	broken := map[string]func(*domain.FulfillmentAttemptSpec){
		"no objects":       func(spec *domain.FulfillmentAttemptSpec) { spec.Objects = nil },
		"duplicate object": func(spec *domain.FulfillmentAttemptSpec) { spec.Objects = append(spec.Objects, spec.Objects[0]) },
		"no evidence":      func(spec *domain.FulfillmentAttemptSpec) { spec.Evidence = domain.AttemptEvidenceReference{} },
		"no window":        func(spec *domain.FulfillmentAttemptSpec) { spec.PlannedFrom = time.Time{} },
		"window inverted":  func(spec *domain.FulfillmentAttemptSpec) { spec.PlannedTo = spec.PlannedFrom.Add(-time.Hour) },
		"no arrival":       func(spec *domain.FulfillmentAttemptSpec) { spec.ArrivedAt = time.Time{} },
		"no executor":      func(spec *domain.FulfillmentAttemptSpec) { spec.ExecutedBy = domain.ExecutingPartyReference{} },
		"no place":         func(spec *domain.FulfillmentAttemptSpec) { spec.Place = domain.AttemptPlaceReference{} },
		"no task":          func(spec *domain.FulfillmentAttemptSpec) { spec.Task = domain.DispatchTaskReference{} },
	}
	for name, breakSpec := range broken {
		t.Run(name, func(t *testing.T) {
			spec := attemptSpec(t, "attempt-x")
			breakSpec(&spec)
			if _, err := domain.FormFulfillmentAttempt(spec); !errors.Is(err, domain.ErrInvalidFulfillmentAttempt) {
				t.Fatalf("error = %v, want ErrInvalidFulfillmentAttempt", err)
			}
		})
	}
}

// Covers: CONTEXT「改约或重派形成新尝试，不覆盖旧尝试」「改约、再次揽收或重新派送不能
// 重开或覆盖旧尝试」——新尝试自有身份并回指前身；顶替旧身份在构造期被拒；旧尝试值原样保留。
func TestARescheduleFormsANewAttemptWithoutOverwritingTheOld(t *testing.T) {
	first := formedAttempt(t, "attempt-1")

	spec := attemptSpec(t, "attempt-2")
	spec.ArrivedAt = attemptArrivedAt.Add(24 * time.Hour)
	spec.PlannedFrom = attemptPlannedFrom.Add(24 * time.Hour)
	spec.PlannedTo = attemptPlannedTo.Add(24 * time.Hour)
	spec.RescheduledFrom = first.Attempt()
	second, err := domain.FormFulfillmentAttempt(spec)
	if err != nil {
		t.Fatalf("form rescheduled attempt: %v", err)
	}

	predecessor, rescheduled := second.RescheduledFrom()
	if !rescheduled || predecessor != first.Attempt() {
		t.Fatalf("rescheduled from = %q present=%v, want attempt-1", predecessor, rescheduled)
	}
	if second.Attempt() == first.Attempt() {
		t.Fatal("新尝试共用了旧身份")
	}
	if !first.ArrivedAt().Equal(attemptArrivedAt) || first.Attempt().String() != "attempt-1" {
		t.Fatal("形成新尝试改写了旧尝试")
	}

	t.Run("reusing the old identity is refused", func(t *testing.T) {
		hijack := attemptSpec(t, "attempt-1")
		hijack.RescheduledFrom = first.Attempt()
		if _, err := domain.FormFulfillmentAttempt(hijack); !errors.Is(err, domain.ErrInvalidFulfillmentAttempt) {
			t.Fatalf("error = %v; 改约顶替旧身份等于重开旧尝试", err)
		}
	})
}

// Covers: CONTEXT「一个任务和一次尝试可以覆盖多个载运对象，但每个对象必须分别保存……
// 结果」——结果只能落在尝试的对象范围内，且不得早于实际到场。
func TestPerObjectResultsStayWithinTheAttemptScope(t *testing.T) {
	attempt := formedAttempt(t, "attempt-1")
	basis := mustValue(t, domain.NewAttemptResultBasisReference, "reason-customer-absent-1")

	t.Run("an in-scope failure records its reason", func(t *testing.T) {
		result, err := domain.FormAttemptObjectResult(
			attempt,
			mustValue(t, domain.NewCarriedObjectReference, "parcel-1"),
			domain.CustomerAbsent,
			basis,
			attemptArrivedAt.Add(5*time.Minute),
		)
		if err != nil {
			t.Fatalf("form result: %v", err)
		}
		if result.Attempt() != attempt.Attempt() {
			t.Fatal("结果没有锚在这次尝试上")
		}
		if got, present := result.Basis(); !present || got != basis {
			t.Fatal("失败原因依据没有随结果保全")
		}
	})

	t.Run("an object outside the scope is refused", func(t *testing.T) {
		_, err := domain.FormAttemptObjectResult(
			attempt,
			mustValue(t, domain.NewCarriedObjectReference, "parcel-9"),
			domain.CustomerAbsent,
			basis,
			attemptArrivedAt.Add(5*time.Minute),
		)
		if !errors.Is(err, domain.ErrObjectOutsideAttempt) {
			t.Fatalf("error = %v, want ErrObjectOutsideAttempt", err)
		}
		if errors.Is(err, domain.ErrInvalidAttemptObjectResult) {
			t.Fatal("范围外对象被压成了形状错误")
		}
	})

	t.Run("a result before arrival is refused", func(t *testing.T) {
		if _, err := domain.FormAttemptObjectResult(
			attempt,
			mustValue(t, domain.NewCarriedObjectReference, "parcel-1"),
			domain.CustomerAbsent,
			basis,
			attemptArrivedAt.Add(-time.Minute),
		); !errors.Is(err, domain.ErrInvalidAttemptObjectResult) {
			t.Fatalf("error = %v; 到场之前没有可记的执行结果", err)
		}
	})
}

// Covers: CONTEXT「客户不在、货物未备好、包装不合格或其他失败结果不制造实际履约段」——
// 结构防线：结果类型上没有任何控制或履约段字段可以冒充有效收寄的控制依据（那份依据只在
// OffsitePickup 上，见 TestAFailedVisitCannotBecomeAPickup 的另一半）。失败必须带原因依据，
// 成功不得携带失败依据。
func TestAFailureCarriesItsReasonAndBuildsNoSegment(t *testing.T) {
	attempt := formedAttempt(t, "attempt-1")
	object := mustValue(t, domain.NewCarriedObjectReference, "parcel-1")
	basis := mustValue(t, domain.NewAttemptResultBasisReference, "reason-packaging-1")

	resultType := reflect.TypeOf(domain.AttemptObjectResult{})
	for index := 0; index < resultType.NumField(); index++ {
		field := resultType.Field(index)
		name := strings.ToLower(field.Name)
		if strings.Contains(name, "control") || strings.Contains(name, "segment") || strings.Contains(name, "leg") {
			t.Fatalf("AttemptObjectResult 携带 %q，失败结果就有了冒充运输控制的地方", field.Name)
		}
		if field.Type == reflect.TypeOf(domain.TransportControlReference{}) {
			t.Fatalf("AttemptObjectResult 的 %q 是控制依据类型，失败结果不得制造履约段", field.Name)
		}
	}

	for name, outcome := range map[string]domain.AttemptObjectOutcome{
		"customer absent":        domain.CustomerAbsent,
		"goods not ready":        domain.GoodsNotReady,
		"packaging unacceptable": domain.PackagingUnacceptable,
	} {
		t.Run(name+" without a basis is refused", func(t *testing.T) {
			if _, err := domain.FormAttemptObjectResult(
				attempt, object, outcome, domain.AttemptResultBasisReference{}, attemptArrivedAt,
			); !errors.Is(err, domain.ErrInvalidAttemptObjectResult) {
				t.Fatalf("error = %v; 没有原因的失败与数据丢失无从分辨", err)
			}
		})
	}

	t.Run("a success carrying a failure basis is refused", func(t *testing.T) {
		if _, err := domain.FormAttemptObjectResult(
			attempt, object, domain.ObjectPickedUp, basis, attemptArrivedAt,
		); !errors.Is(err, domain.ErrInvalidAttemptObjectResult) {
			t.Fatalf("error = %v; 揽收证据在 OffsitePickup 上，这里塞依据只会两处打架", err)
		}
	})
}

// Covers: CONTEXT「一次失败尝试不自动结束任务、实际履约段或客户服务，也不自动创建退运」
// ——一个对象失败后：同尝试内另一对象照常记成功；同任务照常安排新尝试且旧尝试保留。
// 是否重试、改约、终止或退运由适用规则另行决定，结果与尝试上没有替它们作答的字段。
func TestOneFailureEndsNothingAutomatically(t *testing.T) {
	attempt := formedAttempt(t, "attempt-1")
	failed, err := domain.FormAttemptObjectResult(
		attempt,
		mustValue(t, domain.NewCarriedObjectReference, "parcel-1"),
		domain.GoodsNotReady,
		mustValue(t, domain.NewAttemptResultBasisReference, "reason-not-ready-1"),
		attemptArrivedAt.Add(3*time.Minute),
	)
	if err != nil {
		t.Fatalf("form failed result: %v", err)
	}
	if !failed.Outcome().Failed() {
		t.Fatalf("outcome = %q classified as non-failure", failed.Outcome())
	}

	succeeded, err := domain.FormAttemptObjectResult(
		attempt,
		mustValue(t, domain.NewCarriedObjectReference, "parcel-2"),
		domain.ObjectPickedUp,
		domain.AttemptResultBasisReference{},
		attemptArrivedAt.Add(4*time.Minute),
	)
	if err != nil {
		t.Fatalf("一个对象失败挡住了另一个对象的成功: %v", err)
	}
	if !succeeded.Outcome().Succeeded() {
		t.Fatalf("outcome = %q, want PICKED_UP", succeeded.Outcome())
	}

	retry := attemptSpec(t, "attempt-2")
	retry.RescheduledFrom = attempt.Attempt()
	retry.PlannedFrom = attemptPlannedFrom.Add(24 * time.Hour)
	retry.PlannedTo = attemptPlannedTo.Add(24 * time.Hour)
	retry.ArrivedAt = attemptArrivedAt.Add(24 * time.Hour)
	if _, err := domain.FormFulfillmentAttempt(retry); err != nil {
		t.Fatalf("一次失败挡住了新尝试的安排: %v", err)
	}
	if attempt.Attempt().String() != "attempt-1" || len(attempt.Objects()) != 2 {
		t.Fatal("安排新尝试改写了旧尝试")
	}

	t.Run("the outcome set is closed", func(t *testing.T) {
		labels := map[string]struct{}{}
		for _, outcome := range []domain.AttemptObjectOutcome{
			domain.ObjectPickedUp, domain.CustomerAbsent, domain.GoodsNotReady, domain.PackagingUnacceptable,
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
		if domain.AttemptObjectOutcome(len(labels)+1).String() != "" {
			t.Fatal("第五个结果取值带了标签——封闭集合被悄悄放开")
		}
	})
}
