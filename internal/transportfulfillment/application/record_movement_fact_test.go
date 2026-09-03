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
	movedAtFixture       = time.Date(2026, 8, 16, 5, 0, 0, 0, time.UTC)
	movementRecordedAt   = time.Date(2026, 8, 16, 5, 1, 0, 0, time.UTC)
	movementRegistryDown = errors.New("移动事实登记册不可用")
)

type movementFactRegistryDouble struct {
	records map[string]ports.MovementFactRecord
	findErr error
	saveErr error
	saves   int
}

func newMovementFactRegistry() *movementFactRegistryDouble {
	return &movementFactRegistryDouble{records: map[string]ports.MovementFactRecord{}}
}

func movementFactRegistryKey(key ports.MovementFactKey) string {
	return key.TenantID.String() + "|" + key.Fact.String() + "|" + key.Version.String()
}

func (double *movementFactRegistryDouble) FindByKey(
	_ context.Context,
	key ports.MovementFactKey,
) (ports.MovementFactRecord, bool, error) {
	if double.findErr != nil {
		return ports.MovementFactRecord{}, false, double.findErr
	}
	record, found := double.records[movementFactRegistryKey(key)]
	return record, found, nil
}

func (double *movementFactRegistryDouble) Save(
	_ context.Context,
	record ports.MovementFactRecord,
) (ports.MovementFactSaveOutcome, error) {
	double.saves++
	if double.saveErr != nil {
		return ports.MovementFactSaveOutcomeInvalid, double.saveErr
	}
	if _, exists := double.records[movementFactRegistryKey(record.Key)]; exists {
		return ports.MovementFactVersionAlreadyRegistered, nil
	}
	double.records[movementFactRegistryKey(record.Key)] = record
	return ports.MovementFactSaved, nil
}

func newMovementFactHandler(registry *movementFactRegistryDouble) *application.RecordMovementFactHandler {
	return application.NewRecordMovementFactHandler(application.RecordMovementFactDeps{
		Facts: registry,
		Clock: dispatchTaskClock{at: movementRecordedAt},
	})
}

func recordMovementCommand(t *testing.T) application.RecordMovementFactCommand {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	return application.RecordMovementFactCommand{
		TenantID:   tenant,
		Fact:       "movement-fact-1",
		Schedule:   "schedule-1",
		Kind:       domain.ArrivalFact,
		Location:   "hub-1",
		Source:     "carrier-scan/v1",
		Version:    "MFV-000000000001",
		OccurredAt: movedAtFixture,
	}
}

// 一条实际移动事实登记：CONTEXT 开篇把「实际移动」列为本上下文管理的对象之一。
//
// **它不等于交接，也不结束控制。** 所以这里只断言事实本身落了库，不断言任何控制或交接字段
// ——领域类型上根本没有那些字段。
func TestRecordingAMovementFactRegistersIt(t *testing.T) {
	registry := newMovementFactRegistry()

	result, err := newMovementFactHandler(registry).Record(t.Context(), recordMovementCommand(t))
	if err != nil {
		t.Fatalf("登记移动事实：%v", err)
	}
	if result.Outcome() != application.MovementFactRecorded {
		t.Fatalf("outcome = %q, want MOVEMENT_FACT_RECORDED", result.Outcome())
	}

	record, recorded := result.Record()
	if !recorded {
		t.Fatal("成立的事实没有交回记录")
	}
	if record.Fact.Kind() != domain.ArrivalFact {
		t.Fatalf("事实种类 = %q, want ARRIVAL", record.Fact.Kind())
	}
	if _, gated := record.Fact.GateClearance(); gated {
		t.Fatal("到达带上了门禁放行依据——门禁只约束装载出发")
	}
	// 发生时刻取业务时间，登记时刻另取时钟：迟到的回传不该被记成刚刚发生。
	if !record.Fact.OccurredAt().Equal(movedAtFixture) {
		t.Fatalf("发生时刻 = %s, want %s", record.Fact.OccurredAt(), movedAtFixture)
	}
	if !record.RecordedAt.Equal(movementRecordedAt) {
		t.Fatalf("登记时刻 = %s, want %s", record.RecordedAt, movementRecordedAt)
	}
}

// 受监管门禁约束的出发缺放行依据，独立成格而不并进`输入未受理`。
//
// **两者的续办动作相反**：门禁那一格要去取放行结果（判断本身属 customs-compliance），输入
// 未受理要去改输入。并成一格会让调用方读不出该做哪一件。
func TestAGatedDepartureWithoutClearanceIsItsOwnAnswer(t *testing.T) {
	registry := newMovementFactRegistry()
	command := recordMovementCommand(t)
	command.Kind = domain.DepartureFact
	command.GateRequired = true

	result, err := newMovementFactHandler(registry).Record(t.Context(), command)
	if err != nil {
		t.Fatalf("门禁未放行不该上抛技术错误：%v", err)
	}
	if result.Outcome() != application.MovementFactGateBlocked {
		t.Fatalf("outcome = %q, want DEPARTURE_GATE_BLOCKED", result.Outcome())
	}
	if registry.saves != 0 {
		t.Fatalf("门禁未放行却写了 %d 次库", registry.saves)
	}
}

