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
)

// ---- 请求体与 intake 替身 ----

type pickupCorrectionBody struct {
	Object             string `json:"object"`
	Attempt            string `json:"attempt"`
	PredecessorVersion string `json:"predecessorVersion"`
	Place              string `json:"place"`
	Control            string `json:"control"`
	ExecutedBy         string `json:"executedBy"`
	OccurredAt         string `json:"occurredAt"`
	CorrectedAt        string `json:"correctedAt"`
}

type pickupCorrectionIntakeDouble struct {
	tenant domain.TenantID
	err    error
}

func (intake *pickupCorrectionIntakeDouble) IntakePickupCorrection(
	_ context.Context,
	request *http.Request,
) (application.CorrectOffsitePickupCommand, error) {
	if intake.err != nil {
		return application.CorrectOffsitePickupCommand{}, intake.err
	}
	var body pickupCorrectionBody
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		return application.CorrectOffsitePickupCommand{}, fmt.Errorf("%w: %v", tfhttp.ErrMalformedRequest, err)
	}
	occurredAt, err := time.Parse(time.RFC3339, body.OccurredAt)
	if err != nil {
		return application.CorrectOffsitePickupCommand{}, fmt.Errorf("%w: %v", tfhttp.ErrMalformedRequest, err)
	}
	correctedAt, err := time.Parse(time.RFC3339, body.CorrectedAt)
	if err != nil {
		return application.CorrectOffsitePickupCommand{}, fmt.Errorf("%w: %v", tfhttp.ErrMalformedRequest, err)
	}
	return application.CorrectOffsitePickupCommand{
		TenantID:           intake.tenant,
		Object:             body.Object,
		Attempt:            body.Attempt,
		PredecessorVersion: body.PredecessorVersion,
		Place:              body.Place,
		Control:            body.Control,
		ExecutedBy:         body.ExecutedBy,
		OccurredAt:         occurredAt,
		CorrectedAt:        correctedAt,
	}, nil
}

// pickupCorrectionFixture 挂在揽收登记夹具上：更正口与首登口共用同一个应用编排与登记册替身，先登再更正
// 才是真流程——更正不出无中生有的揽收。
type pickupCorrectionFixture struct {
	*pickupRegistrationFixture
	intake  *pickupCorrectionIntakeDouble
	correct http.Handler
}

func newPickupCorrectionFixture(t *testing.T) *pickupCorrectionFixture {
	t.Helper()
	registration := newPickupRegistrationFixture(t)
	fixture := &pickupCorrectionFixture{
		pickupRegistrationFixture: registration,
		intake:                    &pickupCorrectionIntakeDouble{tenant: httpValue(t, domain.NewTenantID, "tenant-1")},
	}
	fixture.correct = tfhttp.NewCorrectOffsitePickupEndpoint(fixture.intake, registration.handler)
	return fixture
}

func (fixture *pickupCorrectionFixture) seedRegistration(t *testing.T) {
	t.Helper()
	if seed := postTo(t, fixture.register, "/transport-fulfillment/offsite-pickups", defaultPickupRegistrationBody()); seed.Code != http.StatusCreated {
		t.Fatalf("seed status = %d body = %s", seed.Code, seed.Body.String())
	}
}

func defaultPickupCorrectionBody() pickupCorrectionBody {
	return pickupCorrectionBody{
		Object:             "parcel-1",
		Attempt:            "attempt-1",
		PredecessorVersion: "pickup-result/v1",
		Place:              "door-2",
		Control:            "control-2",
		ExecutedBy:         "courier-2",
		OccurredAt:         "2026-09-05T08:00:00Z",
		CorrectedAt:        "2026-09-06T09:00:00Z",
	}
}

type pickupCorrectionView struct {
	Outcome                      string `json:"outcome"`
	UndecidedReason              string `json:"undecidedReason"`
	PickupVersion                string `json:"pickupVersion"`
	Object                       string `json:"object"`
	Task                         string `json:"task"`
	Attempt                      string `json:"attempt"`
	Corrects                     string `json:"corrects"`
	ContinuationReference        string `json:"continuationReference"`
	HandoffReference             string `json:"handoffReference"`
	SegmentContinuationReference string `json:"segmentContinuationReference"`
	SegmentEntryRefusal          string `json:"segmentEntryRefusal"`
}

