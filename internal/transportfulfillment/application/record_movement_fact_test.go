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

// readingFor 交回组合根会为这条命令问来的那种读数：移动与到达没有；出发没声明段，读数停在「段未声明」；声明了段，读数问到
// 一件、答 answer。
func readingFor(t *testing.T, command application.RecordMovementFactCommand, answer domain.DepartureGateAnswer) domain.DepartureGateReading {
	t.Helper()
	if command.Kind != domain.DepartureFact {
		return domain.DepartureGateReading{}
	}
	spec := domain.DepartureGateReadingSpec{Stops: []domain.DepartureGateStop{domain.DepartureSegmentNotDeclared}}
	if command.Segment != "" {
		judgement, basis := "AADJ-1", domain.DepartureGateBasis{Action: "LOADING_DEPARTURE", Cause: "SOME_CAUSE"}
		if answer == domain.DepartureGateUnreadable {
			judgement, basis = "", domain.DepartureGateBasis{Cause: "CUSTOMS_ADMISSION_UNAVAILABLE"}
		}
		finding, err := domain.NewDepartureGateFinding(mustRefValue(t, domain.NewCarriedObjectReference, "parcel-1"), answer, judgement,
			[]domain.DepartureGateBasis{basis})
		if err != nil {
			t.Fatalf("逐对象那一格：%v", err)
		}
		spec = domain.DepartureGateReadingSpec{
			From:     mustRefValue(t, domain.NewNetworkNodeReference, "NODE-SHA"),
			To:       mustRefValue(t, domain.NewNetworkNodeReference, "NODE-SZX"),
			Findings: []domain.DepartureGateFinding{finding},
		}
	}
	reading, err := domain.FormDepartureGateReading(spec)
	if err != nil {
		t.Fatalf("读数：%v", err)
	}
	return reading
}

// record 照组合根的次序登记：先问读数（这里按命令现给），再登记。
func record(t *testing.T, handler *application.RecordMovementFactHandler, command application.RecordMovementFactCommand) (application.RecordMovementFactResult, error) {
	t.Helper()
	return handler.Record(t.Context(), command, readingFor(t, command, domain.DepartureGateNotGuarded))
}

