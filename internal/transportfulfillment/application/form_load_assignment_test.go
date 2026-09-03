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

var (
	assignedAtFixture       = time.Date(2026, 8, 15, 6, 0, 0, 0, time.UTC)
	assignmentRecordedAt    = time.Date(2026, 8, 15, 6, 1, 0, 0, time.UTC)
	assignmentRegistryError = errors.New("装载分配登记册不可用")
)

type loadAssignmentRegistryDouble struct {
	records map[string]ports.LoadAssignmentRecord
	findErr error
	saveErr error
	saves   int
}

func newLoadAssignmentRegistry() *loadAssignmentRegistryDouble {
	return &loadAssignmentRegistryDouble{records: map[string]ports.LoadAssignmentRecord{}}
}

func loadAssignmentRegistryKey(key ports.LoadAssignmentKey) string {
	return key.TenantID.String() + "|" + key.Assignment.String() + "|" + key.Version.String()
}

func (double *loadAssignmentRegistryDouble) FindByKey(
	_ context.Context,
	key ports.LoadAssignmentKey,
) (ports.LoadAssignmentRecord, bool, error) {
	if double.findErr != nil {
		return ports.LoadAssignmentRecord{}, false, double.findErr
	}
	record, found := double.records[loadAssignmentRegistryKey(key)]
	return record, found, nil
}

func (double *loadAssignmentRegistryDouble) Save(
	_ context.Context,
	record ports.LoadAssignmentRecord,
) (ports.LoadAssignmentSaveOutcome, error) {
	double.saves++
	if double.saveErr != nil {
		return ports.LoadAssignmentSaveOutcomeInvalid, double.saveErr
	}
	if _, exists := double.records[loadAssignmentRegistryKey(record.Key)]; exists {
		return ports.LoadAssignmentVersionAlreadyRegistered, nil
	}
	double.records[loadAssignmentRegistryKey(record.Key)] = record
	return ports.LoadAssignmentSaved, nil
}

func newLoadAssignmentHandler(registry *loadAssignmentRegistryDouble) *application.FormLoadAssignmentHandler {
	return application.NewFormLoadAssignmentHandler(application.FormLoadAssignmentDeps{
		Assignments: registry,
		Clock:       dispatchTaskClock{at: assignmentRecordedAt},
	})
}

func formAssignmentCommand(t *testing.T) application.FormLoadAssignmentCommand {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	return application.FormLoadAssignmentCommand{
		TenantID:   tenant,
		Assignment: "load-assignment-1",
		Schedule:   "schedule-1",
		Members:    []string{"parcel-1", "parcel-2"},
		Version:    "LAV-000000000001",
		AssignedAt: assignedAtFixture,
	}
}

// 一次装载分配形成：CONTEXT「把明确载运对象安排到具体班次、运输资源或载运位置的执行意图」。
//
// **它不证明物理装载完成，也不证明控制转移**——所以这里只断言对象范围与班次落了库，不断言
// 任何已装载或控制字段（领域类型上根本没有那些字段）。
func TestFormingALoadAssignmentRegistersTheIntent(t *testing.T) {
	registry := newLoadAssignmentRegistry()

	result, err := newLoadAssignmentHandler(registry).Form(t.Context(), formAssignmentCommand(t))
	if err != nil {
		t.Fatalf("形成分配：%v", err)
	}
	if result.Outcome() != application.LoadAssignmentFormed {
		t.Fatalf("outcome = %q, want LOAD_ASSIGNMENT_FORMED", result.Outcome())
	}

	record, recorded := result.Record()
	if !recorded {
		t.Fatal("成立的分配没有交回记录")
	}
	if record.Assignment.Schedule().String() != "schedule-1" {
		t.Fatalf("班次 = %q", record.Assignment.Schedule())
	}
	if members := record.Assignment.Members(); len(members) != 2 {
		t.Fatalf("对象范围 %d 个, want 2", len(members))
	}
	if _, withdrawn := record.Assignment.Withdrawn(); withdrawn {
		t.Fatal("刚形成的分配就是已撤回的")
	}
	if _, corrects := record.Assignment.Corrects(); corrects {
		t.Fatal("首版分配回指了一个前身")
	}
	// 分配时刻取业务时间，登记时刻另取时钟——分配是何时形成的不由写库那一刻决定。
	if !record.Assignment.AssignedAt().Equal(assignedAtFixture) {
		t.Fatalf("分配时刻 = %s, want %s", record.Assignment.AssignedAt(), assignedAtFixture)
	}
	if !record.RecordedAt.Equal(assignmentRecordedAt) {
		t.Fatalf("登记时刻 = %s, want %s", record.RecordedAt, assignmentRecordedAt)
	}
}

