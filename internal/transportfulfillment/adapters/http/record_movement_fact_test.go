package tfhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// ---- 应用编排的端口替身（真实 handler，非结果构造）----

type movementFactRegistryDouble struct {
	records map[string]ports.MovementFactRecord
	findErr error
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
	if _, exists := double.records[movementFactRegistryKey(record.Key)]; exists {
		return ports.MovementFactVersionAlreadyRegistered, nil
	}
	double.records[movementFactRegistryKey(record.Key)] = record
	return ports.MovementFactSaved, nil
}

// ---- 出发读数那三口的替身：段 segment-1 里有 parcel-1，班次 schedule-1 自 NODE-SHA 到 NODE-SZX，关务按预设作答 ----

type movementSegmentsDouble struct {
	segment domain.ActualFulfillmentSegment
}

func (double movementSegmentsDouble) FindByKey(_ context.Context, key ports.FulfillmentSegmentKey) (ports.FulfillmentSegmentRecord, bool, error) {
	if key.Segment != double.segment.Segment() {
		return ports.FulfillmentSegmentRecord{}, false, nil
	}
	return ports.FulfillmentSegmentRecord{Key: key, Segment: double.segment}, true, nil
}

type movementSchedulesDouble struct{ schedule domain.TransportSchedule }

func (double movementSchedulesDouble) FindByKey(_ context.Context, key ports.ScheduleKey) (ports.ScheduleRecord, bool, error) {
	if key.Schedule != double.schedule.Schedule() {
		return ports.ScheduleRecord{}, false, nil
	}
	return ports.ScheduleRecord{Key: key, Schedule: double.schedule}, true, nil
}

type movementCustomsDouble struct{ answer domain.DepartureGateAnswer }

func (double *movementCustomsDouble) ReadDepartureGate(_ context.Context, query ports.DepartureGateQuery) (domain.DepartureGateFinding, error) {
	return domain.NewDepartureGateFinding(query.Object, double.answer, "AADJ-1",
		[]domain.DepartureGateBasis{{Action: "LOADING_DEPARTURE", Cause: "SOME_CAUSE", Sources: []string{"SRC-1"}}})
}

// movementHandoffDouble 收下已形成信封的意图、什么也不做：本包只验线格式与答法，信封入队由 postgres 与 cmd 两层的用例证。
type movementHandoffDouble struct{}

func (movementHandoffDouble) HandOffMovementFact(context.Context, ports.MovementFactIntent) error {
	return nil
}

// readThenRecord 照组合根的次序：同内容重放取在册那一份读数，否则先问读数，再登记。这里没有事务——事务边界归 cmd 的装配点，本包只验线格式与答法。
type readThenRecord struct {
	gate  *application.DepartureGateReader
	inner *application.RecordMovementFactHandler
}

func (composed readThenRecord) Record(ctx context.Context, command application.RecordMovementFactCommand) (application.RecordMovementFactResult, error) {
	reading, replayed := composed.inner.RecordedDepartureReading(ctx, command)
	if !replayed {
		var err error
		if reading, err = composed.gate.Read(ctx, command); err != nil {
			return application.RecordMovementFactResult{}, err
		}
	}
	return composed.inner.Record(ctx, command, reading)
}

