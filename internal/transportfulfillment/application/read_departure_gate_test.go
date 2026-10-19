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

var departureGateDown = errors.New("册不可用")

// departureSegmentsDouble 按段号交回一个段：在册的段由 RehydrateActualFulfillmentSegment 立起，成员都在出发前一小时
// 凭交接入场。
type departureSegmentsDouble struct {
	segments map[string]domain.ActualFulfillmentSegment
	findErr  error
	reads    int
}

func departureSegments(t *testing.T, segment string, objects ...string) *departureSegmentsDouble {
	t.Helper()
	double := &departureSegmentsDouble{segments: map[string]domain.ActualFulfillmentSegment{}}
	if segment == "" {
		return double
	}
	participations := make([]domain.RehydrateParticipationSpec, 0, len(objects))
	for _, object := range objects {
		participations = append(participations, domain.RehydrateParticipationSpec{
			Object:     mustRefValue(t, domain.NewCarriedObjectReference, object),
			EntryKind:  domain.EnteredByTransportHandover,
			EntryBasis: mustRefValue(t, domain.NewParticipationBasisReference, "HO/"+object),
			EnteredAt:  movedAtFixture.Add(-time.Hour),
		})
	}
	built, err := domain.RehydrateActualFulfillmentSegment(domain.RehydrateActualFulfillmentSegmentSpec{
		TenantID:       mustRefValue(t, domain.NewTenantID, "tenant-1"),
		Segment:        mustRefValue(t, domain.NewFulfillmentSegmentReference, segment),
		Participations: participations,
	})
	if err != nil {
		t.Fatalf("立段：%v", err)
	}
	double.segments[segment] = built
	return double
}

func (double *departureSegmentsDouble) FindByKey(_ context.Context, key ports.FulfillmentSegmentKey) (ports.FulfillmentSegmentRecord, bool, error) {
	double.reads++
	if double.findErr != nil {
		return ports.FulfillmentSegmentRecord{}, false, double.findErr
	}
	segment, found := double.segments[key.Segment.String()]
	if !found || key.TenantID.String() != "tenant-1" {
		return ports.FulfillmentSegmentRecord{}, false, nil
	}
	return ports.FulfillmentSegmentRecord{Key: key, Segment: segment}, true, nil
}

// departureSchedulesDouble 交回一个方向为 NODE-SHA → NODE-SZX 的临时班次。
type departureSchedulesDouble struct {
	schedule   string
	registered bool
	findErr    error
}

func (double *departureSchedulesDouble) FindByKey(_ context.Context, key ports.ScheduleKey) (ports.ScheduleRecord, bool, error) {
	if double.findErr != nil {
		return ports.ScheduleRecord{}, false, double.findErr
	}
	if !double.registered || key.Schedule.String() != double.schedule {
		return ports.ScheduleRecord{}, false, nil
	}
	direction, err := domain.NewScheduleDirection("CONN-SHA-SZX", "NODE-SHA", "NODE-SZX")
	if err != nil {
		return ports.ScheduleRecord{}, false, err
	}
	scope, err := domain.NewScheduleServiceScopeReference("SCOPE-1")
	if err != nil {
		return ports.ScheduleRecord{}, false, err
	}
	schedule, err := domain.FormTransportSchedule(domain.TransportScheduleSpec{
		TenantID:     key.TenantID,
		Schedule:     key.Schedule,
		Direction:    direction,
		DepartsAt:    movedAtFixture,
		ArrivesAt:    movedAtFixture.Add(10 * time.Hour),
		ServiceScope: scope,
	})
	if err != nil {
		return ports.ScheduleRecord{}, false, err
	}
	return ports.ScheduleRecord{Key: key, Schedule: schedule}, true, nil
}

// departureCustomsDouble 按对象交回预设的那一格；errors 里的对象答调用失败，untranslatable 里的对象答译不回来。queries
// 记下每一问。
type departureCustomsDouble struct {
	answers        map[string]domain.DepartureGateAnswer
	errors         map[string]bool
	untranslatable map[string]bool
	wrong          bool
	queries        []ports.DepartureGateQuery
}