func decodePickupCorrection(t *testing.T, recorder *httptest.ResponseRecorder) pickupCorrectionView {
	t.Helper()
	var view pickupCorrectionView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return view
}

// Covers: 票 tf-segment-lifecycle-closure/08 的端点面——更正落新版本用 201（ADR-0022：首登与更正都持久化了新
// 版本），响应透出新版本号与 `corrects` 回指；对象、任务、尝试沿用被更正版本；更正不进段，段两格为空。
func TestCorrectOffsitePickupReportsCreatedWithTheNewVersionAndItsPredecessor(t *testing.T) {
	fixture := newPickupCorrectionFixture(t)
	fixture.seedRegistration(t)

	response := postTo(t, fixture.correct, "/transport-fulfillment/offsite-pickup-corrections", defaultPickupCorrectionBody())

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", response.Code, response.Body.String())
	}
	view := decodePickupCorrection(t, response)
	if view.Outcome != "PICKUP_CORRECTED" || view.PickupVersion != "pickup-result/v2" || view.Corrects != "pickup-result/v1" {
		t.Fatalf("view = %+v", view)
	}
	if view.Object != "parcel-1" || view.Task != "pickup-task-1" || view.Attempt != "attempt-1" {
		t.Fatalf("view = %+v（对象、任务、尝试沿用被更正版本）", view)
	}
	if view.SegmentContinuationReference != "" || view.SegmentEntryRefusal != "" {
		t.Fatalf("view = %+v（更正不进段）", view)
	}
	if len(fixture.segments.rows) != 0 {
		t.Fatalf("更正动了段登记册：%d 段", len(fixture.segments.rows))
	}
}

// Covers: ADR-0022——重放、冲突、未受理、未决都是 200 的形成答案；首登口的 `corrects` 格对首登版本为空。
func TestCorrectOffsitePickupReportsOKForEveryAnswerThatCorrectedNothing(t *testing.T) {
	cases := map[string]struct {
		arrange    func(*testing.T, *pickupCorrectionFixture) pickupCorrectionBody
		posts      int
		want       string
		wantReason string
	}{
		"replaying the same correction returns the corrected version": {
			arrange: func(t *testing.T, fixture *pickupCorrectionFixture) pickupCorrectionBody {
				fixture.seedRegistration(t)
				return defaultPickupCorrectionBody()
			},
			posts: 2,
			want:  "EXISTING_VERSION",
		},
		"a different correction naming a superseded version is a conflict": {
			arrange: func(t *testing.T, fixture *pickupCorrectionFixture) pickupCorrectionBody {
				fixture.seedRegistration(t)
				if first := postTo(t, fixture.correct, "/transport-fulfillment/offsite-pickup-corrections", defaultPickupCorrectionBody()); first.Code != http.StatusCreated {
					t.Fatalf("first correction status = %d", first.Code)
				}
				body := defaultPickupCorrectionBody()
				body.Control = "control-3"
				return body
			},
			posts: 1,
			want:  "SOURCE_CONFLICT",
		},
		"a correction without a registered predecessor is not accepted": {
			arrange: func(*testing.T, *pickupCorrectionFixture) pickupCorrectionBody { return defaultPickupCorrectionBody() },
			posts:   1,
			want:    "SOURCE_NOT_ACCEPTED",
		},
		"a correction without control evidence is not accepted": {
			arrange: func(t *testing.T, fixture *pickupCorrectionFixture) pickupCorrectionBody {
				fixture.seedRegistration(t)
				body := defaultPickupCorrectionBody()
				body.Control = ""
				return body
			},
			posts: 1,
			want:  "SOURCE_NOT_ACCEPTED",
		},
		"a correction dated before the registration it corrects is not accepted": {
			arrange: func(t *testing.T, fixture *pickupCorrectionFixture) pickupCorrectionBody {
				fixture.seedRegistration(t)
				body := defaultPickupCorrectionBody()
				body.CorrectedAt = "2026-09-05T11:59:59Z"
				return body
			},
			posts: 1,
			want:  "SOURCE_NOT_ACCEPTED",
		},
		"a registry failure is undecided": {
			arrange: func(t *testing.T, fixture *pickupCorrectionFixture) pickupCorrectionBody {
				fixture.seedRegistration(t)
				fixture.registry.findErr = errors.New("registry down")
				return defaultPickupCorrectionBody()
			},
			posts:      1,
			want:       "PICKUP_UNDECIDED",
			wantReason: "PICKUP_REGISTRY_UNAVAILABLE",
		},
		"a version factory failure is undecided": {
			arrange: func(t *testing.T, fixture *pickupCorrectionFixture) pickupCorrectionBody {
				fixture.seedRegistration(t)
				fixture.versions.err = errors.New("sequence down")
				return defaultPickupCorrectionBody()
			},
			posts:      1,
			want:       "PICKUP_UNDECIDED",
			wantReason: "PICKUP_VERSION_UNAVAILABLE",
		},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newPickupCorrectionFixture(t)
			body := testCase.arrange(t, fixture)
			var response *httptest.ResponseRecorder
			for post := 0; post < testCase.posts; post++ {
				response = postTo(t, fixture.correct, "/transport-fulfillment/offsite-pickup-corrections", body)
			}
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body.String())
			}
			view := decodePickupCorrection(t, response)
			if view.Outcome != testCase.want || view.UndecidedReason != testCase.wantReason {
				t.Fatalf("view = %+v, want outcome %q reason %q", view, testCase.want, testCase.wantReason)
			}
			if testCase.wantReason != "" && view.ContinuationReference == "" {
				t.Fatal("未决没有带 continuationReference")
			}
		})
	}

	t.Run("the registration endpoint leaves corrects empty for a first version", func(t *testing.T) {
		fixture := newPickupCorrectionFixture(t)
		response := postTo(t, fixture.register, "/transport-fulfillment/offsite-pickups", defaultPickupRegistrationBody())
		if response.Code != http.StatusCreated {
			t.Fatalf("status = %d", response.Code)
		}
		if view := decodePickupCorrection(t, response); view.Corrects != "" {
			t.Fatalf("首登响应带了 corrects = %q", view.Corrects)
		}
	})
}

