package application_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 票 tf-face-algebra/09：派送发起铸出的任务引用要是忠实身份——「已在册」认的必须是本拍要的那一项任务。本文件证执行器
// 一侧：新铸法单射；旧拼法在册的任务兼认；键被别的任务占着答`引用被占`并回显占用者；在册之后不再向所有者取件。
// 夹具全为合成登记（S 级）。

// newShapeReference 照新铸法手算本拍的任务引用：根、形状标记，再把段、对象、入场依据各写成「/字节长度:原串」。
// TestTheTaskReferenceSpellsEachPartWithItsLength 另钉一个字面例子——这里的手算与执行器的铸法若一起错了，那一条会红。
func newShapeReference(segment, object, basis string) string {
	return fmt.Sprintf("DELIVERY-DISPATCH/v2/%d:%s/%d:%s/%d:%s", len(segment), segment, len(object), object, len(basis), basis)
}

// legacyShapeReference 是旧拼法：三件以斜杠直拼。旧拼法铸下的存量任务留在这个键上，不回写。
func legacyShapeReference(segment, object, basis string) string {
	return "DELIVERY-DISPATCH/" + segment + "/" + object + "/" + basis
}

// enterSegmentByHandover 让 object 凭`已交接`进入 segment（首个对象成立段时声明为末端派送）。交接版本由调用方给：
// 入场依据是「TRANSPORT-HANDOVER/」加这个版本，撞键用例要靠它构造。
func (fixture *dispatchTriggerFixture) enterSegmentByHandover(t *testing.T, segment, object, version string) {
	t.Helper()
	command := registerHandoverCommand(t)
	command.Object = object
	command.Version = version
	command.Segment = segment
	command.SegmentServiceAction = "FINAL_DELIVERY"
	result, err := fixture.handovers.handler.Register(t.Context(), command)
	if err != nil || result.Outcome() != application.HandoverRegistered {
		t.Fatalf("%s 进 %s：%v %q", object, segment, err, result.Outcome())
	}
	if result.SegmentEntryRefusal() != application.SegmentEntryRefusalNone || result.SegmentContinuationReference() != "" {
		t.Fatalf("%s 进 %s 被拒或欠账：%s %q", object, segment, result.SegmentEntryRefusal(), result.SegmentContinuationReference())
	}
}

func (fixture *dispatchTriggerFixture) ownerCalls() int {
	return fixture.places.calls + fixture.windows.calls + fixture.conditions.calls
}

func (fixture *dispatchTriggerFixture) forgetOwnerCalls() {
	fixture.places.calls, fixture.windows.calls, fixture.conditions.calls = 0, 0, 0
}

func triggerOn(t *testing.T, segment, object string) application.TriggerDeliveryDispatchCommand {
	t.Helper()
	command := triggerCommand(t, object)
	command.Segment = segment
	return command
}

// seedTaskAt 经既有任务口在 registry 里立一项任务。任务口本身不拦保留起头（拦在手工口的载荷翻译里），存量里指名过
// 执行器引用的任务就是经它落库的——用例拿它造「键上已经有一项任务」的前提。
func seedTaskAt(t *testing.T, registry *dispatchTaskRegistryDouble, task string, kind domain.DispatchTaskKind, objects ...string) ports.DispatchTaskRecord {
	t.Helper()
	command := openTaskCommand(t)
	command.Task = task
	command.Kind = kind
	command.Objects = objects
	opened, err := newDispatchTaskHandler(registry).Open(t.Context(), command)
	if err != nil || opened.Outcome() != application.DispatchTaskOpened {
		t.Fatalf("立任务 %s：%v %q", task, err, opened.Outcome())
	}
	record, _ := opened.Record()
	return record
}

func objectsOf(record ports.DispatchTaskRecord) []string {
	var objects []string
	for _, object := range record.Task.Objects() {
		objects = append(objects, object.String())
	}
	return objects
}