func movementGateReader(t *testing.T, customs *movementCustomsDouble) *application.DepartureGateReader {
	t.Helper()
	tenant := httpValue(t, domain.NewTenantID, "tenant-1")
	segment, err := domain.RehydrateActualFulfillmentSegment(domain.RehydrateActualFulfillmentSegmentSpec{
		TenantID: tenant,
		Segment:  httpValue(t, domain.NewFulfillmentSegmentReference, "segment-1"),
		Participations: []domain.RehydrateParticipationSpec{{
			Object:     httpValue(t, domain.NewCarriedObjectReference, "parcel-1"),
			EntryKind:  domain.EnteredByTransportHandover,
			EntryBasis: httpValue(t, domain.NewParticipationBasisReference, "HO/parcel-1"),
			EnteredAt:  time.Date(2026, 9, 5, 9, 0, 0, 0, time.UTC),
		}},
	})
	if err != nil {
		t.Fatalf("立段：%v", err)
	}
	direction, err := domain.NewScheduleDirection("CONN-SHA-SZX", "NODE-SHA", "NODE-SZX")
	if err != nil {
		t.Fatalf("方向：%v", err)
	}
	schedule, err := domain.FormTransportSchedule(domain.TransportScheduleSpec{
		TenantID:     tenant,
		Schedule:     httpValue(t, domain.NewScheduleReference, "schedule-1"),
		Direction:    direction,
		DepartsAt:    time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC),
		ArrivesAt:    time.Date(2026, 9, 5, 20, 0, 0, 0, time.UTC),
		ServiceScope: httpValue(t, domain.NewScheduleServiceScopeReference, "SCOPE-1"),
	})
	if err != nil {
		t.Fatalf("班次：%v", err)
	}
	reader, err := application.NewDepartureGateReader(application.DepartureGateReadDeps{
		Segments:  movementSegmentsDouble{segment: segment},
		Schedules: movementSchedulesDouble{schedule: schedule},
		Customs:   customs,
	})
	if err != nil {
		t.Fatalf("装读数编排：%v", err)
	}
	return reader
}

// ---- 请求体与 intake 替身 ----

type movementFactBody struct {
	Fact          string `json:"fact"`
	Schedule      string `json:"schedule"`
	Kind          string `json:"kind"`
	Location      string `json:"location"`
	Source        string `json:"source"`
	Version       string `json:"version"`
	OccurredAt    string `json:"occurredAt"`
	Segment       string `json:"segment"`
	Resource      string `json:"resource"`
	GateRequired  bool   `json:"gateRequired"`
	GateClearance string `json:"gateClearance"`
}

func movementFactKindOf(raw string) domain.MovementFactKind {
	switch raw {
	case "DEPARTURE":
		return domain.DepartureFact
	case "IN_TRANSIT":
		return domain.InTransitFact
	case "ARRIVAL":
		return domain.ArrivalFact
	default:
		return domain.MovementFactKindInvalid
	}
}

type movementFactIntakeDouble struct {
	tenant domain.TenantID
	err    error
}

func (intake *movementFactIntakeDouble) IntakeMovementFact(
	_ context.Context,
	request *http.Request,
) (application.RecordMovementFactCommand, error) {
	if intake.err != nil {
		return application.RecordMovementFactCommand{}, intake.err
	}
	var body movementFactBody
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		return application.RecordMovementFactCommand{}, fmt.Errorf("%w: %v", tfhttp.ErrMalformedRequest, err)
	}
	occurredAt, err := time.Parse(time.RFC3339, body.OccurredAt)
	if err != nil {
		return application.RecordMovementFactCommand{}, fmt.Errorf("%w: %v", tfhttp.ErrMalformedRequest, err)
	}
	return application.RecordMovementFactCommand{
		TenantID:      intake.tenant,
		Fact:          body.Fact,
		Schedule:      body.Schedule,
		Kind:          movementFactKindOf(body.Kind),
		Location:      body.Location,
		Source:        body.Source,
		Version:       body.Version,
		OccurredAt:    occurredAt,
		Segment:       body.Segment,
		Resource:      body.Resource,
		GateRequired:  body.GateRequired,
		GateClearance: body.GateClearance,
	}, nil
}

type movementFactFixture struct {
	intake   *movementFactIntakeDouble
	registry *movementFactRegistryDouble
	customs  *movementCustomsDouble
	record   http.Handler
}