// Covers: 渠道未配置堵住更正口（ADR-0055）——未配置是渠道这一层的状态，只堵首登不堵更正就是给更正留了条无渠道也能进的路。
func TestCorrectOffsitePickupIsClosedWhileTheAccessChannelIsUnconfigured(t *testing.T) {
	fixture := newPickupCorrectionFixture(t)
	fixture.seedRegistration(t)
	endpoint := tfhttp.NewCorrectOffsitePickupEndpoint(tfhttp.UnconfiguredIntake{}, fixture.handler)

	response := postTo(t, endpoint, "/transport-fulfillment/offsite-pickup-corrections", defaultPickupCorrectionBody())

	if response.Code != http.StatusForbidden || problemCode(t, response) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	assertNoOutcome(t, response)
	if fixture.versions.minted != 1 {
		t.Fatalf("minted = %d——未配置的渠道让更正走到了编排", fixture.versions.minted)
	}
}

// 传输层分法只钉与首登口不同形的那一格：编排 error 是 5xx 且不带 outcome。其余分法由共用的
// commandEndpoint 承担，交接口的测试已逐格钉过。
func TestCorrectOffsitePickupOrchestrationErrorFormsNoAnswer(t *testing.T) {
	fixture := newPickupCorrectionFixture(t)
	endpoint := tfhttp.NewCorrectOffsitePickupEndpoint(fixture.intake, failingPickupCorrectionHandler{})

	response := postTo(t, endpoint, "/transport-fulfillment/offsite-pickup-corrections", defaultPickupCorrectionBody())

	if response.Code != http.StatusInternalServerError || problemCode(t, response) != "NO_ANSWER_FORMED" {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	assertNoOutcome(t, response)
}

type failingPickupCorrectionHandler struct{}

func (failingPickupCorrectionHandler) Correct(
	context.Context,
	application.CorrectOffsitePickupCommand,
) (application.RegisterOffsitePickupResult, error) {
	return application.RegisterOffsitePickupResult{}, errors.New("orchestration exploded")
}