// Covers: 票 09 裁定第 1 条——新引用以 `DELIVERY-DISPATCH/` 起头，后接形状标记，再把段、对象、入场依据各写成「字节
// 长度 + 分隔符 + 原串」。原串照抄不取摘要：引用出现在答复、日志与管理台上，人要读得出它指的是哪个段哪个对象。
func TestTheTaskReferenceSpellsEachPartWithItsLength(t *testing.T) {
	fixture := newDispatchTriggerFixture(t)
	fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)

	result, err := fixture.handler().Trigger(t.Context(), triggerCommand(t, "parcel-1"))
	if err != nil || result.Outcome() != application.DeliveryDispatchTaskFormed {
		t.Fatalf("触发：%v %q", err, result.Outcome())
	}
	record, _ := result.Record()
	const want = "DELIVERY-DISPATCH/v2/9:segment-1/8:parcel-1/46:TRANSPORT-HANDOVER/handover-result/parcel-1/v1"
	if got := record.Key.Task.String(); got != want {
		t.Fatalf("任务引用 = %q, want %q", got, want)
	}
	if want != newShapeReference("segment-1", "parcel-1", "TRANSPORT-HANDOVER/handover-result/parcel-1/v1") {
		t.Fatal("用例里的手算铸法与字面例子不一致")
	}
}

// Covers: 票 09 完成判据「编排单测：新铸法单射，对拼接撞键那组输入铸出不同引用」。段 `seg/A` 加对象 `CU-77` 与段 `seg`
// 加对象 `A/CU-77`、交接版本同为 v1——斜杠直拼时两者铸出同一串，后到的对象首拍就被答成「已形成」。
func TestConcatenationCollidingInputsMintDifferentTaskReferences(t *testing.T) {
	fixture := newDispatchTriggerFixture(t)
	fixture.enterSegmentByHandover(t, "seg/A", "CU-77", "v1")
	fixture.enterSegmentByHandover(t, "seg", "A/CU-77", "v1")
	if legacyShapeReference("seg/A", "CU-77", "TRANSPORT-HANDOVER/v1") != legacyShapeReference("seg", "A/CU-77", "TRANSPORT-HANDOVER/v1") {
		t.Fatal("夹具没构造出旧拼法下的撞键")
	}

	first, err := fixture.handler().Trigger(t.Context(), triggerOn(t, "seg/A", "CU-77"))
	if err != nil || first.Outcome() != application.DeliveryDispatchTaskFormed {
		t.Fatalf("seg/A 的 CU-77：%v %q", err, first.Outcome())
	}
	second, err := fixture.handler().Trigger(t.Context(), triggerOn(t, "seg", "A/CU-77"))
	if err != nil || second.Outcome() != application.DeliveryDispatchTaskFormed {
		record, _ := second.Record()
		t.Fatalf("seg 的 A/CU-77：%v %q（交回任务 %s 对象 %v），want DISPATCH_TASK_FORMED——它还没有派送任务",
			err, second.Outcome(), record.Key.Task, objectsOf(record))
	}
	firstRecord, _ := first.Record()
	secondRecord, _ := second.Record()
	if firstRecord.Key.Task == secondRecord.Key.Task {
		t.Fatalf("两拍铸出同一个引用 %s", firstRecord.Key.Task)
	}
	if objects := objectsOf(secondRecord); len(objects) != 1 || objects[0] != "A/CU-77" {
		t.Fatalf("第二项任务的对象集 = %v, want [A/CU-77]", objects)
	}
	if fixture.tasks.saves != 2 {
		t.Fatalf("任务登记册写了 %d 次, want 2", fixture.tasks.saves)
	}
}