func newMovementFactFixture(t *testing.T) *movementFactFixture {
	t.Helper()
	fixture := &movementFactFixture{
		intake:   &movementFactIntakeDouble{tenant: httpValue(t, domain.NewTenantID, "tenant-1")},
		registry: newMovementFactRegistry(),
		customs:  &movementCustomsDouble{answer: domain.DepartureGateNotGuarded},
	}
	handler := application.NewRecordMovementFactHandler(application.RecordMovementFactDeps{
		Facts:   fixture.registry,
		Handoff: movementHandoffDouble{},
		Clock:   tfClock{at: time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)},
	})
	fixture.record = tfhttp.NewRecordMovementFactEndpoint(fixture.intake,
		readThenRecord{gate: movementGateReader(t, fixture.customs), inner: handler})
	return fixture
}

func defaultMovementFactBody() movementFactBody {
	return movementFactBody{
		Fact:       "movement-fact-1",
		Schedule:   "schedule-1",
		Kind:       "ARRIVAL",
		Location:   "hub-1",
		Source:     "own-fleet-scan/v1",
		Version:    "MFV-000000000001",
		OccurredAt: "2026-09-05T10:00:00Z",
	}
}

type movementFactView struct {
	Outcome       string `json:"outcome"`
	Fact          string `json:"fact"`
	Schedule      string `json:"schedule"`
	Kind          string `json:"kind"`
	Location      string `json:"location"`
	Version       string `json:"version"`
	OccurredAt    string `json:"occurredAt"`
	Segment       string `json:"segment"`
	Resource      string `json:"resource"`
	GateClearance string `json:"gateClearance"`
	GateReading   *struct {
		Answer  string           `json:"answer"`
		From    string           `json:"from"`
		To      string           `json:"to"`
		Stops   []string         `json:"stops"`
		Objects []map[string]any `json:"objects"`
	} `json:"gateReading"`
	ContinuationReference string `json:"continuationReference"`
}

func decodeMovementFact(t *testing.T, recorder *httptest.ResponseRecorder) movementFactView {
	t.Helper()
	var view movementFactView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return view
}

// Covers: ADR-0022「2xx 内部只区分有没有新落一版」——一条到达事实首登 201，事实、班次、种类、版本与
// 发生时刻原样来自请求体（ADR-0023：事实内容从体收，服务器不代铸）。
func TestRecordMovementFactReportsCreatedWithTheReportedFact(t *testing.T) {
	fixture := newMovementFactFixture(t)

	response := postTo(t, fixture.record, "/transport-fulfillment/movement-facts", defaultMovementFactBody())

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", response.Code, response.Body.String())
	}
	view := decodeMovementFact(t, response)
	if view.Outcome != "MOVEMENT_FACT_RECORDED" {
		t.Fatalf("outcome = %q", view.Outcome)
	}
	if view.Fact != "movement-fact-1" || view.Schedule != "schedule-1" || view.Kind != "ARRIVAL" ||
		view.Location != "hub-1" || view.Version != "MFV-000000000001" || view.OccurredAt != "2026-09-05T10:00:00Z" {
		t.Fatalf("view = %+v", view)
	}
}

// Covers: AT-TF-111 线格式半边——出发一律 201：关务那一格答阻断也照样落册，响应带出读数（整格、问关务时的起讫、逐对象的那一格）
// 以及声明的段与资源。设备自报 gateRequired=true 而不带凭据也照记：设门与否不由设备说（ADR-0205 决定三）。
// AT-TF-063：逐对象那一格只有对象与答复，关务判断标识与依据原文不回给设备——同段可能有别的客户的包裹。
func TestRecordMovementFactAnswersCreatedForEveryDepartureAndShowsItsGateReading(t *testing.T) {
	fixture := newMovementFactFixture(t)
	fixture.customs.answer = domain.DepartureGateBlocked
	departure := defaultMovementFactBody()
	departure.Kind = "DEPARTURE"
	departure.Fact = "movement-fact-dep-1"
	departure.Segment = "segment-1"
	departure.Resource = "truck-1"
	departure.GateRequired = true

	response := postTo(t, fixture.record, "/transport-fulfillment/movement-facts", departure)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201（读数阻断也落册）; body = %s", response.Code, response.Body.String())
	}
	view := decodeMovementFact(t, response)
	if view.Outcome != "MOVEMENT_FACT_RECORDED" || view.Segment != "segment-1" || view.Resource != "truck-1" {
		t.Fatalf("view = %+v", view)
	}
	reading := view.GateReading
	if reading == nil || reading.Answer != "BLOCKED" || reading.From != "NODE-SHA" || reading.To != "NODE-SZX" || len(reading.Stops) != 0 {
		t.Fatalf("gateReading = %+v", reading)
	}
	if len(reading.Objects) != 1 || reading.Objects[0]["object"] != "parcel-1" || reading.Objects[0]["answer"] != "BLOCKED" ||
		len(reading.Objects[0]) != 2 {
		t.Fatalf("逐对象那一格只该有对象与答复：%+v", reading.Objects)
	}
	if strings.Contains(response.Body.String(), "AADJ-1") || strings.Contains(response.Body.String(), "SRC-1") {
		t.Fatalf("响应带出了关务判断标识或依据来源：%s", response.Body.String())
	}
}

