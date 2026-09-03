package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 多对象到访的进段（票 tf-unwired-seven/08）。与两条单对象入口的差别只有一个，但它是
// 本票全部难处所在：**一次到访里的对象各自进段各自失败，整批一个结果表达不了「哪几个
// 没进去」**（CONTEXT：任务汇总只能由对象结果派生）。

type multiObjectSegmentFixture struct {
	store    *pickupStoreDouble
	segments *segmentRegistryDouble
	handler  *application.PerformOffsitePickupHandler
}

func newMultiObjectSegmentFixture(t *testing.T) *multiObjectSegmentFixture {
	t.Helper()
	fixture := &multiObjectSegmentFixture{
		store:    newPickupStore(),
		segments: newSegmentRegistry(),
	}
	fixture.handler = application.NewPerformOffsitePickupHandler(application.PerformOffsitePickupDeps{
		Attempts:   fixture.store,
		Segments:   fixture.segments,
		Versions:   &versionFactoryDouble{},
		Downstream: &pickupHandoffDouble{},
		Clock:      fixedClock{at: recordedAt},
	})
	return fixture
}

// entryFor 取某个对象那一格的进段报告；没有报告就是那个对象这一半没有欠账。
func entryFor(entries []application.ObjectSegmentEntry, object string) (application.ObjectSegmentEntry, bool) {
	for _, entry := range entries {
		if entry.Object.String() == object {
			return entry, true
		}
	}
	return application.ObjectSegmentEntry{}, false
}

// 一次到访三个对象取得控制：第一个立段，其余两个加入同一个段。段是否已成立由那道门
// 自己问登记册——编排不记「立了没有」，两个对象并发到达时那份缓存会让同一个段被立两回。
func TestOneVisitPutsEveryPickedUpObjectIntoTheSameSegment(t *testing.T) {
	fixture := newMultiObjectSegmentFixture(t)
	command := pickupCommand(t, "source-1", "attempt-1",
		successSubmission(t, "parcel-1", "TRANSPORT-CONTROL/TF-1"),
		successSubmission(t, "parcel-2", "TRANSPORT-CONTROL/TF-2"),
		successSubmission(t, "parcel-3", "TRANSPORT-CONTROL/TF-3"))
	command.Segment = "segment-1"

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.PickupAttemptRecorded {
		t.Fatalf("outcome = %q, want ATTEMPT_RECORDED", result.Outcome())
	}
	if fixture.segments.saves != 1 {
		t.Fatalf("段登记 saves = %d, want 1——段由首个对象成立，其余是加入", fixture.segments.saves)
	}
	if fixture.segments.joins != 2 {
		t.Fatalf("加入次数 = %d, want 2", fixture.segments.joins)
	}

	record := fixture.segments.saved(t, "tenant-1", "segment-1")
	for _, object := range []string{"parcel-1", "parcel-2", "parcel-3"} {
		reference := value(t, domain.NewCarriedObjectReference, object)
		if _, joined := record.Segment.ParticipationFor(reference); !joined {
			t.Fatalf("%s 取得了控制却没有履约参与关系", object)
		}
	}
	if entries := result.SegmentEntries(); len(entries) != 0 {
		t.Fatalf("全都进去了却报出了欠账：%+v", entries)
	}
}

// 逐对象各带自己的计划履约段：段引用整次到访共用，计划段是每个对象自己的
// （CONTEXT：每个对象必须分别关联自己的计划履约段）。
func TestEachObjectCarriesItsOwnPlannedSegment(t *testing.T) {
	fixture := newMultiObjectSegmentFixture(t)
	first := successSubmission(t, "parcel-1", "TRANSPORT-CONTROL/TF-1")
	first.PlannedSegment = "planned-segment-1"
	second := successSubmission(t, "parcel-2", "TRANSPORT-CONTROL/TF-2")
	second.PlannedSegment = "planned-segment-2"
	command := pickupCommand(t, "source-1", "attempt-1", first, second)
	command.Segment = "segment-1"

	if _, err := fixture.handler.Handle(context.Background(), command); err != nil {
		t.Fatalf("handle: %v", err)
	}

	record := fixture.segments.saved(t, "tenant-1", "segment-1")
	for object, want := range map[string]string{"parcel-1": "planned-segment-1", "parcel-2": "planned-segment-2"} {
		participation, joined := record.Segment.ParticipationFor(value(t, domain.NewCarriedObjectReference, object))
		if !joined {
			t.Fatalf("%s 没有参与关系", object)
		}
		planned, present := participation.PlannedSegment()
		if !present || planned.String() != want {
			t.Fatalf("%s 的计划段 = %q(present=%v), want %q——整批共用一个就抹掉了成员差异",
				object, planned.String(), present, want)
		}
	}
}

