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
	headErr error
	saveErr error
	saves   int
	// rival 在本次插入之前落进另一位写入方的版本：编排读链头与写之间别人先提交了。
	rival *ports.LoadAssignmentRecord
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

// FindChainHead 照库面的派生取链头：同一分配里没被任何版本回指的那一版。
func (double *loadAssignmentRegistryDouble) FindChainHead(
	_ context.Context,
	tenant domain.TenantID,
	assignment domain.LoadAssignmentReference,
) (ports.LoadAssignmentRecord, bool, error) {
	if double.headErr != nil {
		return ports.LoadAssignmentRecord{}, false, double.headErr
	}
	corrected := map[domain.LoadAssignmentVersion]bool{}
	for _, record := range double.records {
		if prior, has := record.Assignment.Corrects(); has && record.Key.TenantID == tenant && record.Key.Assignment == assignment {
			corrected[prior] = true
		}
	}
	for _, record := range double.records {
		if record.Key.TenantID == tenant && record.Key.Assignment == assignment && !corrected[record.Key.Version] {
			return record, true, nil
		}
	}
	return ports.LoadAssignmentRecord{}, false, nil
}

func (double *loadAssignmentRegistryDouble) Save(
	_ context.Context,
	record ports.LoadAssignmentRecord,
) (ports.LoadAssignmentSaveOutcome, error) {
	double.saves++
	if double.saveErr != nil {
		return ports.LoadAssignmentSaveOutcomeInvalid, double.saveErr
	}
	if double.rival != nil {
		double.records[loadAssignmentRegistryKey(double.rival.Key)] = *double.rival
		double.rival = nil
	}
	if _, exists := double.records[loadAssignmentRegistryKey(record.Key)]; exists {
		return ports.LoadAssignmentVersionAlreadyRegistered, nil
	}
	// 首版唯一索引的替身：同一分配已有一个不回指前版的版本，另一个首版同样撞上。
	if _, corrects := record.Assignment.Corrects(); !corrects {
		for _, other := range double.records {
			_, otherCorrects := other.Assignment.Corrects()
			if !otherCorrects && other.Key.TenantID == record.Key.TenantID && other.Key.Assignment == record.Key.Assignment {
				return ports.LoadAssignmentVersionAlreadyRegistered, nil
			}
		}
	}
	double.records[loadAssignmentRegistryKey(record.Key)] = record
	return ports.LoadAssignmentSaved, nil
}

// registeredSchedulesDouble 是班次登记的只读替身：键是（租户 + 班次），与库面同键，他租户的同名班次因此天然读不回。
type registeredSchedulesDouble struct {
	registered map[string]bool
	findErr    error
	reads      int
}

func registeredSchedules(tenantAndSchedule ...[2]string) *registeredSchedulesDouble {
	double := &registeredSchedulesDouble{registered: map[string]bool{}}
	for _, pair := range tenantAndSchedule {
		double.registered[pair[0]+"|"+pair[1]] = true
	}
	return double
}

func (double *registeredSchedulesDouble) FindByKey(
	_ context.Context,
	key ports.ScheduleKey,
) (ports.ScheduleRecord, bool, error) {
	double.reads++
	if double.findErr != nil {
		return ports.ScheduleRecord{}, false, double.findErr
	}
	if !double.registered[key.TenantID.String()+"|"+key.Schedule.String()] {
		return ports.ScheduleRecord{}, false, nil
	}
	return ports.ScheduleRecord{Key: key}, true, nil
}

// newLoadAssignmentHandler 配一份只登了夹具班次的班次登记，班次核对不是重点的用例都走它。
func newLoadAssignmentHandler(registry *loadAssignmentRegistryDouble) *application.FormLoadAssignmentHandler {
	return newLoadAssignmentHandlerOver(registry, registeredSchedules([2]string{"tenant-1", "schedule-1"}))
}

