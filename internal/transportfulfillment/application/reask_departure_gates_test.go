package application_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var reaskRegistryDown = errors.New("补问册不可用")

// reaskSegmentsDouble 在出发读数那只段册替身上加「对象在哪些段里有参与」那一问。
type reaskSegmentsDouble struct {
	*departureSegmentsDouble
	listErr error
	lists   int
}

func (double *reaskSegmentsDouble) FindSegmentsForObject(
	_ context.Context,
	tenant domain.TenantID,
	object domain.CarriedObjectReference,
) ([]ports.FulfillmentSegmentKey, error) {
	double.lists++
	if double.listErr != nil {
		return nil, double.listErr
	}
	var keys []ports.FulfillmentSegmentKey
	for _, segment := range double.segments {
		for _, participation := range segment.Participations() {
			if participation.Object() == object {
				keys = append(keys, ports.FulfillmentSegmentKey{TenantID: tenant, Segment: segment.Segment()})
				break
			}
		}
	}
	slices.SortFunc(keys, func(left, right ports.FulfillmentSegmentKey) int {
		return compareStrings(left.Segment.String(), right.Segment.String())
	})
	return keys, nil
}

func compareStrings(left, right string) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

// reaskFactsDouble 按段交回声明了它的事实。
type reaskFactsDouble struct {
	bySegment map[string][]ports.MovementFactRecord
	err       error
}

func (double *reaskFactsDouble) FindBySegment(_ context.Context, key ports.FulfillmentSegmentKey) ([]ports.MovementFactRecord, error) {
	if double.err != nil {
		return nil, double.err
	}
	return double.bySegment[key.Segment.String()], nil
}

// reaskRegistryDouble 是补问册：按出发列出、按键找、追加；saveOutcome 非零时 Save 照它答。
type reaskRegistryDouble struct {
	records     []ports.DepartureGateReaskRecord
	findErr     error
	saveErr     error
	saveOutcome ports.DepartureGateReaskSaveOutcome
	saved       []ports.DepartureGateReaskRecord
}

func (double *reaskRegistryDouble) FindByKey(_ context.Context, key ports.DepartureGateReaskKey) (ports.DepartureGateReaskRecord, bool, error) {
	if double.findErr != nil {
		return ports.DepartureGateReaskRecord{}, false, double.findErr
	}
	for _, record := range double.records {
		if record.Key == key {
			return record, true, nil
		}
	}
	return ports.DepartureGateReaskRecord{}, false, nil
}

func (double *reaskRegistryDouble) FindByDeparture(_ context.Context, departure ports.MovementFactKey) ([]ports.DepartureGateReaskRecord, error) {
	if double.findErr != nil {
		return nil, double.findErr
	}
	var records []ports.DepartureGateReaskRecord
	for _, record := range double.records {
		if record.Key.Departure() == departure {
			records = append(records, record)
		}
	}
	return records, nil
}

func (double *reaskRegistryDouble) Save(_ context.Context, record ports.DepartureGateReaskRecord) (ports.DepartureGateReaskSaveOutcome, error) {
	if double.saveErr != nil {
		return ports.DepartureGateReaskSaveOutcomeInvalid, double.saveErr
	}
	if double.saveOutcome != ports.DepartureGateReaskSaveOutcomeInvalid {
		return double.saveOutcome, nil
	}
	double.saved = append(double.saved, record)
	double.records = append(double.records, record)
	return ports.DepartureGateReaskSaved, nil
}

type reaskHandoffDouble struct {
	err    error
	handed []ports.DepartureGateReaskRecord
}

func (double *reaskHandoffDouble) HandOffDepartureGateReask(_ context.Context, record ports.DepartureGateReaskRecord) error {
	if double.err != nil {
		return double.err
	}
	double.handed = append(double.handed, record)
	return nil
}

// reaskHandoversDouble 按键交回一版交接。
type reaskHandoversDouble struct {
	records map[ports.TransportHandoverKey]ports.TransportHandoverRecord
	err     error
}

func (double *reaskHandoversDouble) FindByKey(_ context.Context, key ports.TransportHandoverKey) (ports.TransportHandoverRecord, bool, error) {
	if double.err != nil {
		return ports.TransportHandoverRecord{}, false, double.err
	}
	record, found := double.records[key]
	return record, found, nil
}

// reaskPickupsDouble 按键交回一次对象级揽收。
type reaskPickupsDouble struct {
	records map[ports.OffsitePickupKey]ports.OffsitePickupRecord
	err     error
}

func (double *reaskPickupsDouble) FindByKey(_ context.Context, key ports.OffsitePickupKey) (ports.OffsitePickupRecord, bool, error) {
	if double.err != nil {
		return ports.OffsitePickupRecord{}, false, double.err
	}
	record, found := double.records[key]
	return record, found, nil
}

type reaskFixture struct {
	gate      *departureGateFixture
	segments  *reaskSegmentsDouble
	facts     *reaskFactsDouble
	reasks    *reaskRegistryDouble
	handoff   *reaskHandoffDouble
	handovers *reaskHandoversDouble
	pickups   *reaskPickupsDouble
}

