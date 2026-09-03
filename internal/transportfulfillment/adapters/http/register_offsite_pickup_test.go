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

type pickupRegistryDouble struct {
	records map[string]ports.OffsitePickupRecord
	findErr error
}

func newPickupRegistry() *pickupRegistryDouble {
	return &pickupRegistryDouble{records: map[string]ports.OffsitePickupRecord{}}
}

func pickupRegistryKey(key ports.OffsitePickupKey) string {
	return key.TenantID.String() + "|" + key.Object.String() + "|" + key.Attempt.String()
}

func (double *pickupRegistryDouble) FindByKey(
	_ context.Context,
	key ports.OffsitePickupKey,
) (ports.OffsitePickupRecord, bool, error) {
	if double.findErr != nil {
		return ports.OffsitePickupRecord{}, false, double.findErr
	}
	record, found := double.records[pickupRegistryKey(key)]
	return record, found, nil
}

func (double *pickupRegistryDouble) Save(
	_ context.Context,
	record ports.OffsitePickupRecord,
) (ports.OffsitePickupSaveOutcome, error) {
	if _, exists := double.records[pickupRegistryKey(record.Key)]; exists {
		return ports.OffsitePickupAlreadyRegistered, nil
	}
	double.records[pickupRegistryKey(record.Key)] = record
	return ports.OffsitePickupSaved, nil
}

type pickupVersionFactory struct {
	minted int
	err    error
}

func (double *pickupVersionFactory) NextPickupResultVersion(context.Context) (domain.PickupResultVersion, error) {
	if double.err != nil {
		return domain.PickupResultVersion{}, double.err
	}
	double.minted++
	return domain.NewPickupResultVersion(fmt.Sprintf("pickup-result/v%d", double.minted))
}

type pickupRegistrationHandoffDouble struct{}

func (pickupRegistrationHandoffDouble) HandOffOffsitePickupRegistration(
	context.Context,
	ports.OffsitePickupRegistrationIntent,
) error {
	return nil
}

// ---- 请求体与 intake 替身 ----

type pickupRegistrationBody struct {
	Object         string `json:"object"`
	Task           string `json:"task"`
	Attempt        string `json:"attempt"`
	Place          string `json:"place"`
	Control        string `json:"control"`
	ExecutedBy     string `json:"executedBy"`
	OccurredAt     string `json:"occurredAt"`
	Segment        string `json:"segment"`
	PlannedSegment string `json:"plannedSegment"`
}

type pickupRegistrationIntakeDouble struct {
	tenant domain.TenantID
	err    error
}

func (intake *pickupRegistrationIntakeDouble) IntakePickupRegistration(
	_ context.Context,
	request *http.Request,
) (application.RegisterOffsitePickupCommand, error) {
	if intake.err != nil {
		return application.RegisterOffsitePickupCommand{}, intake.err
	}
	var body pickupRegistrationBody
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		return application.RegisterOffsitePickupCommand{}, fmt.Errorf("%w: %v", tfhttp.ErrMalformedRequest, err)
	}
	occurredAt, err := time.Parse(time.RFC3339, body.OccurredAt)
	if err != nil {
		return application.RegisterOffsitePickupCommand{}, fmt.Errorf("%w: %v", tfhttp.ErrMalformedRequest, err)
	}
	return application.RegisterOffsitePickupCommand{
		TenantID:       intake.tenant,
		Object:         body.Object,
		Task:           body.Task,
		Attempt:        body.Attempt,
		Place:          body.Place,
		Control:        body.Control,
		ExecutedBy:     body.ExecutedBy,
		OccurredAt:     occurredAt,
		Segment:        body.Segment,
		PlannedSegment: body.PlannedSegment,
	}, nil
}

type pickupRegistrationFixture struct {
	intake   *pickupRegistrationIntakeDouble
	registry *pickupRegistryDouble
	versions *pickupVersionFactory
	segments *segmentRegistryDouble
	register http.Handler
}

