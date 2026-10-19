package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件在真渠道真库上证票 tf-face-algebra/09：派送发起铸出的任务引用是忠实身份。触发走作业事实渠道的生产 Intake 与
// 生产端点构造，执行器经 deliveryDispatchTriggerOver 装配——段登记册、任务登记册与事务是生产那一份接法，只有三条派送
// 要求缝换成可改答、可计数的替身：真缝在空库上只会答「没有」，走不到撞键那一步（真缝的答法另由
// TestTheWiredDeliveryDispatchTriggerReachesTheOwnersAndReportsWhatTheyLack 钉住）。手工建任务口走运营决定渠道。
// 测试输入是隔离合成，只记 `S`。

const taskReferenceTenant = "SYN-TENANT-01"

// taskReferenceOwners 扮三个所有者，都答「给了」；用例可以把地点改答「没有」、让时间窗读不到、换条件引用，用来证在册
// 之后的重放不随所有者改答漂移。calls 记三条缝一共被问了几次。gate 不为空时，问条件的那一刻要等两拍都到了才放行。
type taskReferenceOwners struct {
	mu           sync.Mutex
	placeMissing bool
	windowFails  bool
	conditions   string
	calls        int
	gate         *firstTickGate
}

func newTaskReferenceOwners() *taskReferenceOwners {
	return &taskReferenceOwners{conditions: "PCC-1:SYN-RESOLUTION-09"}
}

func (owners *taskReferenceOwners) change(apply func(*taskReferenceOwners)) {
	owners.mu.Lock()
	defer owners.mu.Unlock()
	apply(owners)
}

func (owners *taskReferenceOwners) askedSoFar() int {
	owners.mu.Lock()
	defer owners.mu.Unlock()
	return owners.calls
}

func (owners *taskReferenceOwners) LoadDeliveryPlace(context.Context, tfdomain.TenantID, tfdomain.CarriedObjectReference) (string, tfports.RequirementResolution, error) {
	owners.mu.Lock()
	defer owners.mu.Unlock()
	owners.calls++
	if owners.placeMissing {
		return "", tfports.RequirementMissing, nil
	}
	return "DPR-1:SYN-TENANT-01/SYN-REQ-09/DELIVERY_PLACE/baseline", tfports.RequirementResolved, nil
}

func (owners *taskReferenceOwners) LoadDeliveryWindow(context.Context, tfdomain.TenantID, tfdomain.PlannedSegmentReference, bool) (time.Time, time.Time, tfports.RequirementResolution, error) {
	owners.mu.Lock()
	defer owners.mu.Unlock()
	owners.calls++
	if owners.windowFails {
		return time.Time{}, time.Time{}, tfports.RequirementResolutionInvalid, errors.New("network-routing 读面不可用")
	}
	return time.Date(2026, 10, 20, 1, 0, 0, 0, time.UTC), time.Date(2026, 10, 20, 4, 0, 0, 0, time.UTC), tfports.RequirementResolved, nil
}

func (owners *taskReferenceOwners) LoadDeliveryConditions(context.Context, tfdomain.TenantID, tfdomain.CarriedObjectReference) (string, tfports.RequirementResolution, error) {
	owners.mu.Lock()
	owners.calls++
	gate, conditions := owners.gate, owners.conditions
	owners.mu.Unlock()
	if gate != nil {
		gate.arrive()
	}
	return conditions, tfports.RequirementResolved, nil
}

// firstTickGate 让两拍都走到「向所有者取条件」才一起往下走：那时两笔事务都已查过在册、都没查到，接着各自去任务口
// 落库——这正是并发首拍要撞的那个窗口。超时放行时记下来，用例据此判这一轮到底测没测到并发。
type firstTickGate struct {
	mu       sync.Mutex
	arrived  int
	want     int
	release  chan struct{}
	timedOut bool
}

func newFirstTickGate(want int) *firstTickGate {
	return &firstTickGate{want: want, release: make(chan struct{})}
}