// newReaskFixture 立 segment-1：objects 各在出发前一小时凭`已交接`进段（交接声明 NODE-SHA → NODE-SZX，与班次同向）。出发登在 segment-1
// 上、时点 movedAtFixture，首版读数由调用方给。
func newReaskFixture(t *testing.T, objects ...string) *reaskFixture {
	t.Helper()
	gate := newDepartureGateFixture(t, objects...)
	fixture := &reaskFixture{
		gate:     gate,
		segments: &reaskSegmentsDouble{departureSegmentsDouble: gate.segments},
		facts:    &reaskFactsDouble{bySegment: map[string][]ports.MovementFactRecord{}},
		reasks:   &reaskRegistryDouble{},
		handoff:  &reaskHandoffDouble{},
		handovers: &reaskHandoversDouble{
			records: map[ports.TransportHandoverKey]ports.TransportHandoverRecord{},
		},
		pickups: &reaskPickupsDouble{records: map[ports.OffsitePickupKey]ports.OffsitePickupRecord{}},
	}
	for _, handover := range gate.segments.handovers {
		key := ports.TransportHandoverKey{
			TenantID: handover.TenantID(), Object: handover.Object(), Scope: handover.Scope(), Version: handover.Version(),
		}
		fixture.handovers.records[key] = ports.TransportHandoverRecord{Key: key, Handover: handover, RecordedAt: movementRecordedAt}
	}
	return fixture
}

func (fixture *reaskFixture) handler(t *testing.T) *application.ReaskDepartureGatesHandler {
	t.Helper()
	handler, err := application.NewReaskDepartureGatesHandler(application.ReaskDepartureGatesDeps{
		Handovers: fixture.handovers,
		Pickups:   fixture.pickups,
		Segments:  fixture.segments,
		Facts:     fixture.facts,
		Reasks:    fixture.reasks,
		Gate:      fixture.gate.reader(t),
		Handoff:   fixture.handoff,
		Clock:     fixedClock{at: movementRecordedAt.Add(time.Hour)},
	})
	if err != nil {
		t.Fatalf("装补问编排：%v", err)
	}
	return handler
}

// departOn 在 segment 上登一条出发（版本 v1，时点 movedAtFixture），首版读数为 first。
func (fixture *reaskFixture) departOn(t *testing.T, fact, segment string, first domain.DepartureGateReading) ports.MovementFactRecord {
	t.Helper()
	record := departureRecord(t, fact, segment, domain.DepartureFact, first)
	fixture.facts.bySegment[segment] = append(fixture.facts.bySegment[segment], record)
	return record
}

func departureRecord(
	t *testing.T,
	fact, segment string,
	kind domain.MovementFactKind,
	first domain.DepartureGateReading,
) ports.MovementFactRecord {
	t.Helper()
	tenant := mustRefValue(t, domain.NewTenantID, "tenant-1")
	spec := domain.MovementFactSpec{
		TenantID:    tenant,
		Fact:        mustRefValue(t, domain.NewMovementFactReference, fact),
		Schedule:    mustRefValue(t, domain.NewScheduleReference, "schedule-1"),
		Kind:        kind,
		Location:    mustRefValue(t, domain.NewMovementLocationReference, "hub-1"),
		Source:      mustRefValue(t, domain.NewMovementSourceReference, "own-fleet/v1"),
		Version:     mustRefValue(t, domain.NewMovementFactVersion, "v1"),
		OccurredAt:  movedAtFixture,
		Segment:     mustRefValue(t, domain.NewFulfillmentSegmentReference, segment),
		GateReading: first,
	}
	built, err := domain.RecordMovementFact(spec)
	if err != nil {
		t.Fatalf("出发事实：%v", err)
	}
	return ports.MovementFactRecord{
		Key:        ports.MovementFactKey{TenantID: tenant, Fact: spec.Fact, Version: spec.Version},
		Fact:       built,
		RecordedAt: movementRecordedAt,
	}
}

func onHandoverOf(t *testing.T, object string) application.ReaskOnHandoverCommand {
	t.Helper()
	return application.ReaskOnHandoverCommand{
		TenantID: mustRefValue(t, domain.NewTenantID, "tenant-1"),
		Object:   object,
		Scope:    "scope-" + object,
		Version:  "handover-result/" + object + "/v1",
	}
}

func stoppedFirstReading(t *testing.T, stops ...domain.DepartureGateStop) domain.DepartureGateReading {
	t.Helper()
	reading, err := domain.FormDepartureGateReading(domain.DepartureGateReadingSpec{
		From:  mustRefValue(t, domain.NewNetworkNodeReference, "NODE-SHA"),
		To:    mustRefValue(t, domain.NewNetworkNodeReference, "NODE-SZX"),
		Stops: stops,
	})
	if err != nil {
		t.Fatalf("停在本侧的首版读数：%v", err)
	}
	return reading
}

func askedFirstReading(t *testing.T, findings ...domain.DepartureGateFinding) domain.DepartureGateReading {
	t.Helper()
	reading, err := domain.FormDepartureGateReading(domain.DepartureGateReadingSpec{
		From:     mustRefValue(t, domain.NewNetworkNodeReference, "NODE-SHA"),
		To:       mustRefValue(t, domain.NewNetworkNodeReference, "NODE-SZX"),
		Findings: findings,
	})
	if err != nil {
		t.Fatalf("问了关务的首版读数：%v", err)
	}
	return reading
}

func cell(t *testing.T, object string, answer domain.DepartureGateAnswer, cause string) domain.DepartureGateFinding {
	t.Helper()
	judgement, basis := "AADJ-"+object, domain.DepartureGateBasis{Action: "LOADING_DEPARTURE", Cause: cause}
	if answer == domain.DepartureGateUnreadable {
		judgement, basis = "", domain.DepartureGateBasis{Cause: cause}
	}
	finding, err := domain.NewDepartureGateFinding(mustRefValue(t, domain.NewCarriedObjectReference, object), answer, judgement,
		[]domain.DepartureGateBasis{basis})
	if err != nil {
		t.Fatalf("逐对象那一格 %s：%v", object, err)
	}
	return finding
}

func (fixture *reaskFixture) plan(t *testing.T, object string) application.DepartureGateReaskResult {
	t.Helper()
	result, err := fixture.handler(t).PlanOnHandover(t.Context(), onHandoverOf(t, object))
	if err != nil {
		t.Fatalf("补问：%v", err)
	}
	return result
}