// 一条实际移动事实登记：CONTEXT 开篇把「实际移动」列为本上下文管理的对象之一。
//
// **它不等于交接，也不结束控制。** 所以这里只断言事实本身落了库，不断言任何控制或交接字段
// ——领域类型上根本没有那些字段。
func TestRecordingAMovementFactRegistersIt(t *testing.T) {
	registry := newMovementFactRegistry()

	result, err := record(t, newMovementFactHandler(registry), recordMovementCommand(t))
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

// Covers: AT-TF-111、AT-TF-112 编排半边——出发一律落册：读数是阻断、读不动、未决、放行还是不受门禁，都答 MOVEMENT_FACT_RECORDED，
// 读数随事实同一笔保存（ADR-0205 决定一）。拒收一条已经发生的出发只会让离线队列把它出队丢掉。
func TestADepartureIsRecordedWhateverTheGateReadingSays(t *testing.T) {
	for _, answer := range []domain.DepartureGateAnswer{
		domain.DepartureGateBlocked, domain.DepartureGateUnreadable, domain.DepartureGateUndecided,
		domain.DepartureGateAdmitted, domain.DepartureGateNotGuarded,
	} {
		t.Run(answer.String(), func(t *testing.T) {
			registry := newMovementFactRegistry()
			command := recordMovementCommand(t)
			command.Kind = domain.DepartureFact
			command.Segment = "segment-1"

			result, err := newMovementFactHandler(registry).Record(t.Context(), command, readingFor(t, command, answer))
			if err != nil {
				t.Fatalf("登记：%v", err)
			}
			if result.Outcome() != application.MovementFactRecorded {
				t.Fatalf("outcome = %q, want MOVEMENT_FACT_RECORDED", result.Outcome())
			}
			saved, _ := registry.only(t)
			reading, read := saved.Fact.GateReading()
			if !read || reading.Answer() != answer {
				t.Fatalf("落册的读数 = %s read=%v, want %s", reading.Answer(), read, answer)
			}
			if segment, declared := saved.Fact.Segment(); !declared || segment.String() != "segment-1" {
				t.Fatalf("声明的段没落册：%q declared=%v", segment, declared)
			}
		})
	}
}

// Covers: AT-TF-113 编排半边——设备自报的 gateRequired 不读（真也好假也好都不挡、不改读数）；出示的放行凭据原样保全为证据，
// 读数照样是问来的那一份。
func TestTheDeviceGateFlagIsNotReadAndItsClearanceIsKeptAsEvidence(t *testing.T) {
	registry := newMovementFactRegistry()
	command := recordMovementCommand(t)
	command.Kind = domain.DepartureFact
	command.Segment = "segment-1"
	command.GateRequired = true
	command.GateClearance = "cc-clearance/v1"

	result, err := newMovementFactHandler(registry).Record(t.Context(), command, readingFor(t, command, domain.DepartureGateNotGuarded))
	if err != nil {
		t.Fatalf("登记：%v", err)
	}
	if result.Outcome() != application.MovementFactRecorded {
		t.Fatalf("outcome = %q, want MOVEMENT_FACT_RECORDED", result.Outcome())
	}
	saved, _ := result.Record()
	clearance, presented := saved.Fact.GateClearance()
	if !presented || clearance.String() != "cc-clearance/v1" {
		t.Fatalf("凭据没有随行保全：%q presented=%v", clearance, presented)
	}
	if reading, _ := saved.Fact.GateReading(); reading.Answer() != domain.DepartureGateNotGuarded {
		t.Fatalf("读数 = %s——自报的 gateRequired 改了问来的读数", reading.Answer())
	}

	arrival := recordMovementCommand(t)
	arrival.GateRequired = true
	if result, err := record(t, newMovementFactHandler(newMovementFactRegistry()), arrival); err != nil || result.Outcome() != application.MovementFactRecorded {
		t.Fatalf("到达自报要门禁：outcome=%q err=%v，字段不读就不该挡", result.Outcome(), err)
	}
}

// 读数放错地方是接线错误不是输入有误：出发没带读数、到达带了读数，都上抛而不答一格，也不写库。
func TestAGateReadingInTheWrongPlaceIsAWiringError(t *testing.T) {
	departure := recordMovementCommand(t)
	departure.Kind = domain.DepartureFact
	registry := newMovementFactRegistry()
	if _, err := newMovementFactHandler(registry).Record(t.Context(), departure, domain.DepartureGateReading{}); !errors.Is(err, domain.ErrDepartureGateReadingMisplaced) {
		t.Fatalf("出发没带读数：err = %v, want ErrDepartureGateReadingMisplaced", err)
	}
	arrival := recordMovementCommand(t)
	if _, err := newMovementFactHandler(registry).Record(t.Context(), arrival, readingFor(t, departure, domain.DepartureGateNotGuarded)); !errors.Is(err, domain.ErrDepartureGateReadingMisplaced) {
		t.Fatalf("到达带了读数：err = %v, want ErrDepartureGateReadingMisplaced", err)
	}
	if registry.saves != 0 {
		t.Fatalf("接线错误却写了 %d 次库", registry.saves)
	}
}

// 同身份（租户、事实、版本）再到，按内容分两格（票 tf-face-algebra/08）：同内容是重放，交回原版本、不再写库；异内容是
// SOURCE_CONFLICT，不回显、不落库、不另起版本。作业事实族的调用方是离线队列、2xx 即出队，把异内容答成已有版本会让另一台
// 设备的真实作业被静默吞掉（ADR-0023 后果节说的判错方向）。
//
// 业务时间照 ADR-0023 不进摘要，所以只差发生时间仍是重放；来源由这一面注入、GateRequired 不读、门禁读数是登记时问来的判断，三者
// 都不构成内容。声明的段与资源是登记方报的，构成内容。
func TestTheSameVersionIsAReplayOnlyWhenItsContentIsTheSame(t *testing.T) {
	arrival := func(t *testing.T) application.RecordMovementFactCommand { return recordMovementCommand(t) }
	clearedDeparture := func(t *testing.T) application.RecordMovementFactCommand {
		command := recordMovementCommand(t)
		command.Kind = domain.DepartureFact
		command.Segment = "segment-1"
		command.Resource = "truck-1"
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
		{"换声明的段", clearedDeparture, func(command *application.RecordMovementFactCommand) {
			command.Segment = "segment-9"
		}, application.MovementFactSourceConflict},
		{"去掉声明的资源", clearedDeparture, func(command *application.RecordMovementFactCommand) {
			command.Resource = ""
		}, application.MovementFactSourceConflict},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			registry := newMovementFactRegistry()
			handler := newMovementFactHandler(registry)
			first := testCase.first(t)
			if result, err := record(t, handler, first); err != nil || result.Outcome() != application.MovementFactRecorded {
				t.Fatalf("首登：outcome=%q err=%v", result.Outcome(), err)
			}
			original, _ := registry.only(t)

			again := testCase.first(t)
			testCase.change(&again)
			result, err := record(t, handler, again)
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

// Covers: AT-TF-116 编排半边——同身份同内容再到是重放，交回在册那一版连同它首次登记时问来的读数；此刻重问出的读数不同（关务
// 那边已阻断）也不另记一份、不判成冲突：读数是判断不是内容。同身份查不了册答未决，与移动、到达同一格。
func TestAReplayedDepartureEchoesItsFirstReading(t *testing.T) {
	registry := newMovementFactRegistry()
	handler := newMovementFactHandler(registry)
	departure := recordMovementCommand(t)
	departure.Kind = domain.DepartureFact
	departure.Segment = "segment-1"
	if result, err := handler.Record(t.Context(), departure, readingFor(t, departure, domain.DepartureGateNotGuarded)); err != nil ||
		result.Outcome() != application.MovementFactRecorded {
		t.Fatalf("首登：outcome=%q err=%v", result.Outcome(), err)
	}

	result, err := handler.Record(t.Context(), departure, readingFor(t, departure, domain.DepartureGateBlocked))
	if err != nil {
		t.Fatalf("再投：%v", err)
	}
	if result.Outcome() != application.MovementFactVersionExists {
		t.Fatalf("outcome = %q, want MOVEMENT_FACT_VERSION_EXISTS", result.Outcome())
	}
	echoed, _ := result.Record()
	if reading, _ := echoed.Fact.GateReading(); reading.Answer() != domain.DepartureGateNotGuarded {
		t.Fatalf("重放回显的读数 = %s, want 首次的 NOT_GUARDED", reading.Answer())
	}
	if registry.saves != 1 {
		t.Fatalf("重放写了库：saves=%d", registry.saves)
	}

	down := newMovementFactRegistry()
	down.findErr = movementRegistryDown
	undecided, err := newMovementFactHandler(down).Record(t.Context(), departure, readingFor(t, departure, domain.DepartureGateBlocked))
	if err != nil {
		t.Fatalf("登记册故障不该上抛：%v", err)
	}
	if undecided.Outcome() != application.MovementFactUndecided || undecided.ContinuationReference() == "" {
		t.Fatalf("outcome = %q continuation=%q, want MOVEMENT_FACT_UNDECIDED 带续办引用", undecided.Outcome(), undecided.ContinuationReference())
	}
}

// Covers: AT-TF-116 编排半边（重投不重问关务）——同身份同内容的出发在问读数之前就认得出是重放，交回在册那一份读数；
// 换了内容、身份上在册的是到达、不在册、查不了册、不是出发，都答认不出，照常问读数，由 Record 定那一格答复。
func TestAReplayedDepartureHandsBackItsRecordedReadingBeforeCustomsIsAsked(t *testing.T) {
	registry := newMovementFactRegistry()
	handler := newMovementFactHandler(registry)
	departure := recordMovementCommand(t)
	departure.Kind = domain.DepartureFact
	departure.Segment = "segment-1"
	if _, replayed := handler.RecordedDepartureReading(t.Context(), departure); replayed {
		t.Fatal("还没登记就认成了重放")
	}
	if result, err := handler.Record(t.Context(), departure, readingFor(t, departure, domain.DepartureGateNotGuarded)); err != nil ||
		result.Outcome() != application.MovementFactRecorded {
		t.Fatalf("首登：outcome=%q err=%v", result.Outcome(), err)
	}

	reading, replayed := handler.RecordedDepartureReading(t.Context(), departure)
	if !replayed || reading.Answer() != domain.DepartureGateNotGuarded {
		t.Fatalf("同内容重投：replayed=%v 读数=%s, want 在册那一份 NOT_GUARDED", replayed, reading.Answer())
	}
	result, err := handler.Record(t.Context(), departure, reading)
	if err != nil || result.Outcome() != application.MovementFactVersionExists || registry.saves != 1 {
		t.Fatalf("拿在册读数再登记：outcome=%q err=%v saves=%d, want 已有版本且不写库", result.Outcome(), err, registry.saves)
	}

	moved := departure
	moved.Resource = "truck-9"
	if _, replayed := handler.RecordedDepartureReading(t.Context(), moved); replayed {
		t.Fatal("换了资源仍认成重放：内容冲突要照常问读数，再由 Record 答冲突")
	}
	arrival := recordMovementCommand(t)
	arrival.Fact = "movement-fact-arrival"
	if _, err := record(t, handler, arrival); err != nil {
		t.Fatalf("登到达：%v", err)
	}
	asDeparture := arrival
	asDeparture.Kind = domain.DepartureFact
	if _, replayed := handler.RecordedDepartureReading(t.Context(), asDeparture); replayed {
		t.Fatal("在册的是到达，同身份报出发却认成了重放")
	}
	if _, replayed := handler.RecordedDepartureReading(t.Context(), arrival); replayed {
		t.Fatal("到达没有读数可取，不该认成出发的重放")
	}

	down := newMovementFactRegistry()
	down.findErr = movementRegistryDown
	if _, replayed := newMovementFactHandler(down).RecordedDepartureReading(t.Context(), departure); replayed {
		t.Fatal("查不了册却认成了重放")
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
			if _, err := record(t, newMovementFactHandler(registry), recordMovementCommand(t)); err != nil {
				t.Fatalf("赢家：%v", err)
			}
			loser := application.NewRecordMovementFactHandler(application.RecordMovementFactDeps{
				Facts: &staleFirstReadRegistry{movementFactRegistryDouble: registry},
				Clock: dispatchTaskClock{at: movementRecordedAt},
			})
			command := recordMovementCommand(t)
			testCase.change(&command)

			result, err := record(t, loser, command)
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

// 缺任一必备件不受理，且一次库都不碰。**把放行凭据挂到移动或到达上也在这一格**——那等于
// 造了一个不存在的门。声明的段与资源可缺，但声明了就得是一个存得下的引用。
func TestAnIncompleteMovementFactIsNotAccepted(t *testing.T) {
	cases := map[string]func(*application.RecordMovementFactCommand){
		"缺地点":        func(command *application.RecordMovementFactCommand) { command.Location = "" },
		"缺来源":        func(command *application.RecordMovementFactCommand) { command.Source = "" },
		"缺版本":        func(command *application.RecordMovementFactCommand) { command.Version = "" },
		"缺业务发生时间":    func(command *application.RecordMovementFactCommand) { command.OccurredAt = time.Time{} },
		"没有事实种类":     func(command *application.RecordMovementFactCommand) { command.Kind = domain.MovementFactKindInvalid },
		"到达挂了放行依据":   func(command *application.RecordMovementFactCommand) { command.GateClearance = "cc-clearance/v1" },
		"声明的段含 NUL":  func(command *application.RecordMovementFactCommand) { command.Segment = "segment-\x00" },
		"声明的资源含 NUL": func(command *application.RecordMovementFactCommand) { command.Resource = "truck-\x00" },
	}

	for name, breakIt := range cases {
		t.Run(name, func(t *testing.T) {
			registry := newMovementFactRegistry()
			command := recordMovementCommand(t)
			breakIt(&command)

			result, err := record(t, newMovementFactHandler(registry), command)
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

	result, err := record(t, newMovementFactHandler(registry), recordMovementCommand(t))
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