func (gate *firstTickGate) arrive() {
	gate.mu.Lock()
	gate.arrived++
	if gate.arrived == gate.want {
		close(gate.release)
	}
	gate.mu.Unlock()
	select {
	case <-gate.release:
	case <-time.After(10 * time.Second):
		gate.mu.Lock()
		gate.timedOut = true
		gate.mu.Unlock()
	}
}

type taskReferenceRig struct {
	pool         *pgxpool.Pool
	db           *bentopg.DB
	controlFacts controlFactOrchestrations
	owners       *taskReferenceOwners
	trigger      http.Handler
}

func newTaskReferenceRig(t *testing.T) *taskReferenceRig {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	controlFacts, err := buildControlFactOrchestrations(db)
	if err != nil {
		t.Fatalf("装配控制事实编排：%v", err)
	}
	owners := newTaskReferenceOwners()
	trigger, err := deliveryDispatchTriggerOver(db, owners, owners, owners)
	if err != nil {
		t.Fatalf("装配派送发起执行器：%v", err)
	}
	return &taskReferenceRig{
		pool:         pool,
		db:           db,
		controlFacts: controlFacts,
		owners:       owners,
		trigger:      tfhttp.NewTriggerDeliveryDispatchEndpoint(workFactTransportForTest(t), trigger),
	}
}

// enter 让 object 凭`已交接`进入 segment。入场依据是「TRANSPORT-HANDOVER/」加交接版本，撞键用例靠段引用与交接版本构造。
func (rig *taskReferenceRig) enter(t *testing.T, object, segment, scope, version string, action tfdomain.SegmentServiceAction) {
	t.Helper()
	command := registerHandoverCommand(t, object, segment, "")
	command.TenantID = mustValue(t, tfdomain.NewTenantID, taskReferenceTenant)
	command.Scope = scope
	command.Version = version
	command.SegmentServiceAction = action.String()
	result, err := rig.controlFacts.handover.Register(t.Context(), command)
	if err != nil || result.Outcome() != tfapp.HandoverRegistered || result.SegmentContinuationReference() != "" ||
		result.SegmentEntryRefusal() != tfapp.SegmentEntryRefusalNone {
		t.Fatalf("%s 进 %s：err=%v outcome=%v debt=%q refusal=%v", object, segment, err, result.Outcome(),
			result.SegmentContinuationReference(), result.SegmentEntryRefusal())
	}
}

type taskReferenceView struct {
	Outcome    string   `json:"outcome"`
	Task       string   `json:"task"`
	Kind       string   `json:"kind"`
	Objects    []string `json:"objects"`
	Place      string   `json:"place"`
	WindowFrom string   `json:"windowFrom"`
	WindowTo   string   `json:"windowTo"`
	Conditions string   `json:"conditions"`
	OpenedAt   string   `json:"openedAt"`
}

func (view taskReferenceView) echoesTheSameTaskAs(other taskReferenceView) bool {
	return view.Task == other.Task && view.Kind == other.Kind && fmt.Sprint(view.Objects) == fmt.Sprint(other.Objects) &&
		view.Place == other.Place && view.WindowFrom == other.WindowFrom && view.WindowTo == other.WindowTo &&
		view.Conditions == other.Conditions && view.OpenedAt == other.OpenedAt
}

func (rig *taskReferenceRig) tick(t *testing.T, segment, object, occurredAt string) (int, taskReferenceView, string) {
	t.Helper()
	body, err := json.Marshal(map[string]string{"segment": segment, "object": object, "occurredAt": occurredAt})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/transport-fulfillment-delivery-dispatch-triggers", strings.NewReader(string(body)))
	stampWorkFact(request, "OFFSITE", "SYN-OFFSITE-01", "SYN-DEVICE-OFFSITE")
	recorder := httptest.NewRecorder()
	rig.trigger.ServeHTTP(recorder, request)
	var view taskReferenceView
	_ = json.Unmarshal(recorder.Body.Bytes(), &view)
	return recorder.Code, view, recorder.Body.String()
}