// Covers: AT-TF-343 编排半边（ADR-0305）——出发先于段登记，首版读数停在本侧「段不在册」；对象随后凭`已交接`进了那一段，那一封
// 交接上的补问按原出发时点、只替这件对象问关务（起讫取班次方向），追加第 1 次补问并同一笔交一封信；同段别的对象不在这一次里问。
func TestADepartureRecordedBeforeItsSegmentIsReaskedForTheObjectThatEntered(t *testing.T) {
	fixture := newReaskFixture(t, "parcel-1", "parcel-2")
	fixture.gate.customs.answers["parcel-1"] = domain.DepartureGateBlocked
	departure := fixture.departOn(t, "move-1", "segment-1", stoppedFirstReading(t, domain.DepartureSegmentNotRegistered))
	handler := fixture.handler(t)

	planned, err := handler.PlanOnHandover(t.Context(), onHandoverOf(t, "parcel-1"))
	if err != nil {
		t.Fatalf("补问：%v", err)
	}
	if planned.Outcome() != application.DepartureGateReaskScanned || len(planned.Drafts()) != 1 {
		t.Fatalf("补问 = %s %s，%d 份", planned.Outcome(), planned.Stop(), len(planned.Drafts()))
	}
	if len(fixture.gate.customs.queries) != 1 || fixture.gate.customs.queries[0].Object.String() != "parcel-1" ||
		!fixture.gate.customs.queries[0].At.Equal(movedAtFixture) || fixture.gate.customs.queries[0].From.String() != "NODE-SHA" ||
		fixture.gate.customs.queries[0].To.String() != "NODE-SZX" {
		t.Fatalf("问法 = %+v，只问进段的那一件、按原出发时点与班次方向", fixture.gate.customs.queries)
	}
	if len(fixture.reasks.saved) != 0 || len(fixture.handoff.handed) != 0 {
		t.Fatal("问关务那一步就写了册或交了信——写要等进写事务")
	}

	recorded, err := handler.Record(t.Context(), planned)
	if err != nil {
		t.Fatalf("落册：%v", err)
	}
	if recorded.Disposition() != application.DepartureGateReaskSettled || recorded.Recorded() != 1 {
		t.Fatalf("落册 = %s，追加 %d 份", recorded.Disposition(), recorded.Recorded())
	}
	saved := fixture.reasks.saved[0]
	if saved.Key.Departure() != departure.Key || saved.Key.Object.String() != "parcel-1" || saved.Key.Sequence != 1 ||
		saved.Trigger != "transport-handover/parcel-1/scope-parcel-1/handover-result/parcel-1/v1" ||
		!saved.RecordedAt.Equal(movementRecordedAt.Add(time.Hour)) {
		t.Fatalf("补问那一行 = %+v", saved)
	}
	findings := saved.Reading.Findings()
	if saved.Reading.Answer() != domain.DepartureGateBlocked || len(findings) != 1 || findings[0].Object().String() != "parcel-1" {
		t.Fatalf("补问读数 = %s %+v，只该有进段那一件的一格", saved.Reading.Answer(), findings)
	}
	if len(fixture.handoff.handed) != 1 || fixture.handoff.handed[0].Key != saved.Key {
		t.Fatalf("补问信封 = %+v", fixture.handoff.handed)
	}
	if first, _ := departure.Fact.GateReading(); first.Answer() != domain.DepartureGateUndecided {
		t.Fatalf("首版读数被改成了 %s", first.Answer())
	}
}

