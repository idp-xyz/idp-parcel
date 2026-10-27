package transportfulfillment_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/transportfulfillment"
)

// 本文件证补问信号适配器（ADR-0305）：按键回读那一次补问与它补的那条出发，补问读数里新判阻断的那几件各交一次信号，依据与因果条件
// 指那条出发；同一条出发更早的读数里已判阻断的那件不再交；各格失败落进各自的哨兵。

type reaskFinderDouble struct {
	records  []tfports.DepartureGateReaskRecord
	findErr  error
	listErr  error
	unlisted bool
}

func (double *reaskFinderDouble) FindByKey(_ context.Context, key tfports.DepartureGateReaskKey) (tfports.DepartureGateReaskRecord, bool, error) {
	if double.findErr != nil {
		return tfports.DepartureGateReaskRecord{}, false, double.findErr
	}
	for _, record := range double.records {
		if record.Key == key {
			return record, true, nil
		}
	}
	return tfports.DepartureGateReaskRecord{}, false, nil
}

func (double *reaskFinderDouble) FindByDeparture(_ context.Context, departure tfports.MovementFactKey) ([]tfports.DepartureGateReaskRecord, error) {
	if double.listErr != nil {
		return nil, double.listErr
	}
	if double.unlisted {
		return nil, nil
	}
	var listed []tfports.DepartureGateReaskRecord
	for _, record := range double.records {
		if record.Key.Departure() == departure {
			listed = append(listed, record)
		}
	}
	return listed, nil
}

// reaskStoppedDeparture 是 move-1 / v1：出发声明 segment-1，首版读数停在本侧「段不在册」。
func reaskStoppedDeparture(t *testing.T) tfports.MovementFactRecord {
	t.Helper()
	reading, err := tfdomain.FormDepartureGateReading(tfdomain.DepartureGateReadingSpec{
		Stops: []tfdomain.DepartureGateStop{tfdomain.DepartureSegmentNotRegistered},
	})
	if err != nil {
		t.Fatalf("首版读数：%v", err)
	}
	return departureWithReading(t, reading)
}

func departureWithReading(t *testing.T, reading tfdomain.DepartureGateReading) tfports.MovementFactRecord {
	t.Helper()
	tenant := movementValue(t, tfdomain.NewTenantID, "tenant-1")
	spec := tfdomain.MovementFactSpec{
		TenantID:    tenant,
		Fact:        movementValue(t, tfdomain.NewMovementFactReference, "move-1"),
		Schedule:    movementValue(t, tfdomain.NewScheduleReference, "schedule-1"),
		Kind:        tfdomain.DepartureFact,
		Location:    movementValue(t, tfdomain.NewMovementLocationReference, "NODE-SHA"),
		Source:      movementValue(t, tfdomain.NewMovementSourceReference, "own-fleet/v1"),
		Version:     movementValue(t, tfdomain.NewMovementFactVersion, "v1"),
		OccurredAt:  movementDepartedAt,
		Segment:     movementValue(t, tfdomain.NewFulfillmentSegmentReference, "segment-1"),
		GateReading: reading,
	}
	fact, err := tfdomain.RecordMovementFact(spec)
	if err != nil {
		t.Fatalf("出发：%v", err)
	}
	return tfports.MovementFactRecord{
		Key:        tfports.MovementFactKey{TenantID: tenant, Fact: spec.Fact, Version: spec.Version},
		Fact:       fact,
		RecordedAt: movementRecordedAt,
	}
}

