package application_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var (
	taskWindowFrom = time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC)
	taskWindowTo   = time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	taskOpenedAt   = time.Date(2026, 8, 14, 7, 30, 0, 0, time.UTC)
	taskRecordedAt = time.Date(2026, 8, 14, 7, 31, 0, 0, time.UTC)
)

type dispatchTaskRegistryDouble struct {
	records map[string]ports.DispatchTaskRecord
	findErr error
	saveErr error
	saves   int
}

func newDispatchTaskRegistry() *dispatchTaskRegistryDouble {
	return &dispatchTaskRegistryDouble{records: map[string]ports.DispatchTaskRecord{}}
}

func dispatchTaskRegistryKey(key ports.DispatchTaskKey) string {
	return key.TenantID.String() + "|" + key.Task.String()
}

func (double *dispatchTaskRegistryDouble) FindByKey(
	_ context.Context,
	key ports.DispatchTaskKey,
) (ports.DispatchTaskRecord, bool, error) {
	if double.findErr != nil {
		return ports.DispatchTaskRecord{}, false, double.findErr
	}
	record, found := double.records[dispatchTaskRegistryKey(key)]
	return record, found, nil
}

func (double *dispatchTaskRegistryDouble) Save(
	_ context.Context,
	record ports.DispatchTaskRecord,
) (ports.DispatchTaskSaveOutcome, error) {
	double.saves++
	if double.saveErr != nil {
		return ports.DispatchTaskSaveOutcomeInvalid, double.saveErr
	}
	if _, exists := double.records[dispatchTaskRegistryKey(record.Key)]; exists {
		return ports.DispatchTaskAlreadyOpen, nil
	}
	double.records[dispatchTaskRegistryKey(record.Key)] = record
	return ports.DispatchTaskSaved, nil
}

type dispatchTaskClock struct{ at time.Time }

func (clock dispatchTaskClock) Now() time.Time { return clock.at }

func newDispatchTaskHandler(registry *dispatchTaskRegistryDouble) *application.OpenDispatchTaskHandler {
	return application.NewOpenDispatchTaskHandler(application.OpenDispatchTaskDeps{
		Tasks: registry,
		Clock: dispatchTaskClock{at: taskRecordedAt},
	})
}

func openTaskCommand(t *testing.T) application.OpenDispatchTaskCommand {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	return application.OpenDispatchTaskCommand{
		TenantID:   tenant,
		Task:       "dispatch-task-1",
		Kind:       domain.PickupDispatch,
		Objects:    []string{"parcel-1", "parcel-2"},
		Place:      "customer-warehouse-1",
		WindowFrom: taskWindowFrom,
		WindowTo:   taskWindowTo,
		Conditions: "service-condition/v1",
		OpenedAt:   taskOpenedAt,
	}
}

// 一项揽派任务首次建立：CONTEXT「揽派任务与履约尝试分离。一个任务和一次尝试可以覆盖多个
// 载运对象」。任务表达的是**要完成什么**，不是已经到场或取得控制——所以这里只断言工作范围
// 落了库、状态是`开放`，不断言任何到场或控制字段（领域类型上根本没有那些字段）。
func TestOpeningADispatchTaskRegistersTheWorkScope(t *testing.T) {
	registry := newDispatchTaskRegistry()
	handler := newDispatchTaskHandler(registry)

	result, err := handler.Open(t.Context(), openTaskCommand(t))
	if err != nil {
		t.Fatalf("建立任务：%v", err)
	}
	if result.Outcome() != application.DispatchTaskOpened {
		t.Fatalf("outcome = %q, want DISPATCH_TASK_OPENED", result.Outcome())
	}

	record, recorded := result.Record()
	if !recorded {
		t.Fatal("成立的任务没有交回记录")
	}
	if state := record.Task.State(); state != domain.TaskOpen {
		t.Fatalf("任务状态 = %q, want OPEN", state)
	}
	if objects := record.Task.Objects(); len(objects) != 2 {
		t.Fatalf("任务覆盖 %d 个对象, want 2——一个任务可以覆盖多个载运对象", len(objects))
	}
	// 建立时刻取命令给的业务时间，登记时刻另取时钟：两者分开，任务是何时成立的不由写库那一刻决定。
	if !record.Task.OpenedAt().Equal(taskOpenedAt) {
		t.Fatalf("任务建立时刻 = %s, want %s", record.Task.OpenedAt(), taskOpenedAt)
	}
	if !record.RecordedAt.Equal(taskRecordedAt) {
		t.Fatalf("登记时刻 = %s, want %s", record.RecordedAt, taskRecordedAt)
	}
}

// 缺任一必备件的任务不受理，且**一次库都不碰**：工作范围不完整的任务登进去，下游读到的
// 就是一份看起来成立、实际无从执行的工作范围。
func TestAnIncompleteDispatchTaskIsNotAccepted(t *testing.T) {
	cases := map[string]func(*application.OpenDispatchTaskCommand){
		"没有对象":    func(command *application.OpenDispatchTaskCommand) { command.Objects = nil },
		"窗口首尾颠倒":  func(command *application.OpenDispatchTaskCommand) { command.WindowTo = taskWindowFrom.Add(-time.Hour) },
		"缺服务条件":   func(command *application.OpenDispatchTaskCommand) { command.Conditions = "" },
		"缺地点":     func(command *application.OpenDispatchTaskCommand) { command.Place = "" },
		"同一对象重复":  func(command *application.OpenDispatchTaskCommand) { command.Objects = []string{"parcel-1", "parcel-1"} },
		"没有任务种类":  func(command *application.OpenDispatchTaskCommand) { command.Kind = domain.DispatchTaskKindInvalid },
		"缺业务发生时间": func(command *application.OpenDispatchTaskCommand) { command.OpenedAt = time.Time{} },
	}

	for name, breakIt := range cases {
		t.Run(name, func(t *testing.T) {
			registry := newDispatchTaskRegistry()
			command := openTaskCommand(t)
			breakIt(&command)

			result, err := newDispatchTaskHandler(registry).Open(t.Context(), command)
			if err != nil {
				t.Fatalf("不受理不该上抛技术错误：%v", err)
			}
			if result.Outcome() != application.DispatchTaskNotAccepted {
				t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", result.Outcome())
			}
			if registry.saves != 0 {
				t.Fatalf("不受理却写了 %d 次库", registry.saves)
			}
		})
	}
}