// Covers: AT-TF-343 编排半边（揽收）——对象凭对象级场外揽收进段，同一个补问由揽收那一封唤醒，唤醒依据记成那一次揽收。
func TestAPickupWakesTheReaskToo(t *testing.T) {
	fixture := newReaskFixture(t, "parcel-1")
	fixture.departOn(t, "move-1", "segment-1", stoppedFirstReading(t, domain.DepartureSegmentNotRegistered))
	tenant := mustRefValue(t, domain.NewTenantID, "tenant-1")
	key := ports.OffsitePickupKey{
		TenantID: tenant,
		Object:   mustRefValue(t, domain.NewCarriedObjectReference, "parcel-1"),
		Attempt:  mustRefValue(t, domain.NewAttemptReference, "attempt-1"),
	}
	pickup, err := domain.FormOffsitePickup(domain.OffsitePickupSpec{
		TenantID:   tenant,
		Object:     key.Object,
		Task:       mustRefValue(t, domain.NewPickupTaskReference, "task-1"),
		Attempt:    key.Attempt,
		Place:      mustRefValue(t, domain.NewPickupPlaceReference, "place-1"),
		Control:    mustRefValue(t, domain.NewTransportControlReference, "control-1"),
		ExecutedBy: mustRefValue(t, domain.NewExecutingPartyReference, "courier-1"),
		Version:    mustRefValue(t, domain.NewPickupResultVersion, "pickup-v1"),
		OccurredAt: movedAtFixture.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("揽收：%v", err)
	}
	fixture.pickups.records[key] = ports.OffsitePickupRecord{Key: key, Pickup: pickup, RecordedAt: movementRecordedAt}

	planned, err := fixture.handler(t).PlanOnPickup(t.Context(), application.ReaskOnPickupCommand{
		TenantID: tenant, Object: "parcel-1", Attempt: "attempt-1",
	})
	if err != nil {
		t.Fatalf("补问：%v", err)
	}
	drafts := planned.Drafts()
	if planned.Outcome() != application.DepartureGateReaskScanned || len(drafts) != 1 ||
		drafts[0].Trigger != "offsite-pickup/parcel-1/attempt-1" {
		t.Fatalf("揽收唤醒的补问 = %s %+v", planned.Outcome(), drafts)
	}
}

// Covers: AT-TF-344 编排半边（ADR-0305）——只在读数里还没有这件对象的定论时补问：它那一格是可再问的读不动（关务或成员关系调用
// 失败、关务指名自己某一口读不动），或问了关务却没有它的一格（那时它还不在段里），或停在本侧且每个停点都可再问；放行、不受门禁、
// 阻断、关务答的未决都是定论，译不回来再问也一样，没声明段、存量回填、班次不服务该段、装不回再问也一样，都不补问、不问关务。
func TestAReaskOnlyComesWhileTheReadingHasNoVerdictForTheObject(t *testing.T) {
	backfilled, err := domain.RehydrateDepartureGateReading(domain.DepartureGateReadingSpec{
		Stops: []domain.DepartureGateStop{domain.DepartureRecordedBeforeGateReading},
	}, domain.DepartureGateUndecided)
	if err != nil {
		t.Fatalf("存量回填读数：%v", err)
	}
	cases := map[string]struct {
		first func(*testing.T) domain.DepartureGateReading
		due   bool
	}{
		"它那一格放行": {func(t *testing.T) domain.DepartureGateReading {
			return askedFirstReading(t, cell(t, "parcel-1", domain.DepartureGateAdmitted, "RELEASED"))
		}, false},
		"它那一格阻断": {func(t *testing.T) domain.DepartureGateReading {
			return askedFirstReading(t, cell(t, "parcel-1", domain.DepartureGateBlocked, "RESTRICTION_IN_FORCE"))
		}, false},
		"关务对它答未决": {func(t *testing.T) domain.DepartureGateReading {
			return askedFirstReading(t, cell(t, "parcel-1", domain.DepartureGateUndecided, "SCOPE_NOT_RESOLVED"))
		}, false},
		"读数里没有它（那时还不在段里）": {func(t *testing.T) domain.DepartureGateReading {
			return askedFirstReading(t, cell(t, "parcel-2", domain.DepartureGateNotGuarded, "OUTSIDE_CUSTOMS_BOOKS"))
		}, true},
		"关务调用失败": {func(t *testing.T) domain.DepartureGateReading {
			return askedFirstReading(t, cell(t, "parcel-1", domain.DepartureGateUnreadable, "CUSTOMS_ADMISSION_UNAVAILABLE"))
		}, true},
		"关务指名自己一口读不动": {func(t *testing.T) domain.DepartureGateReading {
			return askedFirstReading(t, cell(t, "parcel-1", domain.DepartureGateUnreadable, "RELEASE_OUTCOME_VIEW_UNAVAILABLE"))
		}, true},
		"成员关系调用失败": {func(t *testing.T) domain.DepartureGateReading {
			return askedFirstReading(t, cell(t, "parcel-1", domain.DepartureGateUnreadable, "CONSOLIDATION_MEMBERSHIP_UNAVAILABLE"))
		}, true},
		"关务答复译不回来": {func(t *testing.T) domain.DepartureGateReading {
			return askedFirstReading(t, cell(t, "parcel-1", domain.DepartureGateUnreadable, "CUSTOMS_ADMISSION_UNTRANSLATABLE"))
		}, false},
		"成员关系译不回来": {func(t *testing.T) domain.DepartureGateReading {
			return askedFirstReading(t, cell(t, "parcel-1", domain.DepartureGateUnreadable, "CONSOLIDATION_MEMBERSHIP_UNTRANSLATABLE"))
		}, false},
		"段不在册": {func(t *testing.T) domain.DepartureGateReading {
			return stoppedFirstReading(t, domain.DepartureSegmentNotRegistered)
		}, true},
		"那一刻段里没有在控对象": {func(t *testing.T) domain.DepartureGateReading {
			return stoppedFirstReading(t, domain.DepartureNoObjectUnderControl)
		}, true},
		"段册与班次册连不上": {func(t *testing.T) domain.DepartureGateReading {
			return stoppedFirstReading(t, domain.DepartureSegmentRegistryUnavailable, domain.DepartureScheduleRegistryUnavailable)
		}, true},
		"交接册读不动": {func(t *testing.T) domain.DepartureGateReading {
			return stoppedFirstReading(t, domain.DepartureHandoverRegistryUnavailable)
		}, true},
		"段那一行装不回": {func(t *testing.T) domain.DepartureGateReading {
			return stoppedFirstReading(t, domain.DepartureSegmentRecordUnreadable)
		}, false},
		"段不在册而班次那一行装不回": {func(t *testing.T) domain.DepartureGateReading {
			return stoppedFirstReading(t, domain.DepartureSegmentNotRegistered, domain.DepartureScheduleRecordUnreadable)
		}, false},
		"班次不服务该段": {func(t *testing.T) domain.DepartureGateReading {
			return stoppedFirstReading(t, domain.DepartureScheduleNotServingSegment)
		}, false},
		"存量回填": {func(*testing.T) domain.DepartureGateReading { return backfilled }, false},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newReaskFixture(t, "parcel-1")
			fixture.departOn(t, "move-1", "segment-1", testCase.first(t))

			planned := fixture.plan(t, "parcel-1")
			if planned.Outcome() != application.DepartureGateReaskScanned {
				t.Fatalf("补问 = %s %s", planned.Outcome(), planned.Stop())
			}
			if due := len(planned.Drafts()) == 1; due != testCase.due {
				t.Fatalf("补问了 %d 份, want due=%v", len(planned.Drafts()), testCase.due)
			}
			if asked := len(fixture.gate.customs.queries) > 0; asked != testCase.due {
				t.Fatalf("问了关务 %d 次, want due=%v", len(fixture.gate.customs.queries), testCase.due)
			}
		})
	}
}