func newPickupRegistrationFixture(t *testing.T) *pickupRegistrationFixture {
	t.Helper()
	fixture := &pickupRegistrationFixture{
		intake:   &pickupRegistrationIntakeDouble{tenant: httpValue(t, domain.NewTenantID, "tenant-1")},
		registry: newPickupRegistry(),
		versions: &pickupVersionFactory{},
		segments: newSegmentRegistry(),
	}
	handler := application.NewRegisterOffsitePickupHandler(application.RegisterOffsitePickupDeps{
		Pickups:    fixture.registry,
		Segments:   fixture.segments,
		Versions:   fixture.versions,
		Downstream: pickupRegistrationHandoffDouble{},
		Clock:      tfClock{at: time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)},
	})
	fixture.register = tfhttp.NewRegisterOffsitePickupEndpoint(fixture.intake, handler)
	return fixture
}

func defaultPickupRegistrationBody() pickupRegistrationBody {
	return pickupRegistrationBody{
		Object:     "parcel-1",
		Task:       "pickup-task-1",
		Attempt:    "attempt-1",
		Place:      "door-1",
		Control:    "control-1",
		ExecutedBy: "courier-1",
		OccurredAt: "2026-09-05T10:00:00Z",
	}
}

type pickupRegistrationView struct {
	Outcome                      string `json:"outcome"`
	UndecidedReason              string `json:"undecidedReason"`
	PickupVersion                string `json:"pickupVersion"`
	Object                       string `json:"object"`
	Task                         string `json:"task"`
	Attempt                      string `json:"attempt"`
	ContinuationReference        string `json:"continuationReference"`
	HandoffReference             string `json:"handoffReference"`
	SegmentContinuationReference string `json:"segmentContinuationReference"`
	SegmentEntryRefusal          string `json:"segmentEntryRefusal"`
}

func decodePickupRegistration(t *testing.T, recorder *httptest.ResponseRecorder) pickupRegistrationView {
	t.Helper()
	var view pickupRegistrationView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return view
}

// Covers: 票 04 专钉第 1 条在揽收登记口——Intake 给的 `segment`/`plannedSegment` 让对象真的进了段；
// ADR-0022 首登新落一版 201，版本由编排签发后透出。
func TestRegisterOffsitePickupReportsCreatedAndEntersTheNamedSegment(t *testing.T) {
	fixture := newPickupRegistrationFixture(t)
	body := defaultPickupRegistrationBody()
	body.Segment = "segment-1"
	body.PlannedSegment = "planned-1"

	response := postTo(t, fixture.register, "/transport-fulfillment/offsite-pickups", body)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", response.Code, response.Body.String())
	}
	view := decodePickupRegistration(t, response)
	if view.Outcome != "PICKUP_REGISTERED" || view.PickupVersion != "pickup-result/v1" {
		t.Fatalf("view = %+v", view)
	}
	if view.Object != "parcel-1" || view.Task != "pickup-task-1" || view.Attempt != "attempt-1" {
		t.Fatalf("view = %+v", view)
	}
	if view.SegmentContinuationReference != "" {
		t.Fatalf("segmentContinuationReference = %q，进段成功不该有欠账", view.SegmentContinuationReference)
	}

	participation := fixture.segments.participationOf(t, "tenant-1", "segment-1", "parcel-1")
	planned, present := participation.PlannedSegment()
	if !present || planned.String() != "planned-1" {
		t.Fatalf("plannedSegment = (%q, %v)，Intake 给的计划段没到编排", planned.String(), present)
	}
}

