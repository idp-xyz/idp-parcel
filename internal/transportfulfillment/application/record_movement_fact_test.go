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

func (double *movementFactRegistryDouble) only(t *testing.T) (ports.MovementFactRecord, bool) {
	t.Helper()
	if len(double.records) != 1 {
		t.Fatalf("登记册行数 = %d, want 1", len(double.records))
	}
	for _, record := range double.records {
		return record, true
	}
	return ports.MovementFactRecord{}, false
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

// 同身份（租户、事实、版本）再到，按内容分两格（票 tf-face-algebra/08）：同内容是重放，交回原版本、不再写库；异内容是
// SOURCE_CONFLICT，不回显、不落库、不另起版本。作业事实族的调用方是离线队列、2xx 即出队，把异内容答成已有版本会让另一台
// 设备的真实作业被静默吞掉（ADR-0023 后果节说的判错方向）。
//
// 业务时间照 ADR-0023 不进摘要，所以只差发生时间仍是重放；来源由这一面注入、GateRequired 是门禁判断的输入，两者也不构成内容。
func TestTheSameVersionIsAReplayOnlyWhenItsContentIsTheSame(t *testing.T) {
	arrival := func(t *testing.T) application.RecordMovementFactCommand { return recordMovementCommand(t) }
	clearedDeparture := func(t *testing.T) application.RecordMovementFactCommand {
		command := recordMovementCommand(t)
		command.Kind = domain.DepartureFact
		command.GateRequired = true
		command.GateClearance = "cc-clearance/v1"
		return command
	}
	cases := []struct {
		name   string
		first  func(*testing.T) application.RecordMovementFactCommand
		change func(*application.RecordMovementFactCommand)
		want   application.MovementFactOutcome
	}{
		{"原样重投", arrival, func(*application.RecordMovementFactCommand) {}, application.MovementFactVersionExists},
		{"只差整秒发生时间", arrival, func(command *application.RecordMovementFactCommand) {
			command.OccurredAt = command.OccurredAt.Add(time.Second)
		}, application.MovementFactVersionExists},
		{"只差亚秒发生时间", arrival, func(command *application.RecordMovementFactCommand) {
			command.OccurredAt = command.OccurredAt.Add(250 * time.Millisecond)
		}, application.MovementFactVersionExists},
		{"只差来源", arrival, func(command *application.RecordMovementFactCommand) {
			command.Source = "other-scan/v2"
		}, application.MovementFactVersionExists},
		{"带放行的出发只差 GateRequired", clearedDeparture, func(command *application.RecordMovementFactCommand) {
			command.GateRequired = false
		}, application.MovementFactVersionExists},
		{"改种类", arrival, func(command *application.RecordMovementFactCommand) {
			command.Kind = domain.InTransitFact
		}, application.MovementFactSourceConflict},
		{"改地点", arrival, func(command *application.RecordMovementFactCommand) {
			command.Location = "hub-9"
		}, application.MovementFactSourceConflict},
		{"改班次", arrival, func(command *application.RecordMovementFactCommand) {
			command.Schedule = "schedule-9"
		}, application.MovementFactSourceConflict},
		{"换放行引用", clearedDeparture, func(command *application.RecordMovementFactCommand) {
			command.GateClearance = "cc-clearance/v2"
		}, application.MovementFactSourceConflict},
		{"去掉门禁与放行", clearedDeparture, func(command *application.RecordMovementFactCommand) {
			command.GateRequired = false
			command.GateClearance = ""
		}, application.MovementFactSourceConflict},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			registry := newMovementFactRegistry()
			handler := newMovementFactHandler(registry)
			first := testCase.first(t)
			if result, err := handler.Record(t.Context(), first); err != nil || result.Outcome() != application.MovementFactRecorded {
				t.Fatalf("首登：outcome=%q err=%v", result.Outcome(), err)
			}
			original, _ := registry.only(t)

			again := testCase.first(t)
			testCase.change(&again)
			result, err := handler.Record(t.Context(), again)
			if err != nil {
				t.Fatalf("再投：%v", err)
			}
			if result.Outcome() != testCase.want {
				t.Fatalf("outcome = %q, want %q", result.Outcome(), testCase.want)
			}
			record, echoed := result.Record()
			switch testCase.want {
			case application.MovementFactVersionExists:
				if !echoed || !record.Fact.OccurredAt().Equal(original.Fact.OccurredAt()) || record.Fact.Source() != original.Fact.Source() {
					t.Fatalf("重放没有交回原版本：echoed=%v record=%+v", echoed, record)
				}
			default:
				if echoed {
					t.Fatalf("冲突回显了在册内容：%+v", record)
				}
			}
			if registry.saves != 1 {
				t.Fatalf("再投写了库：saves=%d", registry.saves)
			}
			if kept, _ := registry.only(t); kept.Fact.Location() != original.Fact.Location() || kept.Fact.Kind() != original.Fact.Kind() {
				t.Fatalf("在册版本被顶替：%+v", kept)
			}
		})
	}
}