func (double *departureCustomsDouble) ReadDepartureGate(_ context.Context, query ports.DepartureGateQuery) (domain.DepartureGateFinding, error) {
	double.queries = append(double.queries, query)
	if double.errors[query.Object.String()] {
		return domain.DepartureGateFinding{}, departureGateDown
	}
	if double.untranslatable[query.Object.String()] {
		return domain.DepartureGateFinding{}, fmt.Errorf("adapter: %w", ports.ErrDepartureGateAnswerUntranslatable)
	}
	if double.wrong {
		return domain.DepartureGateFinding{}, nil
	}
	answer, found := double.answers[query.Object.String()]
	if !found {
		answer = domain.DepartureGateNotGuarded
	}
	judgement, basis := "AADJ-"+query.Object.String(), domain.DepartureGateBasis{Action: "LOADING_DEPARTURE", Cause: "OUTSIDE_CUSTOMS_BOOKS"}
	if answer == domain.DepartureGateUnreadable {
		judgement, basis = "", domain.DepartureGateBasis{Cause: "RELEASE_OUTCOME_VIEW_UNAVAILABLE"}
	}
	return domain.NewDepartureGateFinding(query.Object, answer, judgement, []domain.DepartureGateBasis{basis})
}

type departureGateFixture struct {
	segments  *departureSegmentsDouble
	schedules *departureSchedulesDouble
	customs   *departureCustomsDouble
}

func newDepartureGateFixture(t *testing.T, objects ...string) *departureGateFixture {
	t.Helper()
	return &departureGateFixture{
		segments:  departureSegments(t, "segment-1", objects...),
		schedules: &departureSchedulesDouble{schedule: "schedule-1", registered: true},
		customs: &departureCustomsDouble{
			answers: map[string]domain.DepartureGateAnswer{}, errors: map[string]bool{}, untranslatable: map[string]bool{},
		},
	}
}

func (fixture *departureGateFixture) reader(t *testing.T) *application.DepartureGateReader {
	t.Helper()
	reader, err := application.NewDepartureGateReader(application.DepartureGateReadDeps{
		Segments:  fixture.segments,
		Schedules: fixture.schedules,
		Customs:   fixture.customs,
	})
	if err != nil {
		t.Fatalf("装读数编排：%v", err)
	}
	return reader
}

func departureCommand(t *testing.T) application.RecordMovementFactCommand {
	t.Helper()
	command := recordMovementCommand(t)
	command.Kind = domain.DepartureFact
	command.Segment = "segment-1"
	return command
}

func stopWords(reading domain.DepartureGateReading) []string {
	var words []string
	for _, stop := range reading.Stops() {
		words = append(words, stop.String())
	}
	return words
}

// Covers: AT-TF-110 编排半边——段里出发那一刻在控的每一件都问关务，起讫取班次方向、时点取出发的业务时间；整格取最重。
// AT-TF-113：设备自报 gateRequired=false 照样问——设门与否由关务按对象与起讫判，不由设备说。
func TestADepartureAsksCustomsForEveryObjectUnderControlWithTheScheduleDirection(t *testing.T) {
	fixture := newDepartureGateFixture(t, "parcel-2", "parcel-1")
	fixture.customs.answers["parcel-2"] = domain.DepartureGateBlocked
	command := departureCommand(t)
	command.GateRequired = false

	reading, err := fixture.reader(t).Read(t.Context(), command)
	if err != nil {
		t.Fatalf("读数：%v", err)
	}
	if reading.Answer() != domain.DepartureGateBlocked {
		t.Fatalf("整格 = %s, want BLOCKED", reading.Answer())
	}
	if len(fixture.customs.queries) != 2 {
		t.Fatalf("问了关务 %d 次, want 2（gateRequired=false 照样问）", len(fixture.customs.queries))
	}
	for _, query := range fixture.customs.queries {
		if query.From.String() != "NODE-SHA" || query.To.String() != "NODE-SZX" || !query.At.Equal(movedAtFixture) ||
			query.TenantID.String() != "tenant-1" {
			t.Fatalf("问法 = %+v，起讫要取班次方向、时点要取出发的业务时间", query)
		}
	}
	if from, to, directed := reading.Direction(); !directed || from.String() != "NODE-SHA" || to.String() != "NODE-SZX" {
		t.Fatalf("读数没记下问关务时的起讫：%s → %s directed=%v", from, to, directed)
	}
}

