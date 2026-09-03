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

type handoverRegistryDouble struct {
	records map[string]ports.TransportHandoverRecord
	findErr error
}

func newHandoverRegistry() *handoverRegistryDouble {
	return &handoverRegistryDouble{records: map[string]ports.TransportHandoverRecord{}}
}

func handoverRegistryKey(key ports.TransportHandoverKey) string {
	return key.TenantID.String() + "|" + key.Object.String() + "|" + key.Scope.String() + "|" + key.Version.String()
}

func (double *handoverRegistryDouble) FindByKey(
	_ context.Context,
	key ports.TransportHandoverKey,
) (ports.TransportHandoverRecord, bool, error) {
	if double.findErr != nil {
		return ports.TransportHandoverRecord{}, false, double.findErr
	}
	record, found := double.records[handoverRegistryKey(key)]
	return record, found, nil
}

func (double *handoverRegistryDouble) Save(
	_ context.Context,
	record ports.TransportHandoverRecord,
) (ports.HandoverSaveOutcome, error) {
	if _, exists := double.records[handoverRegistryKey(record.Key)]; exists {
		return ports.HandoverAlreadyRegistered, nil
	}
	double.records[handoverRegistryKey(record.Key)] = record
	return ports.HandoverSaved, nil
}

type handoverHandoffDouble struct{ err error }

func (double handoverHandoffDouble) HandOffTransportHandover(
	context.Context,
	ports.TransportHandoverRegistrationIntent,
) error {
	return double.err
}

// ---- 请求体与 intake 替身（信封固定、事实从体收）----

type handoverBody struct {
	Object            string `json:"object"`
	Scope             string `json:"scope"`
	ReleasedBy        string `json:"releasedBy"`
	ReceivedBy        string `json:"receivedBy"`
	Verdict           string `json:"verdict"`
	ReleasingEvidence string `json:"releasingEvidence"`
	ReceivingEvidence string `json:"receivingEvidence"`
	Rule              string `json:"rule"`
	Basis             string `json:"basis"`
	Version           string `json:"version"`
	JudgedAt          string `json:"judgedAt"`
	Segment           string `json:"segment"`
	PlannedSegment    string `json:"plannedSegment"`
}

type handoverCorrectionBody struct {
	Object             string `json:"object"`
	Scope              string `json:"scope"`
	PredecessorVersion string `json:"predecessorVersion"`
	Verdict            string `json:"verdict"`
	ReleasingEvidence  string `json:"releasingEvidence"`
	ReceivingEvidence  string `json:"receivingEvidence"`
	Rule               string `json:"rule"`
	Basis              string `json:"basis"`
	NewVersion         string `json:"newVersion"`
	CorrectedAt        string `json:"correctedAt"`
}

func verdictOf(raw string) domain.HandoverVerdict {
	switch raw {
	case "HANDED_OVER":
		return domain.ObjectHandedOver
	case "REFUSED":
		return domain.HandoverRefused
	case "PENDING_CONFIRMATION":
		return domain.HandoverPendingConfirmation
	default:
		return domain.HandoverVerdict(0)
	}
}

type handoverIntakeDouble struct {
	tenant domain.TenantID
	err    error
}

func (intake *handoverIntakeDouble) IntakeHandoverRegistration(
	_ context.Context,
	request *http.Request,
) (application.RegisterTransportHandoverCommand, error) {
	if intake.err != nil {
		return application.RegisterTransportHandoverCommand{}, intake.err
	}
	var body handoverBody
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		return application.RegisterTransportHandoverCommand{}, fmt.Errorf("%w: %v", tfhttp.ErrMalformedRequest, err)
	}
	judgedAt, err := time.Parse(time.RFC3339, body.JudgedAt)
	if err != nil {
		return application.RegisterTransportHandoverCommand{}, fmt.Errorf("%w: %v", tfhttp.ErrMalformedRequest, err)
	}
	return application.RegisterTransportHandoverCommand{
		TenantID:          intake.tenant,
		Object:            body.Object,
		Scope:             body.Scope,
		ReleasedBy:        body.ReleasedBy,
		ReceivedBy:        body.ReceivedBy,
		Verdict:           verdictOf(body.Verdict),
		ReleasingEvidence: body.ReleasingEvidence,
		ReceivingEvidence: body.ReceivingEvidence,
		Rule:              body.Rule,
		Basis:             body.Basis,
		Version:           body.Version,
		JudgedAt:          judgedAt,
		Segment:           body.Segment,
		PlannedSegment:    body.PlannedSegment,
	}, nil
}

