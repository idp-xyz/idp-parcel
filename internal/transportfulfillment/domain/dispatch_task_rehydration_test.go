package domain_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

func rehydrateTaskSpec(t *testing.T) domain.RehydrateDispatchTaskSpec {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	task, err := domain.NewDispatchTaskReference("dispatch-task-1")
	if err != nil {
		t.Fatalf("task: %v", err)
	}
	place, err := domain.NewAttemptPlaceReference("customer-warehouse-1")
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	conditions, err := domain.NewServiceConditionReference("service-condition/v1")
	if err != nil {
		t.Fatalf("conditions: %v", err)
	}
	object, err := domain.NewCarriedObjectReference("parcel-1")
	if err != nil {
		t.Fatalf("object: %v", err)
	}
	opened := time.Date(2026, 8, 14, 7, 30, 0, 0, time.UTC)
	return domain.RehydrateDispatchTaskSpec{
		TenantID:   tenant,
		Task:       task,
		Kind:       domain.PickupDispatch,
		Objects:    []domain.CarriedObjectReference{object},
		Place:      place,
		WindowFrom: opened.Add(90 * time.Minute),
		WindowTo:   opened.Add(4 * time.Hour),
		Conditions: conditions,
		OpenedAt:   opened,
		State:      domain.TaskOpen,
	}
}

// 库面装回一项开放任务：逐格取值原样回来，且改约次数不因往返丢失（它是任务身上的计数，
// 不是从别处派生的）。
func TestRehydratingAnOpenDispatchTask(t *testing.T) {
	spec := rehydrateTaskSpec(t)
	spec.Reschedules = 2

	task, err := domain.RehydrateDispatchTask(spec)
	if err != nil {
		t.Fatalf("装回：%v", err)
	}
	if task.State() != domain.TaskOpen {
		t.Fatalf("状态 = %q, want OPEN", task.State())
	}
	if task.Reschedules() != 2 {
		t.Fatalf("改约次数 = %d, want 2", task.Reschedules())
	}
	if _, _, closed := task.Closure(); closed {
		t.Fatal("开放任务装回来带了关闭三件")
	}
}

// 关闭三件与状态成组：半截的行装不回来。
//
// **没有依据的关闭与「一次失败尝试自动结束任务」在库里分不开**，而后者正是 CONTEXT 明禁的
// 那条——所以这里拒绝，不是补一个占位依据。
func TestARehydratedClosureNeedsItsBasisAndTime(t *testing.T) {
	basis, err := domain.NewTaskClosureBasisReference("closure-basis/v1")
	if err != nil {
		t.Fatalf("basis: %v", err)
	}
	closedAt := time.Date(2026, 8, 14, 13, 0, 0, 0, time.UTC)

	cases := map[string]func(*domain.RehydrateDispatchTaskSpec){
		"已完成却没有依据": func(spec *domain.RehydrateDispatchTaskSpec) {
			spec.State, spec.ClosedAt = domain.TaskCompleted, closedAt
		},
		"已终止却没有时刻": func(spec *domain.RehydrateDispatchTaskSpec) {
			spec.State, spec.ClosureBasis = domain.TaskTerminated, basis
		},
		"开放却带着关闭三件": func(spec *domain.RehydrateDispatchTaskSpec) {
			spec.ClosureBasis, spec.ClosedAt = basis, closedAt
		},
		"关闭早于建立": func(spec *domain.RehydrateDispatchTaskSpec) {
			spec.State, spec.ClosureBasis = domain.TaskCompleted, basis
			spec.ClosedAt = spec.OpenedAt.Add(-time.Hour)
		},
	}

	for name, breakIt := range cases {
		t.Run(name, func(t *testing.T) {
			spec := rehydrateTaskSpec(t)
			breakIt(&spec)
			if _, err := domain.RehydrateDispatchTask(spec); err == nil {
				t.Fatal("坏行被装了回来")
			}
		})
	}
}

// 空对象集不是「空任务」是坏行：任务由工作范围成立，没有对象的任务从来不曾成立过。
// 同一对象两条也是行上就看得出的坏。
func TestARehydratedTaskNeedsADistinctNonEmptyScope(t *testing.T) {
	object, err := domain.NewCarriedObjectReference("parcel-1")
	if err != nil {
		t.Fatalf("object: %v", err)
	}

	empty := rehydrateTaskSpec(t)
	empty.Objects = nil
	if _, err := domain.RehydrateDispatchTask(empty); err == nil {
		t.Fatal("空对象集被装了回来")
	}

	duplicated := rehydrateTaskSpec(t)
	duplicated.Objects = []domain.CarriedObjectReference{object, object}
	if _, err := domain.RehydrateDispatchTask(duplicated); err == nil {
		t.Fatal("同一对象两条被装了回来")
	}
}

// 已关闭的任务装回来之后仍然关着：装回不是重开。改约与再次关闭都该被领域挡住。
func TestARehydratedClosedTaskStaysClosed(t *testing.T) {
	spec := rehydrateTaskSpec(t)
	basis, err := domain.NewTaskClosureBasisReference("closure-basis/v1")
	if err != nil {
		t.Fatalf("basis: %v", err)
	}
	spec.State = domain.TaskTerminated
	spec.ClosureBasis = basis
	spec.ClosedAt = spec.OpenedAt.Add(6 * time.Hour)

	task, err := domain.RehydrateDispatchTask(spec)
	if err != nil {
		t.Fatalf("装回：%v", err)
	}
	if _, _, closed := task.Closure(); !closed {
		t.Fatal("已关闭的任务装回来却报不出关闭三件")
	}
	if _, err := task.Reschedule(spec.WindowFrom, spec.WindowTo, spec.ClosedAt); err == nil {
		t.Fatal("装回来的已关闭任务还能改约——装回变成了重开")
	}
	if _, err := task.Complete(basis, spec.ClosedAt); err == nil {
		t.Fatal("装回来的已关闭任务还能再关一次")
	}
}