// Covers: 票 09 完成判据「编排单测：旧拼法在册任务兼认」与裁定第 1 条——先查新键，查不到再查旧键，命中且种类与对象集
// 核得过才算已在册；存量不回写。
func TestALegacySpelledTaskIsStillRecognised(t *testing.T) {
	const basis = "TRANSPORT-HANDOVER/handover-result/parcel-1/v1"

	t.Run("旧拼法在册的就是本拍那一项：答已在册并交回它，不取件、不写", func(t *testing.T) {
		fixture := newDispatchTriggerFixture(t)
		fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
		legacy := seedTaskAt(t, fixture.tasks, legacyShapeReference("segment-1", "parcel-1", basis), domain.DeliveryDispatch, "parcel-1")
		saves := fixture.tasks.saves

		result, err := fixture.handler().Trigger(t.Context(), triggerCommand(t, "parcel-1"))
		if err != nil || result.Outcome() != application.DeliveryDispatchTaskAlreadyFormed {
			t.Fatalf("触发：%v %q, want DISPATCH_TASK_ALREADY_FORMED", err, result.Outcome())
		}
		if record, _ := result.Record(); record.Key.Task != legacy.Key.Task {
			t.Fatalf("交回的任务 = %s, want 旧拼法在册的 %s", record.Key.Task, legacy.Key.Task)
		}
		if fixture.tasks.saves != saves {
			t.Fatal("旧拼法已在册却又写了任务登记册——存量不回写，也不在新键上另立一项")
		}
		if calls := fixture.ownerCalls(); calls != 0 {
			t.Fatalf("在册之后又向所有者取了 %d 次件", calls)
		}
	})

	t.Run("旧拼法上是别的任务：不算在册，本拍在新键上形成自己的任务", func(t *testing.T) {
		fixture := newDispatchTriggerFixture(t)
		fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
		occupant := seedTaskAt(t, fixture.tasks, legacyShapeReference("segment-1", "parcel-1", basis), domain.PickupDispatch, "parcel-other-9")

		result, err := fixture.handler().Trigger(t.Context(), triggerCommand(t, "parcel-1"))
		if err != nil || result.Outcome() != application.DeliveryDispatchTaskFormed {
			t.Fatalf("触发：%v %q, want DISPATCH_TASK_FORMED——旧键上那一项不是本拍的任务", err, result.Outcome())
		}
		record, _ := result.Record()
		if record.Key.Task.String() != newShapeReference("segment-1", "parcel-1", basis) {
			t.Fatalf("新任务的引用 = %s, want 新铸法铸出的那一串", record.Key.Task)
		}
		if kept, found, _ := fixture.tasks.FindByKey(t.Context(), occupant.Key); !found || kept.Task.Kind() != domain.PickupDispatch {
			t.Fatal("旧键上的存量任务被动了")
		}
	})

	// 段是调用方给的不透明串，旧拼法串因此可能恰好合新形状。那样的串不按旧拼法去查：真查到的会是别的（段，对象，
	// 入场依据）在新铸法下的任务，同一对象跨段时种类与对象集都对得上，核对拦不住。
	t.Run("旧拼法串本身合新形状：不按旧拼法认", func(t *testing.T) {
		fixture := newDispatchTriggerFixture(t)
		otherBasis := "TRANSPORT-HANDOVER/x/parcel-1/TRANSPORT-HANDOVER/y"
		fixture.enterSegmentByHandover(t, "a", "parcel-1", "x/parcel-1/TRANSPORT-HANDOVER/y")
		earlier, err := fixture.handler().Trigger(t.Context(), triggerOn(t, "a", "parcel-1"))
		if err != nil || earlier.Outcome() != application.DeliveryDispatchTaskFormed {
			t.Fatalf("段 a 那一拍：%v %q", err, earlier.Outcome())
		}
		crafted := fmt.Sprintf("v2/1:a/8:parcel-1/%d:TRANSPORT-HANDOVER/x", len(otherBasis))
		fixture.enterSegmentByHandover(t, crafted, "parcel-1", "y")
		if legacyShapeReference(crafted, "parcel-1", "TRANSPORT-HANDOVER/y") != newShapeReference("a", "parcel-1", otherBasis) {
			t.Fatal("夹具没构造出「旧拼法串等于另一拍的新引用」")
		}

		result, err := fixture.handler().Trigger(t.Context(), triggerOn(t, crafted, "parcel-1"))
		if err != nil || result.Outcome() != application.DeliveryDispatchTaskFormed {
			record, _ := result.Record()
			t.Fatalf("构造段那一拍：%v %q（交回 %s），want DISPATCH_TASK_FORMED——段 a 的任务不是它的", err, result.Outcome(), record.Key.Task)
		}
	})
}

