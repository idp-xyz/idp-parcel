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
	taskOpenedAt   = time.Date(2026, 8, 13, 7, 0, 0, 0, time.UTC)
	taskWindowFrom = time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)
	taskWindowTo   = time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
)

func dispatchTaskSpec(t *testing.T) domain.DispatchTaskSpec {
	t.Helper()
	return domain.DispatchTaskSpec{
		TenantID: mustValue(t, domain.NewTenantID, "tenant-1"),
		Task:     mustValue(t, domain.NewDispatchTaskReference, "pickup-task-1"),
		Kind:     domain.PickupDispatch,
		Objects: []domain.CarriedObjectReference{
			mustValue(t, domain.NewCarriedObjectReference, "parcel-1"),
			mustValue(t, domain.NewCarriedObjectReference, "parcel-2"),
		},
		Place:      mustValue(t, domain.NewAttemptPlaceReference, "customer-warehouse-1"),
		WindowFrom: taskWindowFrom,
		WindowTo:   taskWindowTo,
		Conditions: mustValue(t, domain.NewServiceConditionReference, "service-condition-1"),
		OpenedAt:   taskOpenedAt,
	}
}

func openedTask(t *testing.T) domain.DispatchTask {
	t.Helper()
	task, err := domain.OpenDispatchTask(dispatchTaskSpec(t))
	if err != nil {
		t.Fatalf("open dispatch task: %v", err)
	}
	return task
}

// Covers: CONTEXT「揽派任务」词条「在明确地点、时间范围和服务条件下对一个或多个载运
// 对象执行场外揽收或末端派送的工作范围」与硬句「任务表达需要完成什么，不等于已经到场、
// 取得控制或完成交付」——类型上没有到场/控制/交付字段（那些在尝试、揽收与交付对象上）。
func TestATaskExpressesWorkNotArrival(t *testing.T) {
	taskType := reflect.TypeOf(domain.DispatchTask{})
	for index := 0; index < taskType.NumField(); index++ {
		name := strings.ToLower(taskType.Field(index).Name)
		for _, forbidden := range []string{"arrived", "control", "delivered", "attempt", "pickup"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("DispatchTask 携带 %q——任务就能被读成已到场或已交付", taskType.Field(index).Name)
			}
		}
	}

	task := openedTask(t)
	// 计数锚定本夹具：两对象（parcel-1/parcel-2，窗口 2026-08-13T09:00–12:00Z）。
	if len(task.Objects()) != 2 || task.State() != domain.TaskOpen {
		t.Fatalf("objects = %d state = %q", len(task.Objects()), task.State())
	}
	if task.Kind() != domain.PickupDispatch {
		t.Fatalf("kind = %q", task.Kind())
	}

	broken := map[string]func(*domain.DispatchTaskSpec){
		"no objects":       func(spec *domain.DispatchTaskSpec) { spec.Objects = nil },
		"duplicate object": func(spec *domain.DispatchTaskSpec) { spec.Objects = append(spec.Objects, spec.Objects[0]) },
		"window inverted":  func(spec *domain.DispatchTaskSpec) { spec.WindowTo = spec.WindowFrom.Add(-time.Hour) },
		"no place":         func(spec *domain.DispatchTaskSpec) { spec.Place = domain.AttemptPlaceReference{} },
		"no conditions":    func(spec *domain.DispatchTaskSpec) { spec.Conditions = domain.ServiceConditionReference{} },
		"no kind":          func(spec *domain.DispatchTaskSpec) { spec.Kind = domain.DispatchTaskKindInvalid },
	}
	for name, breakSpec := range broken {
		t.Run(name, func(t *testing.T) {
			spec := dispatchTaskSpec(t)
			breakSpec(&spec)
			if _, err := domain.OpenDispatchTask(spec); !errors.Is(err, domain.ErrInvalidDispatchTask) {
				t.Fatalf("error = %v, want ErrInvalidDispatchTask", err)
			}
		})
	}

	t.Run("the kind set is closed", func(t *testing.T) {
		if domain.PickupDispatch.String() != "PICKUP" || domain.DeliveryDispatch.String() != "DELIVERY" {
			t.Fatal("二值标签不对")
		}
		if domain.DispatchTaskKind(3).String() != "" {
			t.Fatal("第三个任务种类带了标签——封闭集合被悄悄放开")
		}
	})
}