func reaskReading(t *testing.T, answers map[string]tfdomain.DepartureGateAnswer) tfdomain.DepartureGateReading {
	t.Helper()
	var findings []tfdomain.DepartureGateFinding
	for object, answer := range answers {
		judgement, basis := "AADJ-REASK-"+object, tfdomain.DepartureGateBasis{Action: "LOADING_DEPARTURE", Cause: "SOME_CAUSE"}
		if answer == tfdomain.DepartureGateUnreadable {
			judgement, basis = "", tfdomain.DepartureGateBasis{Cause: "CUSTOMS_ADMISSION_UNAVAILABLE"}
		}
		finding, err := tfdomain.NewDepartureGateFinding(movementValue(t, tfdomain.NewCarriedObjectReference, object), answer, judgement,
			[]tfdomain.DepartureGateBasis{basis})
		if err != nil {
			t.Fatalf("补问那一格：%v", err)
		}
		findings = append(findings, finding)
	}
	reading, err := tfdomain.FormDepartureGateReading(tfdomain.DepartureGateReadingSpec{
		From:     movementValue(t, tfdomain.NewNetworkNodeReference, "NODE-SHA"),
		To:       movementValue(t, tfdomain.NewNetworkNodeReference, "NODE-SZX"),
		Findings: findings,
	})
	if err != nil {
		t.Fatalf("补问读数：%v", err)
	}
	return reading
}

func reaskRecord(
	t *testing.T,
	departure tfports.MovementFactRecord,
	object string,
	sequence int,
	at time.Time,
	reading tfdomain.DepartureGateReading,
) tfports.DepartureGateReaskRecord {
	t.Helper()
	return tfports.DepartureGateReaskRecord{
		Key: tfports.DepartureGateReaskKey{
			TenantID: departure.Key.TenantID, Fact: departure.Key.Fact, Version: departure.Key.Version,
			Object: movementValue(t, tfdomain.NewCarriedObjectReference, object), Sequence: sequence,
		},
		Reading: reading, Trigger: "transport-handover/" + object, RecordedAt: at,
	}
}

func recordedReaskRef(object string, sequence int) veinbox.RecordedDepartureGateReask {
	return veinbox.RecordedDepartureGateReask{
		TenantID: "tenant-1", Fact: "move-1", Version: "v1", Object: object, Sequence: fmt.Sprint(sequence),
	}
}

type reaskSignalFixture struct {
	subject *adapter.RaiseOnDepartureGateReaskAdapter
	reasks  *reaskFinderDouble
	facts   *movementFinderDouble
	signals *movementSignalDouble
}

func newReaskSignalFixture(t *testing.T, departure tfports.MovementFactRecord, reasks ...tfports.DepartureGateReaskRecord) *reaskSignalFixture {
	t.Helper()
	fixture := &reaskSignalFixture{
		reasks:  &reaskFinderDouble{records: reasks},
		facts:   &movementFinderDouble{records: map[tfports.MovementFactKey]tfports.MovementFactRecord{departure.Key: departure}},
		signals: &movementSignalDouble{t: t, answer: signalAnswering(movementRuleDouble{configured: false})},
	}
	subject, err := adapter.NewRaiseOnDepartureGateReaskAdapter(fixture.reasks, fixture.facts, fixture.signals)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	fixture.subject = subject
	return fixture
}

// Covers: AT-VE-205 适配器半边（ADR-0305）——补问读数里阻断的那一件交进信号编排，依据与因果条件指那条出发：这件包裹上的事实引用
// （与投影同一把键）、出发那一版的版本，判断标识取补问读数里它那一格；放行、读不动的那几件不交。
func TestABlockedReaskRaisesTheSignalOnTheDeparture(t *testing.T) {
	departure := reaskStoppedDeparture(t)
	reask := reaskRecord(t, departure, "parcel-1", 1, movementRecordedAt.Add(time.Hour), reaskReading(t,
		map[string]tfdomain.DepartureGateAnswer{"parcel-1": tfdomain.DepartureGateBlocked}))
	fixture := newReaskSignalFixture(t, departure, reask)

	if err := fixture.subject.HandleRecordedDepartureGateReask(t.Context(), recordedReaskRef("parcel-1", 1)); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(fixture.signals.commands) != 1 {
		t.Fatalf("交进信号编排 %d 件, want 1", len(fixture.signals.commands))
	}
	command := fixture.signals.commands[0]
	if command.Parcel.String() != "parcel-1" || command.Fact.String() != "movement-fact/parcel-1/move-1" ||
		command.Version.String() != "v1" || command.Judgement != "AADJ-REASK-parcel-1" {
		t.Fatalf("信号命令 = %s %s %s %q", command.Parcel, command.Fact, command.Version, command.Judgement)
	}

	quiet := reaskRecord(t, departure, "parcel-2", 1, movementRecordedAt.Add(time.Hour), reaskReading(t,
		map[string]tfdomain.DepartureGateAnswer{"parcel-2": tfdomain.DepartureGateAdmitted}))
	fixture = newReaskSignalFixture(t, departure, quiet)
	if err := fixture.subject.HandleRecordedDepartureGateReask(t.Context(), recordedReaskRef("parcel-2", 1)); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(fixture.signals.commands) != 0 {
		t.Fatalf("放行的补问也交了信号：%v", fixture.signals.parcels)
	}
}