// 失败对象不进段。这条由构造门守着（形不成 OffsitePickup 就走不到进段那一步），本测试
// 钉的是接线没有绕过它自己判一遍。
func TestFailedObjectsOfAVisitEnterNoSegment(t *testing.T) {
	fixture := newMultiObjectSegmentFixture(t)
	command := pickupCommand(t, "source-1", "attempt-1",
		successSubmission(t, "parcel-1", "TRANSPORT-CONTROL/TF-1"),
		failureSubmission(t, "parcel-2", domain.CustomerAbsent, "reason-absent-1"))
	command.Segment = "segment-1"

	if _, err := fixture.handler.Handle(context.Background(), command); err != nil {
		t.Fatalf("handle: %v", err)
	}

	record := fixture.segments.saved(t, "tenant-1", "segment-1")
	absent := value(t, domain.NewCarriedObjectReference, "parcel-2")
	if _, joined := record.Segment.ParticipationFor(absent); joined {
		t.Fatal("失败对象进了段——CONTEXT：失败结果不制造实际履约段")
	}
	if record.Segment.ActiveParticipations() != 1 {
		t.Fatalf("活跃参与 = %d, want 1", record.Segment.ActiveParticipations())
	}
}

// **本票与票 02 真正的差别。** 一次到访里只有部分对象进段失败时，整批一个续办引用说不出
// 「哪几个没进去」。逐对象报，且没进去的那些不影响已经进去的。
func TestOnlyTheObjectsThatFailedToEnterAreReported(t *testing.T) {
	fixture := newMultiObjectSegmentFixture(t)
	fixture.segments.joinErrObject = "parcel-3"
	fixture.segments.joinErr = errors.New("段登记册暂时不可用")
	command := pickupCommand(t, "source-1", "attempt-1",
		successSubmission(t, "parcel-1", "TRANSPORT-CONTROL/TF-1"),
		successSubmission(t, "parcel-2", "TRANSPORT-CONTROL/TF-2"),
		successSubmission(t, "parcel-3", "TRANSPORT-CONTROL/TF-3"))
	command.Segment = "segment-1"

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	// 来源保全一侧照常成立：一次段登记故障抹不掉三件已经发生的物理事实。
	if result.Outcome() != application.PickupAttemptRecorded {
		t.Fatalf("outcome = %q, want ATTEMPT_RECORDED——进段失败不得回滚来源登记", result.Outcome())
	}

	entries := result.SegmentEntries()
	if len(entries) != 1 {
		t.Fatalf("报出 %d 格, want 1（只有 parcel-3 没进去）：%+v", len(entries), entries)
	}
	failed, present := entryFor(entries, "parcel-3")
	if !present {
		t.Fatalf("没进去的那个没被报出来：%+v", entries)
	}
	if failed.ContinuationReference == "" {
		t.Fatal("登记册故障是欠账，要留续办引用——等它恢复重试同一份")
	}
	for _, object := range []string{"parcel-1", "parcel-2"} {
		if _, reported := entryFor(entries, object); reported {
			t.Fatalf("%s 已经进去了却也被报了欠账", object)
		}
	}
}

// 只有空白的段引用等同于「没给」，不是「给坏了」。
//
// **这条钉的是票 08 一条前提的作废。** 票面原写着「段引用格式不合法时静默不立段，『这个入参
// 没被受理』从结果上看不出来」，并要求为它开一格答案。取证之后那一格不存在：
// `NewFulfillmentSegmentReference` 与 `NewPlannedSegmentReference` 唯一的失败是去空白后为空
// （`newRequiredValue`），而那一种在进段门里早就被缺席判据筛走了——**非空的引用一定构造得出来**。
// 于是「写坏了」在今天的类型下不是一个可达状态，开那一格等于造一个永远答不出来的答案。
// 经过记在票面；引用日后真加了格式规则，这条测试与那一格要一起回来。
func TestABlankSegmentReferenceMeansNotRequestedNotMalformed(t *testing.T) {
	fixture := newMultiObjectSegmentFixture(t)
	command := pickupCommand(t, "source-1", "attempt-1",
		successSubmission(t, "parcel-1", "TRANSPORT-CONTROL/TF-1"),
		successSubmission(t, "parcel-2", "TRANSPORT-CONTROL/TF-2"))
	command.Segment = "   "

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.PickupAttemptRecorded {
		t.Fatalf("outcome = %q, want ATTEMPT_RECORDED", result.Outcome())
	}
	if entries := result.SegmentEntries(); len(entries) != 0 {
		t.Fatalf("没要求进段却报了欠账：%+v", entries)
	}
	if fixture.segments.saves != 0 || fixture.segments.joins != 0 {
		t.Fatalf("只有空白的段引用却动了段登记册：saves=%d joins=%d", fixture.segments.saves, fixture.segments.joins)
	}
}