// Covers: AT-TF-114 线格式半边——没声明段的出发照记：读数停在「段未声明」答未决，没有逐对象那一格；到达不带读数。
func TestRecordMovementFactShowsTheStopWhenNoSegmentIsDeclared(t *testing.T) {
	fixture := newMovementFactFixture(t)
	departure := defaultMovementFactBody()
	departure.Kind = "DEPARTURE"
	departure.GateClearance = "gate-clearance-1"

	response := postTo(t, fixture.record, "/transport-fulfillment/movement-facts", departure)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", response.Code, response.Body.String())
	}
	view := decodeMovementFact(t, response)
	if view.GateClearance != "gate-clearance-1" || view.GateReading == nil || view.GateReading.Answer != "UNDECIDED" ||
		len(view.GateReading.Stops) != 1 || view.GateReading.Stops[0] != "SEGMENT_NOT_DECLARED" || len(view.GateReading.Objects) != 0 {
		t.Fatalf("view = %+v reading = %+v", view, view.GateReading)
	}

	arrival := postTo(t, fixture.record, "/transport-fulfillment/movement-facts", defaultMovementFactBody())
	if view := decodeMovementFact(t, arrival); view.GateReading != nil {
		t.Fatalf("到达带了读数：%+v", view.GateReading)
	}
}

// Covers: MovementFactPayload 的线格式——声明的段与资源逐字到命令，gateRequired 照收（字段暂留，编排不读）。
func TestMovementFactPayloadCarriesTheDeclaredSegmentAndResource(t *testing.T) {
	command, err := tfhttp.MovementFactPayload{
		Fact: "f", Schedule: "s", Kind: "DEPARTURE", Location: "l", Version: "1", OccurredAt: "2026-09-05T10:00:00Z",
		Segment: "segment-1", Resource: "truck-1", GateRequired: true, GateClearance: "g",
	}.Command(httpValue(t, domain.NewTenantID, "tenant-1"), "SELF_OPERATED")
	if err != nil {
		t.Fatalf("command: %v", err)
	}
	if command.Segment != "segment-1" || command.Resource != "truck-1" || !command.GateRequired || command.GateClearance != "g" ||
		command.Source != "SELF_OPERATED" {
		t.Fatalf("command = %+v", command)
	}
}