// Covers: AT-TF-344 编排半边（集运单元）——单元那一格由成员推出：成员身份读不动或某件成员那一格是可再问的读不动，单元就该补问；只因
// 成员关系成环而读不动的不补问。
func TestAUnitIsReaskedOnlyWhileAMemberCouldStillAnswer(t *testing.T) {
	unitCell := func(t *testing.T, members []domain.GateFinding, gaps []domain.ConsolidationMemberGap) domain.GateFinding {
		t.Helper()
		finding, err := domain.FormConsolidationUnitGateFinding(mustRefValue(t, domain.NewCarriedObjectReference, "bag-1"), members, gaps)
		if err != nil {
			t.Fatalf("单元那一格：%v", err)
		}
		return finding
	}
	cases := map[string]struct {
		findings func(*testing.T) []domain.DepartureGateFinding
		due      bool
	}{
		"成员那一格关务调用失败": {func(t *testing.T) []domain.DepartureGateFinding {
			member := cell(t, "parcel-9", domain.DepartureGateUnreadable, "CUSTOMS_ADMISSION_UNAVAILABLE")
			return []domain.DepartureGateFinding{unitCell(t, []domain.GateFinding{member}, nil), member}
		}, true},
		"成员身份读不动": {func(t *testing.T) []domain.DepartureGateFinding {
			return []domain.DepartureGateFinding{unitCell(t, nil, []domain.ConsolidationMemberGap{
				{Stop: domain.ConsolidationMemberIdentityUnreadable, Sources: []string{"HANDLING-1"}},
			})}
		}, true},
		"成员关系成环": {func(t *testing.T) []domain.DepartureGateFinding {
			return []domain.DepartureGateFinding{unitCell(t, nil, []domain.ConsolidationMemberGap{
				{Stop: domain.ConsolidationMembershipCycle, Sources: []string{"bag-1"}},
			})}
		}, false},
		"成员那一格译不回来": {func(t *testing.T) []domain.DepartureGateFinding {
			member := cell(t, "parcel-9", domain.DepartureGateUnreadable, "CUSTOMS_ADMISSION_UNTRANSLATABLE")
			return []domain.DepartureGateFinding{unitCell(t, []domain.GateFinding{member}, nil), member}
		}, false},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newReaskFixture(t, "bag-1")
			fixture.departOn(t, "move-1", "segment-1", askedFirstReading(t, testCase.findings(t)...))

			planned := fixture.plan(t, "bag-1")
			if due := len(planned.Drafts()) == 1; due != testCase.due {
				t.Fatalf("补问 %s 了 %d 份, want due=%v", planned.Outcome(), len(planned.Drafts()), testCase.due)
			}
		})
	}
}

// Covers: AT-TF-345 编排半边（ADR-0305 次数上限，产品策略）——补问看这件对象最近一次补问：有了定论就不再问；同一条出发替同一件对象
// 补到上限仍没有定论就不再问、记一条到上限；别的对象的补问不算这一件的次数。
func TestReaskingFollowsTheLatestReaskAndStopsAtTheCap(t *testing.T) {
	unreadable := func(t *testing.T) domain.DepartureGateReading {
		return askedFirstReading(t, cell(t, "parcel-1", domain.DepartureGateUnreadable, "CUSTOMS_ADMISSION_UNAVAILABLE"))
	}
	cases := map[string]struct {
		earlier  func(*testing.T) map[string][]domain.DepartureGateReading
		sequence int
		exhaust  int
	}{
		"补问过、已有定论": {func(t *testing.T) map[string][]domain.DepartureGateReading {
			return map[string][]domain.DepartureGateReading{"parcel-1": {
				askedFirstReading(t, cell(t, "parcel-1", domain.DepartureGateAdmitted, "RELEASED")),
			}}
		}, 0, 0},
		"补问过两次仍读不动": {func(t *testing.T) map[string][]domain.DepartureGateReading {
			return map[string][]domain.DepartureGateReading{"parcel-1": {unreadable(t), unreadable(t)}}
		}, 3, 0},
		"补到上限仍读不动": {func(t *testing.T) map[string][]domain.DepartureGateReading {
			readings := make([]domain.DepartureGateReading, 0, application.MaxDepartureGateReasks)
			for range application.MaxDepartureGateReasks {
				readings = append(readings, unreadable(t))
			}
			return map[string][]domain.DepartureGateReading{"parcel-1": readings}
		}, 0, 1},
		"只有别的对象补问过": {func(t *testing.T) map[string][]domain.DepartureGateReading {
			return map[string][]domain.DepartureGateReading{"parcel-2": {unreadable(t), unreadable(t), unreadable(t)}}
		}, 1, 0},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newReaskFixture(t, "parcel-1", "parcel-2")
			departure := fixture.departOn(t, "move-1", "segment-1", stoppedFirstReading(t, domain.DepartureSegmentNotRegistered))
			for object, readings := range testCase.earlier(t) {
				for index, reading := range readings {
					fixture.reasks.records = append(fixture.reasks.records, ports.DepartureGateReaskRecord{
						Key: ports.DepartureGateReaskKey{
							TenantID: departure.Key.TenantID, Fact: departure.Key.Fact, Version: departure.Key.Version,
							Object: mustRefValue(t, domain.NewCarriedObjectReference, object), Sequence: index + 1,
						},
						Reading: reading, Trigger: "earlier", RecordedAt: movementRecordedAt,
					})
				}
			}

			planned := fixture.plan(t, "parcel-1")
			drafts := planned.Drafts()
			if testCase.sequence == 0 {
				if len(drafts) != 0 || len(fixture.gate.customs.queries) != 0 {
					t.Fatalf("不该补问却补了 %d 份、问了关务 %d 次", len(drafts), len(fixture.gate.customs.queries))
				}
			} else if len(drafts) != 1 || drafts[0].Key.Sequence != testCase.sequence {
				t.Fatalf("补问 %+v, want 第 %d 次", drafts, testCase.sequence)
			}
			if planned.Exhausted() != testCase.exhaust {
				t.Fatalf("到上限 %d 条, want %d", planned.Exhausted(), testCase.exhaust)
			}
		})
	}
	t.Run("落册时刻与次序颠倒", func(t *testing.T) {
		// 第 2 次已有定论、却比第 1 次（读不动）落册得早（时钟回拨）：最近一份按次序取第 2 次，不再补问。
		fixture := newReaskFixture(t, "parcel-1")
		departure := fixture.departOn(t, "move-1", "segment-1", stoppedFirstReading(t, domain.DepartureSegmentNotRegistered))
		for _, reask := range []struct {
			sequence int
			reading  domain.DepartureGateReading
			at       time.Time
		}{
			{2, askedFirstReading(t, cell(t, "parcel-1", domain.DepartureGateAdmitted, "RELEASED")), movementRecordedAt},
			{1, unreadable(t), movementRecordedAt.Add(time.Minute)},
		} {
			fixture.reasks.records = append(fixture.reasks.records, ports.DepartureGateReaskRecord{
				Key: ports.DepartureGateReaskKey{
					TenantID: departure.Key.TenantID, Fact: departure.Key.Fact, Version: departure.Key.Version,
					Object: mustRefValue(t, domain.NewCarriedObjectReference, "parcel-1"), Sequence: reask.sequence,
				},
				Reading: reask.reading, Trigger: "earlier", RecordedAt: reask.at,
			})
		}
		if drafts := fixture.plan(t, "parcel-1").Drafts(); len(drafts) != 0 || len(fixture.gate.customs.queries) != 0 {
			t.Fatalf("按落册时刻取了最近一份：补问 %d 份、问关务 %d 次", len(drafts), len(fixture.gate.customs.queries))
		}
	})
}