func (rig *taskReferenceRig) taskRows(t *testing.T) int {
	t.Helper()
	var rows int
	if err := rig.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM transport_fulfillment.dispatch_task WHERE tenant_id = $1`, taskReferenceTenant).Scan(&rows); err != nil {
		t.Fatalf("数任务行：%v", err)
	}
	return rows
}

// formsItsOwnTask 断言这一拍新形成了一项只装本对象的末端派送任务。
func (rig *taskReferenceRig) formsItsOwnTask(t *testing.T, segment, object string) taskReferenceView {
	t.Helper()
	code, view, raw := rig.tick(t, segment, object, "2026-10-19T07:00:00+08:00")
	if code != http.StatusCreated || view.Outcome != "DISPATCH_TASK_FORMED" || view.Kind != "DELIVERY" ||
		len(view.Objects) != 1 || view.Objects[0] != object {
		t.Fatalf("段 %s 对象 %s 那一拍 → %d %s，want 201 DISPATCH_TASK_FORMED 且任务只装 %s", segment, object, code, raw, object)
	}
	return view
}

// executorReference 照新铸法手算执行器的任务引用；application 的编排单测另钉了一个字面例子。
func executorReference(segment, object, version string) string {
	basis := "TRANSPORT-HANDOVER/" + version
	return fmt.Sprintf("DELIVERY-DISPATCH/v2/%d:%s/%d:%s/%d:%s", len(segment), segment, len(object), object, len(basis), basis)
}

func legacyExecutorReference(segment, object, version string) string {
	return "DELIVERY-DISPATCH/" + segment + "/" + object + "/TRANSPORT-HANDOVER/" + version
}

// stockTask 经任务编排（不经 HTTP 载荷翻译）在库里立一项任务。保留起头拦在手工口的载荷翻译里、不在编排里，存量里指名过
// 执行器引用的任务就是经这条编排落库的；用例拿它造「库里已有一项这样的存量任务」的前提——存量不回写。
func stockTask(t *testing.T, db *bentopg.DB, task string, kind tfdomain.DispatchTaskKind, object string) {
	t.Helper()
	operations, err := buildSegmentOperations(db)
	if err != nil {
		t.Fatalf("装配段运营编排：%v", err)
	}
	opened, err := operations.opener.Open(t.Context(), tfapp.OpenDispatchTaskCommand{
		TenantID:   mustValue(t, tfdomain.NewTenantID, taskReferenceTenant),
		Task:       task,
		Kind:       kind,
		Objects:    []string{object},
		Place:      "SYN-PLACE/elsewhere",
		WindowFrom: time.Date(2026, 10, 18, 1, 0, 0, 0, time.UTC),
		WindowTo:   time.Date(2026, 10, 18, 4, 0, 0, 0, time.UTC),
		Conditions: "SYN-CONDITION/other",
		OpenedAt:   time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC),
	})
	if err != nil || opened.Outcome() != tfapp.DispatchTaskOpened {
		t.Fatalf("立存量任务 %s：%v %q", task, err, opened.Outcome())
	}
}

// Covers: 票 tf-face-algebra/09 完成判据「真渠道真库回归：手工口占用：答新格并回显占用者」与裁定第 2、4 条。手工建任务口
// 指名执行器的引用（新拼法、旧拼法都算）答 400 MALFORMED_REQUEST、不带 outcome、库里不落行；存量里若有一项手工任务恰好
// 占着本拍的键，派送发起答 200 `DISPATCH_TASK_REFERENCE_TAKEN` 并回显占用者——不写、不另铸、不向所有者取件。
func TestTheManualFaceCannotTakeTheExecutorsReferenceAndAStockOccupantIsAnsweredAsTaken(t *testing.T) {
	rig := newTaskReferenceRig(t)
	const object, segment, version = "SYN-PARCEL-09-01", "SYN-DELIVERY-SEGMENT-09-01", "SYN-HANDOVER-RESULT/09-01/v1"
	rig.enter(t, object, segment, "SYN-HANDOVER-SCOPE-09-01", version, tfdomain.SegmentServesFinalDelivery)
	reference := executorReference(segment, object, version)

	operations, err := buildSegmentOperations(rig.db)
	if err != nil {
		t.Fatalf("装配段运营编排：%v", err)
	}
	manual := tfhttp.NewOpenDispatchTaskEndpoint(operatorDecisionTransportForTest(t, rig.db), operations.opener)
	for _, named := range []string{reference, legacyExecutorReference(segment, object, version)} {
		request := httptest.NewRequest(http.MethodPost, "/transport-fulfillment-dispatch-task-registrations", strings.NewReader(fmt.Sprintf(
			`{"task":%q,"kind":"PICKUP","objects":["SYN-PARCEL-OTHER-99"],"place":"SYN-PLACE/elsewhere",`+
				`"windowFrom":"2026-10-18T09:00:00+08:00","windowTo":"2026-10-18T12:00:00+08:00","conditions":"SYN-CONDITION/other",`+
				`"openedAt":"2026-10-17T20:00:00+08:00"}`, named)))
		stampWorkFact(request, "OFFSITE", "SYN-OFFSITE-01", "SYN-DEVICE-OFFSITE")
		recorder := httptest.NewRecorder()
		manual.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest || problemCode(t, recorder) != "MALFORMED_REQUEST" {
			t.Fatalf("手工口指名 %s → %d %s，want 400 MALFORMED_REQUEST", named, recorder.Code, recorder.Body.String())
		}
		assertNoOutcome(t, recorder, "/transport-fulfillment-dispatch-task-registrations")
	}
	if rows := rig.taskRows(t); rows != 0 {
		t.Fatalf("手工口拒了却落了 %d 行任务", rows)
	}

	stockTask(t, rig.db, reference, tfdomain.PickupDispatch, "SYN-PARCEL-OTHER-99")
	code, view, raw := rig.tick(t, segment, object, "2026-10-19T07:00:00+08:00")
	if code != http.StatusOK || view.Outcome != "DISPATCH_TASK_REFERENCE_TAKEN" || view.Task != reference || view.Kind != "PICKUP" ||
		len(view.Objects) != 1 || view.Objects[0] != "SYN-PARCEL-OTHER-99" || view.Conditions != "SYN-CONDITION/other" {
		t.Fatalf("键被存量占着的那一拍 → %d %s，want 200 DISPATCH_TASK_REFERENCE_TAKEN 回显占用者", code, raw)
	}
	if rows := rig.taskRows(t); rows != 1 {
		t.Fatalf("引用被占后任务行 = %d，want 仍只有占用者那 1 行", rows)
	}
	if asked := rig.owners.askedSoFar(); asked != 0 {
		t.Fatalf("引用被占却向所有者取了 %d 次件", asked)
	}
}

// Covers: 票 09 裁定第 1 条在真库上——旧拼法兼认、存量不回写。旧拼法键上是本拍那一项（执行器按旧拼法铸的），答已在册并
// 回显它、不另立；旧拼法键上是别的任务（手工口指名过那一串），不算在册，对象照新键形成自己的任务，不会在入场依据
// 不变期间一直形成不了派送任务。
func TestStockTasksOnTheLegacySpellingAreRecognisedOrStepAround(t *testing.T) {
	rig := newTaskReferenceRig(t)
	const formedObject, blockedObject = "SYN-PARCEL-09-02", "SYN-PARCEL-09-03"
	const segment = "SYN-DELIVERY-SEGMENT-09-02"
	rig.enter(t, formedObject, segment, "SYN-HANDOVER-SCOPE-09-02", "SYN-HANDOVER-RESULT/09-02/v1", tfdomain.SegmentServesFinalDelivery)
	rig.enter(t, blockedObject, segment, "SYN-HANDOVER-SCOPE-09-03", "SYN-HANDOVER-RESULT/09-03/v1", tfdomain.SegmentServesFinalDelivery)
	legacyFormed := legacyExecutorReference(segment, formedObject, "SYN-HANDOVER-RESULT/09-02/v1")
	stockTask(t, rig.db, legacyFormed, tfdomain.DeliveryDispatch, formedObject)
	stockTask(t, rig.db, legacyExecutorReference(segment, blockedObject, "SYN-HANDOVER-RESULT/09-03/v1"), tfdomain.PickupDispatch, "SYN-PARCEL-OTHER-99")

	code, view, raw := rig.tick(t, segment, formedObject, "2026-10-19T07:00:00+08:00")
	if code != http.StatusOK || view.Outcome != "DISPATCH_TASK_ALREADY_FORMED" || view.Task != legacyFormed {
		t.Fatalf("旧拼法已在册的对象 → %d %s，want 200 DISPATCH_TASK_ALREADY_FORMED 回显旧拼法那一项", code, raw)
	}
	if rows := rig.taskRows(t); rows != 2 {
		t.Fatalf("兼认旧拼法后任务行 = %d，want 2——存量不回写、不另立", rows)
	}

	formed := rig.formsItsOwnTask(t, segment, blockedObject)
	if formed.Task != executorReference(segment, blockedObject, "SYN-HANDOVER-RESULT/09-03/v1") {
		t.Fatalf("新任务引用 = %s，want 新铸法那一串", formed.Task)
	}
	if rows := rig.taskRows(t); rows != 3 {
		t.Fatalf("任务行 = %d，want 3——旧键上的存量原样留着", rows)
	}
}

// Covers: 票 09 完成判据「真渠道真库回归：拼接撞键：两个对象各自形成任务」。段 `…/A` 加对象 `SYN-CU-77` 与段 `…` 加对象
// `A/SYN-CU-77`，交接版本同为 v1（交接键含对象，不同对象同版本合法）。
func TestConcatenationCollidingObjectsEachFormTheirOwnTask(t *testing.T) {
	rig := newTaskReferenceRig(t)
	rig.enter(t, "SYN-CU-77", "SYN-SEG-09/A", "SYN-HANDOVER-SCOPE-CAT", "v1", tfdomain.SegmentServesFinalDelivery)
	rig.enter(t, "A/SYN-CU-77", "SYN-SEG-09", "SYN-HANDOVER-SCOPE-CAT", "v1", tfdomain.SegmentServesFinalDelivery)
	if legacyExecutorReference("SYN-SEG-09/A", "SYN-CU-77", "v1") != legacyExecutorReference("SYN-SEG-09", "A/SYN-CU-77", "v1") {
		t.Fatal("夹具没构造出旧拼法下的撞键")
	}

	first := rig.formsItsOwnTask(t, "SYN-SEG-09/A", "SYN-CU-77")
	second := rig.formsItsOwnTask(t, "SYN-SEG-09", "A/SYN-CU-77")
	if first.Task == second.Task {
		t.Fatalf("两个对象铸出同一个引用 %s", first.Task)
	}
	if rows := rig.taskRows(t); rows != 2 {
		t.Fatalf("任务行 = %d，want 2", rows)
	}
}

// Covers: 票 09 完成判据「真渠道真库回归：生产形状对象撞键：两个对象各自形成任务」。对象取不含斜杠的生产形状，只靠精心
// 构造的段引用与交接版本让旧拼法撞上：第二段的引用嵌第一个对象，第一次交接的版本嵌第二个对象。
func TestProductionShapedObjectsCraftedToCollideEachFormTheirOwnTask(t *testing.T) {
	rig := newTaskReferenceRig(t)
	const first, second = "SYN-DPCL-0901", "SYN-DPCL-0902"
	firstSegment, firstVersion := "SYN-SEG-09-V", "X/"+second+"/TRANSPORT-HANDOVER/Y"
	secondSegment, secondVersion := "SYN-SEG-09-V/"+first+"/TRANSPORT-HANDOVER/X", "Y"
	rig.enter(t, first, firstSegment, "SYN-SCOPE-09-V1", firstVersion, tfdomain.SegmentServesFinalDelivery)
	rig.enter(t, second, secondSegment, "SYN-SCOPE-09-V2", secondVersion, tfdomain.SegmentServesFinalDelivery)
	if legacyExecutorReference(firstSegment, first, firstVersion) != legacyExecutorReference(secondSegment, second, secondVersion) {
		t.Fatal("夹具没构造出旧拼法下的撞键")
	}

	firstTask := rig.formsItsOwnTask(t, firstSegment, first)
	secondTask := rig.formsItsOwnTask(t, secondSegment, second)
	if firstTask.Task == secondTask.Task {
		t.Fatalf("两个对象铸出同一个引用 %s", firstTask.Task)
	}
	if rows := rig.taskRows(t); rows != 2 {
		t.Fatalf("任务行 = %d，want 2", rows)
	}
}

// Covers: 票 09 完成判据「真渠道真库回归：同对象跨段：在第二段形成自己的任务」。同一对象换到第二个派送段，段与第一次
// 交接的版本经构造后让旧拼法撞上第一段那项任务——那时在册任务的种类与对象集都对得上，只核这两样拦不住，靠的是单射铸法。
func TestTheSameObjectInASecondDeliverySegmentFormsItsOwnTask(t *testing.T) {
	rig := newTaskReferenceRig(t)
	const parcel = "SYN-DPCL-0903"
	firstSegment, firstVersion := "SYN-SEG-09-U", "X/"+parcel+"/TRANSPORT-HANDOVER/Y"
	secondSegment, secondVersion := "SYN-SEG-09-U/"+parcel+"/TRANSPORT-HANDOVER/X", "Y"
	if legacyExecutorReference(firstSegment, parcel, firstVersion) != legacyExecutorReference(secondSegment, parcel, secondVersion) {
		t.Fatal("夹具没构造出旧拼法下的撞键")
	}

	rig.enter(t, parcel, firstSegment, "SYN-SCOPE-09-U1", firstVersion, tfdomain.SegmentServesFinalDelivery)
	firstTask := rig.formsItsOwnTask(t, firstSegment, parcel)
	rig.enter(t, parcel, secondSegment, "SYN-SCOPE-09-U2", secondVersion, tfdomain.SegmentServesFinalDelivery)
	secondTask := rig.formsItsOwnTask(t, secondSegment, parcel)
	if firstTask.Task == secondTask.Task {
		t.Fatalf("两段铸出同一个引用 %s", firstTask.Task)
	}
	if rows := rig.taskRows(t); rows != 2 {
		t.Fatalf("任务行 = %d，want 2", rows)
	}
}

// Covers: 票 09 完成判据「真渠道真库回归：所有者改答后原样重放：答已在册并回显」与裁定第 3 条。首拍形成之后，地点所有者
// 改答没有、时间窗所有者读不到、条件所有者换了引用、对象凭下一次交接离开派送段——原样重放一律 200
// `DISPATCH_TASK_ALREADY_FORMED`，回显与首拍逐格相同，不再向所有者取件，库里仍只有那一行。
func TestAReplayAfterTheOwnersChangeTheirAnswersEchoesTheRegisteredTask(t *testing.T) {
	rig := newTaskReferenceRig(t)
	const object, segment = "SYN-PARCEL-09-04", "SYN-DELIVERY-SEGMENT-09-04"
	rig.enter(t, object, segment, "SYN-HANDOVER-SCOPE-09-04", "SYN-HANDOVER-RESULT/09-04/v1", tfdomain.SegmentServesFinalDelivery)
	formed := rig.formsItsOwnTask(t, segment, object)
	asked := rig.owners.askedSoFar()

	replay := func(situation string) {
		t.Helper()
		code, view, raw := rig.tick(t, segment, object, "2026-10-19T07:00:00+08:00")
		if code != http.StatusOK || view.Outcome != "DISPATCH_TASK_ALREADY_FORMED" || !view.echoesTheSameTaskAs(formed) {
			t.Fatalf("%s后原样重放 → %d %s，want 200 DISPATCH_TASK_ALREADY_FORMED 回显首拍那一项", situation, code, raw)
		}
		if now := rig.owners.askedSoFar(); now != asked {
			t.Fatalf("%s后重放又向所有者取了 %d 次件", situation, now-asked)
		}
		if rows := rig.taskRows(t); rows != 1 {
			t.Fatalf("%s后任务行 = %d，want 1", situation, rows)
		}
	}

	rig.owners.change(func(owners *taskReferenceOwners) { owners.placeMissing = true })
	replay("地点所有者改答没有")
	rig.owners.change(func(owners *taskReferenceOwners) { owners.windowFails = true })
	replay("时间窗所有者读不到")
	rig.owners.change(func(owners *taskReferenceOwners) { owners.conditions = "PCC-1:SYN-RESOLUTION-09-B" })
	replay("条件所有者换了引用")
	rig.enter(t, object, "SYN-LINEHAUL-SEGMENT-09-05", "SYN-HANDOVER-SCOPE-09-05", "SYN-HANDOVER-RESULT/09-04/v2", tfdomain.SegmentServesLinehaul)
	replay("对象离开派送段")
}

// Covers: 票 09 完成判据「并发首拍真库回归照过：一笔 201，另一笔读回赢家答已在册」。两拍都查过在册、都没查到，在问条件
// 那一刻会合后各自去任务口落库：先提交的那笔 201，另一笔撞键读回赢家，种类与对象集核得过，答 200 已在册、回显同一项。
func TestConcurrentFirstTicksFormOneTaskAndTheOtherReadsTheWinner(t *testing.T) {
	rig := newTaskReferenceRig(t)
	const object, segment = "SYN-PARCEL-09-06", "SYN-DELIVERY-SEGMENT-09-06"
	rig.enter(t, object, segment, "SYN-HANDOVER-SCOPE-09-06", "SYN-HANDOVER-RESULT/09-06/v1", tfdomain.SegmentServesFinalDelivery)
	gate := newFirstTickGate(2)
	rig.owners.change(func(owners *taskReferenceOwners) { owners.gate = gate })

	type answer struct {
		code int
		view taskReferenceView
		raw  string
	}
	answers := make([]answer, 2)
	var group sync.WaitGroup
	for index, occurredAt := range []string{"2026-10-19T07:00:00+08:00", "2026-10-19T07:05:00+08:00"} {
		group.Add(1)
		go func() {
			defer group.Done()
			code, view, raw := rig.tick(t, segment, object, occurredAt)
			answers[index] = answer{code: code, view: view, raw: raw}
		}()
	}
	group.Wait()
	if gate.timedOut {
		t.Fatal("两拍没有在取件处会合——这一轮没测到并发首拍")
	}

	var formed, replayed *answer
	for index := range answers {
		switch {
		case answers[index].code == http.StatusCreated && answers[index].view.Outcome == "DISPATCH_TASK_FORMED":
			formed = &answers[index]
		case answers[index].code == http.StatusOK && answers[index].view.Outcome == "DISPATCH_TASK_ALREADY_FORMED":
			replayed = &answers[index]
		}
	}
	if formed == nil || replayed == nil {
		t.Fatalf("两拍答 %d %s / %d %s，want 一笔 201 DISPATCH_TASK_FORMED、一笔 200 DISPATCH_TASK_ALREADY_FORMED",
			answers[0].code, answers[0].raw, answers[1].code, answers[1].raw)
	}
	if !replayed.view.echoesTheSameTaskAs(formed.view) {
		t.Fatalf("输家回显 %s，want 赢家那一项 %s", replayed.raw, formed.raw)
	}
	if rows := rig.taskRows(t); rows != 1 {
		t.Fatalf("任务行 = %d，want 1", rows)
	}
}