// Covers: ADR-0022——重放、未受理、未决都是 200 的形成答案，`outcome` 区分。
func TestRecordMovementFactReportsOKForEveryAnswerThatRecordedNothing(t *testing.T) {
	cases := map[string]struct {
		arrange          func(*movementFactFixture) movementFactBody
		posts            int
		want             string
		wantContinuation bool
	}{
		"replay returns the existing version": {
			arrange: func(*movementFactFixture) movementFactBody { return defaultMovementFactBody() },
			posts:   2,
			want:    "MOVEMENT_FACT_VERSION_EXISTS",
		},
		"a blank location is not accepted": {
			arrange: func(*movementFactFixture) movementFactBody {
				body := defaultMovementFactBody()
				body.Location = "   "
				return body
			},
			posts: 1,
			want:  "INPUT_NOT_ACCEPTED",
		},
		"a registry failure is undecided with a continuation": {
			arrange: func(fixture *movementFactFixture) movementFactBody {
				fixture.registry.findErr = errors.New("registry down")
				return defaultMovementFactBody()
			},
			posts:            1,
			want:             "MOVEMENT_FACT_UNDECIDED",
			wantContinuation: true,
		},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newMovementFactFixture(t)
			body := testCase.arrange(fixture)
			var response *httptest.ResponseRecorder
			for post := 0; post < testCase.posts; post++ {
				response = postTo(t, fixture.record, "/transport-fulfillment/movement-facts", body)
			}
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body.String())
			}
			view := decodeMovementFact(t, response)
			if view.Outcome != testCase.want {
				t.Fatalf("outcome = %q, want %q", view.Outcome, testCase.want)
			}
			if testCase.wantContinuation && view.ContinuationReference == "" {
				t.Fatal("未决没有带 continuationReference")
			}
		})
	}
}

// Covers: 票 tf-face-algebra/08——同身份异内容是 200 的形成答案 SOURCE_CONFLICT，响应只有 outcome：在册那一份不回显，
// 另一台设备或另一作业范围就读不到这条事实的内容。
func TestRecordMovementFactAnswersASourceConflictWithoutTheRecord(t *testing.T) {
	fixture := newMovementFactFixture(t)
	if first := postTo(t, fixture.record, "/transport-fulfillment/movement-facts", defaultMovementFactBody()); first.Code != http.StatusCreated {
		t.Fatalf("首登 status = %d; body = %s", first.Code, first.Body.String())
	}
	different := defaultMovementFactBody()
	different.Location = "hub-9"

	response := postTo(t, fixture.record, "/transport-fulfillment/movement-facts", different)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body) != 1 || body["outcome"] != "SOURCE_CONFLICT" {
		t.Fatalf("body = %s, want 只有 outcome=SOURCE_CONFLICT", response.Body.String())
	}
}

func TestRecordMovementFactOrchestrationErrorFormsNoAnswer(t *testing.T) {
	fixture := newMovementFactFixture(t)
	endpoint := tfhttp.NewRecordMovementFactEndpoint(fixture.intake, failingMovementFactHandler{})

	response := postTo(t, endpoint, "/transport-fulfillment/movement-facts", defaultMovementFactBody())

	if response.Code != http.StatusInternalServerError || problemCode(t, response) != "NO_ANSWER_FORMED" {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	assertNoOutcome(t, response)
}

// Covers: ADR-0055 在移动事实口——未配置 Intake 403、不读体、不构造命令。它不与控制事实那组同表，
// 因为它不是控制事实；堵法一样。
func TestUnconfiguredMovementFactIntakeRefusesWithoutReadingTheBody(t *testing.T) {
	endpoint := tfhttp.NewRecordMovementFactEndpoint(tfhttp.UnconfiguredIntake{}, unreachableMovementFactHandler{t: t})
	probe := &readProbe{}
	request := httptest.NewRequest(http.MethodPost, "/transport-fulfillment/movement-facts", probe)
	response := httptest.NewRecorder()

	endpoint.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden || problemCode(t, response) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	assertNoOutcome(t, response)
	if probe.read {
		t.Fatal("an unconfigured intake read the business content")
	}
}

type failingMovementFactHandler struct{}

func (failingMovementFactHandler) Record(
	context.Context,
	application.RecordMovementFactCommand,
) (application.RecordMovementFactResult, error) {
	return application.RecordMovementFactResult{}, errors.New("orchestration exploded")
}

type unreachableMovementFactHandler struct{ t *testing.T }

func (handler unreachableMovementFactHandler) Record(
	context.Context,
	application.RecordMovementFactCommand,
) (application.RecordMovementFactResult, error) {
	handler.t.Fatal("a request passed the unconfigured intake and reached the orchestration")
	return application.RecordMovementFactResult{}, nil
}