func newLoadAssignmentHandlerOver(
	registry *loadAssignmentRegistryDouble,
	schedules *registeredSchedulesDouble,
) *application.FormLoadAssignmentHandler {
	return newLoadAssignmentHandlerWith(registry, schedules, assignmentPools())
}

func newLoadAssignmentHandlerWith(
	registry *loadAssignmentRegistryDouble,
	schedules *registeredSchedulesDouble,
	pools *assignmentPoolsDouble,
) *application.FormLoadAssignmentHandler {
	return application.NewFormLoadAssignmentHandler(application.FormLoadAssignmentDeps{
		Assignments: registry,
		Schedules:   schedules,
		Pools:       pools,
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

// 同一分配换一个版本号再投答`分配已形成`并交回链头，不登记：本口只形成一次分配的首版，换版本号不是变化——
// 变化要回指前版、沿用分配时刻，走领域 ReviseMembers 的入口；在这里另起一个首版，一次分配在库里就成了两条链。
func TestAnotherVersionOfAFormedAssignmentAnswersAlreadyFormed(t *testing.T) {
	registry := newLoadAssignmentRegistry()
	handler := newLoadAssignmentHandler(registry)
	if _, err := handler.Form(t.Context(), formAssignmentCommand(t)); err != nil {
		t.Fatalf("首版：%v", err)
	}
	savesAfterFirst := registry.saves

	another := formAssignmentCommand(t)
	another.Version = "LAV-000000000002"
	another.Schedule = "schedule-2"
	another.Members = []string{"parcel-9"}
	another.AssignedAt = assignedAtFixture.Add(-24 * time.Hour)
	result, err := handler.Form(t.Context(), another)
	if err != nil {
		t.Fatalf("另一版本号：%v", err)
	}
	if result.Outcome() != application.LoadAssignmentAlreadyFormed {
		t.Fatalf("outcome = %q, want LOAD_ASSIGNMENT_ALREADY_FORMED", result.Outcome())
	}
	head, recorded := result.Record()
	if !recorded || head.Key.Version.String() != "LAV-000000000001" || head.Assignment.Schedule().String() != "schedule-1" {
		t.Fatalf("没有交回链头首版：%+v recorded=%v", head.Key, recorded)
	}
	if registry.saves != savesAfterFirst {
		t.Fatalf("已形成的分配又登记了 %d 次", registry.saves-savesAfterFirst)
	}
}

// 已撤回的分配再投新版本号同答`分配已形成`，交回的链头就是撤回版：领域 Withdraw「后续要装载只能是新的分配」，
// 在已撤回的链旁另起一个活跃首版正是它要防的。
func TestAWithdrawnAssignmentAnswersAlreadyFormedWithTheWithdrawal(t *testing.T) {
	registry := newLoadAssignmentRegistry()
	handler := newLoadAssignmentHandler(registry)
	formed, err := handler.Form(t.Context(), formAssignmentCommand(t))
	if err != nil {
		t.Fatalf("首版：%v", err)
	}
	first, _ := formed.Record()
	withdrawalVersion, err := domain.NewLoadAssignmentVersion("LAV-000000000002")
	if err != nil {
		t.Fatal(err)
	}
	withdrawn, err := first.Assignment.Withdraw(withdrawalVersion, assignedAtFixture.Add(time.Hour))
	if err != nil {
		t.Fatalf("撤回：%v", err)
	}
	withdrawalKey := first.Key
	withdrawalKey.Version = withdrawalVersion
	registry.records[loadAssignmentRegistryKey(withdrawalKey)] = ports.LoadAssignmentRecord{
		Key: withdrawalKey, Assignment: withdrawn, RecordedAt: assignmentRecordedAt.Add(time.Hour),
	}
	savesBefore := registry.saves

	later := formAssignmentCommand(t)
	later.Version = "LAV-000000000003"
	result, err := handler.Form(t.Context(), later)
	if err != nil {
		t.Fatalf("撤回之后再投：%v", err)
	}
	if result.Outcome() != application.LoadAssignmentAlreadyFormed {
		t.Fatalf("outcome = %q, want LOAD_ASSIGNMENT_ALREADY_FORMED", result.Outcome())
	}
	head, _ := result.Record()
	if _, isWithdrawn := head.Assignment.Withdrawn(); !isWithdrawn || head.Key.Version != withdrawalVersion {
		t.Fatalf("交回的链头不是撤回版：%+v", head.Key)
	}
	if registry.saves != savesBefore {
		t.Fatal("已撤回的分配旁又登记了一个首版")
	}
}

// 读链头失败与写失败同答`未决`，且不登记：读不到链头就判断不了「这次分配形成过没有」，贸然写就可能另起一根。
func TestAChainHeadReadFailureIsUndecided(t *testing.T) {
	registry := newLoadAssignmentRegistry()
	registry.headErr = assignmentRegistryError

	result, err := newLoadAssignmentHandler(registry).Form(t.Context(), formAssignmentCommand(t))
	if err != nil {
		t.Fatalf("登记册故障不该上抛：%v", err)
	}
	if result.Outcome() != application.LoadAssignmentUndecided || result.ContinuationReference() == "" {
		t.Fatalf("outcome = %q continuation=%q, want 未决带续办引用", result.Outcome(), result.ContinuationReference())
	}
	if registry.saves != 0 {
		t.Fatalf("读不到链头却写了 %d 次库", registry.saves)
	}
}

// 读链头与写之间另一位写入方先落了首版：插入撞上首版唯一索引答`已登记`，本次的键却读不回——按分配读回链头答
// `分配已形成`交回赢家，不宣称自己形成了它，也不因按本次键读空就落成`未决`。
func TestALosingConcurrentFirstVersionAnswersAlreadyFormedWithTheWinner(t *testing.T) {
	winnerCommand := formAssignmentCommand(t)
	winnerCommand.Version = "LAV-000000000009"
	formed, err := newLoadAssignmentHandler(newLoadAssignmentRegistry()).Form(t.Context(), winnerCommand)
	if err != nil {
		t.Fatalf("赢家夹具：%v", err)
	}
	winner, _ := formed.Record()
	registry := newLoadAssignmentRegistry()
	registry.rival = &winner

	result, err := newLoadAssignmentHandler(registry).Form(t.Context(), formAssignmentCommand(t))
	if err != nil {
		t.Fatalf("并发落败：%v", err)
	}
	if result.Outcome() != application.LoadAssignmentAlreadyFormed {
		t.Fatalf("outcome = %q, want LOAD_ASSIGNMENT_ALREADY_FORMED", result.Outcome())
	}
	if head, _ := result.Record(); head.Key.Version != winner.Key.Version {
		t.Fatalf("交回的不是赢家：%+v", head.Key)
	}
	if len(registry.records) != 1 {
		t.Fatalf("登记册里 %d 个版本, want 只有赢家", len(registry.records))
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

// Covers: 票 demo-chain-gaps/19（AT-TF-099）——分配引用的班次不在本租户班次登记里，答`班次不在册`且不登记：从没登过的、乱填的、
// 只登在他租户名下的同名班次同落这一格。续办是去登班次或换班次，与改载荷的`输入未受理`分开（ADR-0029）。
func TestALoadAssignmentOnAnUnregisteredScheduleAnswersScheduleNotFound(t *testing.T) {
	cases := map[string]*registeredSchedulesDouble{
		"班次登记为空":       registeredSchedules(),
		"本租户只登了别的班次":   registeredSchedules([2]string{"tenant-1", "schedule-9"}),
		"同名班次只登在他租户名下": registeredSchedules([2]string{"tenant-2", "schedule-1"}),
	}
	for name, schedules := range cases {
		t.Run(name, func(t *testing.T) {
			registry := newLoadAssignmentRegistry()

			result, err := newLoadAssignmentHandlerOver(registry, schedules).Form(t.Context(), formAssignmentCommand(t))
			if err != nil {
				t.Fatalf("班次不在册不该上抛：%v", err)
			}
			if result.Outcome() != application.LoadAssignmentScheduleNotFound {
				t.Fatalf("outcome = %q, want SCHEDULE_NOT_FOUND", result.Outcome())
			}
			if _, recorded := result.Record(); recorded || result.ContinuationReference() != "" {
				t.Fatal("班次不在册不该回显记录，也不该带续办引用")
			}
			if registry.saves != 0 {
				t.Fatalf("班次不在册却写了 %d 次库", registry.saves)
			}
		})
	}
}

// Covers: 票 demo-chain-gaps/19（AT-TF-099）——班次登记读不动答`未决`带续办引用，且不登记：读不到就判断不了班次在不在册，
// 原样重试就可能过，与`班次不在册`的续办相反。
func TestAScheduleRegistryReadFailureIsUndecided(t *testing.T) {
	registry := newLoadAssignmentRegistry()
	schedules := registeredSchedules([2]string{"tenant-1", "schedule-1"})
	schedules.findErr = errors.New("班次登记不可用")

	result, err := newLoadAssignmentHandlerOver(registry, schedules).Form(t.Context(), formAssignmentCommand(t))
	if err != nil {
		t.Fatalf("班次登记故障不该上抛：%v", err)
	}
	if result.Outcome() != application.LoadAssignmentUndecided || result.ContinuationReference() == "" {
		t.Fatalf("outcome = %q continuation=%q, want 未决带续办引用", result.Outcome(), result.ContinuationReference())
	}
	if registry.saves != 0 {
		t.Fatalf("读不到班次登记却写了 %d 次库", registry.saves)
	}
}

// Covers: 票 demo-chain-gaps/19（AT-TF-099）——班次核对排在重放与链头之后：同一版本重投、同一分配换版本号再投，即便这次送来的
// 班次不在册（甚至班次登记读不动），仍各答`已在册`与`分配已形成`并回显库里那一版，一次都不读班次登记。
func TestReplayAndChainHeadAnswerBeforeTheScheduleIsChecked(t *testing.T) {
	registry := newLoadAssignmentRegistry()
	if _, err := newLoadAssignmentHandler(registry).Form(t.Context(), formAssignmentCommand(t)); err != nil {
		t.Fatalf("首版：%v", err)
	}
	savesAfterFirst := registry.saves

	for name, schedules := range map[string]*registeredSchedulesDouble{
		"班次登记为空":  registeredSchedules(),
		"班次登记读不动": {registered: map[string]bool{}, findErr: errors.New("班次登记不可用")},
	} {
		t.Run(name, func(t *testing.T) {
			handler := newLoadAssignmentHandlerOver(registry, schedules)

			replay := formAssignmentCommand(t)
			replay.Schedule = "schedule-unregistered"
			replayed, err := handler.Form(t.Context(), replay)
			if err != nil || replayed.Outcome() != application.LoadAssignmentVersionExists {
				t.Fatalf("同版本重投：outcome=%q err=%v, want LOAD_ASSIGNMENT_VERSION_EXISTS", replayed.Outcome(), err)
			}
			if record, _ := replayed.Record(); record.Assignment.Schedule().String() != "schedule-1" {
				t.Fatalf("重放没有回显原版本的班次：%q", record.Assignment.Schedule())
			}

			another := formAssignmentCommand(t)
			another.Version = "LAV-000000000002"
			another.Schedule = "schedule-unregistered"
			again, err := handler.Form(t.Context(), another)
			if err != nil || again.Outcome() != application.LoadAssignmentAlreadyFormed {
				t.Fatalf("换版本号再投：outcome=%q err=%v, want LOAD_ASSIGNMENT_ALREADY_FORMED", again.Outcome(), err)
			}

			if schedules.reads != 0 {
				t.Fatalf("重放与链头之前读了 %d 次班次登记", schedules.reads)
			}
			if registry.saves != savesAfterFirst {
				t.Fatalf("又登记了 %d 次", registry.saves-savesAfterFirst)
			}
		})
	}
}