func (intake *handoverIntakeDouble) IntakeHandoverCorrection(
	_ context.Context,
	request *http.Request,
) (application.CorrectTransportHandoverCommand, error) {
	if intake.err != nil {
		return application.CorrectTransportHandoverCommand{}, intake.err
	}
	var body handoverCorrectionBody
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		return application.CorrectTransportHandoverCommand{}, fmt.Errorf("%w: %v", tfhttp.ErrMalformedRequest, err)
	}
	correctedAt, err := time.Parse(time.RFC3339, body.CorrectedAt)
	if err != nil {
		return application.CorrectTransportHandoverCommand{}, fmt.Errorf("%w: %v", tfhttp.ErrMalformedRequest, err)
	}
	return application.CorrectTransportHandoverCommand{
		TenantID:           intake.tenant,
		Object:             body.Object,
		Scope:              body.Scope,
		PredecessorVersion: body.PredecessorVersion,
		Verdict:            verdictOf(body.Verdict),
		ReleasingEvidence:  body.ReleasingEvidence,
		ReceivingEvidence:  body.ReceivingEvidence,
		Rule:               body.Rule,
		Basis:              body.Basis,
		NewVersion:         body.NewVersion,
		CorrectedAt:        correctedAt,
	}, nil
}

type handoverFixture struct {
	intake   *handoverIntakeDouble
	registry *handoverRegistryDouble
	segments *segmentRegistryDouble
	register http.Handler
	correct  http.Handler
}

func newHandoverFixture(t *testing.T) *handoverFixture {
	t.Helper()
	fixture := &handoverFixture{
		intake:   &handoverIntakeDouble{tenant: httpValue(t, domain.NewTenantID, "tenant-1")},
		registry: newHandoverRegistry(),
		segments: newSegmentRegistry(),
	}
	// 结束参与那一半接真处理器、共用同一个段登记册替身：交接口的测试里对象都是第一次到场，答案恒为
	// NO_ACTIVE_PARTICIPATION；它被透出这件事由 TestRegisterHandoverExposesTheParticipationEnd 钉。
	handler := application.NewRegisterTransportHandoverHandler(application.RegisterTransportHandoverDeps{
		Handovers:  fixture.registry,
		Segments:   fixture.segments,
		Downstream: handoverHandoffDouble{},
		Clock:      tfClock{at: time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)},
		ParticipationEnds: application.NewEndFulfillmentParticipationHandler(application.EndFulfillmentParticipationDeps{
			Segments:   fixture.segments,
			Handovers:  fixture.registry,
			Deliveries: newDeliveryStore(),
			Clock:      tfClock{at: time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)},
		}),
	})
	fixture.register = tfhttp.NewRegisterTransportHandoverEndpoint(fixture.intake, handler)
	fixture.correct = tfhttp.NewCorrectTransportHandoverEndpoint(fixture.intake, handler)
	return fixture
}

func defaultHandoverBody() handoverBody {
	return handoverBody{
		Object:            "parcel-1",
		Scope:             "handover-scope-1",
		ReleasedBy:        "node-1",
		ReceivedBy:        "carrier-1",
		Verdict:           "HANDED_OVER",
		ReleasingEvidence: "evidence-release-1",
		ReceivingEvidence: "evidence-receive-1",
		Rule:              "handover-rule/v1",
		Version:           "handover-result/parcel-1/v1",
		JudgedAt:          "2026-09-05T10:00:00Z",
	}
}

func postTo(t *testing.T, endpoint http.Handler, path string, value any) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(payload)))
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, request)
	return recorder
}

type handoverView struct {
	Outcome                      string `json:"outcome"`
	UndecidedReason              string `json:"undecidedReason"`
	HandoverVersion              string `json:"handoverVersion"`
	Object                       string `json:"object"`
	Scope                        string `json:"scope"`
	Verdict                      string `json:"verdict"`
	Corrects                     string `json:"corrects"`
	ContinuationReference        string `json:"continuationReference"`
	HandoffReference             string `json:"handoffReference"`
	SegmentContinuationReference string `json:"segmentContinuationReference"`
	SegmentEntryRefusal          string `json:"segmentEntryRefusal"`
	ParticipationEnd             string `json:"participationEnd"`
}