// Covers: ADR-0022——重放、冲突、未受理、两种未决都是 200 的形成答案；`未决`区分是登记册还是
// 版本签发没答上（`undecidedReason`），并带 `continuationReference`。
func TestRegisterOffsitePickupReportsOKForEveryAnswerThatRegisteredNothing(t *testing.T) {
	cases := map[string]struct {
		arrange    func(*pickupRegistrationFixture) pickupRegistrationBody
		posts      int
		want       string
		wantReason string
	}{
		"replay returns the existing version": {
			arrange: func(*pickupRegistrationFixture) pickupRegistrationBody { return defaultPickupRegistrationBody() },
			posts:   2,
			want:    "EXISTING_VERSION",
		},
		"a different control under the same object and attempt is a conflict": {
			arrange: func(fixture *pickupRegistrationFixture) pickupRegistrationBody {
				if seed := postTo(t, fixture.register, "/transport-fulfillment/offsite-pickups", defaultPickupRegistrationBody()); seed.Code != http.StatusCreated {
					t.Fatalf("seed status = %d", seed.Code)
				}
				body := defaultPickupRegistrationBody()
				body.Control = "control-2"
				return body
			},
			posts: 1,
			want:  "SOURCE_CONFLICT",
		},
		"a visit without control evidence is not accepted": {
			arrange: func(*pickupRegistrationFixture) pickupRegistrationBody {
				body := defaultPickupRegistrationBody()
				body.Control = ""
				return body
			},
			posts: 1,
			want:  "SOURCE_NOT_ACCEPTED",
		},
		"a registry failure is undecided": {
			arrange: func(fixture *pickupRegistrationFixture) pickupRegistrationBody {
				fixture.registry.findErr = errors.New("registry down")
				return defaultPickupRegistrationBody()
			},
			posts:      1,
			want:       "PICKUP_UNDECIDED",
			wantReason: "PICKUP_REGISTRY_UNAVAILABLE",
		},
		"a version factory failure is undecided": {
			arrange: func(fixture *pickupRegistrationFixture) pickupRegistrationBody {
				fixture.versions.err = errors.New("sequence down")
				return defaultPickupRegistrationBody()
			},
			posts:      1,
			want:       "PICKUP_UNDECIDED",
			wantReason: "PICKUP_VERSION_UNAVAILABLE",
		},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newPickupRegistrationFixture(t)
			body := testCase.arrange(fixture)
			var response *httptest.ResponseRecorder
			for post := 0; post < testCase.posts; post++ {
				response = postTo(t, fixture.register, "/transport-fulfillment/offsite-pickups", body)
			}
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body.String())
			}
			view := decodePickupRegistration(t, response)
			if view.Outcome != testCase.want || view.UndecidedReason != testCase.wantReason {
				t.Fatalf("view = %+v, want outcome %q reason %q", view, testCase.want, testCase.wantReason)
			}
			if testCase.wantReason != "" && view.ContinuationReference == "" {
				t.Fatal("未决没有带 continuationReference")
			}
		})
	}
}

// Covers: 票 04 专钉第 2 条在揽收登记口——段登记册故障时收寄照登（201），`segmentContinuationReference` 非空。
func TestRegisterOffsitePickupExposesTheSegmentDebtWhenTheRegistryIsDown(t *testing.T) {
	fixture := newPickupRegistrationFixture(t)
	fixture.segments.findErr = errors.New("segment registry down")
	body := defaultPickupRegistrationBody()
	body.Segment = "segment-1"

	response := postTo(t, fixture.register, "/transport-fulfillment/offsite-pickups", body)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", response.Code, response.Body.String())
	}
	if view := decodePickupRegistration(t, response); view.SegmentContinuationReference == "" {
		t.Fatal("segmentContinuationReference 为空：进段那一半的欠账被传输层吞了")
	}
}

// 传输层分法只钉与交接口不同形的那一格：编排 error 是 5xx 且不带 outcome。其余分法由共用的
// commandEndpoint 承担，交接口的测试已逐格钉过。
func TestRegisterOffsitePickupOrchestrationErrorFormsNoAnswer(t *testing.T) {
	fixture := newPickupRegistrationFixture(t)
	endpoint := tfhttp.NewRegisterOffsitePickupEndpoint(fixture.intake, failingPickupRegistrationHandler{})

	response := postTo(t, endpoint, "/transport-fulfillment/offsite-pickups", defaultPickupRegistrationBody())

	if response.Code != http.StatusInternalServerError || problemCode(t, response) != "NO_ANSWER_FORMED" {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	assertNoOutcome(t, response)
}

type failingPickupRegistrationHandler struct{}

func (failingPickupRegistrationHandler) Register(
	context.Context,
	application.RegisterOffsitePickupCommand,
) (application.RegisterOffsitePickupResult, error) {
	return application.RegisterOffsitePickupResult{}, errors.New("orchestration exploded")
}