// 身份在册时先比内容、后判门禁：在册是一条没带放行的出发，同身份再到时多声称一句「要门禁」，内容（班次、种类、地点、放行
// 缺席）仍同，是重放；若先判门禁，这条重放会被支去取放行。
func TestARegisteredIdentityIsComparedBeforeTheDepartureGate(t *testing.T) {
	registry := newMovementFactRegistry()
	handler := newMovementFactHandler(registry)
	departure := recordMovementCommand(t)
	departure.Kind = domain.DepartureFact
	if result, err := handler.Record(t.Context(), departure); err != nil || result.Outcome() != application.MovementFactRecorded {
		t.Fatalf("首登：outcome=%q err=%v", result.Outcome(), err)
	}

	departure.GateRequired = true
	result, err := handler.Record(t.Context(), departure)
	if err != nil {
		t.Fatalf("再投：%v", err)
	}
	if result.Outcome() != application.MovementFactVersionExists {
		t.Fatalf("outcome = %q, want MOVEMENT_FACT_VERSION_EXISTS", result.Outcome())
	}
}

// 门禁挡着而身份查不了册：此刻判断不了这是不是一条在册版本的重放，答未决而不答门禁格——答门禁格会把一条可能的重放支去取放行。
func TestAGatedDepartureWhoseIdentityCannotBeLookedUpIsUndecided(t *testing.T) {
	registry := newMovementFactRegistry()
	registry.findErr = movementRegistryDown
	command := recordMovementCommand(t)
	command.Kind = domain.DepartureFact
	command.GateRequired = true

	result, err := newMovementFactHandler(registry).Record(t.Context(), command)
	if err != nil {
		t.Fatalf("登记册故障不该上抛：%v", err)
	}
	if result.Outcome() != application.MovementFactUndecided || result.ContinuationReference() == "" {
		t.Fatalf("outcome = %q continuation=%q, want MOVEMENT_FACT_UNDECIDED 带续办引用", result.Outcome(), result.ContinuationReference())
	}
}

// 并发输家：首读时赢家还没提交，撞键后读回赢家，同样按内容分两格——读回那一支若不比，输家带的另一份内容就被吞掉。
func TestACollisionLoserIsComparedWithTheWinner(t *testing.T) {
	cases := map[string]struct {
		change func(*application.RecordMovementFactCommand)
		want   application.MovementFactOutcome
	}{
		"同内容": {func(*application.RecordMovementFactCommand) {}, application.MovementFactVersionExists},
		"异内容": {func(command *application.RecordMovementFactCommand) { command.Location = "hub-9" }, application.MovementFactSourceConflict},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			registry := newMovementFactRegistry()
			if _, err := newMovementFactHandler(registry).Record(t.Context(), recordMovementCommand(t)); err != nil {
				t.Fatalf("赢家：%v", err)
			}
			loser := application.NewRecordMovementFactHandler(application.RecordMovementFactDeps{
				Facts: &staleFirstReadRegistry{movementFactRegistryDouble: registry},
				Clock: dispatchTaskClock{at: movementRecordedAt},
			})
			command := recordMovementCommand(t)
			testCase.change(&command)

			result, err := loser.Record(t.Context(), command)
			if err != nil {
				t.Fatalf("输家：%v", err)
			}
			if result.Outcome() != testCase.want {
				t.Fatalf("outcome = %q, want %q", result.Outcome(), testCase.want)
			}
			if _, echoed := result.Record(); echoed != (testCase.want == application.MovementFactVersionExists) {
				t.Fatalf("回显 = %v：重放交回赢家，冲突不回显", echoed)
			}
			if kept, _ := registry.only(t); kept.Fact.Location().String() != "hub-1" {
				t.Fatalf("赢家被顶替：%q", kept.Fact.Location())
			}
		})
	}
}

// staleFirstReadRegistry 让首读答「没有」，模拟输家的首读落在赢家提交之前。
type staleFirstReadRegistry struct {
	*movementFactRegistryDouble
	read bool
}

func (stale *staleFirstReadRegistry) FindByKey(
	ctx context.Context,
	key ports.MovementFactKey,
) (ports.MovementFactRecord, bool, error) {
	if !stale.read {
		stale.read = true
		return ports.MovementFactRecord{}, false, nil
	}
	return stale.movementFactRegistryDouble.FindByKey(ctx, key)
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