// Covers: 票 06——`已交接`落库后结束前段参与那一半的答案透出（`participationEnd`）。第一次到场的对象答
// NO_ACTIVE_PARTICIPATION 而不是空：交接登上了、没有任何参与被它结束，调用方要看得见。
func TestRegisterHandoverExposesTheParticipationEnd(t *testing.T) {
	fixture := newHandoverFixture(t)
	body := defaultHandoverBody()
	body.Segment = "segment-1"

	response := postTo(t, fixture.register, "/transport-fulfillment/handovers", body)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	if view := decodeHandover(t, response); view.ParticipationEnd != "NO_ACTIVE_PARTICIPATION" {
		t.Fatalf("participationEnd = %q, want NO_ACTIVE_PARTICIPATION", view.ParticipationEnd)
	}

	next := defaultHandoverBody()
	next.Scope = "handover-scope-2"
	next.Version = "handover-result/parcel-1/v2"
	next.JudgedAt = "2026-09-05T11:00:00Z"
	next.Segment = "segment-2"
	moved := postTo(t, fixture.register, "/transport-fulfillment/handovers", next)
	if moved.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", moved.Code, moved.Body.String())
	}
	if view := decodeHandover(t, moved); view.ParticipationEnd != "PARTICIPATION_ENDED" {
		t.Fatalf("participationEnd = %q, want PARTICIPATION_ENDED", view.ParticipationEnd)
	}
	if fixture.segments.participationOf(t, "tenant-1", "segment-1", "parcel-1").Active() {
		t.Fatal("下一次交接后前段参与仍在场")
	}
}

func decodeHandover(t *testing.T, recorder *httptest.ResponseRecorder) handoverView {
	t.Helper()
	var view handoverView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return view
}

// Covers: 票 04 专钉第 1 条——Intake 交出的 `segment`/`plannedSegment` 要原样到编排，判据不是
// 看命令镜像，而是看对象**真的进了那个段**：段登记册里有 (tenant-1, segment-1)，parcel-1 的
// 参与关系带 planned-1。HTTP 适配器若在中间立一个自己的请求形状漏掉那一格，这里就红——
// 那正是「编排接上了、端点没让它接上」（票 08 那个缝）。同时钉 ADR-0022：首登新落一版 201。
func TestRegisterHandoverReportsCreatedAndEntersTheNamedSegment(t *testing.T) {
	fixture := newHandoverFixture(t)
	body := defaultHandoverBody()
	body.Segment = "segment-1"
	body.PlannedSegment = "planned-1"

	response := postTo(t, fixture.register, "/transport-fulfillment/handovers", body)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", response.Code, response.Body.String())
	}
	view := decodeHandover(t, response)
	if view.Outcome != "HANDOVER_REGISTERED" {
		t.Fatalf("outcome = %q", view.Outcome)
	}
	if view.HandoverVersion != "handover-result/parcel-1/v1" || view.Object != "parcel-1" || view.Scope != "handover-scope-1" {
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

// Covers: 更正走版本链——201 新落一版、`corrects` 透出前版引用；更正无中生有是 200 未受理答案。
func TestCorrectHandoverExposesThePredecessorVersion(t *testing.T) {
	fixture := newHandoverFixture(t)
	if seed := postTo(t, fixture.register, "/transport-fulfillment/handovers", defaultHandoverBody()); seed.Code != http.StatusCreated {
		t.Fatalf("seed status = %d body = %s", seed.Code, seed.Body.String())
	}

	response := postTo(t, fixture.correct, "/transport-fulfillment/handover-corrections", handoverCorrectionBody{
		Object:             "parcel-1",
		Scope:              "handover-scope-1",
		PredecessorVersion: "handover-result/parcel-1/v1",
		Verdict:            "REFUSED",
		ReleasingEvidence:  "evidence-release-1",
		ReceivingEvidence:  "evidence-receive-1",
		Rule:               "handover-rule/v1",
		Basis:              "refusal-basis-1",
		NewVersion:         "handover-result/parcel-1/v2",
		CorrectedAt:        "2026-09-05T11:00:00Z",
	})

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201（更正也新落了一版）; body = %s", response.Code, response.Body.String())
	}
	view := decodeHandover(t, response)
	if view.Outcome != "HANDOVER_CORRECTED" || view.Corrects != "handover-result/parcel-1/v1" {
		t.Fatalf("view = %+v", view)
	}
	if view.HandoverVersion != "handover-result/parcel-1/v2" || view.Verdict != domain.HandoverRefused.String() {
		t.Fatalf("view = %+v", view)
	}

	t.Run("correcting an absent judgment is an answer, not a transport error", func(t *testing.T) {
		absent := postTo(t, fixture.correct, "/transport-fulfillment/handover-corrections", handoverCorrectionBody{
			Object:             "parcel-9",
			Scope:              "handover-scope-9",
			PredecessorVersion: "handover-result/parcel-9/v1",
			Verdict:            "HANDED_OVER",
			ReleasingEvidence:  "e1",
			ReceivingEvidence:  "e2",
			Rule:               "handover-rule/v1",
			NewVersion:         "handover-result/parcel-9/v2",
			CorrectedAt:        "2026-09-05T11:00:00Z",
		})
		if absent.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", absent.Code)
		}
		if view := decodeHandover(t, absent); view.Outcome != "SOURCE_NOT_ACCEPTED" {
			t.Fatalf("outcome = %q", view.Outcome)
		}
	})
}