// Covers: 票 09 完成判据「编排单测：被手工口占用答新格」与裁定第 2 条——键上已有一项任务，但种类不是末端派送或对象集
// 不恰为本对象：答 `DISPATCH_TASK_REFERENCE_TAKEN` 并交回占用者；不写、不另铸、不向所有者取件。
func TestAReferenceTakenByAnotherTaskIsAnsweredAsTaken(t *testing.T) {
	const basis = "TRANSPORT-HANDOVER/handover-result/parcel-1/v1"
	occupants := map[string]struct {
		kind    domain.DispatchTaskKind
		objects []string
	}{
		"手工口建的揽收任务，对象是别的": {domain.PickupDispatch, []string{"parcel-other-9"}},
		"揽收任务，对象相同":       {domain.PickupDispatch, []string{"parcel-1"}},
		"派送任务，对象集多一件":     {domain.DeliveryDispatch, []string{"parcel-1", "parcel-2"}},
		"派送任务，对象是别的":      {domain.DeliveryDispatch, []string{"parcel-other-9"}},
	}
	for name, occupant := range occupants {
		t.Run(name, func(t *testing.T) {
			fixture := newDispatchTriggerFixture(t)
			fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
			taken := seedTaskAt(t, fixture.tasks, newShapeReference("segment-1", "parcel-1", basis), occupant.kind, occupant.objects...)
			saves := fixture.tasks.saves

			result, err := fixture.handler().Trigger(t.Context(), triggerCommand(t, "parcel-1"))
			if err != nil {
				t.Fatalf("触发：%v", err)
			}
			if result.Outcome().String() != "DISPATCH_TASK_REFERENCE_TAKEN" {
				t.Fatalf("outcome = %q, want DISPATCH_TASK_REFERENCE_TAKEN", result.Outcome())
			}
			record, present := result.Record()
			if !present || record.Key.Task != taken.Key.Task || record.Task.Kind() != occupant.kind || fmt.Sprint(objectsOf(record)) != fmt.Sprint(occupant.objects) {
				t.Fatalf("回显 = %v %s %v, want 占用者 %s %s %v", present, record.Task.Kind(), objectsOf(record), taken.Key.Task, occupant.kind, occupant.objects)
			}
			if fixture.tasks.saves != saves {
				t.Fatal("引用被占却写了任务登记册")
			}
			if calls := fixture.ownerCalls(); calls != 0 {
				t.Fatalf("引用被占却向所有者取了 %d 次件", calls)
			}
		})
	}

	// 查在册与任务口落库之间另有一方先占了键：任务口读回的赢家同样要核，核不过照样答引用被占。
	t.Run("任务口读回的赢家不是本拍那一项", func(t *testing.T) {
		fixture := newDispatchTriggerFixture(t)
		fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
		fixture.deps.Tasks = newDispatchTaskRegistry()
		seedTaskAt(t, fixture.tasks, newShapeReference("segment-1", "parcel-1", basis), domain.PickupDispatch, "parcel-other-9")

		result, err := fixture.handler().Trigger(t.Context(), triggerCommand(t, "parcel-1"))
		if err != nil || result.Outcome().String() != "DISPATCH_TASK_REFERENCE_TAKEN" {
			t.Fatalf("触发：%v %q, want DISPATCH_TASK_REFERENCE_TAKEN", err, result.Outcome())
		}
		if record, _ := result.Record(); record.Task.Kind() != domain.PickupDispatch {
			t.Fatalf("回显的种类 = %s, want 占用者的 PICKUP", record.Task.Kind())
		}
	})
}