// Covers: CONTEXT「改约、再次揽收或重新派送不能重开或覆盖旧尝试」的任务半边——改约
// 换窗口可多次，任务身份与对象集不变（新的是**尝试**，不是任务）；已关闭任务改不了约。
func TestRescheduleKeepsTheTaskIdentity(t *testing.T) {
	task := openedTask(t)

	first, err := task.Reschedule(taskWindowFrom.Add(24*time.Hour), taskWindowTo.Add(24*time.Hour), taskOpenedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("first reschedule: %v", err)
	}
	second, err := first.Reschedule(taskWindowFrom.Add(48*time.Hour), taskWindowTo.Add(48*time.Hour), taskOpenedAt.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("second reschedule: %v", err)
	}
	if second.Task() != task.Task() {
		t.Fatal("改约换了任务身份")
	}
	if second.Reschedules() != 2 {
		t.Fatalf("reschedules = %d, want 2（改约可多次）", second.Reschedules())
	}
	from, to := second.Window()
	if !from.Equal(taskWindowFrom.Add(48*time.Hour)) || !to.Equal(taskWindowTo.Add(48*time.Hour)) {
		t.Fatalf("window = %s..%s", from, to)
	}
	if origFrom, _ := task.Window(); !origFrom.Equal(taskWindowFrom) {
		t.Fatal("改约改写了原任务值——值语义破了")
	}

	t.Run("a closed task cannot reschedule", func(t *testing.T) {
		terminated, err := task.Terminate(
			mustValue(t, domain.NewTaskClosureBasisReference, "authorized-stop-1"),
			taskOpenedAt.Add(3*time.Hour))
		if err != nil {
			t.Fatalf("terminate: %v", err)
		}
		if _, err := terminated.Reschedule(taskWindowFrom.Add(72*time.Hour), taskWindowTo.Add(72*time.Hour), taskOpenedAt.Add(4*time.Hour)); !errors.Is(err, domain.ErrTaskClosed) {
			t.Fatalf("error = %v, want ErrTaskClosed", err)
		}
	})
}

// Covers: CONTEXT「一次失败尝试不自动结束任务……是否再次尝试、改约、终止或交由客户
// 送站，必须使用适用产品、合同和授权规则」与「任务汇总只能由对象结果派生」——终止与
// 完成都必须显式带依据；没有依据的关闭立不成；关闭一次为限。
func TestClosureNeedsAnExplicitBasis(t *testing.T) {
	basis := mustValue(t, domain.NewTaskClosureBasisReference, "per-object-results-1")

	t.Run("completion carries its per-object basis", func(t *testing.T) {
		completed, err := openedTask(t).Complete(basis, taskOpenedAt.Add(6*time.Hour))
		if err != nil {
			t.Fatalf("complete: %v", err)
		}
		if completed.State() != domain.TaskCompleted {
			t.Fatalf("state = %q", completed.State())
		}
		closureBasis, _, closed := completed.Closure()
		if !closed || closureBasis != basis {
			t.Fatal("完成丢了对象级结果依据")
		}
	})

	t.Run("termination carries its basis", func(t *testing.T) {
		terminated, err := openedTask(t).Terminate(basis, taskOpenedAt.Add(6*time.Hour))
		if err != nil {
			t.Fatalf("terminate: %v", err)
		}
		if terminated.State() != domain.TaskTerminated {
			t.Fatalf("state = %q", terminated.State())
		}
	})

	t.Run("closing without a basis is refused", func(t *testing.T) {
		if _, err := openedTask(t).Terminate(domain.TaskClosureBasisReference{}, taskOpenedAt.Add(6*time.Hour)); !errors.Is(err, domain.ErrInvalidDispatchTask) {
			t.Fatalf("terminate error = %v; 没有依据的终止与「一次失败自动关任务」分不开", err)
		}
		if _, err := openedTask(t).Complete(domain.TaskClosureBasisReference{}, taskOpenedAt.Add(6*time.Hour)); !errors.Is(err, domain.ErrInvalidDispatchTask) {
			t.Fatalf("complete error = %v; 完成不是从尝试状态自动长出来的", err)
		}
	})

	t.Run("a task closes once", func(t *testing.T) {
		completed, err := openedTask(t).Complete(basis, taskOpenedAt.Add(6*time.Hour))
		if err != nil {
			t.Fatalf("complete: %v", err)
		}
		if _, err := completed.Terminate(basis, taskOpenedAt.Add(7*time.Hour)); !errors.Is(err, domain.ErrTaskClosed) {
			t.Fatalf("error = %v, want ErrTaskClosed", err)
		}
		if _, err := completed.Complete(basis, taskOpenedAt.Add(7*time.Hour)); !errors.Is(err, domain.ErrTaskClosed) {
			t.Fatalf("error = %v; 完成完了两次", err)
		}
	})

	t.Run("the state set is closed", func(t *testing.T) {
		labels := map[string]struct{}{}
		for _, state := range []domain.TaskState{domain.TaskOpen, domain.TaskTerminated, domain.TaskCompleted} {
			label := state.String()
			if label == "" {
				t.Fatalf("state %d has no label", state)
			}
			labels[label] = struct{}{}
		}
		if len(labels) != 3 {
			t.Fatalf("labels collapsed into %d", len(labels))
		}
		if domain.TaskState(len(labels)+1).String() != "" {
			t.Fatal("第四个任务状态带了标签——封闭集合被悄悄放开")
		}
	})
}