// Covers: AT-TF-345 编排半边——只补原版出发、只补出发那一刻在控的那一件：移动与到达、更正版、对象那一刻还没进段的出发都不补；交接
// 拒收或待确认没让对象进段，连段都不读。
func TestReaskSkipsWhatIsNotAnOriginalDepartureUnderControl(t *testing.T) {
	t.Run("移动与更正版", func(t *testing.T) {
		fixture := newReaskFixture(t, "parcel-1")
		stopped := stoppedFirstReading(t, domain.DepartureSegmentNotRegistered)
		fixture.facts.bySegment["segment-1"] = append(fixture.facts.bySegment["segment-1"],
			departureRecord(t, "move-arrive", "segment-1", domain.ArrivalFact, domain.DepartureGateReading{}))
		original := departureRecord(t, "move-1", "segment-1", domain.DepartureFact, stopped)
		corrected, err := original.Fact.Correct(original.Fact.Location(), movedAtFixture,
			mustRefValue(t, domain.NewMovementSourceReference, "own-fleet/v2"),
			mustRefValue(t, domain.NewMovementFactVersion, "v2"), movementRecordedAt)
		if err != nil {
			t.Fatalf("更正：%v", err)
		}
		fixture.facts.bySegment["segment-1"] = append(fixture.facts.bySegment["segment-1"], ports.MovementFactRecord{
			Key:  ports.MovementFactKey{TenantID: original.Key.TenantID, Fact: original.Key.Fact, Version: corrected.Version()},
			Fact: corrected, RecordedAt: movementRecordedAt,
		})

		drafts := fixture.plan(t, "parcel-1").Drafts()
		if len(drafts) != 0 {
			t.Fatalf("移动与更正版补问了 %+v", drafts)
		}
	})
	t.Run("出发那一刻它还不在段里", func(t *testing.T) {
		fixture := newReaskFixture(t, "parcel-1")
		record := departureRecord(t, "move-early", "segment-1", domain.DepartureFact,
			stoppedFirstReading(t, domain.DepartureSegmentNotRegistered))
		early, err := domain.RehydrateMovementFact(domain.RehydrateMovementFactSpec{
			TenantID: record.Key.TenantID, Fact: record.Key.Fact, Version: record.Key.Version,
			Schedule: record.Fact.Schedule(), Kind: domain.DepartureFact, Location: record.Fact.Location(),
			Source: record.Fact.Source(), OccurredAt: movedAtFixture.Add(-2 * time.Hour),
			Segment: mustRefValue(t, domain.NewFulfillmentSegmentReference, "segment-1"), GateReading: stoppedFirstReading(t, domain.DepartureSegmentNotRegistered),
		})
		if err != nil {
			t.Fatalf("早于进段的出发：%v", err)
		}
		record.Fact = early
		fixture.facts.bySegment["segment-1"] = []ports.MovementFactRecord{record}

		if drafts := fixture.plan(t, "parcel-1").Drafts(); len(drafts) != 0 {
			t.Fatalf("对象那一刻不在控也补问了 %+v", drafts)
		}
	})
	t.Run("交接待确认", func(t *testing.T) {
		fixture := newReaskFixture(t, "parcel-1")
		fixture.departOn(t, "move-1", "segment-1", stoppedFirstReading(t, domain.DepartureSegmentNotRegistered))
		for key, record := range fixture.handovers.records {
			pending, err := domain.FormTransportHandover(domain.TransportHandoverSpec{
				TenantID: key.TenantID, Object: key.Object, Scope: key.Scope, Version: key.Version,
				ReleasedBy:        mustRefValue(t, domain.NewHandoverPartyReference, "party-sha-hub"),
				ReceivedBy:        mustRefValue(t, domain.NewHandoverPartyReference, "party-own-linehaul"),
				Verdict:           domain.HandoverPendingConfirmation,
				ReleasingEvidence: mustRefValue(t, domain.NewHandoverEvidenceReference, "evidence-release"),
				Rule:              mustRefValue(t, domain.NewHandoverRuleReference, "handover-rule/v1"),
				JudgedAt:          record.Handover.JudgedAt(),
				Basis:             mustRefValue(t, domain.NewHandoverBasisReference, "receiver-scan-missing"),
			})
			if err != nil {
				t.Fatalf("待确认交接：%v", err)
			}
			record.Handover = pending
			fixture.handovers.records[key] = record
		}

		planned := fixture.plan(t, "parcel-1")
		if planned.Outcome() != application.DepartureGateReaskNotHandedOver || planned.Disposition() != application.DepartureGateReaskSettled {
			t.Fatalf("待确认 = %s %s", planned.Outcome(), planned.Disposition())
		}
		if fixture.segments.lists != 0 {
			t.Fatal("待确认没让对象进段，却读了段")
		}
	})
}