// 某个对象的计划段留空时，它照常进段、只是没有计划段关联——计划段本就可缺席
// （CONTEXT：待路由的产品在没有可行候选时照样实际揽收）。别人留空不牵连有计划段的那个。
func TestAnObjectWithoutAPlannedSegmentStillEntersTheSegment(t *testing.T) {
	fixture := newMultiObjectSegmentFixture(t)
	withPlan := successSubmission(t, "parcel-1", "TRANSPORT-CONTROL/TF-1")
	withPlan.PlannedSegment = "planned-segment-1"
	command := pickupCommand(t, "source-1", "attempt-1",
		withPlan,
		successSubmission(t, "parcel-2", "TRANSPORT-CONTROL/TF-2"))
	command.Segment = "segment-1"

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if entries := result.SegmentEntries(); len(entries) != 0 {
		t.Fatalf("计划段可缺席，缺席却被记成了欠账：%+v", entries)
	}

	record := fixture.segments.saved(t, "tenant-1", "segment-1")
	bare, joined := record.Segment.ParticipationFor(value(t, domain.NewCarriedObjectReference, "parcel-2"))
	if !joined {
		t.Fatal("没有计划段的对象被挡在了段外")
	}
	if _, present := bare.PlannedSegment(); present {
		t.Fatal("没给计划段却凭空关联了一个")
	}
	planned, joined := record.Segment.ParticipationFor(value(t, domain.NewCarriedObjectReference, "parcel-1"))
	if !joined {
		t.Fatal("有计划段的对象没进段")
	}
	if reference, present := planned.PlannedSegment(); !present || reference.String() != "planned-segment-1" {
		t.Fatalf("计划段 = %q(present=%v)，被同伴的缺席带偏了", reference.String(), present)
	}
}

// 不给段引用就不立段，一次不碰段登记册——今天全部调用方都不给段号，这条保住它们。
func TestAVisitWithoutASegmentReferenceTouchesNoSegmentRegistry(t *testing.T) {
	fixture := newMultiObjectSegmentFixture(t)

	result, err := fixture.handler.Handle(context.Background(), pickupCommand(t, "source-1", "attempt-1",
		successSubmission(t, "parcel-1", "TRANSPORT-CONTROL/TF-1"),
		successSubmission(t, "parcel-2", "TRANSPORT-CONTROL/TF-2")))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.PickupAttemptRecorded {
		t.Fatalf("outcome = %q, want ATTEMPT_RECORDED", result.Outcome())
	}
	if fixture.segments.saves != 0 || fixture.segments.joins != 0 {
		t.Fatalf("没给段引用却动了段登记册：saves=%d joins=%d", fixture.segments.saves, fixture.segments.joins)
	}
	if entries := result.SegmentEntries(); len(entries) != 0 {
		t.Fatalf("没要求进段却报了欠账：%+v", entries)
	}
}

// 段登记册缺席时照登不误：派生一侧缺席不该让来源保全停摆，也不报成输入未受理。
func TestAVisitWithoutASegmentRegistryStillRecords(t *testing.T) {
	store := newPickupStore()
	handler := application.NewPerformOffsitePickupHandler(application.PerformOffsitePickupDeps{
		Attempts:   store,
		Versions:   &versionFactoryDouble{},
		Downstream: &pickupHandoffDouble{},
		Clock:      fixedClock{at: recordedAt},
	})
	command := pickupCommand(t, "source-1", "attempt-1",
		successSubmission(t, "parcel-1", "TRANSPORT-CONTROL/TF-1"))
	command.Segment = "segment-1"

	result, err := handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.PickupAttemptRecorded {
		t.Fatalf("outcome = %q, want ATTEMPT_RECORDED", result.Outcome())
	}
	if entries := result.SegmentEntries(); len(entries) != 0 {
		t.Fatalf("没有段登记册却报了逐对象欠账：%+v", entries)
	}
}