// 带放行依据的受管出发照常成立，且依据随行保全。
func TestAClearedGatedDepartureIsRecordedWithItsClearance(t *testing.T) {
	registry := newMovementFactRegistry()
	command := recordMovementCommand(t)
	command.Kind = domain.DepartureFact
	command.GateRequired = true
	command.GateClearance = "cc-clearance/v1"

	result, err := newMovementFactHandler(registry).Record(t.Context(), command)
	if err != nil {
		t.Fatalf("登记：%v", err)
	}
	if result.Outcome() != application.MovementFactRecorded {
		t.Fatalf("outcome = %q, want MOVEMENT_FACT_RECORDED", result.Outcome())
	}
	record, _ := result.Record()
	clearance, gated := record.Fact.GateClearance()
	if !gated || clearance.String() != "cc-clearance/v1" {
		t.Fatalf("放行依据没有随行保全：%q gated=%v", clearance, gated)
	}
}

// 同一版本重投交回原版本，不顶替——更正走新版本，原记录与其派生历史不被改写。
func TestASecondRecordOfTheSameVersionKeepsTheOriginal(t *testing.T) {
	registry := newMovementFactRegistry()
	handler := newMovementFactHandler(registry)

	if _, err := handler.Record(t.Context(), recordMovementCommand(t)); err != nil {
		t.Fatalf("首登：%v", err)
	}

	replay := recordMovementCommand(t)
	replay.Location = "hub-9"
	result, err := handler.Record(t.Context(), replay)
	if err != nil {
		t.Fatalf("重投：%v", err)
	}
	if result.Outcome() != application.MovementFactVersionExists {
		t.Fatalf("outcome = %q, want MOVEMENT_FACT_VERSION_EXISTS", result.Outcome())
	}
	record, _ := result.Record()
	if record.Fact.Location().String() != "hub-1" {
		t.Fatalf("重投顶替了原地点：%q", record.Fact.Location())
	}
}

// 缺任一必备件不受理，且一次库都不碰。**把放行依据挂到移动或到达上也在这一格**——那等于
// 造了一个不存在的门。
func TestAnIncompleteMovementFactIsNotAccepted(t *testing.T) {
	cases := map[string]func(*application.RecordMovementFactCommand){
		"缺地点":      func(command *application.RecordMovementFactCommand) { command.Location = "" },
		"缺来源":      func(command *application.RecordMovementFactCommand) { command.Source = "" },
		"缺版本":      func(command *application.RecordMovementFactCommand) { command.Version = "" },
		"缺业务发生时间":  func(command *application.RecordMovementFactCommand) { command.OccurredAt = time.Time{} },
		"没有事实种类":   func(command *application.RecordMovementFactCommand) { command.Kind = domain.MovementFactKindInvalid },
		"到达挂了放行依据": func(command *application.RecordMovementFactCommand) { command.GateClearance = "cc-clearance/v1" },
		"到达声称受门禁管": func(command *application.RecordMovementFactCommand) { command.GateRequired = true },
	}

	for name, breakIt := range cases {
		t.Run(name, func(t *testing.T) {
			registry := newMovementFactRegistry()
			command := recordMovementCommand(t)
			breakIt(&command)

			result, err := newMovementFactHandler(registry).Record(t.Context(), command)
			if err != nil {
				t.Fatalf("不受理不该上抛技术错误：%v", err)
			}
			if result.Outcome() != application.MovementFactNotAccepted {
				t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", result.Outcome())
			}
			if registry.saves != 0 {
				t.Fatalf("不受理却写了 %d 次库", registry.saves)
			}
		})
	}
}

// 登记册故障形成未决并带续办引用，不上抛技术错误。
func TestAMovementFactRegistryFailureIsUndecided(t *testing.T) {
	registry := newMovementFactRegistry()
	registry.saveErr = movementRegistryDown

	result, err := newMovementFactHandler(registry).Record(t.Context(), recordMovementCommand(t))
	if err != nil {
		t.Fatalf("登记册故障不该上抛：%v", err)
	}
	if result.Outcome() != application.MovementFactUndecided {
		t.Fatalf("outcome = %q, want MOVEMENT_FACT_UNDECIDED", result.Outcome())
	}
	if result.ContinuationReference() == "" {
		t.Fatal("未决没有续办引用")
	}
}