// Covers: AT-TF-346 编排半边（ADR-0305）——补问的失败分格：控制事实读不回、段册、移动事实册或补问册读不动、补问交不出去是未决、
// 原样重投；段行、事实行或补问行装不回、按对象列出的段按键答没有、列出的事实没声明这一段、控制事实键立不起来是缺陷、重投不会变。
// 关务那一问读不动不是失败：它是补问读数里的一格。
func TestReaskFailuresLandInTheirPartition(t *testing.T) {
	unreadableRow := func(sentinel error) error { return fmt.Errorf("adapter: %w: bad row", sentinel) }
	cases := map[string]struct {
		arrange     func(*testing.T, *reaskFixture, *application.ReaskOnHandoverCommand)
		stop        application.DepartureGateReaskStop
		disposition application.DepartureGateReaskDisposition
	}{
		"交接册读不动": {func(_ *testing.T, fixture *reaskFixture, _ *application.ReaskOnHandoverCommand) {
			fixture.handovers.err = departureGateDown
		}, application.DepartureGateReaskHandoverRegistryUnavailable, application.DepartureGateReaskRetry},
		"交接读不回": {func(_ *testing.T, _ *reaskFixture, command *application.ReaskOnHandoverCommand) {
			command.Version = "handover-result/parcel-1/v9"
		}, application.DepartureGateReaskHandoverNotVisible, application.DepartureGateReaskRetry},
		"交接键立不起来": {func(_ *testing.T, _ *reaskFixture, command *application.ReaskOnHandoverCommand) {
			command.Scope = ""
		}, application.DepartureGateReaskKeyNotAccepted, application.DepartureGateReaskFailed},
		"对象的段列不出": {func(_ *testing.T, fixture *reaskFixture, _ *application.ReaskOnHandoverCommand) {
			fixture.segments.listErr = departureGateDown
		}, application.DepartureGateReaskSegmentRegistryUnavailable, application.DepartureGateReaskRetry},
		"段按键读不动": {func(_ *testing.T, fixture *reaskFixture, _ *application.ReaskOnHandoverCommand) {
			fixture.segments.findErr = departureGateDown
		}, application.DepartureGateReaskSegmentRegistryUnavailable, application.DepartureGateReaskRetry},
		"段那一行装不回": {func(_ *testing.T, fixture *reaskFixture, _ *application.ReaskOnHandoverCommand) {
			fixture.segments.findErr = unreadableRow(ports.ErrFulfillmentSegmentRecordUnreadable)
		}, application.DepartureGateReaskSegmentRecordUnreadable, application.DepartureGateReaskFailed},
		"按段列事实读不动": {func(_ *testing.T, fixture *reaskFixture, _ *application.ReaskOnHandoverCommand) {
			fixture.facts.err = departureGateDown
		}, application.DepartureGateReaskMovementFactRegistryUnavailable, application.DepartureGateReaskRetry},
		"按段列出的事实装不回": {func(_ *testing.T, fixture *reaskFixture, _ *application.ReaskOnHandoverCommand) {
			fixture.facts.err = unreadableRow(ports.ErrMovementFactRecordUnreadable)
		}, application.DepartureGateReaskMovementFactRecordUnreadable, application.DepartureGateReaskFailed},
		"列出的事实没声明这一段": {func(t *testing.T, fixture *reaskFixture, _ *application.ReaskOnHandoverCommand) {
			fixture.facts.bySegment["segment-1"] = []ports.MovementFactRecord{departureRecord(t, "move-2", "segment-2",
				domain.DepartureFact, stoppedFirstReading(t, domain.DepartureSegmentNotRegistered))}
		}, application.DepartureGateReaskMovementFactInconsistent, application.DepartureGateReaskFailed},
		"补问册读不动": {func(_ *testing.T, fixture *reaskFixture, _ *application.ReaskOnHandoverCommand) {
			fixture.reasks.findErr = reaskRegistryDown
		}, application.DepartureGateReaskRegistryUnavailable, application.DepartureGateReaskRetry},
		"补问那一行装不回": {func(_ *testing.T, fixture *reaskFixture, _ *application.ReaskOnHandoverCommand) {
			fixture.reasks.findErr = unreadableRow(ports.ErrDepartureGateReaskRecordUnreadable)
		}, application.DepartureGateReaskRecordUnreadable, application.DepartureGateReaskFailed},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newReaskFixture(t, "parcel-1")
			fixture.departOn(t, "move-1", "segment-1", stoppedFirstReading(t, domain.DepartureSegmentNotRegistered))
			command := onHandoverOf(t, "parcel-1")
			testCase.arrange(t, fixture, &command)

			handler := fixture.handler(t)
			planned, err := handler.PlanOnHandover(t.Context(), command)
			if err != nil {
				t.Fatalf("补问：%v", err)
			}
			recorded, err := handler.Record(t.Context(), planned)
			if err != nil {
				t.Fatalf("落册：%v", err)
			}
			if recorded.Stop() != testCase.stop || recorded.Disposition() != testCase.disposition {
				t.Fatalf("补问 = %s %s %s, want %s %s", recorded.Outcome(), recorded.Stop(), recorded.Disposition(),
					testCase.stop, testCase.disposition)
			}
			if len(fixture.reasks.saved) != 0 || len(fixture.handoff.handed) != 0 {
				t.Fatal("停下了还落了册或交了信")
			}
		})
	}

	t.Run("关务读不动是读数里的一格", func(t *testing.T) {
		fixture := newReaskFixture(t, "parcel-1")
		fixture.gate.customs.errors["parcel-1"] = true
		fixture.departOn(t, "move-1", "segment-1", stoppedFirstReading(t, domain.DepartureSegmentNotRegistered))
		handler := fixture.handler(t)
		planned, err := handler.PlanOnHandover(t.Context(), onHandoverOf(t, "parcel-1"))
		if err != nil {
			t.Fatalf("补问：%v", err)
		}
		recorded, err := handler.Record(t.Context(), planned)
		if err != nil || recorded.Disposition() != application.DepartureGateReaskSettled || recorded.Recorded() != 1 {
			t.Fatalf("关务读不动 = %s %v", recorded.Disposition(), err)
		}
		if answer := fixture.reasks.saved[0].Reading.Answer(); answer != domain.DepartureGateUnreadable {
			t.Fatalf("补问读数 = %s, want UNREADABLE", answer)
		}
	})
}