// Covers: AT-VE-206 适配器半边——同一条出发更早的一份读数里已判阻断的那一件不再交：首版读数里阻断过的、更早一次补问里阻断过的都
// 算；晚于这一次的补问不算。
func TestAParcelAlreadyBlockedOnTheDepartureIsNotSignalledAgain(t *testing.T) {
	t.Run("首版读数里阻断过", func(t *testing.T) {
		departure := movementRecord(t, tfdomain.DepartureFact) // 首版：parcel-1 阻断、parcel-2 不受门禁
		reask := reaskRecord(t, departure, "bag-1", 1, movementRecordedAt.Add(time.Hour), reaskReading(t,
			map[string]tfdomain.DepartureGateAnswer{"parcel-1": tfdomain.DepartureGateBlocked, "parcel-3": tfdomain.DepartureGateBlocked}))
		fixture := newReaskSignalFixture(t, departure, reask)
		if err := fixture.subject.HandleRecordedDepartureGateReask(t.Context(), recordedReaskRef("bag-1", 1)); err != nil {
			t.Fatalf("handle: %v", err)
		}
		if len(fixture.signals.parcels) != 1 || fixture.signals.parcels[0] != "parcel-3" {
			t.Fatalf("交进信号编排 = %v, want 只有新判阻断的 parcel-3", fixture.signals.parcels)
		}
	})
	t.Run("更早一次补问里阻断过", func(t *testing.T) {
		departure := reaskStoppedDeparture(t)
		at := movementRecordedAt.Add(time.Hour)
		earlier := reaskRecord(t, departure, "parcel-1", 1, at, reaskReading(t,
			map[string]tfdomain.DepartureGateAnswer{"parcel-1": tfdomain.DepartureGateBlocked}))
		this := reaskRecord(t, departure, "bag-1", 1, at.Add(time.Minute), reaskReading(t,
			map[string]tfdomain.DepartureGateAnswer{"parcel-1": tfdomain.DepartureGateBlocked}))
		later := reaskRecord(t, departure, "bag-2", 1, at.Add(2*time.Minute), reaskReading(t,
			map[string]tfdomain.DepartureGateAnswer{"parcel-5": tfdomain.DepartureGateBlocked}))
		fixture := newReaskSignalFixture(t, departure, earlier, this, later)
		if err := fixture.subject.HandleRecordedDepartureGateReask(t.Context(), recordedReaskRef("bag-1", 1)); err != nil {
			t.Fatalf("handle: %v", err)
		}
		if len(fixture.signals.parcels) != 0 {
			t.Fatalf("更早补问里阻断过的又交了：%v", fixture.signals.parcels)
		}
		if err := fixture.subject.HandleRecordedDepartureGateReask(t.Context(), recordedReaskRef("parcel-1", 1)); err != nil {
			t.Fatalf("handle earlier: %v", err)
		}
		if len(fixture.signals.parcels) != 1 || fixture.signals.parcels[0] != "parcel-1" {
			t.Fatalf("更早那一次 = %v, want 它自己交 parcel-1——晚于它的补问不算", fixture.signals.parcels)
		}
	})
}