// Covers: AT-TF-112 编排半边——关务那一问调用失败时，那一件记读不动（依据指名关务那一口），其余照问，读数照样形成。
// 答复译不回来的那一件同样记读不动，依据种类与调用失败分开：一个过一会儿可能就好，一个重问也是同一个结果。
func TestACustomsFailureIsRecordedAsUnreadableForThatObjectOnly(t *testing.T) {
	fixture := newDepartureGateFixture(t, "parcel-1", "parcel-2", "parcel-3")
	fixture.customs.errors["parcel-1"] = true
	fixture.customs.untranslatable["parcel-3"] = true

	reading, err := fixture.reader(t).Read(t.Context(), departureCommand(t))
	if err != nil {
		t.Fatalf("关务读不动不该上抛：%v", err)
	}
	if reading.Answer() != domain.DepartureGateUnreadable {
		t.Fatalf("整格 = %s, want UNREADABLE", reading.Answer())
	}
	findings := reading.Findings()
	if len(findings) != 3 || findings[0].Answer() != domain.DepartureGateUnreadable ||
		findings[1].Answer() != domain.DepartureGateNotGuarded || findings[2].Answer() != domain.DepartureGateUnreadable {
		t.Fatalf("逐对象 = %+v，读不动只落在没问出答复的那几件上", findings)
	}
	if bases := findings[0].Bases(); len(bases) != 1 || bases[0].Cause != "CUSTOMS_ADMISSION_UNAVAILABLE" {
		t.Fatalf("调用失败的依据 = %+v，要指名关务那一口", bases)
	}
	if bases := findings[2].Bases(); len(bases) != 1 || bases[0].Cause != "CUSTOMS_ADMISSION_UNTRANSLATABLE" {
		t.Fatalf("译不回来的依据 = %+v，要与调用失败分开", bases)
	}
}