// Covers: ADR-0022「业务判别一律进响应体」——重放、冲突、未受理、未决都是形成了的答案，都是
// 200，`outcome` 区分；`未决`带 `continuationReference` 与 `undecidedReason`，调用方据以续办。
// **未决不是 5xx**：编排交回的是结果不是 error，状态码镜像的是那个签名。
func TestRegisterHandoverReportsOKForEveryAnswerThatRegisteredNothing(t *testing.T) {
	cases := map[string]struct {
		arrange    func(*handoverFixture) handoverBody
		posts      int
		want       string
		wantReason string
	}{
		"replay returns the existing version": {
			arrange: func(*handoverFixture) handoverBody { return defaultHandoverBody() },
			posts:   2,
			want:    "EXISTING_VERSION",
		},
		"a different verdict under the same version is a conflict": {
			arrange: func(fixture *handoverFixture) handoverBody {
				if seed := postTo(t, fixture.register, "/transport-fulfillment/handovers", defaultHandoverBody()); seed.Code != http.StatusCreated {
					t.Fatalf("seed status = %d", seed.Code)
				}
				body := defaultHandoverBody()
				body.Verdict = "REFUSED"
				body.Basis = "refusal-basis-1"
				return body
			},
			posts: 1,
			want:  "SOURCE_CONFLICT",
		},
		"a handed-over verdict without receiving evidence is not accepted": {
			arrange: func(*handoverFixture) handoverBody {
				body := defaultHandoverBody()
				body.ReceivingEvidence = ""
				return body
			},
			posts: 1,
			want:  "SOURCE_NOT_ACCEPTED",
		},
		"a registry failure is undecided with a continuation": {
			arrange: func(fixture *handoverFixture) handoverBody {
				fixture.registry.findErr = errors.New("registry down")
				return defaultHandoverBody()
			},
			posts:      1,
			want:       "HANDOVER_UNDECIDED",
			wantReason: "HANDOVER_REGISTRY_UNAVAILABLE",
		},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newHandoverFixture(t)
			body := testCase.arrange(fixture)
			var response *httptest.ResponseRecorder
			for post := 0; post < testCase.posts; post++ {
				response = postTo(t, fixture.register, "/transport-fulfillment/handovers", body)
			}
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200（未决也是答案）; body = %s", response.Code, response.Body.String())
			}
			view := decodeHandover(t, response)
			if view.Outcome != testCase.want {
				t.Fatalf("outcome = %q, want %q", view.Outcome, testCase.want)
			}
			if view.UndecidedReason != testCase.wantReason {
				t.Fatalf("undecidedReason = %q, want %q", view.UndecidedReason, testCase.wantReason)
			}
			if testCase.wantReason != "" && view.ContinuationReference == "" {
				t.Fatal("未决没有带 continuationReference，调用方无从续办")
			}
		})
	}
}

// Covers: 票 04 专钉第 2 条——段那一半的欠账要透出。段立不起来（写不进）时交接照登（201），响应里
// `segmentContinuationReference` 非空；这一格被传输层吞掉，调用方就会以为对象已经进段。
// 用 saveErr 而不是 findErr：票 06 之后按对象找段那一步读不回是整笔不落（5xx），不再是欠账。
func TestRegisterHandoverExposesTheSegmentDebtWhenTheRegistryIsDown(t *testing.T) {
	fixture := newHandoverFixture(t)
	fixture.segments.saveErr = errors.New("segment registry cannot write")
	body := defaultHandoverBody()
	body.Segment = "segment-1"

	response := postTo(t, fixture.register, "/transport-fulfillment/handovers", body)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201（交接本身登上了）; body = %s", response.Code, response.Body.String())
	}
	view := decodeHandover(t, response)
	if view.Outcome != "HANDOVER_REGISTERED" {
		t.Fatalf("outcome = %q", view.Outcome)
	}
	if view.SegmentContinuationReference == "" {
		t.Fatal("segmentContinuationReference 为空：进段那一半的欠账被传输层吞了")
	}
	if fixture.segments.hasSegment("tenant-1", "segment-1") {
		t.Fatal("段登记册故障时不该有段")
	}

	t.Run("a registry that cannot even be read fails the whole registration", func(t *testing.T) {
		fixture := newHandoverFixture(t)
		fixture.segments.findErr = errors.New("segment registry down")
		response := postTo(t, fixture.register, "/transport-fulfillment/handovers", body)
		if response.Code != http.StatusInternalServerError || problemCode(t, response) != "NO_ANSWER_FORMED" {
			t.Fatalf("status = %d body = %s，want 5xx——结束参与那一半没答上，交付/交接不落半成品（票 06）", response.Code, response.Body.String())
		}
	})
}

