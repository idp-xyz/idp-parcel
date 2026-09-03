package tfhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
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

// ---- 请求体与 intake 替身 ----

type movementFactBody struct {
	Fact          string `json:"fact"`
	Schedule      string `json:"schedule"`
	Kind          string `json:"kind"`
	Location      string `json:"location"`
	Source        string `json:"source"`
	Version       string `json:"version"`
	OccurredAt    string `json:"occurredAt"`
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
		GateRequired:  body.GateRequired,
		GateClearance: body.GateClearance,
	}, nil
}

type movementFactFixture struct {
	intake   *movementFactIntakeDouble
	registry *movementFactRegistryDouble
	record   http.Handler
}

func newMovementFactFixture(t *testing.T) *movementFactFixture {
	t.Helper()
	fixture := &movementFactFixture{
		intake:   &movementFactIntakeDouble{tenant: httpValue(t, domain.NewTenantID, "tenant-1")},
		registry: newMovementFactRegistry(),
	}
	handler := application.NewRecordMovementFactHandler(application.RecordMovementFactDeps{
		Facts: fixture.registry,
		Clock: tfClock{at: time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)},
	})
	fixture.record = tfhttp.NewRecordMovementFactEndpoint(fixture.intake, handler)
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
	Outcome               string `json:"outcome"`
	Fact                  string `json:"fact"`
	Schedule              string `json:"schedule"`
	Kind                  string `json:"kind"`
	Location              string `json:"location"`
	Version               string `json:"version"`
	OccurredAt            string `json:"occurredAt"`
	GateClearance         string `json:"gateClearance"`
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

// Covers: 票 05 专钉——`gateRequired` / `gateClearance` 原样到编排。判据不是看命令镜像，而是看门禁那道
// 领域门真的被触到：出发要门禁而没带放行是 DEPARTURE_GATE_BLOCKED（200，形成了的答案），带了放行
// 就登上并把放行引用透出。传输层若漏收这两格，前者会变成一条登上了的出发。
func TestRecordMovementFactCarriesTheDepartureGateThroughToTheOrchestration(t *testing.T) {
	fixture := newMovementFactFixture(t)
	departure := defaultMovementFactBody()
	departure.Kind = "DEPARTURE"
	departure.Fact = "movement-fact-dep-1"
	departure.GateRequired = true

	blocked := postTo(t, fixture.record, "/transport-fulfillment/movement-facts", departure)
	if blocked.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200（门禁未放行是形成了的答案）; body = %s", blocked.Code, blocked.Body.String())
	}
	if view := decodeMovementFact(t, blocked); view.Outcome != "DEPARTURE_GATE_BLOCKED" {
		t.Fatalf("outcome = %q, want DEPARTURE_GATE_BLOCKED", view.Outcome)
	}

	departure.GateClearance = "gate-clearance-1"
	cleared := postTo(t, fixture.record, "/transport-fulfillment/movement-facts", departure)
	if cleared.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", cleared.Code, cleared.Body.String())
	}
	if view := decodeMovementFact(t, cleared); view.Outcome != "MOVEMENT_FACT_RECORDED" || view.GateClearance != "gate-clearance-1" {
		t.Fatalf("view = %+v", view)
	}
}

// Covers: ADR-0022——重放、门禁未放行、未受理、未决都是 200 的形成答案，`outcome` 区分；
// `DEPARTURE_GATE_BLOCKED` 与 `INPUT_NOT_ACCEPTED` 两格不合并（续办动作不同）。
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