// Covers: AT-VE-222 适配器半边（ADR-0331）——随袋走过出发、当时身份未决的成员后来识别成功，替它补问的读数（键是成员包裹，首版读数里它没有自己
// 的一格，只有袋那一格答未决）里阻断：为这件成员包裹形成信号，依据与因果条件指那条出发、事实引用是这件包裹上的引用；之后替袋补问的那一份读数
// 展开整袋、里面同一件成员又判阻断，不再交——一次违规出发只交分诊一次，跨补问的两条链也一样。
func TestAMemberReaskedAfterItsBagDepartedRaisesItsSignalOnceAcrossBothChains(t *testing.T) {
	unit, err := tfdomain.FormConsolidationUnitGateFinding(movementValue(t, tfdomain.NewCarriedObjectReference, "bag-1"), nil,
		[]tfdomain.ConsolidationMemberGap{{Stop: tfdomain.ConsolidationMemberIdentityUnresolved, Sources: []string{"hu-1"}}})
	if err != nil {
		t.Fatalf("袋那一格：%v", err)
	}
	first, err := tfdomain.FormDepartureGateReading(tfdomain.DepartureGateReadingSpec{
		From:     movementValue(t, tfdomain.NewNetworkNodeReference, "NODE-SHA"),
		To:       movementValue(t, tfdomain.NewNetworkNodeReference, "NODE-SZX"),
		Findings: []tfdomain.DepartureGateFinding{unit},
	})
	if err != nil {
		t.Fatalf("袋出发的首版读数：%v", err)
	}
	departure := departureWithReading(t, first)
	at := movementRecordedAt.Add(time.Hour)
	member := reaskRecord(t, departure, "parcel-1", 1, at, reaskReading(t,
		map[string]tfdomain.DepartureGateAnswer{"parcel-1": tfdomain.DepartureGateBlocked}))
	bag := reaskRecord(t, departure, "bag-1", 1, at.Add(time.Minute), reaskReading(t,
		map[string]tfdomain.DepartureGateAnswer{"parcel-1": tfdomain.DepartureGateBlocked, "parcel-2": tfdomain.DepartureGateAdmitted}))
	fixture := newReaskSignalFixture(t, departure, member, bag)

	if err := fixture.subject.HandleRecordedDepartureGateReask(t.Context(), recordedReaskRef("parcel-1", 1)); err != nil {
		t.Fatalf("handle member: %v", err)
	}
	if len(fixture.signals.commands) != 1 {
		t.Fatalf("成员补问交进信号编排 %d 件, want 1", len(fixture.signals.commands))
	}
	command := fixture.signals.commands[0]
	if command.Parcel.String() != "parcel-1" || command.Fact.String() != "movement-fact/parcel-1/move-1" ||
		command.Version.String() != "v1" || command.Judgement != "AADJ-REASK-parcel-1" {
		t.Fatalf("信号命令 = %s %s %s %q", command.Parcel, command.Fact, command.Version, command.Judgement)
	}
	if err := fixture.subject.HandleRecordedDepartureGateReask(t.Context(), recordedReaskRef("bag-1", 1)); err != nil {
		t.Fatalf("handle bag: %v", err)
	}
	if len(fixture.signals.commands) != 1 {
		t.Fatalf("替袋补问的那一份又交了信号：%v", fixture.signals.parcels)
	}
}