// 传输层分法：405 带 Allow；构造不出命令是 4xx；接入层其他失败与编排错误是 5xx，且 5xx 体里
// 不带 outcome；无名 outcome 是 500。错误体只有稳定 code 不回显请求（ADR-0029）。
func TestHandoverTransportProblemsCarryOnlyStableCodes(t *testing.T) {
	t.Run("method not allowed", func(t *testing.T) {
		fixture := newHandoverFixture(t)
		request := httptest.NewRequest(http.MethodGet, "/transport-fulfillment/handovers", nil)
		recorder := httptest.NewRecorder()
		fixture.register.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != http.MethodPost {
			t.Fatalf("status = %d allow = %q", recorder.Code, recorder.Header().Get("Allow"))
		}
	})

	t.Run("a malformed request is 400 and does not echo the body", func(t *testing.T) {
		fixture := newHandoverFixture(t)
		request := httptest.NewRequest(http.MethodPost, "/transport-fulfillment/handovers", strings.NewReader("secret-not-json"))
		recorder := httptest.NewRecorder()
		fixture.register.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "MALFORMED_REQUEST") {
			t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
		}
		if strings.Contains(recorder.Body.String(), "secret-not-json") {
			t.Fatal("错误体回显了请求内容")
		}
	})

	t.Run("an intake infrastructure failure is 500", func(t *testing.T) {
		fixture := newHandoverFixture(t)
		fixture.intake.err = errors.New("session store down")
		response := postTo(t, fixture.register, "/transport-fulfillment/handovers", defaultHandoverBody())
		if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "INTAKE_FAILED") {
			t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
		}
	})

	t.Run("an orchestration error is 500 NO_ANSWER_FORMED without an outcome", func(t *testing.T) {
		fixture := newHandoverFixture(t)
		endpoint := tfhttp.NewRegisterTransportHandoverEndpoint(fixture.intake, failingHandoverHandler{})
		response := postTo(t, endpoint, "/transport-fulfillment/handovers", defaultHandoverBody())
		if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "NO_ANSWER_FORMED") {
			t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), "outcome") {
			t.Fatal("5xx 响应不得携带 outcome（ADR-0022）")
		}
	})

	t.Run("an unnamed outcome is a 500, not a new business answer", func(t *testing.T) {
		fixture := newHandoverFixture(t)
		endpoint := tfhttp.NewCorrectTransportHandoverEndpoint(fixture.intake, unnamedHandoverHandler{})
		response := postTo(t, endpoint, "/transport-fulfillment/handover-corrections", handoverCorrectionBody{CorrectedAt: "2026-09-05T11:00:00Z"})
		if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "UNNAMED_OUTCOME") {
			t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
		}
	})
}

type failingHandoverHandler struct{}

func (failingHandoverHandler) Register(
	context.Context,
	application.RegisterTransportHandoverCommand,
) (application.RegisterTransportHandoverResult, error) {
	return application.RegisterTransportHandoverResult{}, errors.New("orchestration exploded")
}

func (failingHandoverHandler) Correct(
	context.Context,
	application.CorrectTransportHandoverCommand,
) (application.RegisterTransportHandoverResult, error) {
	return application.RegisterTransportHandoverResult{}, errors.New("orchestration exploded")
}

type unnamedHandoverHandler struct{}

func (unnamedHandoverHandler) Register(
	context.Context,
	application.RegisterTransportHandoverCommand,
) (application.RegisterTransportHandoverResult, error) {
	return application.RegisterTransportHandoverResult{}, nil
}

func (unnamedHandoverHandler) Correct(
	context.Context,
	application.CorrectTransportHandoverCommand,
) (application.RegisterTransportHandoverResult, error) {
	return application.RegisterTransportHandoverResult{}, nil
}