// Covers: 票 09 完成判据「编排单测：在册后不再取件（所有者替身计数为零）」与裁定第 3 条——查在册在参与在场判断之前、
// 读回段与确认链尾凭已交接之后；在册就不再向所有者取件。同一拍原样重放不随所有者的后续改答漂移，参与离场之后重放
// 也答已在册（不形成新任务，ADR-0114 决定二「已离场的参与不触发」照样守住）。
func TestARegisteredTaskIsAnsweredWithoutAskingTheOwnersAgain(t *testing.T) {
	fixture := newDispatchTriggerFixture(t)
	fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
	first, err := fixture.handler().Trigger(t.Context(), triggerCommand(t, "parcel-1"))
	if err != nil || first.Outcome() != application.DeliveryDispatchTaskFormed {
		t.Fatalf("首拍：%v %q", err, first.Outcome())
	}
	formed, _ := first.Record()

	replay := func(t *testing.T, situation string) {
		t.Helper()
		fixture.forgetOwnerCalls()
		result, err := fixture.handler().Trigger(t.Context(), triggerCommand(t, "parcel-1"))
		if err != nil || result.Outcome() != application.DeliveryDispatchTaskAlreadyFormed {
			t.Fatalf("%s后原样重放：%v %q（refusal=%s missing=%v undecided=%s），want DISPATCH_TASK_ALREADY_FORMED",
				situation, err, result.Outcome(), result.Refusal(), result.Missing(), result.UndecidedReason())
		}
		record, _ := result.Record()
		if record.Key.Task != formed.Key.Task || record.Task.Place() != formed.Task.Place() || record.Task.Conditions() != formed.Task.Conditions() {
			t.Fatalf("%s后重放交回 %s / %s / %s, want 首拍那一项", situation, record.Key.Task, record.Task.Place(), record.Task.Conditions())
		}
		if calls := fixture.ownerCalls(); calls != 0 {
			t.Fatalf("%s后重放又向所有者取了 %d 次件", situation, calls)
		}
		if fixture.tasks.saves != 1 {
			t.Fatalf("%s后重放写了任务登记册：saves=%d", situation, fixture.tasks.saves)
		}
	}

	fixture.places.resolution = ports.RequirementMissing
	replay(t, "地点所有者改答没有")
	fixture.windows.err = errors.New("network-routing 读面不可用")
	replay(t, "时间窗所有者读不到")
	fixture.conditions.conditions = "delivery-conditions/contract-1/v4"
	replay(t, "条件所有者换了引用")
	fixture.deps.Places = nil
	replay(t, "地点缝没接")

	ender := application.NewEndFulfillmentParticipationHandler(application.EndFulfillmentParticipationDeps{
		Segments: fixture.handovers.segments,
		Clock:    dispatchTaskClock{at: taskRecordedAt},
	})
	ended, err := ender.End(t.Context(), application.EndFulfillmentParticipationCommand{
		TenantID: triggerCommand(t, "parcel-1").TenantID,
		Segment:  "segment-1",
		Object:   "parcel-1",
		Source:   application.ParticipationEndedByTermination,
		Basis:    "control-termination/parcel-1",
		EndedAt:  handoverJudgedTime.Add(30 * time.Minute),
	})
	if err != nil || ended.Outcome() != application.ParticipationEndedNow {
		t.Fatalf("离场：%v %q", err, ended.Outcome())
	}
	replay(t, "参与离场")
}

// 查在册读不回是欠账：答未决并点名任务登记册，带续办引用；这一拍还没定出是不是在册，不去白读所有者。
func TestAnUnreadableTaskRegistryIsUndecidedBeforeAnyOwnerIsAsked(t *testing.T) {
	fixture := newDispatchTriggerFixture(t)
	fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
	fixture.tasks.findErr = errors.New("任务登记册不可用")

	result, err := fixture.handler().Trigger(t.Context(), triggerCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("读不回不该上抛技术错误：%v", err)
	}
	if result.Outcome() != application.DeliveryDispatchUndecided || result.UndecidedReason() != application.DeliveryDispatchTaskUndecided {
		t.Fatalf("outcome = %q reason = %s, want DISPATCH_UNDECIDED / DISPATCH_TASK_UNDECIDED", result.Outcome(), result.UndecidedReason())
	}
	if result.ContinuationReference() == "" {
		t.Fatal("未决没有续办引用")
	}
	if calls := fixture.ownerCalls(); calls != 0 {
		t.Fatalf("还没查清在不在册就向所有者取了 %d 次件", calls)
	}
}