// Covers: AT-VE-207 适配器半边——补问信号那一路的失败分格：补问读不回或列不出、出发读不动、信号搁置是未决；补问或出发装不回、对不上
// 键、补的不是原版出发、补问没列在它的出发下、阻断那一格没有判断标识是仓储不变量已破；引用译不出是引用坏了。
func TestReaskSignalFailuresKeepTheirSentinels(t *testing.T) {
	departure := reaskStoppedDeparture(t)
	blocked := func() tfports.DepartureGateReaskRecord {
		return reaskRecord(t, departure, "parcel-1", 1, movementRecordedAt.Add(time.Hour), reaskReading(t,
			map[string]tfdomain.DepartureGateAnswer{"parcel-1": tfdomain.DepartureGateBlocked}))
	}
	unreadableRow := fmt.Errorf("adapter: %w: bad row", tfports.ErrDepartureGateReaskRecordUnreadable)
	down := errors.New("册不可用")
	cases := map[string]struct {
		arrange func(*reaskSignalFixture, *veinbox.RecordedDepartureGateReask)
		want    error
	}{
		"补问读不回": {func(_ *reaskSignalFixture, ref *veinbox.RecordedDepartureGateReask) { ref.Sequence = "9" },
			adapter.ErrDepartureGateReaskNotVisible},
		"补问册读不动": {func(fixture *reaskSignalFixture, _ *veinbox.RecordedDepartureGateReask) {
			fixture.reasks.findErr = down
		},
			adapter.ErrDepartureGateReaskNotVisible},
		"补问那一行装不回": {func(fixture *reaskSignalFixture, _ *veinbox.RecordedDepartureGateReask) {
			fixture.reasks.findErr = unreadableRow
		},
			adapter.ErrMovementFactRecordInconsistent},
		"按出发列补问读不动": {func(fixture *reaskSignalFixture, _ *veinbox.RecordedDepartureGateReask) {
			fixture.reasks.listErr = down
		},
			adapter.ErrDepartureGateReaskNotVisible},
		"按出发列补问装不回": {func(fixture *reaskSignalFixture, _ *veinbox.RecordedDepartureGateReask) {
			fixture.reasks.listErr = unreadableRow
		}, adapter.ErrMovementFactRecordInconsistent},
		"出发读不动": {func(fixture *reaskSignalFixture, _ *veinbox.RecordedDepartureGateReask) { fixture.facts.err = down },
			adapter.ErrMovementFactNotVisible},
		"补问挂着的出发读不回": {func(fixture *reaskSignalFixture, _ *veinbox.RecordedDepartureGateReask) {
			fixture.facts.records = map[tfports.MovementFactKey]tfports.MovementFactRecord{}
		}, adapter.ErrMovementFactRecordInconsistent},
		"补的不是原版出发": {func(fixture *reaskSignalFixture, _ *veinbox.RecordedDepartureGateReask) {
			original := departure.Fact
			corrected, err := original.Correct(original.Location(), movementDepartedAt, original.Source(),
				movementValue(t, tfdomain.NewMovementFactVersion, "v2"), movementRecordedAt)
			if err != nil {
				t.Fatalf("更正：%v", err)
			}
			// 键仍是 v1 那一份，取回的却是一版更正：仓储答非所问。
			fixture.facts.records[departure.Key] = tfports.MovementFactRecord{Key: departure.Key, Fact: corrected, RecordedAt: movementRecordedAt}
		}, adapter.ErrMovementFactRecordInconsistent},
		"补问没列在它的出发下": {func(fixture *reaskSignalFixture, _ *veinbox.RecordedDepartureGateReask) {
			fixture.reasks.unlisted = true
		}, adapter.ErrMovementFactRecordInconsistent},
		"次序译不出": {func(_ *reaskSignalFixture, ref *veinbox.RecordedDepartureGateReask) { ref.Sequence = "first" },
			adapter.ErrMovementFactUntranslatableAnswer},
		"信号搁置": {func(fixture *reaskSignalFixture, _ *veinbox.RecordedDepartureGateReask) {
			fixture.signals.answer = signalAnswering(movementRuleDouble{err: down})
		}, adapter.ErrDepartureGateSignalUndecided},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newReaskSignalFixture(t, departure, blocked())
			ref := recordedReaskRef("parcel-1", 1)
			testCase.arrange(fixture, &ref)
			if err := fixture.subject.HandleRecordedDepartureGateReask(t.Context(), ref); !errors.Is(err, testCase.want) {
				t.Fatalf("err = %v, want %v", err, testCase.want)
			}
		})
	}
	if _, err := adapter.NewRaiseOnDepartureGateReaskAdapter(nil, &movementFinderDouble{}, &movementSignalDouble{}); err == nil {
		t.Fatal("缺补问册也装得起来")
	}
}