// 同一版本重投交回原版本，不顶替：**变化与撤回都走新版本**，沿用原版本号就是覆盖分配历史，
// 而 CONTEXT 要求「装载分配形成、变化或撤回时保存版本和对象范围」。
func TestASecondFormOfTheSameVersionKeepsTheOriginal(t *testing.T) {
	registry := newLoadAssignmentRegistry()
	handler := newLoadAssignmentHandler(registry)

	if _, err := handler.Form(t.Context(), formAssignmentCommand(t)); err != nil {
		t.Fatalf("首登：%v", err)
	}

	replay := formAssignmentCommand(t)
	replay.Members = []string{"parcel-9"}
	result, err := handler.Form(t.Context(), replay)
	if err != nil {
		t.Fatalf("重投：%v", err)
	}
	if result.Outcome() != application.LoadAssignmentVersionExists {
		t.Fatalf("outcome = %q, want LOAD_ASSIGNMENT_VERSION_EXISTS", result.Outcome())
	}
	record, recorded := result.Record()
	if !recorded {
		t.Fatal("已在册没有交回原版本")
	}
	members := record.Assignment.Members()
	if len(members) != 2 {
		t.Fatalf("重投顶替了原对象范围：%v", members)
	}
}

// 缺任一必备件的分配不受理，且一次库都不碰。
func TestAnIncompleteLoadAssignmentIsNotAccepted(t *testing.T) {
	cases := map[string]func(*application.FormLoadAssignmentCommand){
		"没有成员":    func(command *application.FormLoadAssignmentCommand) { command.Members = nil },
		"同一对象重复":  func(command *application.FormLoadAssignmentCommand) { command.Members = []string{"p", "p"} },
		"缺班次":     func(command *application.FormLoadAssignmentCommand) { command.Schedule = "" },
		"缺版本":     func(command *application.FormLoadAssignmentCommand) { command.Version = "" },
		"缺业务发生时间": func(command *application.FormLoadAssignmentCommand) { command.AssignedAt = time.Time{} },
	}

	for name, breakIt := range cases {
		t.Run(name, func(t *testing.T) {
			registry := newLoadAssignmentRegistry()
			command := formAssignmentCommand(t)
			breakIt(&command)

			result, err := newLoadAssignmentHandler(registry).Form(t.Context(), command)
			if err != nil {
				t.Fatalf("不受理不该上抛技术错误：%v", err)
			}
			if result.Outcome() != application.LoadAssignmentNotAccepted {
				t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", result.Outcome())
			}
			if registry.saves != 0 {
				t.Fatalf("不受理却写了 %d 次库", registry.saves)
			}
		})
	}
}

// 登记册故障形成本上下文自己的未决并带续办引用，不上抛技术错误——`未决`与`输入未受理`分开，
// 因为续办动作相反：前者原样重试就可能过，后者重试一万次都是同一格。
func TestALoadAssignmentRegistryFailureIsUndecided(t *testing.T) {
	registry := newLoadAssignmentRegistry()
	registry.saveErr = assignmentRegistryError

	result, err := newLoadAssignmentHandler(registry).Form(t.Context(), formAssignmentCommand(t))
	if err != nil {
		t.Fatalf("登记册故障不该上抛：%v", err)
	}
	if result.Outcome() != application.LoadAssignmentUndecided {
		t.Fatalf("outcome = %q, want LOAD_ASSIGNMENT_UNDECIDED", result.Outcome())
	}
	if result.ContinuationReference() == "" {
		t.Fatal("未决没有续办引用——调用方无从重试同一次分配")
	}
}