// Covers: AT-TF-345 编排半边——两步之间另一方先补了同一次：撞键认先到者、不覆盖、不另交信封，这一拍答未决、整笔回滚重投（重投时按
// 先到那一份重算）；补问册写不进、补问信封交不出同样是未决。
func TestRecordKeepsTheFirstComerAndHandsOffOnlyWhatItAppended(t *testing.T) {
	cases := map[string]struct {
		arrange     func(*reaskFixture)
		recorded    int
		stop        application.DepartureGateReaskStop
		disposition application.DepartureGateReaskDisposition
	}{
		"另一方先补了同一次": {func(fixture *reaskFixture) {
			fixture.reasks.saveOutcome = ports.DepartureGateReaskAlreadyRegistered
		}, 0, application.DepartureGateReaskSequenceTaken, application.DepartureGateReaskRetry},
		"补问册写不进": {func(fixture *reaskFixture) {
			fixture.reasks.saveErr = reaskRegistryDown
		}, 0, application.DepartureGateReaskRegistryUnavailable, application.DepartureGateReaskRetry},
		"补问信封交不出": {func(fixture *reaskFixture) {
			fixture.handoff.err = errors.New("outbox 不可用")
		}, 0, application.DepartureGateReaskHandoffUnavailable, application.DepartureGateReaskRetry},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newReaskFixture(t, "parcel-1")
			fixture.departOn(t, "move-1", "segment-1", stoppedFirstReading(t, domain.DepartureSegmentNotRegistered))
			handler := fixture.handler(t)
			planned, err := handler.PlanOnHandover(t.Context(), onHandoverOf(t, "parcel-1"))
			if err != nil || len(planned.Drafts()) != 1 {
				t.Fatalf("补问：%d 份 %v", len(planned.Drafts()), err)
			}
			testCase.arrange(fixture)

			recorded, err := handler.Record(t.Context(), planned)
			if err != nil {
				t.Fatalf("落册：%v", err)
			}
			if recorded.Recorded() != testCase.recorded || recorded.Stop() != testCase.stop ||
				recorded.Disposition() != testCase.disposition {
				t.Fatalf("落册 = %d 份 %s %s", recorded.Recorded(), recorded.Stop(), recorded.Disposition())
			}
			if len(fixture.handoff.handed) != 0 {
				t.Fatal("没落成的那一份还交了信封")
			}
		})
	}
}

// Covers: AT-TF-343 编排半边——补问的读数路径与登记那一刻同一条（readIntent）：只问那一件，那一件此刻在那一刻不在控时停在「出发那一刻
// 段里没有在控对象」、不问关务；交来的不是声明了段的出发是接线错了，上抛。缺件的编排装不起来。
func TestTheReaskReadsTheOneObjectThroughTheDepartureReadingPath(t *testing.T) {
	fixture := newReaskFixture(t, "parcel-1", "parcel-2")
	departure := departureRecord(t, "move-1", "segment-1", domain.DepartureFact, stoppedFirstReading(t, domain.DepartureSegmentNotRegistered))
	reader := fixture.gate.reader(t)

	reading, err := reader.Reask(t.Context(), departure.Fact, mustRefValue(t, domain.NewCarriedObjectReference, "parcel-3"))
	if err != nil {
		t.Fatalf("补问不在段里的那一件：%v", err)
	}
	if !slices.Equal(stopWords(reading), []string{"NO_OBJECT_UNDER_CONTROL"}) || len(fixture.gate.customs.queries) != 0 {
		t.Fatalf("不在控的那一件 = %v，问了关务 %d 次", stopWords(reading), len(fixture.gate.customs.queries))
	}

	arrival := departureRecord(t, "move-2", "segment-1", domain.ArrivalFact, domain.DepartureGateReading{})
	if _, err := reader.Reask(t.Context(), arrival.Fact, mustRefValue(t, domain.NewCarriedObjectReference, "parcel-1")); err == nil {
		t.Fatal("到达也补问了")
	}
	if _, err := application.NewReaskDepartureGatesHandler(application.ReaskDepartureGatesDeps{}); err == nil {
		t.Fatal("缺件的补问编排也装得起来")
	}
}