// Covers: AT-TF-114 编排半边——本上下文这一侧找不齐问关务要的东西就停下、不问关务，停点说缺什么或哪一口读不动；
// 段与班次各停各的，班次读到了就记下方向。
func TestADepartureStopsBeforeCustomsWhenItCannotSayWhatOrWhere(t *testing.T) {
	cases := map[string]struct {
		arrange func(*departureGateFixture, *application.RecordMovementFactCommand)
		stops   []string
		answer  domain.DepartureGateAnswer
		direct  bool
	}{
		"没声明段": {
			arrange: func(_ *departureGateFixture, command *application.RecordMovementFactCommand) { command.Segment = "" },
			stops:   []string{"SEGMENT_NOT_DECLARED"}, answer: domain.DepartureGateUndecided, direct: true,
		},
		"段不在册": {
			arrange: func(_ *departureGateFixture, command *application.RecordMovementFactCommand) {
				command.Segment = "segment-9"
			},
			stops: []string{"SEGMENT_NOT_REGISTERED"}, answer: domain.DepartureGateUndecided, direct: true,
		},
		"段册读不动": {
			arrange: func(fixture *departureGateFixture, _ *application.RecordMovementFactCommand) {
				fixture.segments.findErr = departureGateDown
			},
			stops: []string{"SEGMENT_REGISTRY_UNAVAILABLE"}, answer: domain.DepartureGateUnreadable, direct: true,
		},
		"出发那一刻段里没有在控的对象": {
			arrange: func(_ *departureGateFixture, command *application.RecordMovementFactCommand) {
				command.OccurredAt = movedAtFixture.Add(-2 * time.Hour)
			},
			stops: []string{"NO_OBJECT_UNDER_CONTROL"}, answer: domain.DepartureGateUndecided, direct: true,
		},
		"班次不在册": {
			arrange: func(fixture *departureGateFixture, _ *application.RecordMovementFactCommand) {
				fixture.schedules.registered = false
			},
			stops: []string{"SCHEDULE_NOT_REGISTERED"}, answer: domain.DepartureGateUndecided,
		},
		"班次册读不动且段不在册": {
			arrange: func(fixture *departureGateFixture, command *application.RecordMovementFactCommand) {
				fixture.schedules.findErr = departureGateDown
				command.Segment = "segment-9"
			},
			stops: []string{"SEGMENT_NOT_REGISTERED", "SCHEDULE_REGISTRY_UNAVAILABLE"}, answer: domain.DepartureGateUnreadable,
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newDepartureGateFixture(t, "parcel-1")
			command := departureCommand(t)
			testCase.arrange(fixture, &command)

			reading, err := fixture.reader(t).Read(t.Context(), command)
			if err != nil {
				t.Fatalf("读数：%v", err)
			}
			if reading.Answer() != testCase.answer || !slices.Equal(stopWords(reading), testCase.stops) {
				t.Fatalf("读数 = %s %v, want %s %v", reading.Answer(), stopWords(reading), testCase.answer, testCase.stops)
			}
			if len(fixture.customs.queries) != 0 {
				t.Fatalf("停下了还问了关务 %d 次", len(fixture.customs.queries))
			}
			if _, _, directed := reading.Direction(); directed != testCase.direct {
				t.Fatalf("起讫 directed = %v, want %v", directed, testCase.direct)
			}
		})
	}
}

// 移动、到达与受理不了的命令不问任何一口，交回零值读数：门禁只约束装载出发，受理不了的那一条由登记编排答未受理。
func TestOnlyAcceptableDeparturesAreRead(t *testing.T) {
	cases := map[string]func(*application.RecordMovementFactCommand){
		"到达":      func(command *application.RecordMovementFactCommand) { command.Kind = domain.ArrivalFact },
		"途中":      func(command *application.RecordMovementFactCommand) { command.Kind = domain.InTransitFact },
		"缺地点的出发":  func(command *application.RecordMovementFactCommand) { command.Location = "" },
		"零点发生的出发": func(command *application.RecordMovementFactCommand) { command.OccurredAt = time.Time{} },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newDepartureGateFixture(t, "parcel-1")
			command := departureCommand(t)
			change(&command)

			reading, err := fixture.reader(t).Read(t.Context(), command)
			if err != nil {
				t.Fatalf("读数：%v", err)
			}
			if reading.Answer() != domain.DepartureGateAnswerInvalid {
				t.Fatalf("不该读却读出了 %s", reading.Answer())
			}
			if fixture.segments.reads != 0 || len(fixture.customs.queries) != 0 {
				t.Fatalf("不该读却问了：段册 %d 次、关务 %d 次", fixture.segments.reads, len(fixture.customs.queries))
			}
		})
	}
}

// 端口答非所问（答了另一件、或交回零值却不报错）是实现坏了，上抛而不记成读不动——记成一格会把它藏起来。缺口的构造门在
// 启动时拒。
func TestAMisansweringCustomsViewIsAnErrorAndMissingDependenciesAreRefused(t *testing.T) {
	fixture := newDepartureGateFixture(t, "parcel-1")
	fixture.customs.wrong = true
	if _, err := fixture.reader(t).Read(t.Context(), departureCommand(t)); err == nil {
		t.Fatal("答非所问被记成了一格")
	}
	if _, err := application.NewDepartureGateReader(application.DepartureGateReadDeps{
		Segments: fixture.segments, Schedules: fixture.schedules,
	}); err == nil {
		t.Fatal("缺关务那一口也装得起来")
	}
}
